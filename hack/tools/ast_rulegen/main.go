package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type architecturePolicy struct {
	ModulePath             string                 `yaml:"modulePath"`
	LayerCoverage          layerCoverage          `yaml:"layerCoverage"`
	ControllerCoverage     controllerCoverage     `yaml:"controllerCoverage"`
	ServiceImportRoots     []string               `yaml:"serviceImportRoots"`
	AdapterImportRoots     []string               `yaml:"adapterImportRoots"`
	ControllerBoundaries   []controllerBoundary   `yaml:"controllerBoundaries"`
	GlobalImportBoundaries []globalImportBoundary `yaml:"globalImportBoundaries"`
}

type layerCoverage struct {
	Root   string              `yaml:"root"`
	Layers map[string][]string `yaml:"layers"`
	Exempt []string            `yaml:"exempt"`
}

type controllerCoverage struct {
	Root   string   `yaml:"root"`
	Exempt []string `yaml:"exempt"`
}

type controllerBoundary struct {
	Name            string   `yaml:"name"`
	DisplayName     string   `yaml:"displayName"`
	Files           []string `yaml:"files"`
	Ignores         []string `yaml:"ignores"`
	DisallowImports []string `yaml:"disallowImports"`
	AppFacadeRoot   string   `yaml:"appFacadeRoot"`
	AllowService    []string `yaml:"allowServiceImports"`
	AllowAdapter    []string `yaml:"allowAdapterImports"`
}

type globalImportBoundary struct {
	ID                            string   `yaml:"id"`
	Message                       string   `yaml:"message"`
	Note                          string   `yaml:"note"`
	Files                         []string `yaml:"files"`
	Ignores                       []string `yaml:"ignores"`
	DisallowImports               []string `yaml:"disallowImports"`
	DisallowExternalImports       []string `yaml:"disallowExternalImports"`
	DisallowExternalExactImports  []string `yaml:"disallowExternalExactImports"`
}

type ruleSpec struct {
	ID      string
	Message string
	Note    string
	Files   []string
	Ignores []string
	Regex   string
}

type astRuleDoc struct {
	ID       string   `yaml:"id"`
	Message  string   `yaml:"message"`
	Severity string   `yaml:"severity"`
	Language string   `yaml:"language"`
	Files    []string `yaml:"files"`
	Ignores  []string `yaml:"ignores,omitempty"`
	Rule     astRule  `yaml:"rule"`
	Note     string   `yaml:"note,omitempty"`
}

type astRule struct {
	All []astPredicate `yaml:"all"`
}

type astPredicate struct {
	Kind  string `yaml:"kind,omitempty"`
	Regex string `yaml:"regex,omitempty"`
}

func main() {
	var (
		policyPath string
		outDir     string
	)

	flag.StringVar(
		&policyPath,
		"policy",
		".ast-grep/policy/architecture-boundaries.yml",
		"Path to ast-grep architecture policy YAML",
	)
	flag.StringVar(
		&outDir,
		"out-dir",
		".ast-grep/rules/generated/architecture-boundary",
		"Output directory for generated ast-grep rules",
	)
	flag.Parse()

	if err := run(policyPath, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "ast_rulegen: %v\n", err)
		os.Exit(1)
	}
}

func run(policyPath, outDir string) error {
	policy, err := loadPolicy(policyPath)
	if err != nil {
		return err
	}

	if err := validatePolicy(policy); err != nil {
		return err
	}

	if err := verifyControllerCoverage(policy); err != nil {
		return err
	}

	if err := verifyLayerCoverage(policy); err != nil {
		return err
	}

	specs, err := buildRuleSpecs(policy)
	if err != nil {
		return err
	}

	if err := writeRuleSpecs(policyPath, outDir, specs); err != nil {
		return err
	}

	return nil
}

func loadPolicy(path string) (architecturePolicy, error) {
	var policy architecturePolicy

	data, err := os.ReadFile(path)
	if err != nil {
		return architecturePolicy{}, fmt.Errorf("read policy %s: %w", path, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&policy); err != nil {
		return architecturePolicy{}, fmt.Errorf("parse policy %s: %w", path, err)
	}

	return policy, nil
}

func validatePolicy(policy architecturePolicy) error {
	if strings.TrimSpace(policy.ModulePath) == "" {
		return errors.New("policy modulePath is required")
	}

	serviceRoots := normalizedUnique(policy.ServiceImportRoots)
	if len(serviceRoots) == 0 {
		return errors.New("policy serviceImportRoots must not be empty")
	}

	adapterRoots := normalizedUnique(policy.AdapterImportRoots)
	if len(adapterRoots) == 0 {
		return errors.New("policy adapterImportRoots must not be empty")
	}

	seenController := make(map[string]struct{}, len(policy.ControllerBoundaries))
	for _, boundary := range policy.ControllerBoundaries {
		if strings.TrimSpace(boundary.Name) == "" {
			return errors.New("controllerBoundaries.name is required")
		}
		if _, exists := seenController[boundary.Name]; exists {
			return fmt.Errorf("duplicate controllerBoundaries entry for %q", boundary.Name)
		}
		seenController[boundary.Name] = struct{}{}

		if len(boundary.Files) == 0 {
			return fmt.Errorf("controllerBoundaries[%s].files must not be empty", boundary.Name)
		}
		if len(boundary.DisallowImports) == 0 {
			return fmt.Errorf("controllerBoundaries[%s].disallowImports must not be empty", boundary.Name)
		}
		if strings.TrimSpace(boundary.AppFacadeRoot) == "" {
			return fmt.Errorf("controllerBoundaries[%s].appFacadeRoot must not be empty", boundary.Name)
		}
		if err := ensureSubset(
			boundary.Name,
			"allowServiceImports",
			boundary.AllowService,
			serviceRoots,
		); err != nil {
			return err
		}
		if err := ensureSubset(
			boundary.Name,
			"allowAdapterImports",
			boundary.AllowAdapter,
			adapterRoots,
		); err != nil {
			return err
		}
	}

	seenRule := make(map[string]struct{}, len(policy.GlobalImportBoundaries))
	for _, boundary := range policy.GlobalImportBoundaries {
		if strings.TrimSpace(boundary.ID) == "" {
			return errors.New("globalImportBoundaries.id is required")
		}
		if _, exists := seenRule[boundary.ID]; exists {
			return fmt.Errorf("duplicate globalImportBoundaries entry for %q", boundary.ID)
		}
		seenRule[boundary.ID] = struct{}{}

		if strings.TrimSpace(boundary.Message) == "" {
			return fmt.Errorf("globalImportBoundaries[%s].message is required", boundary.ID)
		}
		if len(boundary.Files) == 0 {
			return fmt.Errorf("globalImportBoundaries[%s].files must not be empty", boundary.ID)
		}
		if len(boundary.DisallowImports) == 0 &&
			len(boundary.DisallowExternalImports) == 0 &&
			len(boundary.DisallowExternalExactImports) == 0 {
			return fmt.Errorf(
				"globalImportBoundaries[%s] must define at least one of disallowImports, disallowExternalImports, or disallowExternalExactImports",
				boundary.ID,
			)
		}
	}

	return nil
}

func ensureSubset(
	boundaryName, fieldName string,
	values, allowed []string,
) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, root := range allowed {
		allowedSet[root] = struct{}{}
	}

	for _, value := range normalizedUnique(values) {
		if _, ok := allowedSet[value]; ok {
			continue
		}
		return fmt.Errorf(
			"controllerBoundaries[%s].%s contains unknown root %q",
			boundaryName,
			fieldName,
			value,
		)
	}
	return nil
}

func verifyControllerCoverage(policy architecturePolicy) error {
	coverageRoot := strings.TrimSpace(policy.ControllerCoverage.Root)
	if coverageRoot == "" {
		return nil
	}

	entries, err := os.ReadDir(coverageRoot)
	if err != nil {
		return fmt.Errorf("read controller coverage root %s: %w", coverageRoot, err)
	}

	configured := make(map[string]struct{}, len(policy.ControllerBoundaries))
	for _, boundary := range policy.ControllerBoundaries {
		configured[boundary.Name] = struct{}{}
	}

	exempt := make(map[string]struct{}, len(policy.ControllerCoverage.Exempt))
	for _, name := range policy.ControllerCoverage.Exempt {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		exempt[name] = struct{}{}
	}

	missing := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if _, ok := configured[name]; ok {
			continue
		}
		if _, ok := exempt[name]; ok {
			continue
		}
		missing = append(missing, name)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("controller boundary policy missing coverage for: %s", strings.Join(missing, ", "))
	}

	var unknownConfigured []string
	for name := range configured {
		controllerPath := filepath.Join(coverageRoot, name)
		info, err := os.Stat(controllerPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				unknownConfigured = append(unknownConfigured, name)
				continue
			}
			return fmt.Errorf("stat controller coverage path %s: %w", controllerPath, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("configured controller path %s is not a directory", controllerPath)
		}
	}
	sort.Strings(unknownConfigured)
	if len(unknownConfigured) > 0 {
		return fmt.Errorf(
			"controller boundary policy references non-existent controller packages: %s",
			strings.Join(unknownConfigured, ", "),
		)
	}

	return nil
}

func verifyLayerCoverage(policy architecturePolicy) error {
	root := strings.TrimSpace(policy.LayerCoverage.Root)
	if root == "" {
		return nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read layer coverage root %s: %w", root, err)
	}

	layerAssignments := make(map[string]string, len(policy.LayerCoverage.Layers))
	for layer, packages := range policy.LayerCoverage.Layers {
		for _, pkg := range normalizedUnique(packages) {
			if prev, exists := layerAssignments[pkg]; exists {
				return fmt.Errorf(
					"layer coverage duplicates package %q in layers %q and %q",
					pkg,
					prev,
					layer,
				)
			}
			layerAssignments[pkg] = layer
		}
	}

	exempt := make(map[string]struct{}, len(policy.LayerCoverage.Exempt))
	for _, name := range normalizedUnique(policy.LayerCoverage.Exempt) {
		exempt[name] = struct{}{}
	}

	missing := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if _, ok := layerAssignments[name]; ok {
			continue
		}
		if _, ok := exempt[name]; ok {
			continue
		}
		missing = append(missing, name)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf(
			"layer coverage missing internal package classification for: %s",
			strings.Join(missing, ", "),
		)
	}

	var unknown []string
	for name := range layerAssignments {
		path := filepath.Join(root, name)
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				unknown = append(unknown, name)
				continue
			}
			return fmt.Errorf("stat layer coverage path %s: %w", path, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("configured layer package path %s is not a directory", path)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		return fmt.Errorf(
			"layer coverage references non-existent internal packages: %s",
			strings.Join(unknown, ", "),
		)
	}

	return nil
}

func buildRuleSpecs(policy architecturePolicy) ([]ruleSpec, error) {
	serviceRoots := normalizedUnique(policy.ServiceImportRoots)
	adapterRoots := normalizedUnique(policy.AdapterImportRoots)

	specs := make(
		[]ruleSpec,
		0,
		len(policy.ControllerBoundaries)*4+len(policy.GlobalImportBoundaries),
	)

	modulePath := strings.TrimSuffix(strings.TrimSpace(policy.ModulePath), "/")

	for _, boundary := range policy.ControllerBoundaries {
		controllerLabel := strings.TrimSpace(boundary.DisplayName)
		if controllerLabel == "" {
			controllerLabel = strings.TrimSpace(boundary.Name)
		}

		directRegex, err := importRegex(modulePath, boundary.DisallowImports, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("build direct-import regex for controller %s: %w", boundary.Name, err)
		}

		specs = append(specs, ruleSpec{
			ID:      "no-" + sanitizeName(boundary.Name) + "-controller-direct-domain-imports",
			Message: controllerDirectDomainMessage(controllerLabel),
			Note: controllerDirectDomainNote(
				controllerLabel,
				boundary.AppFacadeRoot,
			),
			Files:   boundary.Files,
			Ignores: boundary.Ignores,
			Regex:   directRegex,
		})

		appSubpackageRegex, err := appSubpackageRegex(modulePath, boundary.AppFacadeRoot)
		if err != nil {
			return nil, fmt.Errorf("build app-subpackage regex for controller %s: %w", boundary.Name, err)
		}

		specs = append(specs, ruleSpec{
			ID: "no-" + sanitizeName(boundary.Name) + "-controller-app-subpackage-imports",
			Message: controllerAppFacadeMessage(
				controllerLabel,
				boundary.AppFacadeRoot,
			),
			Note: controllerAppFacadeNote(
				boundary.AppFacadeRoot,
			),
			Files:   boundary.Files,
			Ignores: boundary.Ignores,
			Regex:   appSubpackageRegex,
		})

		unapprovedServiceImports := differenceRoots(
			serviceRoots,
			boundary.AllowService,
		)
		if len(unapprovedServiceImports) > 0 {
			serviceRegex, err := importRegex(modulePath, unapprovedServiceImports, nil, nil)
			if err != nil {
				return nil, fmt.Errorf(
					"build unapproved-service regex for controller %s: %w",
					boundary.Name,
					err,
				)
			}

			specs = append(specs, ruleSpec{
				ID: "no-" + sanitizeName(boundary.Name) +
					"-controller-unapproved-service-imports",
				Message: controllerServiceImportsMessage(controllerLabel),
				Note: controllerServiceImportsNote(
					boundary.AllowService,
				),
				Files:   boundary.Files,
				Ignores: boundary.Ignores,
				Regex:   serviceRegex,
			})
		}

		unapprovedAdapterImports := differenceRoots(
			adapterRoots,
			boundary.AllowAdapter,
		)
		if len(unapprovedAdapterImports) > 0 {
			adapterRegex, err := importRegex(modulePath, unapprovedAdapterImports, nil, nil)
			if err != nil {
				return nil, fmt.Errorf(
					"build unapproved-adapter regex for controller %s: %w",
					boundary.Name,
					err,
				)
			}

			specs = append(specs, ruleSpec{
				ID: "no-" + sanitizeName(boundary.Name) +
					"-controller-unapproved-adapter-imports",
				Message: controllerAdapterImportsMessage(controllerLabel),
				Note: controllerAdapterImportsNote(
					boundary.AllowAdapter,
				),
				Files:   boundary.Files,
				Ignores: boundary.Ignores,
				Regex:   adapterRegex,
			})
		}
	}

	for _, boundary := range policy.GlobalImportBoundaries {
		regex, err := importRegex(
			modulePath,
			boundary.DisallowImports,
			boundary.DisallowExternalImports,
			boundary.DisallowExternalExactImports,
		)
		if err != nil {
			return nil, fmt.Errorf("build global-import regex for rule %s: %w", boundary.ID, err)
		}

		specs = append(specs, ruleSpec{
			ID:      boundary.ID,
			Message: boundary.Message,
			Note:    boundary.Note,
			Files:   boundary.Files,
			Ignores: boundary.Ignores,
			Regex:   regex,
		})
	}

	sort.Slice(specs, func(i, j int) bool {
		return specs[i].ID < specs[j].ID
	})

	return specs, nil
}

func importRegex(
	modulePath string,
	importRoots []string,
	externalImportRoots []string,
	externalExactImports []string,
) (string, error) {
	internalRoots := normalizedUnique(importRoots)
	externalRoots := normalizedUnique(externalImportRoots)
	exactExternalRoots := normalizedUnique(externalExactImports)

	if len(internalRoots) == 0 && len(externalRoots) == 0 && len(exactExternalRoots) == 0 {
		return "", errors.New("no valid import roots were provided")
	}

	// Keep existing formatting stable for internal-only boundaries so generated diffs stay minimal.
	if len(internalRoots) > 0 && len(externalRoots) == 0 && len(exactExternalRoots) == 0 {
		parts := make([]string, 0, len(internalRoots))
		for _, root := range internalRoots {
			parts = append(parts, regexp.QuoteMeta(root)+`(/[^"]*)?`)
		}
		return fmt.Sprintf(`"%s/(%s)"`, regexp.QuoteMeta(modulePath), strings.Join(parts, "|")), nil
	}

	parts := make([]string, 0, len(internalRoots)+len(externalRoots)+len(exactExternalRoots))
	modulePath = strings.Trim(strings.TrimSpace(modulePath), "/")
	for _, root := range internalRoots {
		parts = append(parts, regexp.QuoteMeta(modulePath+"/"+root)+`(/[^"]*)?`)
	}
	for _, root := range externalRoots {
		parts = append(parts, regexp.QuoteMeta(root)+`(/[^"]*)?`)
	}
	for _, root := range exactExternalRoots {
		parts = append(parts, regexp.QuoteMeta(root))
	}

	return fmt.Sprintf(`"(%s)"`, strings.Join(parts, "|")), nil
}

func appSubpackageRegex(modulePath, appFacadeRoot string) (string, error) {
	root := strings.Trim(appFacadeRoot, "/")
	if root == "" {
		return "", errors.New("app facade root must not be empty")
	}

	return fmt.Sprintf(`"%s/%s/.+"`, regexp.QuoteMeta(modulePath), regexp.QuoteMeta(root)), nil
}

func normalizedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))

	for _, value := range values {
		normalized := strings.Trim(strings.TrimSpace(value), "/")
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}

	sort.Strings(result)
	return result
}

func differenceRoots(base, allow []string) []string {
	allowed := make(map[string]struct{}, len(allow))
	for _, root := range normalizedUnique(allow) {
		allowed[root] = struct{}{}
	}

	result := make([]string, 0, len(base))
	for _, root := range normalizedUnique(base) {
		if _, ok := allowed[root]; ok {
			continue
		}
		result = append(result, root)
	}
	return result
}

func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "controller"
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}

	sanitized := strings.Trim(builder.String(), "-")
	if sanitized == "" {
		return "controller"
	}
	return sanitized
}

func controllerDirectDomainMessage(controllerLabel string) string {
	return fmt.Sprintf(
		"%s controller must delegate domain logic via app facade, not direct domain package imports.",
		controllerLabel,
	)
}

func controllerDirectDomainNote(controllerLabel, appFacadeRoot string) string {
	return fmt.Sprintf(
		"Keep %s controller focused on reconcile plumbing and route domain orchestration through %s.",
		controllerLabel,
		appFacadeRoot,
	)
}

func controllerAppFacadeMessage(controllerLabel, appFacadeRoot string) string {
	return fmt.Sprintf(
		"%s controller should import only %s facade, not app subpackages.",
		controllerLabel,
		appFacadeRoot,
	)
}

func controllerAppFacadeNote(appFacadeRoot string) string {
	return fmt.Sprintf(
		"Import only the app root facade (%s) from controller layer to preserve a stable dependency surface.",
		appFacadeRoot,
	)
}

func controllerServiceImportsMessage(controllerLabel string) string {
	return fmt.Sprintf(
		"%s controller must only import explicitly approved service packages.",
		controllerLabel,
	)
}

func controllerServiceImportsNote(allowed []string) string {
	return fmt.Sprintf(
		"Approved service imports for this controller: %s.",
		formatAllowList(allowed),
	)
}

func controllerAdapterImportsMessage(controllerLabel string) string {
	return fmt.Sprintf(
		"%s controller must only import explicitly approved adapter packages.",
		controllerLabel,
	)
}

func controllerAdapterImportsNote(allowed []string) string {
	return fmt.Sprintf(
		"Approved adapter imports for this controller: %s.",
		formatAllowList(allowed),
	)
}

func formatAllowList(values []string) string {
	roots := normalizedUnique(values)
	if len(roots) == 0 {
		return "(none)"
	}
	return strings.Join(roots, ", ")
}

func writeRuleSpecs(policyPath, outDir string, specs []ruleSpec) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output directory %s: %w", outDir, err)
	}

	existingFiles, err := filepath.Glob(filepath.Join(outDir, "*.yml"))
	if err != nil {
		return fmt.Errorf("list existing generated rule files: %w", err)
	}
	for _, path := range existingFiles {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove stale generated rule %s: %w", path, err)
		}
	}

	for _, spec := range specs {
		doc := astRuleDoc{
			ID:       spec.ID,
			Message:  spec.Message,
			Severity: "warning",
			Language: "Go",
			Files:    spec.Files,
			Ignores:  spec.Ignores,
			Rule: astRule{
				All: []astPredicate{
					{Kind: "import_spec"},
					{Regex: spec.Regex},
				},
			},
			Note: spec.Note,
		}

		data, err := yaml.Marshal(doc)
		if err != nil {
			return fmt.Errorf("marshal rule %s: %w", spec.ID, err)
		}

		var builder strings.Builder
		builder.WriteString("# Code generated by hack/tools/ast_rulegen from ")
		builder.WriteString(policyPath)
		builder.WriteString(". DO NOT EDIT.\n")
		builder.WriteString(
			"# yaml-language-server: $schema=https://raw.githubusercontent.com/ast-grep/ast-grep/main/schemas/rule.json\n\n",
		)
		builder.Write(data)

		path := filepath.Join(outDir, spec.ID+".yml")
		if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
			return fmt.Errorf("write generated rule %s: %w", path, err)
		}
	}

	return nil
}
