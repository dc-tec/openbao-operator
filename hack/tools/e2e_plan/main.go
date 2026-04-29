package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultManifestPath = "test/e2e/suites.yaml"

var idPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type options struct {
	ManifestPath string
	Format       string
	Profile      string
	Lane         string
	Kubernetes   string
}

type nightlyFilters struct {
	Lane       string
	Kubernetes string
}

type manifest struct {
	Version     int               `yaml:"version"`
	Versions    versionPolicy     `yaml:"versions"`
	Parallelism parallelismPolicy `yaml:"parallelism"`
	CILanes     []ciLaneConfig    `yaml:"ciLanes"`
	Nightly     nightlyConfig     `yaml:"nightly"`
}

type versionPolicy struct {
	OpenBao    openBaoVersionPolicy    `yaml:"openbao"`
	Kubernetes kubernetesVersionPolicy `yaml:"kubernetes"`
}

type openBaoVersionPolicy struct {
	DefaultImage string `yaml:"defaultImage"`
}

type kubernetesVersionPolicy struct {
	Primary       string   `yaml:"primary"`
	Compatibility []string `yaml:"compatibility"`
	ReleaseGate   []string `yaml:"releaseGate"`
	NextCandidate string   `yaml:"nextCandidate"`
}

type parallelismPolicy struct {
	DefaultNodes int `yaml:"defaultNodes"`
	MaxNodes     int `yaml:"maxNodes"`
}

type nightlyConfig struct {
	Profiles []nightlyProfile `yaml:"profiles"`
}

type nightlyProfile struct {
	ID          string             `yaml:"id"`
	Description string             `yaml:"description"`
	LaneSets    []nightlyLaneSet   `yaml:"laneSets"`
	Rows        []nightlyRowConfig `yaml:"rows"`
}

type nightlyLaneSet struct {
	Coverage       string   `yaml:"coverage"`
	Kubernetes     []string `yaml:"kubernetes"`
	Lanes          []string `yaml:"lanes"`
	TimeoutMinutes int      `yaml:"timeoutMinutes"`
	E2ETimeout     string   `yaml:"e2eTimeout"`
}

type nightlyRowConfig struct {
	Lane           string `yaml:"lane"`
	Kubernetes     string `yaml:"kubernetes"`
	Coverage       string `yaml:"coverage"`
	LabelFilter    string `yaml:"labelFilter"`
	TimeoutMinutes int    `yaml:"timeoutMinutes"`
	E2ETimeout     string `yaml:"e2eTimeout"`
	OpenBaoImage   string `yaml:"openbaoImage"`
}

type ciLaneConfig struct {
	ID                       string `yaml:"id"`
	Name                     string `yaml:"name"`
	LabelFilter              string `yaml:"labelFilter"`
	PRLabelFilter            string `yaml:"prLabelFilter"`
	PRScope                  string `yaml:"prScope"`
	TimeoutMinutes           int    `yaml:"timeoutMinutes"`
	E2ETimeout               string `yaml:"e2eTimeout"`
	ParallelNodes            int    `yaml:"parallelNodes"`
	IncludeInPRMatrix        *bool  `yaml:"includeInPRMatrix"`
	ExcludePentestOnDefault  bool   `yaml:"excludePentestOnDefaultPR"`
	HardenedSigned           bool   `yaml:"hardenedSigned"`
	OpenBaoImage             string `yaml:"openbaoImage"`
	HardenedInitImage        string `yaml:"hardenedInitImage"`
	HardenedUpgradeImage     string `yaml:"hardenedUpgradeExecutorImage"`
	LoadBackupExecutorImage  bool   `yaml:"loadBackupExecutorImage"`
	LoadUpgradeExecutorImage bool   `yaml:"loadUpgradeExecutorImage"`
	PreloadUpgradeImages     bool   `yaml:"preloadUpgradeImages"`
	PreloadHardenedAssets    bool   `yaml:"preloadHardenedAssets"`
}

type githubMatrix struct {
	Include []matrixRow `json:"include"`
}

type matrixRow struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	LabelFilter              string `json:"label_filter"`
	PRLabelFilter            string `json:"pr_label_filter"`
	PRScope                  string `json:"pr_scope"`
	TimeoutMinutes           int    `json:"timeout_minutes"`
	E2ETimeout               string `json:"e2e_timeout"`
	ParallelNodes            int    `json:"parallel_nodes"`
	Kubernetes               string `json:"k8s,omitempty"`
	KindNodeImage            string `json:"kind_node_image,omitempty"`
	Coverage                 string `json:"coverage,omitempty"`
	Profile                  string `json:"profile,omitempty"`
	ExcludePentestOnDefault  string `json:"exclude_pentest_on_default_pr"`
	HardenedSigned           string `json:"hardened_signed"`
	OpenBaoImage             string `json:"openbao_image"`
	HardenedInitImage        string `json:"hardened_init_image"`
	HardenedUpgradeImage     string `json:"hardened_upgrade_executor_image"`
	LoadBackupExecutorImage  string `json:"load_backup_executor_image"`
	LoadUpgradeExecutorImage string `json:"load_upgrade_executor_image"`
	PreloadUpgradeImages     string `json:"preload_upgrade_images"`
	PreloadHardenedAssets    string `json:"preload_hardened_assets"`
}

func main() {
	opts, err := parseOptions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e_plan: %v\n", err)
		os.Exit(2)
	}

	m, err := loadManifest(opts.ManifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e_plan: %v\n", err)
		os.Exit(1)
	}

	switch opts.Format {
	case "github-matrix":
		matrix, err := buildGithubMatrix(m)
		if err != nil {
			fmt.Fprintf(os.Stderr, "e2e_plan: %v\n", err)
			os.Exit(1)
		}
		if err := writeJSON(matrix); err != nil {
			fmt.Fprintf(os.Stderr, "e2e_plan: %v\n", err)
			os.Exit(1)
		}
	case "github-nightly-matrix":
		matrix, err := buildGithubNightlyMatrix(m, opts.Profile, nightlyFilters{
			Lane:       opts.Lane,
			Kubernetes: opts.Kubernetes,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "e2e_plan: %v\n", err)
			os.Exit(1)
		}
		if err := writeJSON(matrix); err != nil {
			fmt.Fprintf(os.Stderr, "e2e_plan: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "e2e_plan: unsupported format %q\n", opts.Format)
		os.Exit(2)
	}
}

func parseOptions() (options, error) {
	var opts options
	flag.StringVar(&opts.ManifestPath, "manifest", defaultManifestPath, "E2E suite manifest path")
	flag.StringVar(&opts.Format, "format", "github-matrix", "output format: github-matrix or github-nightly-matrix")
	flag.StringVar(&opts.Profile, "profile", "daily", "nightly profile for github-nightly-matrix")
	flag.StringVar(&opts.Lane, "lane", "", "optional nightly lane id filter for github-nightly-matrix")
	flag.StringVar(&opts.Kubernetes, "kubernetes", "", "optional Kubernetes version filter for github-nightly-matrix")
	flag.Parse()

	if strings.TrimSpace(opts.ManifestPath) == "" {
		return options{}, fmt.Errorf("manifest path is required")
	}
	opts.Profile = strings.TrimSpace(opts.Profile)
	opts.Lane = strings.TrimSpace(opts.Lane)
	opts.Kubernetes = strings.TrimSpace(opts.Kubernetes)
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

func buildGithubMatrix(m manifest) (githubMatrix, error) {
	var errs []string
	if m.Version != 1 {
		errs = append(errs, fmt.Sprintf("version = %d, want 1", m.Version))
	}
	errs = append(errs, validateVersionPolicy(m.Versions)...)
	errs = append(errs, validateParallelismPolicy(m.Parallelism)...)
	if len(m.CILanes) == 0 {
		errs = append(errs, "ciLanes must not be empty")
	}

	seen := map[string]bool{}
	matrix := githubMatrix{Include: make([]matrixRow, 0, len(m.CILanes))}
	for idx, lane := range m.CILanes {
		prefix := fmt.Sprintf("ciLanes[%d]", idx)
		errs = append(errs, validateLane(prefix, lane, seen, m.Parallelism)...)
		if includeInPRMatrix(lane) {
			matrix.Include = append(matrix.Include, matrixRowFromLane(m.Versions, m.Parallelism, lane))
		}
	}

	if len(errs) > 0 {
		return githubMatrix{}, fmt.Errorf("build github matrix:\n- %s", strings.Join(errs, "\n- "))
	}
	return matrix, nil
}

func buildGithubNightlyMatrix(m manifest, profileID string, filters nightlyFilters) (githubMatrix, error) {
	lanes, errs := validateLanes(m)
	errs = append(errs, validateVersionPolicy(m.Versions)...)
	profile, ok := findNightlyProfile(m.Nightly.Profiles, profileID)
	if !ok {
		errs = append(errs, fmt.Sprintf("nightly profile %q is not defined", profileID))
	}
	if filters.Lane != "" {
		if _, ok := lanes[filters.Lane]; !ok {
			errs = append(errs, fmt.Sprintf("nightly lane filter %q is not defined", filters.Lane))
		}
	}
	if len(errs) > 0 {
		return githubMatrix{}, fmt.Errorf("build github nightly matrix:\n- %s", strings.Join(errs, "\n- "))
	}

	var rows []matrixRow
	for _, set := range profile.LaneSets {
		versions, err := resolveKubernetesRefs(m.Versions.Kubernetes, set.Kubernetes)
		if err != nil {
			errs = append(errs, fmt.Sprintf("nightly profile %s lane set %q: %v", profile.ID, set.Coverage, err))
			continue
		}
		for _, version := range versions {
			if filters.Kubernetes != "" && version != filters.Kubernetes {
				continue
			}
			for _, laneID := range set.Lanes {
				if filters.Lane != "" && laneID != filters.Lane {
					continue
				}
				lane, ok := lanes[laneID]
				if !ok {
					errs = append(errs, fmt.Sprintf("nightly profile %s references undefined lane %q", profile.ID, laneID))
					continue
				}
				row := matrixRowFromNightlyLane(profile.ID, lane, nightlyRowConfig{
					Lane:           laneID,
					Kubernetes:     version,
					Coverage:       set.Coverage,
					TimeoutMinutes: set.TimeoutMinutes,
					E2ETimeout:     set.E2ETimeout,
				}, m.Versions, m.Parallelism)
				rows = append(rows, row)
			}
		}
	}
	for _, config := range profile.Rows {
		versions, err := resolveKubernetesRefs(m.Versions.Kubernetes, []string{config.Kubernetes})
		if err != nil {
			errs = append(errs, fmt.Sprintf("nightly profile %s row %q: %v", profile.ID, config.Lane, err))
			continue
		}
		lane, ok := lanes[config.Lane]
		if !ok {
			errs = append(errs, fmt.Sprintf("nightly profile %s references undefined lane %q", profile.ID, config.Lane))
			continue
		}
		if filters.Lane != "" && config.Lane != filters.Lane {
			continue
		}
		for _, version := range versions {
			if filters.Kubernetes != "" && version != filters.Kubernetes {
				continue
			}
			config.Kubernetes = version
			rows = append(rows, matrixRowFromNightlyLane(profile.ID, lane, config, m.Versions, m.Parallelism))
		}
	}
	if len(errs) > 0 {
		return githubMatrix{}, fmt.Errorf("build github nightly matrix:\n- %s", strings.Join(errs, "\n- "))
	}
	if len(rows) == 0 {
		return githubMatrix{}, fmt.Errorf(
			"build github nightly matrix:\n- nightly profile %q produced no rows for lane %q and kubernetes %q",
			profile.ID,
			filters.Lane,
			filters.Kubernetes,
		)
	}
	return githubMatrix{Include: rows}, nil
}

func findNightlyProfile(profiles []nightlyProfile, id string) (nightlyProfile, bool) {
	for _, profile := range profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return nightlyProfile{}, false
}

func includeInPRMatrix(lane ciLaneConfig) bool {
	return lane.IncludeInPRMatrix == nil || *lane.IncludeInPRMatrix
}

func validateLane(prefix string, lane ciLaneConfig, seen map[string]bool, parallelism parallelismPolicy) []string {
	var errs []string
	if lane.ID == "" {
		errs = append(errs, fmt.Sprintf("%s.id is required", prefix))
	} else {
		if !idPattern.MatchString(lane.ID) {
			errs = append(errs, fmt.Sprintf("%s.id %q must be a lowercase slug", prefix, lane.ID))
		}
		if seen[lane.ID] {
			errs = append(errs, fmt.Sprintf("%s.id %q is duplicated", prefix, lane.ID))
		}
		seen[lane.ID] = true
	}
	if strings.TrimSpace(lane.Name) == "" {
		errs = append(errs, fmt.Sprintf("%s.name is required", prefix))
	}
	if strings.TrimSpace(lane.LabelFilter) == "" {
		errs = append(errs, fmt.Sprintf("%s.labelFilter is required", prefix))
	}
	if !allowedPRScopes[lane.PRScope] {
		errs = append(errs, fmt.Sprintf("%s.prScope %q is not recognized", prefix, lane.PRScope))
	}
	if lane.TimeoutMinutes <= 0 {
		errs = append(errs, fmt.Sprintf("%s.timeoutMinutes must be > 0", prefix))
	}
	if strings.TrimSpace(lane.E2ETimeout) == "" {
		errs = append(errs, fmt.Sprintf("%s.e2eTimeout is required", prefix))
	}
	errs = append(errs, validateLaneParallelism(prefix, lane, parallelism)...)
	return errs
}

func validateLanes(m manifest) (map[string]ciLaneConfig, []string) {
	var errs []string
	if m.Version != 1 {
		errs = append(errs, fmt.Sprintf("version = %d, want 1", m.Version))
	}
	if len(m.CILanes) == 0 {
		errs = append(errs, "ciLanes must not be empty")
	}
	errs = append(errs, validateParallelismPolicy(m.Parallelism)...)

	seen := map[string]bool{}
	lanes := map[string]ciLaneConfig{}
	for idx, lane := range m.CILanes {
		prefix := fmt.Sprintf("ciLanes[%d]", idx)
		errs = append(errs, validateLane(prefix, lane, seen, m.Parallelism)...)
		if lane.ID != "" {
			lanes[lane.ID] = lane
		}
	}
	return lanes, errs
}

func validateVersionPolicy(policy versionPolicy) []string {
	var errs []string
	if strings.TrimSpace(policy.OpenBao.DefaultImage) == "" {
		errs = append(errs, "versions.openbao.defaultImage is required")
	}
	if strings.TrimSpace(policy.Kubernetes.Primary) == "" {
		errs = append(errs, "versions.kubernetes.primary is required")
	}
	if len(policy.Kubernetes.Compatibility) == 0 {
		errs = append(errs, "versions.kubernetes.compatibility must not be empty")
	}
	if len(policy.Kubernetes.ReleaseGate) == 0 {
		errs = append(errs, "versions.kubernetes.releaseGate must not be empty")
	}
	errs = append(errs, validateVersionList("versions.kubernetes.compatibility", policy.Kubernetes.Compatibility)...)
	errs = append(errs, validateVersionList("versions.kubernetes.releaseGate", policy.Kubernetes.ReleaseGate)...)
	return errs
}

func validateParallelismPolicy(policy parallelismPolicy) []string {
	var errs []string
	if policy.DefaultNodes <= 0 {
		errs = append(errs, "parallelism.defaultNodes must be > 0")
	}
	if policy.MaxNodes <= 0 {
		errs = append(errs, "parallelism.maxNodes must be > 0")
	}
	if policy.DefaultNodes > 0 && policy.MaxNodes > 0 && policy.DefaultNodes > policy.MaxNodes {
		errs = append(errs, "parallelism.defaultNodes must be <= parallelism.maxNodes")
	}
	return errs
}

func validateLaneParallelism(prefix string, lane ciLaneConfig, policy parallelismPolicy) []string {
	var errs []string
	if lane.ParallelNodes < 0 {
		errs = append(errs, fmt.Sprintf("%s.parallelNodes must be >= 0", prefix))
	}
	if lane.ParallelNodes > 0 && policy.MaxNodes > 0 && lane.ParallelNodes > policy.MaxNodes {
		errs = append(errs, fmt.Sprintf("%s.parallelNodes must be <= parallelism.maxNodes (%d)", prefix, policy.MaxNodes))
	}
	return errs
}

func validateVersionList(prefix string, versions []string) []string {
	var errs []string
	for idx, version := range versions {
		if strings.TrimSpace(version) == "" {
			errs = append(errs, fmt.Sprintf("%s[%d] must not be empty", prefix, idx))
		}
	}
	return errs
}

func resolveKubernetesRefs(policy kubernetesVersionPolicy, refs []string) ([]string, error) {
	var resolved []string
	seen := map[string]bool{}
	for _, ref := range refs {
		versions, err := kubernetesVersionsForRef(policy, strings.TrimSpace(ref))
		if err != nil {
			return nil, err
		}
		for _, version := range versions {
			if version == "" || seen[version] {
				continue
			}
			seen[version] = true
			resolved = append(resolved, version)
		}
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("kubernetes version reference list resolved to no versions")
	}
	return resolved, nil
}

func kubernetesVersionsForRef(policy kubernetesVersionPolicy, ref string) ([]string, error) {
	switch ref {
	case "@primary":
		return []string{strings.TrimSpace(policy.Primary)}, nil
	case "@compatibility":
		return normalizeVersions(policy.Compatibility), nil
	case "@releaseGate":
		return normalizeVersions(policy.ReleaseGate), nil
	default:
		if strings.HasPrefix(ref, "@") {
			return nil, fmt.Errorf("unknown Kubernetes version set %q", ref)
		}
		return []string{ref}, nil
	}
}

func normalizeVersions(versions []string) []string {
	out := make([]string, 0, len(versions))
	for _, version := range versions {
		if version = strings.TrimSpace(version); version != "" {
			out = append(out, version)
		}
	}
	return out
}

func matrixRowFromLane(policy versionPolicy, parallelism parallelismPolicy, lane ciLaneConfig) matrixRow {
	openBaoImage := strings.TrimSpace(lane.OpenBaoImage)
	if openBaoImage == "" || openBaoImage == "@default" {
		openBaoImage = policy.OpenBao.DefaultImage
	}
	return matrixRow{
		ID:                       lane.ID,
		Name:                     lane.Name,
		LabelFilter:              lane.LabelFilter,
		PRLabelFilter:            lane.PRLabelFilter,
		PRScope:                  lane.PRScope,
		TimeoutMinutes:           lane.TimeoutMinutes,
		E2ETimeout:               lane.E2ETimeout,
		ParallelNodes:            effectiveParallelNodes(parallelism, lane),
		Kubernetes:               policy.Kubernetes.Primary,
		KindNodeImage:            "kindest/node:v" + policy.Kubernetes.Primary,
		ExcludePentestOnDefault:  strconv.FormatBool(lane.ExcludePentestOnDefault),
		HardenedSigned:           strconv.FormatBool(lane.HardenedSigned),
		OpenBaoImage:             openBaoImage,
		HardenedInitImage:        lane.HardenedInitImage,
		HardenedUpgradeImage:     lane.HardenedUpgradeImage,
		LoadBackupExecutorImage:  strconv.FormatBool(lane.LoadBackupExecutorImage),
		LoadUpgradeExecutorImage: strconv.FormatBool(lane.LoadUpgradeExecutorImage),
		PreloadUpgradeImages:     strconv.FormatBool(lane.PreloadUpgradeImages),
		PreloadHardenedAssets:    strconv.FormatBool(lane.PreloadHardenedAssets),
	}
}

func effectiveParallelNodes(policy parallelismPolicy, lane ciLaneConfig) int {
	if lane.ParallelNodes > 0 {
		return lane.ParallelNodes
	}
	if policy.DefaultNodes > 0 {
		return policy.DefaultNodes
	}
	return 1
}

func matrixRowFromNightlyLane(
	profile string,
	lane ciLaneConfig,
	config nightlyRowConfig,
	policy versionPolicy,
	parallelism parallelismPolicy,
) matrixRow {
	row := matrixRowFromLane(policy, parallelism, lane)
	row.Kubernetes = config.Kubernetes
	row.KindNodeImage = "kindest/node:v" + config.Kubernetes
	row.Coverage = config.Coverage
	row.Profile = profile
	if config.OpenBaoImage != "" && config.OpenBaoImage != "@default" {
		row.OpenBaoImage = config.OpenBaoImage
	}
	if config.LabelFilter != "" {
		row.LabelFilter = config.LabelFilter
	}
	if config.TimeoutMinutes > 0 {
		row.TimeoutMinutes = config.TimeoutMinutes
	}
	if config.E2ETimeout != "" {
		row.E2ETimeout = config.E2ETimeout
	}
	return row
}

var allowedPRScopes = map[string]bool{
	"always":   true,
	"backup":   true,
	"hardened": true,
	"manual":   true,
	"upgrade":  true,
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}
