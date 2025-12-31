package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type options struct {
	inputPath          string
	outputPath         string
	crdInputDir        string
	crdOutputDir       string
	operatorNamespace  string
	operatorImageRef   string
	operatorVersionEnv string
	sentinelRepoEnv    string
}

func main() {
	var opts options
	flag.StringVar(&opts.inputPath, "in", "dist/install.yaml", "Path to dist/install.yaml")
	flag.StringVar(
		&opts.outputPath,
		"out",
		"charts/openbao-operator/files/install.yaml.tpl",
		"Path to charts/openbao-operator/files/install.yaml.tpl",
	)
	flag.StringVar(&opts.crdInputDir, "crd-in-dir", "config/crd/bases", "Path to config/crd/bases directory")
	flag.StringVar(
		&opts.crdOutputDir,
		"crd-out-dir",
		"charts/openbao-operator/crds",
		"Path to charts/openbao-operator/crds directory",
	)
	flag.StringVar(
		&opts.operatorNamespace,
		"namespace",
		"openbao-operator-system",
		"Namespace value to template in the installer manifest",
	)
	flag.StringVar(
		&opts.operatorImageRef,
		"operator-image",
		"controller:latest",
		"Operator image reference value to template in the installer manifest",
	)
	flag.StringVar(
		&opts.operatorVersionEnv,
		"operator-version-env",
		"OPERATOR_VERSION",
		"Environment variable name to template the operator version from",
	)
	flag.StringVar(
		&opts.sentinelRepoEnv,
		"sentinel-repo-env",
		"OPERATOR_SENTINEL_IMAGE_REPOSITORY",
		"Environment variable name to template the sentinel image repository from",
	)
	flag.Parse()

	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	inputLines, err := readLines(opts.inputPath)
	if err != nil {
		return fmt.Errorf("read installer input %q: %w", opts.inputPath, err)
	}

	docs := splitYAMLDocuments(inputLines)
	renderedDocs := make([][]string, 0, len(docs))
	for _, doc := range docs {
		doc = trimEmptyLines(doc)
		if len(doc) == 0 {
			continue
		}
		if kindOfYAMLDocument(doc) == "CustomResourceDefinition" {
			continue
		}

		rendered, err := templateInstallerDocument(doc, opts)
		if err != nil {
			return fmt.Errorf("template installer document: %w", err)
		}
		renderedDocs = append(renderedDocs, rendered)
	}

	out, err := joinYAMLDocuments(renderedDocs)
	if err != nil {
		return fmt.Errorf("render output: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(opts.outputPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(opts.outputPath, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write output %q: %w", opts.outputPath, err)
	}

	if err := syncCRDs(opts); err != nil {
		return fmt.Errorf("sync CRDs: %w", err)
	}

	return nil
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
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

func splitYAMLDocuments(lines []string) [][]string {
	docCount := 1
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			docCount++
		}
	}

	docs := make([][]string, 0, docCount)
	current := make([]string, 0, len(lines)/docCount+1)

	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			docs = append(docs, current)
			current = make([]string, 0, len(lines)/docCount+1)
			continue
		}
		current = append(current, line)
	}
	docs = append(docs, current)

	return docs
}

func trimEmptyLines(lines []string) []string {
	start := 0
	for start < len(lines) {
		if strings.TrimSpace(lines[start]) != "" {
			break
		}
		start++
	}

	end := len(lines)
	for end > start {
		if strings.TrimSpace(lines[end-1]) != "" {
			break
		}
		end--
	}

	return lines[start:end]
}

func kindOfYAMLDocument(doc []string) string {
	for _, line := range doc {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "kind:") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "kind:"))
	}
	return ""
}

func templateInstallerDocument(doc []string, opts options) ([]string, error) {
	out := make([]string, 0, len(doc)+4)

	expectOperatorVersionValue := false
	expectSentinelRepoValue := false

	namespaceNeedle := fmt.Sprintf("namespace: %s", opts.operatorNamespace)
	serviceAccountNeedle := fmt.Sprintf("system:serviceaccount:%s:", opts.operatorNamespace)

	for _, line := range doc {
		if expectOperatorVersionValue {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "value:") {
				return nil, fmt.Errorf("expected value line after %s, got %q", opts.operatorVersionEnv, trimmed)
			}
			indent := leadingWhitespace(line)
			out = append(out, indent+`value: {{ include "openbao-operator.operatorVersion" . | quote }}`)
			expectOperatorVersionValue = false
			continue
		}
		if expectSentinelRepoValue {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "value:") {
				return nil, fmt.Errorf("expected value line after %s, got %q", opts.sentinelRepoEnv, trimmed)
			}
			indent := leadingWhitespace(line)
			out = append(out, indent+`value: {{ include "openbao-operator.sentinelImageRepository" . | quote }}`)
			expectSentinelRepoValue = false
			continue
		}

		templated := strings.ReplaceAll(line, namespaceNeedle, `namespace: {{ .Release.Namespace }}`)
		templated = strings.ReplaceAll(templated, serviceAccountNeedle, `system:serviceaccount:{{ .Release.Namespace }}:`)

		trimmed := strings.TrimSpace(templated)
		switch {
		case trimmed == fmt.Sprintf("- name: %s", opts.operatorVersionEnv):
			out = append(out, templated)
			expectOperatorVersionValue = true
			continue
		case trimmed == fmt.Sprintf("- name: %s", opts.sentinelRepoEnv):
			out = append(out, templated)
			expectSentinelRepoValue = true
			continue
		case trimmed == fmt.Sprintf("image: %s", opts.operatorImageRef):
			indent := leadingWhitespace(templated)
			out = append(out, indent+`image: {{ include "openbao-operator.managerImage" . }}`)
			out = append(out, indent+`imagePullPolicy: {{ .Values.image.pullPolicy }}`)
			continue
		default:
		}

		out = append(out, templated)
	}

	if expectOperatorVersionValue {
		return nil, fmt.Errorf("expected value after %s but reached end of document", opts.operatorVersionEnv)
	}
	if expectSentinelRepoValue {
		return nil, fmt.Errorf("expected value after %s but reached end of document", opts.sentinelRepoEnv)
	}

	return out, nil
}

func leadingWhitespace(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return s[:i]
		}
	}
	return s
}

func joinYAMLDocuments(docs [][]string) (string, error) {
	if len(docs) == 0 {
		return "", fmt.Errorf("no documents to write")
	}

	var b strings.Builder
	for i, doc := range docs {
		doc = trimEmptyLines(doc)
		if len(doc) == 0 {
			continue
		}
		if i > 0 {
			b.WriteString("---\n")
		}
		for _, line := range doc {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	return b.String(), nil
}

func syncCRDs(opts options) error {
	entries, err := os.ReadDir(opts.crdInputDir)
	if err != nil {
		return fmt.Errorf("read CRD input dir %q: %w", opts.crdInputDir, err)
	}
	if err := os.MkdirAll(opts.crdOutputDir, 0o755); err != nil {
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
		if err := os.WriteFile(outPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
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
