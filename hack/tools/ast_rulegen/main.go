package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
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
	ServiceBoundaries      []serviceBoundary      `yaml:"serviceBoundaries"`
	AppBoundaries          []appBoundary          `yaml:"appBoundaries"`
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

type serviceBoundary struct {
	Name         string   `yaml:"name"`
	DisplayName  string   `yaml:"displayName"`
	PackageRoot  string   `yaml:"packageRoot"`
	Files        []string `yaml:"files"`
	Ignores      []string `yaml:"ignores"`
	AllowService []string `yaml:"allowServiceImports"`
	AllowAdapter []string `yaml:"allowAdapterImports"`
}

type appBoundary struct {
	Name         string   `yaml:"name"`
	DisplayName  string   `yaml:"displayName"`
	Files        []string `yaml:"files"`
	Ignores      []string `yaml:"ignores"`
	AllowService []string `yaml:"allowServiceImports"`
}

type globalImportBoundary struct {
	ID                           string   `yaml:"id"`
	Message                      string   `yaml:"message"`
	Note                         string   `yaml:"note"`
	Files                        []string `yaml:"files"`
	Ignores                      []string `yaml:"ignores"`
	DisallowImports              []string `yaml:"disallowImports"`
	DisallowExternalImports      []string `yaml:"disallowExternalImports"`
	DisallowExternalExactImports []string `yaml:"disallowExternalExactImports"`
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

func loadPolicy(policyPath string) (architecturePolicy, error) {
	var policy architecturePolicy

	data, err := os.ReadFile(policyPath)
	if err != nil {
		return architecturePolicy{}, fmt.Errorf("read policy %s: %w", policyPath, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&policy); err != nil {
		return architecturePolicy{}, fmt.Errorf("parse policy %s: %w", policyPath, err)
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

	if err := validateControllerBoundaries(policy.ControllerBoundaries, serviceRoots, adapterRoots); err != nil {
		return err
	}
	if err := validateServiceBoundaries(policy.ServiceBoundaries, serviceRoots, adapterRoots); err != nil {
		return err
	}
	if err := validateAppBoundaries(policy.AppBoundaries, serviceRoots); err != nil {
		return err
	}
	if err := validateGlobalImportBoundaries(policy.GlobalImportBoundaries); err != nil {
		return err
	}

	return nil
}

func validateControllerBoundaries(
	boundaries []controllerBoundary,
	serviceRoots []string,
	adapterRoots []string,
) error {
	seenController := make(map[string]struct{}, len(boundaries))
	for _, boundary := range boundaries {
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
		if err := ensureKnownRoots(
			fmt.Sprintf("controllerBoundaries[%s].allowServiceImports", boundary.Name),
			boundary.AllowService,
			serviceRoots,
		); err != nil {
			return err
		}
		if err := ensureKnownRoots(
			fmt.Sprintf("controllerBoundaries[%s].allowAdapterImports", boundary.Name),
			boundary.AllowAdapter,
			adapterRoots,
		); err != nil {
			return err
		}
	}

	return nil
}

func validateServiceBoundaries(boundaries []serviceBoundary, serviceRoots []string, adapterRoots []string) error {
	seenService := make(map[string]struct{}, len(boundaries))
	for _, boundary := range boundaries {
		if strings.TrimSpace(boundary.Name) == "" {
			return errors.New("serviceBoundaries.name is required")
		}
		if _, exists := seenService[boundary.Name]; exists {
			return fmt.Errorf("duplicate serviceBoundaries entry for %q", boundary.Name)
		}
		seenService[boundary.Name] = struct{}{}

		if strings.TrimSpace(boundary.PackageRoot) == "" {
			return fmt.Errorf("serviceBoundaries[%s].packageRoot is required", boundary.Name)
		}
		if len(boundary.Files) == 0 {
			return fmt.Errorf("serviceBoundaries[%s].files must not be empty", boundary.Name)
		}
		if err := ensureKnownRoot(
			fmt.Sprintf("serviceBoundaries[%s].packageRoot", boundary.Name),
			boundary.PackageRoot,
			serviceRoots,
		); err != nil {
			return err
		}
		if err := ensureKnownRoots(
			fmt.Sprintf("serviceBoundaries[%s].allowServiceImports", boundary.Name),
			boundary.AllowService,
			serviceRoots,
		); err != nil {
			return err
		}
		if err := ensureKnownRoots(
			fmt.Sprintf("serviceBoundaries[%s].allowAdapterImports", boundary.Name),
			boundary.AllowAdapter,
			adapterRoots,
		); err != nil {
			return err
		}
	}

	return nil
}

func validateAppBoundaries(boundaries []appBoundary, serviceRoots []string) error {
	seenApp := make(map[string]struct{}, len(boundaries))
	for _, boundary := range boundaries {
		if strings.TrimSpace(boundary.Name) == "" {
			return errors.New("appBoundaries.name is required")
		}
		if _, exists := seenApp[boundary.Name]; exists {
			return fmt.Errorf("duplicate appBoundaries entry for %q", boundary.Name)
		}
		seenApp[boundary.Name] = struct{}{}

		if len(boundary.Files) == 0 {
			return fmt.Errorf("appBoundaries[%s].files must not be empty", boundary.Name)
		}
		if err := ensureKnownRoots(
			fmt.Sprintf("appBoundaries[%s].allowServiceImports", boundary.Name),
			boundary.AllowService,
			serviceRoots,
		); err != nil {
			return err
		}
	}

	return nil
}

func validateGlobalImportBoundaries(boundaries []globalImportBoundary) error {
	seenRule := make(map[string]struct{}, len(boundaries))
	for _, boundary := range boundaries {
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
				"globalImportBoundaries[%s] must define at least one of "+
					"disallowImports, disallowExternalImports, or disallowExternalExactImports",
				boundary.ID,
			)
		}
	}

	return nil
}

func ensureKnownRoots(location string, values, allowed []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, root := range allowed {
		allowedSet[root] = struct{}{}
	}

	for _, value := range normalizedUnique(values) {
		if _, ok := allowedSet[value]; ok {
			continue
		}
		return fmt.Errorf("%s contains unknown root %q", location, value)
	}
	return nil
}

func ensureKnownRoot(location, value string, allowed []string) error {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return fmt.Errorf("%s is required", location)
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, root := range allowed {
		allowedSet[root] = struct{}{}
	}
	if _, ok := allowedSet[value]; ok {
		return nil
	}

	return fmt.Errorf("%s contains unknown root %q", location, value)
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

	layerAssignments, exempt, err := buildLayerCoverageMaps(policy.LayerCoverage)
	if err != nil {
		return err
	}

	missing, err := findMissingLayerCoverage(root, entries, layerAssignments, exempt)
	if err != nil {
		return err
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf(
			"layer coverage missing internal package classification for: %s",
			strings.Join(missing, ", "),
		)
	}

	unknown, err := findUnknownLayerCoveragePaths(root, layerAssignments, exempt)
	if err != nil {
		return err
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

func buildLayerCoverageMaps(
	coverage layerCoverage,
) (map[string]string, map[string]struct{}, error) {
	layerAssignments := make(map[string]string, len(coverage.Layers))
	for layer, packages := range coverage.Layers {
		for _, pkg := range normalizedLayerCoveragePaths(packages) {
			if prev, exists := layerAssignments[pkg]; exists {
				return nil, nil, fmt.Errorf(
					"layer coverage duplicates package %q in layers %q and %q",
					pkg,
					prev,
					layer,
				)
			}
			if err := validateLayerCoveragePath(pkg); err != nil {
				return nil, nil, err
			}
			layerAssignments[pkg] = layer
		}
	}

	exempt := make(map[string]struct{}, len(coverage.Exempt))
	for _, name := range normalizedLayerCoveragePaths(coverage.Exempt) {
		if err := validateLayerCoveragePath(name); err != nil {
			return nil, nil, err
		}
		exempt[name] = struct{}{}
	}

	return layerAssignments, exempt, nil
}

func findMissingLayerCoverage(
	root string,
	entries []os.DirEntry,
	layerAssignments map[string]string,
	exempt map[string]struct{},
) ([]string, error) {
	missing := make([]string, 0, len(entries))
	for _, entry := range entries {
		entryMissing, err := missingLayerCoverageForEntry(root, entry, layerAssignments, exempt)
		if err != nil {
			return nil, err
		}
		missing = append(missing, entryMissing...)
	}
	return missing, nil
}

func missingLayerCoverageForEntry(
	root string,
	entry os.DirEntry,
	layerAssignments map[string]string,
	exempt map[string]struct{},
) ([]string, error) {
	if !entry.IsDir() {
		return nil, nil
	}

	name := entry.Name()
	if strings.HasPrefix(name, ".") {
		return nil, nil
	}
	if _, ok := layerAssignments[name]; ok {
		return nil, nil
	}
	if _, ok := exempt[name]; ok {
		return nil, nil
	}

	if !hasNestedLayerCoverage(name, layerAssignments, exempt) {
		return []string{name}, nil
	}

	return missingGroupedLayerCoverage(root, name, layerAssignments, exempt)
}

func missingGroupedLayerCoverage(
	root, group string,
	layerAssignments map[string]string,
	exempt map[string]struct{},
) ([]string, error) {
	children, err := os.ReadDir(filepath.Join(root, group))
	if err != nil {
		return nil, fmt.Errorf("read grouped layer path %s: %w", filepath.Join(root, group), err)
	}

	groupCovered := false
	missing := make([]string, 0, len(children))
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		childName := child.Name()
		if strings.HasPrefix(childName, ".") {
			continue
		}
		groupCovered = true
		rel := path.Join(group, childName)
		if _, ok := layerAssignments[rel]; ok {
			continue
		}
		if _, ok := exempt[rel]; ok {
			continue
		}
		missing = append(missing, rel)
	}

	if !groupCovered {
		return []string{group}, nil
	}

	return missing, nil
}

func findUnknownLayerCoveragePaths(
	root string,
	layerAssignments map[string]string,
	exempt map[string]struct{},
) ([]string, error) {
	unknown, err := appendUnknownLayerCoveragePaths(root, mapKeys(layerAssignments))
	if err != nil {
		return nil, err
	}

	exemptUnknown, err := appendUnknownLayerCoveragePaths(root, setKeys(exempt))
	if err != nil {
		return nil, err
	}

	return append(unknown, exemptUnknown...), nil
}

func appendUnknownLayerCoveragePaths(root string, names []string) ([]string, error) {
	unknown := make([]string, 0, len(names))
	for _, name := range names {
		coveragePath := filepath.Join(root, filepath.FromSlash(name))
		info, err := os.Stat(coveragePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				unknown = append(unknown, name)
				continue
			}
			return nil, fmt.Errorf("stat layer coverage path %s: %w", coveragePath, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("configured layer coverage path %s is not a directory", coveragePath)
		}
	}
	return unknown, nil
}

func normalizedLayerCoveragePaths(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))

	for _, value := range values {
		normalized := normalizeLayerCoveragePath(value)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}

	return result
}

func normalizeLayerCoveragePath(value string) string {
	normalized := strings.Trim(strings.TrimSpace(value), "/")
	if normalized == "" {
		return ""
	}
	normalized = path.Clean(normalized)
	if normalized == "." {
		return ""
	}
	return strings.Trim(normalized, "/")
}

func validateLayerCoveragePath(pkg string) error {
	parts := strings.Split(pkg, "/")
	if len(parts) == 0 || len(parts) > 2 {
		return fmt.Errorf(
			"layer coverage path %q must be relative to internal and use depth internal/<pkg> or internal/<group>/<pkg>",
			pkg,
		)
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || part == "." || part == ".." {
			return fmt.Errorf("layer coverage path %q contains invalid segment %q", pkg, part)
		}
	}
	return nil
}

func hasNestedLayerCoverage(
	root string,
	layerAssignments map[string]string,
	exempt map[string]struct{},
) bool {
	prefix := root + "/"
	for pkg := range layerAssignments {
		if strings.HasPrefix(pkg, prefix) {
			return true
		}
	}
	for pkg := range exempt {
		if strings.HasPrefix(pkg, prefix) {
			return true
		}
	}
	return false
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func setKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func buildRuleSpecs(policy architecturePolicy) ([]ruleSpec, error) {
	serviceRoots := normalizedUnique(policy.ServiceImportRoots)
	adapterRoots := normalizedUnique(policy.AdapterImportRoots)

	specs := make(
		[]ruleSpec,
		0,
		len(policy.ControllerBoundaries)*4+
			len(policy.ServiceBoundaries)+
			len(policy.AppBoundaries)+
			len(policy.GlobalImportBoundaries),
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

	for _, boundary := range policy.ServiceBoundaries {
		serviceLabel := strings.TrimSpace(boundary.DisplayName)
		if serviceLabel == "" {
			serviceLabel = strings.TrimSpace(boundary.Name)
		}

		allowedRoots := append([]string{boundary.PackageRoot}, boundary.AllowService...)
		unapprovedServiceImports := differenceRoots(serviceRoots, allowedRoots)
		if len(unapprovedServiceImports) == 0 {
			continue
		}

		serviceRegex, err := importRegex(modulePath, unapprovedServiceImports, nil, nil)
		if err != nil {
			return nil, fmt.Errorf(
				"build unapproved-service regex for service %s: %w",
				boundary.Name,
				err,
			)
		}

		specs = append(specs, ruleSpec{
			ID:      "no-" + sanitizeName(boundary.Name) + "-service-unapproved-service-imports",
			Message: serviceImportsMessage(serviceLabel),
			Note:    serviceImportsNote(boundary.PackageRoot, boundary.AllowService),
			Files:   boundary.Files,
			Ignores: boundary.Ignores,
			Regex:   serviceRegex,
		})

		unapprovedAdapterImports := differenceRoots(adapterRoots, boundary.AllowAdapter)
		if len(unapprovedAdapterImports) > 0 {
			adapterRegex, err := importRegex(modulePath, unapprovedAdapterImports, nil, nil)
			if err != nil {
				return nil, fmt.Errorf(
					"build unapproved-adapter regex for service %s: %w",
					boundary.Name,
					err,
				)
			}

			specs = append(specs, ruleSpec{
				ID:      "no-" + sanitizeName(boundary.Name) + "-service-unapproved-adapter-imports",
				Message: serviceAdapterImportsMessage(serviceLabel),
				Note:    serviceAdapterImportsNote(boundary.AllowAdapter),
				Files:   boundary.Files,
				Ignores: boundary.Ignores,
				Regex:   adapterRegex,
			})
		}
	}

	for _, boundary := range policy.AppBoundaries {
		appLabel := strings.TrimSpace(boundary.DisplayName)
		if appLabel == "" {
			appLabel = strings.TrimSpace(boundary.Name)
		}

		unapprovedServiceImports := differenceRoots(serviceRoots, boundary.AllowService)
		if len(unapprovedServiceImports) == 0 {
			continue
		}

		serviceRegex, err := importRegex(modulePath, unapprovedServiceImports, nil, nil)
		if err != nil {
			return nil, fmt.Errorf(
				"build unapproved-service regex for app %s: %w",
				boundary.Name,
				err,
			)
		}

		specs = append(specs, ruleSpec{
			ID:      "no-" + sanitizeName(boundary.Name) + "-app-unapproved-service-imports",
			Message: appServiceImportsMessage(appLabel),
			Note:    appServiceImportsNote(boundary.AllowService),
			Files:   boundary.Files,
			Ignores: boundary.Ignores,
			Regex:   serviceRegex,
		})
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

func serviceImportsMessage(serviceLabel string) string {
	return fmt.Sprintf(
		"%s service must only import explicitly approved service packages.",
		serviceLabel,
	)
}

func serviceImportsNote(packageRoot string, allowed []string) string {
	approved := append([]string{packageRoot}, allowed...)
	return fmt.Sprintf(
		"Approved service imports for this service: %s.",
		formatAllowList(approved),
	)
}

func serviceAdapterImportsMessage(serviceLabel string) string {
	return fmt.Sprintf(
		"%s service must only import explicitly approved adapter packages.",
		serviceLabel,
	)
}

func serviceAdapterImportsNote(allowed []string) string {
	return fmt.Sprintf(
		"Approved adapter imports for this service: %s.",
		formatAllowList(allowed),
	)
}

func appServiceImportsMessage(appLabel string) string {
	return fmt.Sprintf(
		"%s app package must only import explicitly approved service packages.",
		appLabel,
	)
}

func appServiceImportsNote(allowed []string) string {
	return fmt.Sprintf(
		"Approved service imports for this app package: %s.",
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
	for _, existingPath := range existingFiles {
		if err := os.Remove(existingPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("remove stale generated rule %s: %w", existingPath, err)
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

		rulePath := filepath.Join(outDir, spec.ID+".yml")
		if err := os.WriteFile(rulePath, []byte(builder.String()), 0o644); err != nil {
			return fmt.Errorf("write generated rule %s: %w", rulePath, err)
		}
	}

	return nil
}
