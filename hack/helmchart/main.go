// Package main provides a tool to sync kustomize manifests to the Helm chart.
// The tool syncs CRDs, ValidatingAdmissionPolicies, and RBAC resources.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type options struct {
	crdInputDir     string
	crdOutputDir    string
	policyInputDir  string
	policyOutputDir string
	rbacInputDir    string
	rbacOutputDir   string
	helmChartDir    string
}

func main() {
	var opts options
	flag.StringVar(&opts.crdInputDir, "crd-in-dir", "config/crd/bases", "Path to config/crd/bases directory")
	flag.StringVar(&opts.crdOutputDir, "crd-out-dir", "charts/openbao-operator/crds", "Path to Helm chart CRDs directory")
	flag.StringVar(&opts.policyInputDir, "policy-in-dir", "config/policy", "Path to config/policy directory")
	flag.StringVar(&opts.policyOutputDir, "policy-out-dir", "charts/openbao-operator/templates/admission",
		"Path to Helm chart admission templates directory")
	flag.StringVar(&opts.rbacInputDir, "rbac-in-dir", "config/rbac", "Path to config/rbac directory")
	flag.StringVar(&opts.rbacOutputDir, "rbac-out-dir", "charts/openbao-operator/templates/rbac",
		"Path to Helm chart RBAC templates directory")
	flag.StringVar(&opts.helmChartDir, "chart-dir", "charts/openbao-operator", "Path to Helm chart root directory")
	flag.Parse()

	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	if err := syncCRDs(opts); err != nil {
		return fmt.Errorf("sync CRDs: %w", err)
	}
	if err := syncPolicies(opts); err != nil {
		return fmt.Errorf("sync policies: %w", err)
	}
	if err := syncRBAC(opts); err != nil {
		return fmt.Errorf("sync RBAC: %w", err)
	}
	return nil
}

func readFile(path string) (string, error) {
	// #nosec G304 -- path is constructed from a controlled input directory.
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readLines(path string) ([]string, error) {
	// #nosec G304 -- path is constructed from a controlled input directory and file name.
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func leadingWhitespace(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return s[:i]
		}
	}
	return s
}

func writeFile(path, content string) error {
	// #nosec G306 -- writes non-sensitive YAML intended to be committed to the repo.
	return os.WriteFile(path, []byte(content), 0o644)
}

// syncCRDs syncs CRD YAMLs from config/crd/bases to the Helm chart.
func syncCRDs(opts options) error {
	entries, err := os.ReadDir(opts.crdInputDir)
	if err != nil {
		return fmt.Errorf("read CRD input dir %q: %w", opts.crdInputDir, err)
	}
	if err := os.MkdirAll(opts.crdOutputDir, 0o750); err != nil {
		return fmt.Errorf("create CRD output dir %q: %w", opts.crdOutputDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}

		inPath := filepath.Join(opts.crdInputDir, name)
		lines, err := readLines(inPath)
		if err != nil {
			return fmt.Errorf("read %q: %w", inPath, err)
		}

		lines, err = ensureHelmKeepAnnotation(lines)
		if err != nil {
			return fmt.Errorf("ensure Helm keep annotation for %q: %w", inPath, err)
		}

		outPath := filepath.Join(opts.crdOutputDir, name)
		if err := writeFile(outPath, strings.Join(lines, "\n")+"\n"); err != nil {
			return fmt.Errorf("write %q: %w", outPath, err)
		}
	}

	return nil
}

func ensureHelmKeepAnnotation(lines []string) ([]string, error) {
	for _, line := range lines {
		if strings.TrimSpace(line) == "helm.sh/resource-policy: keep" {
			return lines, nil
		}
	}

	for i, line := range lines {
		if strings.TrimSpace(line) != "annotations:" {
			continue
		}
		indent := leadingWhitespace(line)
		insert := indent + "  helm.sh/resource-policy: keep"
		lines = append(lines[:i+1], append([]string{insert}, lines[i+1:]...)...)
		return lines, nil
	}

	return nil, fmt.Errorf("annotations block not found")
}

// Policy file mapping from kustomize to Helm output names.
var policyFileMapping = map[string]string{
	"lock-managed-resource-mutations.yaml":               "lock-managed-resources.yaml",
	"lock-managed-resource-mutations-binding.yaml":       "lock-managed-resources.yaml", // merged
	"lock-controller-statefulset-mutations.yaml":         "validating-policies.yaml",
	"lock-controller-statefulset-mutations-binding.yaml": "validating-policies.yaml", // merged
	"restrict-provisioner-delegate.yaml":                 "provisioner-delegate.yaml",
	"restrict-provisioner-delegate-binding.yaml":         "provisioner-delegate.yaml", // merged
	"validate-openbaocluster.yaml":                       "validate-openbaocluster.yaml",
	"validate-openbaocluster-binding.yaml":               "validate-openbaocluster.yaml", // merged
}

// syncPolicies syncs ValidatingAdmissionPolicy YAMLs from config/policy to Helm templates.
func syncPolicies(opts options) error {
	entries, err := os.ReadDir(opts.policyInputDir)
	if err != nil {
		return fmt.Errorf("read policy input dir %q: %w", opts.policyInputDir, err)
	}
	if err := os.MkdirAll(opts.policyOutputDir, 0o750); err != nil {
		return fmt.Errorf("create policy output dir %q: %w", opts.policyOutputDir, err)
	}

	// Group files by output file
	outputGroups := make(map[string][]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") || name == "kustomization.yaml" {
			continue
		}
		outName, ok := policyFileMapping[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: unknown policy file %q, skipping\n", name)
			continue
		}
		outputGroups[outName] = append(outputGroups[outName], name)
	}

	// Sort the output groups so policies come before bindings (binding files have -binding suffix)
	for _, files := range outputGroups {
		sort.Slice(files, func(i, j int) bool {
			// Non-binding files should come first
			iBinding := strings.Contains(files[i], "-binding")
			jBinding := strings.Contains(files[j], "-binding")
			if iBinding != jBinding {
				return !iBinding // non-binding first
			}
			return files[i] < files[j]
		})
	}

	// Process each output group
	for outName, inputFiles := range outputGroups {
		var parts []string
		for _, inputFile := range inputFiles {
			inPath := filepath.Join(opts.policyInputDir, inputFile)
			content, err := readFile(inPath)
			if err != nil {
				return fmt.Errorf("read %q: %w", inPath, err)
			}
			transformed := transformPolicyToHelm(content)
			parts = append(parts, transformed)
		}

		// Wrap in conditional and join
		output := fmt.Sprintf("{{- if .Values.admissionPolicies.enabled }}\n%s{{- end }}\n", strings.Join(parts, "---\n"))
		outPath := filepath.Join(opts.policyOutputDir, outName)
		if err := writeFile(outPath, output); err != nil {
			return fmt.Errorf("write %q: %w", outPath, err)
		}
	}

	return nil
}

// transformPolicyToHelm transforms a kustomize policy YAML to Helm template.
func transformPolicyToHelm(content string) string {

	// Add Helm labels after metadata.name
	content = addHelmLabels(content)

	// Replace static name with Helm template
	// Pattern: name: <policy-name> -> name: {{ include "openbao-operator.fullname" . }}-<policy-name>
	namePattern := regexp.MustCompile(`(\n\s+name:\s+)([\w-]+)(\s*\n)`)
	content = namePattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := namePattern.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}
		prefix := parts[1]
		name := parts[2]
		suffix := parts[3]
		// Don't transform names that are already templates or nested names
		if strings.Contains(name, "{{") || strings.Contains(prefix, "policyName") {
			return match
		}
		return fmt.Sprintf("%s{{ include \"openbao-operator.fullname\" . }}-%s%s", prefix, name, suffix)
	})

	// Replace policyName references - strip existing openbao-operator- prefix if present
	policyNamePattern := regexp.MustCompile(`(policyName:\s*)(openbao-operator-)?(lock-|restrict-|validate-)([\w-]+)`)
	content = policyNamePattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := policyNamePattern.FindStringSubmatch(match)
		if len(parts) < 5 {
			return match
		}
		prefix := parts[1]
		// parts[2] is the optional "openbao-operator-" which we strip
		policyPrefix := parts[3] // lock-, restrict-, validate-
		policySuffix := parts[4]
		return fmt.Sprintf("%s{{ include \"openbao-operator.fullname\" . }}-%s%s", prefix, policyPrefix, policySuffix)
	})

	// Replace namespace references in expressions
	content = strings.ReplaceAll(content, "openbao-operator-system", "{{ .Release.Namespace }}")

	// Replace service account references in expressions
	// Note: Helm templates inside YAML multiline strings need unescaped quotes
	content = strings.ReplaceAll(content,
		`"system:serviceaccount:{{ .Release.Namespace }}:openbao-operator-controller"`,
		`"system:serviceaccount:{{ .Release.Namespace }}:{{ include "openbao-operator.controllerServiceAccountName" . }}"`)

	return content
}

// addHelmLabels adds standard Helm labels after the metadata block.
func addHelmLabels(content string) string {
	// Find metadata: block and add labels after name
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines)+2) // Pre-allocate with extra space for labels
	inMetadata := false
	labelsAdded := false

	for i, line := range lines {
		result = append(result, line)

		if strings.TrimSpace(line) == "metadata:" {
			inMetadata = true
			continue
		}

		if inMetadata && !labelsAdded && strings.HasPrefix(strings.TrimSpace(line), "name:") {
			// Add labels after name line
			indent := leadingWhitespace(line)
			result = append(result, indent+"labels:")
			result = append(result, indent+"  {{- include \"openbao-operator.labels\" . | nindent 4 }}")
			labelsAdded = true

			// If next line is spec:, we're done with metadata
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "spec:" {
				inMetadata = false
			}
		}

		if inMetadata && strings.TrimSpace(line) == "spec:" {
			inMetadata = false
		}
	}

	return strings.Join(result, "\n")
}

// syncRBAC syncs RBAC resources from config/rbac to Helm templates.
// Note: RBAC templates have complex conditional logic, so we only sync the core
// ClusterRoles that are generated and should match. Deployments and ServiceAccounts
// remain hand-maintained.
func syncRBAC(opts options) error {
	// For now, skip RBAC sync - it's more complex due to:
	// 1. Multiple files need to be merged into single Helm templates
	// 2. Conditional logic differs between tenancy modes
	// 3. The current Helm templates have extensive value-based conditionals
	//
	// TODO: Implement RBAC sync in a future iteration
	fmt.Println("RBAC sync: skipped (complex conditional logic requires hand-maintenance)")
	return nil
}
