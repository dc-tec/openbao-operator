package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultManifestPath = "test/e2e/suites.yaml"
	defaultCatalogPath  = "test/e2e/catalog/cases.json"
)

var idPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type options struct {
	ManifestPath   string
	CatalogPath    string
	RequireCaseIDs bool
}

type manifest struct {
	Version int             `yaml:"version"`
	Suites  []manifestSuite `yaml:"suites"`
}

type manifestSuite struct {
	ID        string       `yaml:"id"`
	Title     string       `yaml:"title"`
	Owner     string       `yaml:"owner"`
	RiskTier  string       `yaml:"riskTier"`
	Isolation string       `yaml:"isolation"`
	Files     []string     `yaml:"files"`
	Labels    []string     `yaml:"labels"`
	Coverage  []string     `yaml:"coverage"`
	CI        suiteCI      `yaml:"ci"`
	Nightly   suiteNightly `yaml:"nightly"`
}

type suiteCI struct {
	Lanes       []string `yaml:"lanes"`
	PullRequest string   `yaml:"pullRequest"`
}

type suiteNightly struct {
	Policy string `yaml:"policy"`
}

type catalogCase struct {
	ID           string   `json:"id"`
	CaseLabel    string   `json:"caseLabel"`
	File         string   `json:"file"`
	Path         []string `json:"path"`
	DomainLabels []string `json:"domainLabels"`
	Coverage     []string `json:"coverage"`
	Pending      bool     `json:"pending"`
}

type suiteFacts struct {
	File           string
	Title          string
	Labels         []string
	Coverage       []string
	Cases          int
	ExplicitCaseID int
	Pending        int
}

func main() {
	opts, err := parseOptions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e_manifest: %v\n", err)
		os.Exit(2)
	}

	m, err := loadManifest(opts.ManifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e_manifest: %v\n", err)
		os.Exit(1)
	}

	cases, err := loadCatalog(opts.CatalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e_manifest: %v\n", err)
		os.Exit(1)
	}

	facts := buildSuiteFacts(cases)
	if err := validateManifest(m, facts, opts); err != nil {
		fmt.Fprintf(os.Stderr, "e2e_manifest: %v\n", err)
		os.Exit(1)
	}

	totalCases := 0
	explicitCaseIDs := 0
	for _, fact := range facts {
		totalCases += fact.Cases
		explicitCaseIDs += fact.ExplicitCaseID
	}
	fmt.Printf(
		"E2E manifest valid: %d suites, %d files, %d cases, %d explicit case IDs\n",
		len(m.Suites),
		len(facts),
		totalCases,
		explicitCaseIDs,
	)
}

func parseOptions() (options, error) {
	var opts options
	flag.StringVar(&opts.ManifestPath, "manifest", defaultManifestPath, "E2E suite manifest path")
	flag.StringVar(&opts.CatalogPath, "catalog", defaultCatalogPath, "E2E catalog cases.json path")
	flag.BoolVar(&opts.RequireCaseIDs, "require-case-ids", false, "fail if any catalog case lacks an explicit case: label")
	flag.Parse()

	if strings.TrimSpace(opts.ManifestPath) == "" {
		return options{}, fmt.Errorf("manifest path is required")
	}
	if strings.TrimSpace(opts.CatalogPath) == "" {
		return options{}, fmt.Errorf("catalog path is required")
	}
	return opts, nil
}

func loadManifest(path string) (manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}

	var m manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return manifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	return m, nil
}

func loadCatalog(path string) ([]catalogCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog %s: %w", path, err)
	}

	var cases []catalogCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("parse catalog %s: %w", path, err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("catalog %s has no cases", path)
	}
	return cases, nil
}

func buildSuiteFacts(cases []catalogCase) map[string]suiteFacts {
	byFile := map[string][]catalogCase{}
	for _, tc := range cases {
		file := filepath.ToSlash(tc.File)
		byFile[file] = append(byFile[file], tc)
	}

	facts := map[string]suiteFacts{}
	for file, fileCases := range byFile {
		fact := suiteFacts{
			File:  file,
			Title: suiteTitle(fileCases),
			Cases: len(fileCases),
		}
		labelSet := map[string]bool{}
		coverageSet := map[string]bool{}
		for _, tc := range fileCases {
			if strings.TrimSpace(tc.CaseLabel) != "" {
				fact.ExplicitCaseID++
			}
			if tc.Pending {
				fact.Pending++
			}
			for _, label := range tc.DomainLabels {
				label = strings.TrimSpace(label)
				if label != "" {
					labelSet[label] = true
				}
			}
			for _, coverage := range tc.Coverage {
				coverage = strings.TrimSpace(coverage)
				if coverage != "" {
					coverageSet[coverage] = true
				}
			}
		}
		fact.Labels = sortedKeys(labelSet)
		fact.Coverage = sortedKeys(coverageSet)
		facts[file] = fact
	}
	return facts
}

func validateManifest(m manifest, facts map[string]suiteFacts, opts options) error {
	var errs []string
	if m.Version != 1 {
		errs = append(errs, fmt.Sprintf("version = %d, want 1", m.Version))
	}
	if len(m.Suites) == 0 {
		errs = append(errs, "suites must not be empty")
	}

	seenIDs := map[string]string{}
	seenFiles := map[string]string{}
	for idx, suite := range m.Suites {
		prefix := fmt.Sprintf("suites[%d]", idx)
		errs = append(errs, validateSuite(prefix, suite, facts, seenIDs, seenFiles)...)
	}

	for file := range facts {
		if seenFiles[file] == "" {
			errs = append(errs, fmt.Sprintf("catalog file %s is not owned by any manifest suite", file))
		}
	}

	if opts.RequireCaseIDs {
		for _, fact := range sortedFacts(facts) {
			if fact.ExplicitCaseID != fact.Cases {
				errs = append(errs, fmt.Sprintf("%s has %d/%d explicit case IDs", fact.File, fact.ExplicitCaseID, fact.Cases))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("manifest validation failed:\n- %s", strings.Join(errs, "\n- "))
	}
	return nil
}

func validateSuite(
	prefix string,
	suite manifestSuite,
	facts map[string]suiteFacts,
	seenIDs map[string]string,
	seenFiles map[string]string,
) []string {
	var errs []string
	if suite.ID == "" {
		errs = append(errs, fmt.Sprintf("%s.id is required", prefix))
	} else {
		if !idPattern.MatchString(suite.ID) {
			errs = append(errs, fmt.Sprintf("%s.id %q must be a lowercase slug", prefix, suite.ID))
		}
		if previous := seenIDs[suite.ID]; previous != "" {
			errs = append(errs, fmt.Sprintf("%s.id %q duplicates %s", prefix, suite.ID, previous))
		}
		seenIDs[suite.ID] = prefix
	}

	if strings.TrimSpace(suite.Title) == "" {
		errs = append(errs, fmt.Sprintf("%s.title is required", prefix))
	}
	if !allowedOwners[suite.Owner] {
		errs = append(errs, fmt.Sprintf("%s.owner %q is not recognized", prefix, suite.Owner))
	}
	if !allowedRiskTiers[suite.RiskTier] {
		errs = append(errs, fmt.Sprintf("%s.riskTier %q is not recognized", prefix, suite.RiskTier))
	}
	if !allowedIsolation[suite.Isolation] {
		errs = append(errs, fmt.Sprintf("%s.isolation %q is not recognized", prefix, suite.Isolation))
	}
	if len(suite.Files) == 0 {
		errs = append(errs, fmt.Sprintf("%s.files must not be empty", prefix))
	}
	if len(suite.Labels) == 0 {
		errs = append(errs, fmt.Sprintf("%s.labels must not be empty", prefix))
	}
	if len(suite.CI.Lanes) == 0 {
		errs = append(errs, fmt.Sprintf("%s.ci.lanes must not be empty", prefix))
	}
	for _, lane := range suite.CI.Lanes {
		if !allowedCILanes[lane] {
			errs = append(errs, fmt.Sprintf("%s.ci.lanes contains unknown lane %q", prefix, lane))
		}
	}
	if !allowedPullRequestPolicies[suite.CI.PullRequest] {
		errs = append(errs, fmt.Sprintf("%s.ci.pullRequest %q is not recognized", prefix, suite.CI.PullRequest))
	}
	if !allowedNightlyPolicies[suite.Nightly.Policy] {
		errs = append(errs, fmt.Sprintf("%s.nightly.policy %q is not recognized", prefix, suite.Nightly.Policy))
	}

	var suiteLabels []string
	var suiteCoverage []string
	suiteTitles := make([]string, 0, len(suite.Files))
	for _, file := range suite.Files {
		file = filepath.ToSlash(strings.TrimSpace(file))
		if file == "" {
			errs = append(errs, fmt.Sprintf("%s.files contains an empty path", prefix))
			continue
		}
		if previous := seenFiles[file]; previous != "" {
			errs = append(errs, fmt.Sprintf("%s.files contains %s already owned by %s", prefix, file, previous))
		}
		seenFiles[file] = suite.ID
		if _, err := os.Stat(file); err != nil {
			errs = append(errs, fmt.Sprintf("%s.files contains %s that cannot be read: %v", prefix, file, err))
		}
		fact, ok := facts[file]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s.files contains %s that is absent from catalog", prefix, file))
			continue
		}
		suiteLabels = append(suiteLabels, fact.Labels...)
		suiteCoverage = append(suiteCoverage, fact.Coverage...)
		suiteTitles = append(suiteTitles, fact.Title)
	}

	if len(suiteTitles) == 1 && suite.Title != suiteTitles[0] {
		errs = append(errs, fmt.Sprintf("%s.title %q does not match catalog title %q", prefix, suite.Title, suiteTitles[0]))
	}
	if got, want := normalizeList(suite.Labels), normalizeList(suiteLabels); !equalStringSlices(got, want) {
		errs = append(
			errs,
			fmt.Sprintf("%s.labels = [%s], want catalog labels [%s]", prefix, strings.Join(got, ", "), strings.Join(want, ", ")),
		)
	}
	if got, want := normalizeList(suite.Coverage), normalizeList(suiteCoverage); !equalStringSlices(got, want) {
		errs = append(
			errs,
			fmt.Sprintf(
				"%s.coverage = [%s], want catalog coverage [%s]",
				prefix,
				strings.Join(got, ", "),
				strings.Join(want, ", "),
			),
		)
	}

	return errs
}

var allowedOwners = map[string]bool{
	"backup-restore": true,
	"core":           true,
	"gitops":         true,
	"hardening":      true,
	"manager":        true,
	"platform":       true,
	"security":       true,
	"upgrade":        true,
}

var allowedRiskTiers = map[string]bool{
	"critical":    true,
	"high":        true,
	"medium":      true,
	"low":         true,
	"exploratory": true,
}

var allowedIsolation = map[string]bool{
	"parallel-safe":    true,
	"shared-cluster":   true,
	"global-mutator":   true,
	"serial":           true,
	"external-cluster": true,
	"multi-cluster":    true,
}

var allowedCILanes = map[string]bool{
	"backup-restore":     true,
	"core":               true,
	"hardened":           true,
	"platform-openshift": true,
	"security":           true,
	"upgrade-bluegreen":  true,
	"upgrade-rolling":    true,
}

var allowedPullRequestPolicies = map[string]bool{
	"always":        true,
	"changed-paths": true,
	"manual":        true,
}

var allowedNightlyPolicies = map[string]bool{
	"external-cluster-scheduled": true,
	"primary-version-full":       true,
}

func suiteTitle(cases []catalogCase) string {
	for _, tc := range cases {
		if len(tc.Path) > 0 && strings.TrimSpace(tc.Path[0]) != "" {
			return strings.TrimSpace(tc.Path[0])
		}
	}
	return ""
}

func sortedFacts(facts map[string]suiteFacts) []suiteFacts {
	out := make([]suiteFacts, 0, len(facts))
	for _, fact := range facts {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].File < out[j].File
	})
	return out
}

func normalizeList(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = true
		}
	}
	return sortedKeys(seen)
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
