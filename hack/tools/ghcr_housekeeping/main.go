package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	defaultOwner                  = "dc-tec"
	defaultOwnerKind              = "org"
	defaultPolicyPath             = "hack/tools/ghcr_housekeeping/policy.json"
	defaultReportPath             = "dist/housekeeping-report.json"
	defaultMode                   = "dry-run"
	defaultMaxDelete              = 100
	ownerKindOrg                  = "org"
	ownerKindUser                 = "user"
	modeDryRun                    = "dry-run"
	modeEnforce                   = "enforce"
	actionKeep                    = "keep"
	actionDeleteAfter             = "delete_after"
	githubAPIBaseURL              = "https://api.github.com"
	githubAPIVersion              = "2022-11-28"
	perPage                       = 100
	requestTimeout                = 30 * time.Second
	summaryEnvVar                 = "GITHUB_STEP_SUMMARY"
	defaultErrorMessage           = "unknown API error"
	unknownReasonUntagged         = unknownReason("untagged")
	unknownReasonUnmatchedTag     = unknownReason("unmatched_tag")
	unknownReasonNoTransientMatch = unknownReason("no_transient_match")
	summaryTableHeader            = "| Package | Scanned | Candidates | Deleted | Kept protected | Kept unknown | " +
		"Unknown untagged | Unknown unmatched | Unknown no-transient | Errors |\n"
)

var defaultPackages = []string{
	"openbao-operator",
	"openbao-init",
	"openbao-backup",
	"openbao-upgrade",
	"ci-e2e-openbao-operator",
	"ci-e2e-openbao-init",
	"ci-e2e-openbao-backup",
	"ci-e2e-openbao-upgrade",
	"ci-e2e-openbao-softhsm",
	"ci-e2e-pykmip-server",
	"pr-e2e-openbao-operator",
	"pr-e2e-openbao-init",
	"pr-e2e-openbao-backup",
	"pr-e2e-openbao-upgrade",
	"pr-e2e-openbao-softhsm",
	"pr-e2e-pykmip-server",
	"nightly-e2e-openbao-operator",
	"nightly-e2e-openbao-init",
	"nightly-e2e-openbao-backup",
	"nightly-e2e-openbao-upgrade",
}

type multiStringFlag []string

func (m *multiStringFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiStringFlag) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	*m = append(*m, trimmed)
	return nil
}

type options struct {
	Owner               string
	OwnerKind           string
	Packages            []string
	Mode                string
	PolicyFile          string
	MaxDeletePerPackage int
	ReportJSON          string
}

type policyConfig struct {
	ProtectUnknown bool         `json:"protect_unknown"`
	Rules          []policyRule `json:"rules"`
}

type policyRule struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Action  string `json:"action"`
	TTLDays int    `json:"ttl_days,omitempty"`
}

type compiledRule struct {
	Name    string
	Pattern string
	Action  string
	TTLDays int
	Regexp  *regexp.Regexp
}

type housekeepingReport struct {
	Run      runReport       `json:"run"`
	Packages []packageReport `json:"packages"`
}

type runReport struct {
	Mode                string `json:"mode"`
	TimestampUTC        string `json:"timestamp_utc"`
	Owner               string `json:"owner"`
	OwnerKind           string `json:"owner_kind"`
	PolicyFile          string `json:"policy_file"`
	MaxDeletePerPackage int    `json:"max_delete_per_package"`
}

type packageReport struct {
	Name                        string            `json:"name"`
	ScannedVersions             int               `json:"scanned_versions"`
	Candidates                  int               `json:"candidates"`
	Deleted                     int               `json:"deleted"`
	KeptProtected               int               `json:"kept_protected"`
	KeptUnknown                 int               `json:"kept_unknown"`
	KeptUnknownUntagged         int               `json:"kept_unknown_untagged"`
	KeptUnknownUnmatchedTag     int               `json:"kept_unknown_unmatched_tag"`
	KeptUnknownNoTransientMatch int               `json:"kept_unknown_no_transient_match"`
	Errors                      []string          `json:"errors,omitempty"`
	CandidateItems              []candidateReport `json:"candidate_items,omitempty"`
}

type candidateReport struct {
	ID              int64    `json:"id"`
	Name            string   `json:"name"`
	UpdatedAt       string   `json:"updated_at"`
	AgeDays         int      `json:"age_days"`
	RequiredAgeDays int      `json:"required_age_days"`
	Tags            []string `json:"tags"`
	MatchedRules    []string `json:"matched_rules"`
}

type packageClient interface {
	ListPackageVersions(ctx context.Context, ownerKind, owner, pkg string) ([]packageVersion, error)
	DeletePackageVersion(ctx context.Context, ownerKind, owner, pkg string, id int64) error
}

type packageVersion struct {
	ID        int64
	Name      string
	UpdatedAt time.Time
	Tags      []string
}

type versionEval struct {
	Unknown         bool
	UnknownReason   unknownReason
	Protected       bool
	Candidate       bool
	AgeDays         int
	RequiredAgeDays int
	MatchedRules    []string
}

type unknownReason string

type tagEval struct {
	Known    bool
	Action   string
	TTLDays  int
	RuleName string
}

type githubPackagesClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func main() {
	opts, err := parseOptions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ghcr_housekeeping: %v\n", err)
		os.Exit(2)
	}

	policy, rules, err := loadPolicy(opts.PolicyFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ghcr_housekeeping: %v\n", err)
		os.Exit(1)
	}

	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GH_TOKEN"))
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "ghcr_housekeeping: missing token. Set GITHUB_TOKEN (preferred) or GH_TOKEN.")
		os.Exit(1)
	}

	client := &githubPackagesClient{
		baseURL: githubAPIBaseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}

	now := time.Now().UTC()
	report, runErr := runHousekeeping(context.Background(), opts, policy, rules, client, now)

	if err := writeReportJSON(opts.ReportJSON, report); err != nil {
		fmt.Fprintf(os.Stderr, "ghcr_housekeeping: write report %s: %v\n", opts.ReportJSON, err)
		os.Exit(1)
	}

	summary := renderSummary(report)
	fmt.Print(summary)
	if err := writeStepSummary(summary); err != nil {
		fmt.Fprintf(os.Stderr, "ghcr_housekeeping: write step summary: %v\n", err)
		os.Exit(1)
	}

	if runErr != nil {
		fmt.Fprintf(os.Stderr, "ghcr_housekeeping: %v\n", runErr)
		os.Exit(1)
	}
}

func parseOptions() (options, error) {
	opts := options{}
	var pkgFlags multiStringFlag

	flag.StringVar(&opts.Owner, "owner", defaultOwner, "Package owner (org/user)")
	flag.StringVar(&opts.OwnerKind, "owner-kind", defaultOwnerKind, "Owner type: org or user")
	flag.Var(&pkgFlags, "package", "Container package name (repeatable). Defaults to operator image packages")
	flag.StringVar(&opts.Mode, "mode", defaultMode, "Run mode: dry-run or enforce")
	flag.StringVar(&opts.PolicyFile, "policy-file", defaultPolicyPath, "Policy JSON file path")
	flag.IntVar(
		&opts.MaxDeletePerPackage,
		"max-delete-per-package",
		defaultMaxDelete,
		"Safety brake: max candidates allowed per package in enforce mode",
	)
	flag.StringVar(&opts.ReportJSON, "report-json", defaultReportPath, "Output JSON report path")
	flag.Parse()

	opts.Owner = strings.TrimSpace(opts.Owner)
	opts.OwnerKind = strings.TrimSpace(opts.OwnerKind)
	opts.Mode = strings.TrimSpace(opts.Mode)
	opts.PolicyFile = strings.TrimSpace(opts.PolicyFile)
	opts.ReportJSON = strings.TrimSpace(opts.ReportJSON)

	if len(pkgFlags) == 0 {
		opts.Packages = append(opts.Packages, defaultPackages...)
	} else {
		opts.Packages = append(opts.Packages, pkgFlags...)
	}

	if opts.Owner == "" {
		return opts, errors.New("--owner is required")
	}
	if opts.OwnerKind != ownerKindOrg && opts.OwnerKind != ownerKindUser {
		return opts, fmt.Errorf("--owner-kind must be %q or %q", ownerKindOrg, ownerKindUser)
	}
	if opts.Mode != modeDryRun && opts.Mode != modeEnforce {
		return opts, fmt.Errorf("--mode must be %q or %q", modeDryRun, modeEnforce)
	}
	if opts.PolicyFile == "" {
		return opts, errors.New("--policy-file is required")
	}
	if opts.ReportJSON == "" {
		return opts, errors.New("--report-json is required")
	}
	if opts.MaxDeletePerPackage <= 0 {
		return opts, errors.New("--max-delete-per-package must be > 0")
	}
	for _, pkg := range opts.Packages {
		if strings.TrimSpace(pkg) == "" {
			return opts, errors.New("--package entries must not be empty")
		}
	}

	return opts, nil
}

func loadPolicy(path string) (policyConfig, []compiledRule, error) {
	var cfg policyConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil, fmt.Errorf("read policy file %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, nil, fmt.Errorf("parse policy file %s: %w", path, err)
	}
	if len(cfg.Rules) == 0 {
		return cfg, nil, fmt.Errorf("policy %s has no rules", path)
	}

	compiled := make([]compiledRule, 0, len(cfg.Rules))
	for i, rule := range cfg.Rules {
		rule.Name = strings.TrimSpace(rule.Name)
		rule.Pattern = strings.TrimSpace(rule.Pattern)
		rule.Action = strings.TrimSpace(rule.Action)
		if rule.Name == "" {
			return cfg, nil, fmt.Errorf("policy rule #%d has empty name", i+1)
		}
		if rule.Pattern == "" {
			return cfg, nil, fmt.Errorf("policy rule %q has empty pattern", rule.Name)
		}
		if rule.Action != actionKeep && rule.Action != actionDeleteAfter {
			return cfg, nil, fmt.Errorf("policy rule %q has unsupported action %q", rule.Name, rule.Action)
		}
		if rule.Action == actionKeep && rule.TTLDays != 0 {
			return cfg, nil, fmt.Errorf("policy rule %q action keep cannot set ttl_days", rule.Name)
		}
		if rule.Action == actionDeleteAfter && rule.TTLDays <= 0 {
			return cfg, nil, fmt.Errorf("policy rule %q action delete_after requires ttl_days > 0", rule.Name)
		}

		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return cfg, nil, fmt.Errorf("compile pattern for rule %q: %w", rule.Name, err)
		}

		compiled = append(compiled, compiledRule{
			Name:    rule.Name,
			Pattern: rule.Pattern,
			Action:  rule.Action,
			TTLDays: rule.TTLDays,
			Regexp:  re,
		})
	}

	return cfg, compiled, nil
}

func runHousekeeping(
	ctx context.Context,
	opts options,
	policy policyConfig,
	rules []compiledRule,
	client packageClient,
	now time.Time,
) (housekeepingReport, error) {
	report := housekeepingReport{
		Run: runReport{
			Mode:                opts.Mode,
			TimestampUTC:        now.Format(time.RFC3339),
			Owner:               opts.Owner,
			OwnerKind:           opts.OwnerKind,
			PolicyFile:          opts.PolicyFile,
			MaxDeletePerPackage: opts.MaxDeletePerPackage,
		},
		Packages: make([]packageReport, 0, len(opts.Packages)),
	}

	problems := make([]string, 0, len(opts.Packages))
	for _, pkg := range opts.Packages {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			continue
		}

		result := processPackage(ctx, opts, policy, rules, client, now, pkg)
		if len(result.Errors) > 0 {
			problems = append(problems, result.Errors...)
		}
		report.Packages = append(report.Packages, result)
	}

	if len(problems) > 0 {
		return report, errors.New(strings.Join(problems, "; "))
	}
	if opts.Mode == modeDryRun {
		return report, nil
	}

	for i := range report.Packages {
		pkgReport := &report.Packages[i]
		if len(pkgReport.CandidateItems) <= opts.MaxDeletePerPackage {
			continue
		}
		errMsg := fmt.Sprintf(
			"%s: candidate count %d exceeds max-delete-per-package=%d; use workflow_dispatch override to continue",
			pkgReport.Name,
			len(pkgReport.CandidateItems),
			opts.MaxDeletePerPackage,
		)
		pkgReport.Errors = append(pkgReport.Errors, errMsg)
		problems = append(problems, errMsg)
	}
	if len(problems) > 0 {
		return report, errors.New(strings.Join(problems, "; "))
	}

	for i := range report.Packages {
		pkgReport := &report.Packages[i]
		for _, candidate := range pkgReport.CandidateItems {
			if err := client.DeletePackageVersion(ctx, opts.OwnerKind, opts.Owner, pkgReport.Name, candidate.ID); err != nil {
				errMsg := fmt.Sprintf("%s: delete version %d failed: %v", pkgReport.Name, candidate.ID, err)
				pkgReport.Errors = append(pkgReport.Errors, errMsg)
				problems = append(problems, errMsg)
				continue
			}
			pkgReport.Deleted++
		}
	}
	if len(problems) > 0 {
		return report, errors.New(strings.Join(problems, "; "))
	}

	return report, nil
}

func processPackage(
	ctx context.Context,
	opts options,
	policy policyConfig,
	rules []compiledRule,
	client packageClient,
	now time.Time,
	pkg string,
) packageReport {
	result := packageReport{
		Name:   pkg,
		Errors: make([]string, 0),
	}

	versions, err := client.ListPackageVersions(ctx, opts.OwnerKind, opts.Owner, pkg)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s: list versions failed: %v", pkg, err))
		return result
	}

	result.ScannedVersions = len(versions)
	candidates := make([]candidateReport, 0)

	for _, version := range versions {
		eval := evaluateVersion(version, rules, policy.ProtectUnknown, now)
		switch {
		case eval.Unknown:
			result.KeptUnknown++
			switch eval.UnknownReason {
			case unknownReasonUntagged:
				result.KeptUnknownUntagged++
			case unknownReasonUnmatchedTag:
				result.KeptUnknownUnmatchedTag++
			case unknownReasonNoTransientMatch:
				result.KeptUnknownNoTransientMatch++
			default:
				result.KeptUnknownNoTransientMatch++
			}
		case eval.Protected:
			result.KeptProtected++
		case eval.Candidate:
			candidates = append(candidates, candidateReport{
				ID:              version.ID,
				Name:            version.Name,
				UpdatedAt:       version.UpdatedAt.UTC().Format(time.RFC3339),
				AgeDays:         eval.AgeDays,
				RequiredAgeDays: eval.RequiredAgeDays,
				Tags:            append([]string{}, version.Tags...),
				MatchedRules:    append([]string{}, eval.MatchedRules...),
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].UpdatedAt == candidates[j].UpdatedAt {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].UpdatedAt < candidates[j].UpdatedAt
	})

	result.Candidates = len(candidates)
	result.CandidateItems = candidates
	return result
}

func evaluateVersion(version packageVersion, rules []compiledRule, protectUnknown bool, now time.Time) versionEval {
	eval := versionEval{}
	if len(version.Tags) == 0 {
		eval.Unknown = true
		eval.UnknownReason = unknownReasonUntagged
		return eval
	}

	requiredAgeDays := 0
	for _, tag := range version.Tags {
		tagResult := classifyTag(tag, rules)
		if !tagResult.Known {
			if protectUnknown {
				eval.Unknown = true
				eval.UnknownReason = unknownReasonUnmatchedTag
			}
			continue
		}

		eval.MatchedRules = append(eval.MatchedRules, tagResult.RuleName)
		if tagResult.Action == actionKeep {
			eval.Protected = true
			continue
		}
		if tagResult.Action == actionDeleteAfter && tagResult.TTLDays > requiredAgeDays {
			requiredAgeDays = tagResult.TTLDays
		}
	}

	if eval.Unknown || eval.Protected {
		return eval
	}
	if requiredAgeDays == 0 {
		// No matching transient rules and no protected tags means unknown/non-deletable by policy.
		eval.Unknown = true
		eval.UnknownReason = unknownReasonNoTransientMatch
		return eval
	}

	ageDays := int(now.Sub(version.UpdatedAt).Hours() / 24)
	if ageDays < 0 {
		ageDays = 0
	}

	eval.AgeDays = ageDays
	eval.RequiredAgeDays = requiredAgeDays
	eval.Candidate = ageDays >= requiredAgeDays
	return eval
}

func classifyTag(tag string, rules []compiledRule) tagEval {
	for _, rule := range rules {
		if rule.Regexp.MatchString(tag) {
			return tagEval{
				Known:    true,
				Action:   rule.Action,
				TTLDays:  rule.TTLDays,
				RuleName: rule.Name,
			}
		}
	}
	return tagEval{}
}

func writeReportJSON(path string, report housekeepingReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func renderSummary(report housekeepingReport) string {
	var b strings.Builder
	b.WriteString("## GHCR Housekeeping\n\n")
	b.WriteString(fmt.Sprintf("- Mode: `%s`\n", report.Run.Mode))
	b.WriteString(fmt.Sprintf("- Timestamp: `%s`\n", report.Run.TimestampUTC))
	b.WriteString(fmt.Sprintf("- Owner: `%s` (%s)\n", report.Run.Owner, report.Run.OwnerKind))
	b.WriteString(fmt.Sprintf("- Max delete per package: `%d`\n\n", report.Run.MaxDeletePerPackage))
	b.WriteString(summaryTableHeader)
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, pkg := range report.Packages {
		b.WriteString(
			fmt.Sprintf(
				"| `%s` | %d | %d | %d | %d | %d | %d | %d | %d | %d |\n",
				pkg.Name,
				pkg.ScannedVersions,
				pkg.Candidates,
				pkg.Deleted,
				pkg.KeptProtected,
				pkg.KeptUnknown,
				pkg.KeptUnknownUntagged,
				pkg.KeptUnknownUnmatchedTag,
				pkg.KeptUnknownNoTransientMatch,
				len(pkg.Errors),
			),
		)
	}

	errorCount := 0
	for _, pkg := range report.Packages {
		errorCount += len(pkg.Errors)
	}
	if errorCount > 0 {
		b.WriteString("\n### Errors\n")
		for _, pkg := range report.Packages {
			for _, err := range pkg.Errors {
				b.WriteString(fmt.Sprintf("- `%s`: %s\n", pkg.Name, err))
			}
		}
	}

	return b.String()
}

func writeStepSummary(summary string) error {
	path := strings.TrimSpace(os.Getenv(summaryEnvVar))
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(summary), 0o644)
}

func (c *githubPackagesClient) ListPackageVersions(
	ctx context.Context,
	ownerKind, owner, pkg string,
) ([]packageVersion, error) {
	scopePath, err := ownerScopePath(ownerKind, owner)
	if err != nil {
		return nil, err
	}

	versions := make([]packageVersion, 0)
	for page := 1; ; page++ {
		endpoint := fmt.Sprintf(
			"%s/%s/packages/container/%s/versions?per_page=%d&page=%d",
			c.baseURL,
			scopePath,
			url.PathEscape(pkg),
			perPage,
			page,
		)

		resp, err := c.do(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("list package versions: %w", err)
		}
		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return versions, nil
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			msg := extractAPIErrorMessage(resp)
			_ = resp.Body.Close()
			return nil, fmt.Errorf("list package versions: %w", classifyAPIError(resp.StatusCode, msg))
		}

		var apiItems []apiPackageVersion
		if err := json.NewDecoder(resp.Body).Decode(&apiItems); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("list package versions: decode JSON response: %w", err)
		}
		_ = resp.Body.Close()
		if len(apiItems) == 0 {
			break
		}

		for _, item := range apiItems {
			tags := item.Metadata.Container.Tags
			if tags == nil {
				tags = []string{}
			}
			versions = append(versions, packageVersion{
				ID:        item.ID,
				Name:      item.Name,
				UpdatedAt: item.UpdatedAt.UTC(),
				Tags:      append([]string{}, tags...),
			})
		}

		if len(apiItems) < perPage {
			break
		}
	}

	return versions, nil
}

func (c *githubPackagesClient) DeletePackageVersion(ctx context.Context, ownerKind, owner, pkg string, id int64) error {
	scopePath, err := ownerScopePath(ownerKind, owner)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf(
		"%s/%s/packages/container/%s/versions/%d",
		c.baseURL,
		scopePath,
		url.PathEscape(pkg),
		id,
	)

	resp, err := c.do(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}

	msg := extractAPIErrorMessage(resp)
	return classifyAPIError(resp.StatusCode, msg)
}

func (c *githubPackagesClient) do(
	ctx context.Context,
	method, endpoint string,
	body io.Reader,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	return resp, nil
}

func ownerScopePath(ownerKind, owner string) (string, error) {
	switch ownerKind {
	case ownerKindOrg:
		return "orgs/" + url.PathEscape(owner), nil
	case ownerKindUser:
		return "users/" + url.PathEscape(owner), nil
	default:
		return "", fmt.Errorf("unsupported owner-kind %q", ownerKind)
	}
}

func extractAPIErrorMessage(resp *http.Response) string {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return defaultErrorMessage
	}
	if len(body) == 0 {
		return defaultErrorMessage
	}

	var parsed struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && strings.TrimSpace(parsed.Message) != "" {
		return strings.TrimSpace(parsed.Message)
	}
	return strings.TrimSpace(string(body))
}

func classifyAPIError(statusCode int, message string) error {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf(
			"GitHub API authorization failed (%d): %s. "+
				"Ensure the workflow has packages:write and GITHUB_TOKEN has package admin access",
			statusCode,
			message,
		)
	case http.StatusNotFound:
		return fmt.Errorf("GitHub API resource not found (%d): %s", statusCode, message)
	default:
		return fmt.Errorf("GitHub API request failed (%d): %s", statusCode, message)
	}
}

type apiPackageVersion struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
	Metadata  struct {
		Container struct {
			Tags []string `json:"tags"`
		} `json:"container"`
	} `json:"metadata"`
}
