package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDefaultPackagesIncludeSealFixtures(t *testing.T) {
	t.Parallel()

	packages := make(map[string]bool, len(defaultPackages))
	for _, name := range defaultPackages {
		packages[name] = true
	}

	for _, name := range []string{
		"ci-e2e-openbao-softhsm",
		"ci-e2e-pykmip-server",
		"pr-e2e-openbao-softhsm",
		"pr-e2e-pykmip-server",
	} {
		if !packages[name] {
			t.Errorf("default packages are missing seal fixture package %q", name)
		}
	}
}

func TestEvaluateVersionProtectsSemverWithTransientAlias(t *testing.T) {
	cfg, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	version := packageVersion{
		ID:        1,
		Name:      "sha256:deadbeef",
		UpdatedAt: now.AddDate(0, 0, -40),
		Tags: []string{
			"0.1.0-rc.3",
			"edge-build-c30c5a9596b50fe286e40575f2e2f46812208234",
		},
	}

	eval := evaluateVersion(version, rules, cfg.ProtectUnknown, now)
	if !eval.Protected {
		t.Fatalf("expected protected=true")
	}
	if eval.Candidate {
		t.Fatalf("expected candidate=false for protected version")
	}
}

func TestEvaluateVersionE2ECandidateAfterTTL(t *testing.T) {
	cfg, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	version := packageVersion{
		ID:        2,
		Name:      "sha256:e2e",
		UpdatedAt: now.AddDate(0, 0, -8),
		Tags:      []string{"e2e-12345-1"},
	}

	eval := evaluateVersion(version, rules, cfg.ProtectUnknown, now)
	if !eval.Candidate {
		t.Fatalf("expected candidate=true after e2e ttl")
	}
	if eval.RequiredAgeDays != 7 {
		t.Fatalf("required age = %d, want 7", eval.RequiredAgeDays)
	}
}

func TestEvaluateVersionNightlyBuildBeforeTTL(t *testing.T) {
	cfg, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	version := packageVersion{
		ID:        3,
		Name:      "sha256:nightly-build",
		UpdatedAt: now.AddDate(0, 0, -10),
		Tags:      []string{"nightly-build-c30c5a9596b50fe286e40575f2e2f46812208234"},
	}

	eval := evaluateVersion(version, rules, cfg.ProtectUnknown, now)
	if eval.Candidate {
		t.Fatalf("expected candidate=false before nightly-build ttl")
	}
	if eval.RequiredAgeDays != 21 {
		t.Fatalf("required age = %d, want 21", eval.RequiredAgeDays)
	}
}

func TestEvaluateVersionNightlyBuildAfterTTL(t *testing.T) {
	cfg, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	version := packageVersion{
		ID:        4,
		Name:      "sha256:nightly-build-old",
		UpdatedAt: now.AddDate(0, 0, -25),
		Tags:      []string{"nightly-build-c30c5a9596b50fe286e40575f2e2f46812208234"},
	}

	eval := evaluateVersion(version, rules, cfg.ProtectUnknown, now)
	if !eval.Candidate {
		t.Fatalf("expected candidate=true after nightly-build ttl")
	}
}

func TestEvaluateVersionUnknownTagIsProtectedByFailSafe(t *testing.T) {
	cfg, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	version := packageVersion{
		ID:        5,
		Name:      "sha256:unknown",
		UpdatedAt: now.AddDate(0, 0, -100),
		Tags:      []string{"mystery-tag"},
	}

	eval := evaluateVersion(version, rules, cfg.ProtectUnknown, now)
	if !eval.Unknown {
		t.Fatalf("expected unknown=true for unmatched tag")
	}
	if eval.UnknownReason != unknownReasonUnmatchedTag {
		t.Fatalf("unknown reason = %q, want %q", eval.UnknownReason, unknownReasonUnmatchedTag)
	}
	if eval.Candidate {
		t.Fatalf("expected candidate=false for unknown tag")
	}
}

func TestEvaluateVersionUnknownReasonUntagged(t *testing.T) {
	cfg, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	version := packageVersion{
		ID:        8,
		Name:      "sha256:untagged",
		UpdatedAt: now.AddDate(0, 0, -30),
		Tags:      nil,
	}

	eval := evaluateVersion(version, rules, cfg.ProtectUnknown, now)
	if !eval.Unknown {
		t.Fatalf("expected unknown=true for untagged version")
	}
	if eval.UnknownReason != unknownReasonUntagged {
		t.Fatalf("unknown reason = %q, want %q", eval.UnknownReason, unknownReasonUntagged)
	}
}

func TestEvaluateVersionUnknownReasonNoTransientWhenProtectUnknownDisabled(t *testing.T) {
	_, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	version := packageVersion{
		ID:        9,
		Name:      "sha256:no-transient",
		UpdatedAt: now.AddDate(0, 0, -30),
		Tags:      []string{"mystery-tag"},
	}

	eval := evaluateVersion(version, rules, false, now)
	if !eval.Unknown {
		t.Fatalf("expected unknown=true when no transient rule matches")
	}
	if eval.UnknownReason != unknownReasonNoTransientMatch {
		t.Fatalf("unknown reason = %q, want %q", eval.UnknownReason, unknownReasonNoTransientMatch)
	}
}

func TestEvaluateVersionSha256TagAlwaysProtected(t *testing.T) {
	cfg, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	version := packageVersion{
		ID:        6,
		Name:      "sha256:sig",
		UpdatedAt: now.AddDate(0, 0, -300),
		Tags:      []string{"sha256-c0f7cbdccf832280eddb8c7bdd8ad538e57e243c65aed6e3db6f1c3847db87e0.sig"},
	}

	eval := evaluateVersion(version, rules, cfg.ProtectUnknown, now)
	if !eval.Protected {
		t.Fatalf("expected protected=true for sha256-* tag")
	}
	if eval.Candidate {
		t.Fatalf("expected candidate=false for protected tag")
	}
}

func TestEvaluateVersionMixedTTLUsesMaxRequirement(t *testing.T) {
	cfg, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	version := packageVersion{
		ID:        7,
		Name:      "sha256:mixed",
		UpdatedAt: now.AddDate(0, 0, -10),
		Tags: []string{
			"edge-aaaaaaaaaaaa",
			"e2e-222-1",
		},
	}

	eval := evaluateVersion(version, rules, cfg.ProtectUnknown, now)
	if eval.RequiredAgeDays != 30 {
		t.Fatalf("required age = %d, want 30", eval.RequiredAgeDays)
	}
	if eval.Candidate {
		t.Fatalf("expected candidate=false at age 10 with required 30")
	}

	version.UpdatedAt = now.AddDate(0, 0, -31)
	eval = evaluateVersion(version, rules, cfg.ProtectUnknown, now)
	if !eval.Candidate {
		t.Fatalf("expected candidate=true at age 31 with required 30")
	}
}

func TestRunHousekeepingEnforceStopsAboveSafetyBrake(t *testing.T) {
	cfg, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	client := &fakePackageClient{
		versionsByPackage: map[string][]packageVersion{
			"openbao-operator": {
				{
					ID:        10,
					Name:      "sha256:a",
					UpdatedAt: now.AddDate(0, 0, -8),
					Tags:      []string{"e2e-100-1"},
				},
				{
					ID:        11,
					Name:      "sha256:b",
					UpdatedAt: now.AddDate(0, 0, -8),
					Tags:      []string{"e2e-101-1"},
				},
			},
		},
	}

	opts := options{
		Owner:               "dc-tec",
		OwnerKind:           ownerKindOrg,
		Packages:            []string{"openbao-operator"},
		Mode:                modeEnforce,
		PolicyFile:          "test-policy.json",
		MaxDeletePerPackage: 1,
		MaxDeleteTotal:      1,
		ReportJSON:          "dist/report.json",
	}

	report, err := runHousekeeping(context.Background(), opts, cfg, rules, client, nil, now)
	if err == nil {
		t.Fatalf("expected safety-brake error in enforce mode")
	}
	if len(client.deleteCalls) != 0 {
		t.Fatalf("expected zero delete calls, got %d", len(client.deleteCalls))
	}
	if report.Packages[0].Deleted != 0 {
		t.Fatalf("expected deleted=0")
	}
}

func TestRunHousekeepingDryRunNeverDeletes(t *testing.T) {
	cfg, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	client := &fakePackageClient{
		versionsByPackage: map[string][]packageVersion{
			"openbao-operator": {
				{
					ID:        20,
					Name:      "sha256:a",
					UpdatedAt: now.AddDate(0, 0, -8),
					Tags:      []string{"e2e-200-1"},
				},
				{
					ID:        21,
					Name:      "sha256:b",
					UpdatedAt: now.AddDate(0, 0, -8),
					Tags:      []string{"nightly-e2e-200-1"},
				},
			},
		},
	}

	opts := options{
		Owner:               "dc-tec",
		OwnerKind:           ownerKindOrg,
		Packages:            []string{"openbao-operator"},
		Mode:                modeDryRun,
		PolicyFile:          "test-policy.json",
		MaxDeletePerPackage: 1,
		MaxDeleteTotal:      1,
		ReportJSON:          "dist/report.json",
	}

	report, err := runHousekeeping(context.Background(), opts, cfg, rules, client, nil, now)
	if err != nil {
		t.Fatalf("runHousekeeping dry-run returned error: %v", err)
	}
	if len(client.deleteCalls) != 0 {
		t.Fatalf("expected zero delete calls in dry-run, got %d", len(client.deleteCalls))
	}
	if report.Packages[0].Candidates != 2 {
		t.Fatalf("expected 2 candidates, got %d", report.Packages[0].Candidates)
	}
	if report.Packages[0].Deleted != 0 {
		t.Fatalf("expected deleted=0 in dry-run")
	}
}

func TestRunHousekeepingEnforceAbortsAllDeletesWhenOnePackageExceedsSafetyBrake(t *testing.T) {
	cfg, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	client := &fakePackageClient{
		versionsByPackage: map[string][]packageVersion{
			"openbao-operator": {
				{ID: 101, Name: "sha256:a", UpdatedAt: now.AddDate(0, 0, -8), Tags: []string{"e2e-900-1"}},
				{ID: 102, Name: "sha256:b", UpdatedAt: now.AddDate(0, 0, -8), Tags: []string{"e2e-901-1"}},
			},
			"openbao-init": {
				{ID: 201, Name: "sha256:c", UpdatedAt: now.AddDate(0, 0, -8), Tags: []string{"e2e-902-1"}},
			},
		},
	}

	opts := options{
		Owner:               "dc-tec",
		OwnerKind:           ownerKindOrg,
		Packages:            []string{"openbao-operator", "openbao-init"},
		Mode:                modeEnforce,
		PolicyFile:          "test-policy.json",
		MaxDeletePerPackage: 1,
		MaxDeleteTotal:      1,
		ReportJSON:          "dist/report.json",
	}

	report, err := runHousekeeping(context.Background(), opts, cfg, rules, client, nil, now)
	if err == nil {
		t.Fatalf("expected safety-brake error in enforce mode")
	}
	if len(client.deleteCalls) != 0 {
		t.Fatalf("expected zero delete calls across all packages, got %d", len(client.deleteCalls))
	}
	if report.Packages[0].Deleted != 0 || report.Packages[1].Deleted != 0 {
		t.Fatalf("expected deleted=0 for all packages")
	}
}

func TestRunHousekeepingTracksUnknownBreakdown(t *testing.T) {
	cfg, rules := testPolicy(t)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	client := &fakePackageClient{
		versionsByPackage: map[string][]packageVersion{
			"openbao-operator": {
				{ID: 301, Name: "sha256:untagged", UpdatedAt: now.AddDate(0, 0, -1), Tags: nil},
				{ID: 302, Name: "sha256:unknown-tag", UpdatedAt: now.AddDate(0, 0, -1), Tags: []string{"mystery"}},
				{ID: 303, Name: "sha256:protected", UpdatedAt: now.AddDate(0, 0, -1), Tags: []string{"edge"}},
			},
		},
	}

	opts := options{
		Owner:               "dc-tec",
		OwnerKind:           ownerKindOrg,
		Packages:            []string{"openbao-operator"},
		Mode:                modeDryRun,
		PolicyFile:          "test-policy.json",
		MaxDeletePerPackage: 100,
		MaxDeleteTotal:      100,
		ReportJSON:          "dist/report.json",
	}

	report, err := runHousekeeping(context.Background(), opts, cfg, rules, client, nil, now)
	if err != nil {
		t.Fatalf("runHousekeeping returned error: %v", err)
	}
	got := report.Packages[0]
	if got.KeptUnknown != 2 {
		t.Fatalf("kept_unknown = %d, want 2", got.KeptUnknown)
	}
	if got.KeptUnknownUntagged != 1 {
		t.Fatalf("kept_unknown_untagged = %d, want 1", got.KeptUnknownUntagged)
	}
	if got.KeptUnknownUnmatchedTag != 1 {
		t.Fatalf("kept_unknown_unmatched_tag = %d, want 1", got.KeptUnknownUnmatchedTag)
	}
	if got.KeptUnknownNoTransientMatch != 0 {
		t.Fatalf("kept_unknown_no_transient_match = %d, want 0", got.KeptUnknownNoTransientMatch)
	}
}

func TestGitHubClientPaginatesVersions(t *testing.T) {
	pageOne := make([]apiPackageVersion, 0, perPage)
	for i := 0; i < perPage; i++ {
		pageOne = append(pageOne, apiPackageVersion{
			ID:        int64(i + 1),
			Name:      fmt.Sprintf("sha256:%d", i+1),
			UpdatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Metadata: struct {
				Container struct {
					Tags []string `json:"tags"`
				} `json:"container"`
			}{
				Container: struct {
					Tags []string `json:"tags"`
				}{
					Tags: []string{fmt.Sprintf("e2e-%d-1", i+1)},
				},
			},
		})
	}
	pageTwo := []apiPackageVersion{
		{
			ID:        101,
			Name:      "sha256:101",
			UpdatedAt: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
			Metadata: struct {
				Container struct {
					Tags []string `json:"tags"`
				} `json:"container"`
			}{
				Container: struct {
					Tags []string `json:"tags"`
				}{
					Tags: []string{"e2e-101-1"},
				},
			},
		},
	}

	handlerErrors := newHTTPHandlerErrors(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "Bearer ") {
			handlerErrors.Errorf("missing bearer auth header")
			http.Error(w, "missing bearer auth header", http.StatusUnauthorized)
			return
		}
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}

		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "1":
			if err := json.NewEncoder(w).Encode(pageOne); err != nil {
				handlerErrors.Errorf("encode page1: %v", err)
			}
		case "2":
			if err := json.NewEncoder(w).Encode(pageTwo); err != nil {
				handlerErrors.Errorf("encode page2: %v", err)
			}
		default:
			if err := json.NewEncoder(w).Encode([]apiPackageVersion{}); err != nil {
				handlerErrors.Errorf("encode empty page: %v", err)
			}
		}
	}))
	defer server.Close()

	client := &githubPackagesClient{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	got, err := client.ListPackageVersions(context.Background(), ownerKindOrg, "dc-tec", "openbao-operator")
	if err != nil {
		t.Fatalf("ListPackageVersions() error = %v", err)
	}
	if len(got) != 101 {
		t.Fatalf("len(versions) = %d, want 101", len(got))
	}
	if got[0].ID != 1 || got[100].ID != 101 {
		t.Fatalf("unexpected IDs: first=%d last=%d", got[0].ID, got[100].ID)
	}
}

func TestGitHubClientListPackageVersionsMissingPackageReturnsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))
	defer server.Close()

	client := &githubPackagesClient{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	got, err := client.ListPackageVersions(context.Background(), ownerKindOrg, "dc-tec", "ci-e2e-openbao-operator")
	if err != nil {
		t.Fatalf("ListPackageVersions() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(versions) = %d, want 0", len(got))
	}
}

type fakePackageClient struct {
	versionsByPackage map[string][]packageVersion
	listErrors        map[string]error
	deleteErrors      map[int64]error
	deleteCalls       []int64
}

func (f *fakePackageClient) ListPackageVersions(
	_ context.Context,
	_ string,
	_ string,
	pkg string,
) ([]packageVersion, error) {
	if err, ok := f.listErrors[pkg]; ok {
		return nil, err
	}
	versions := f.versionsByPackage[pkg]
	out := make([]packageVersion, 0, len(versions))
	for _, item := range versions {
		out = append(out, packageVersion{
			ID:        item.ID,
			Name:      item.Name,
			UpdatedAt: item.UpdatedAt,
			Tags:      append([]string{}, item.Tags...),
		})
	}
	return out, nil
}

func (f *fakePackageClient) DeletePackageVersion(_ context.Context, _ string, _ string, _ string, id int64) error {
	f.deleteCalls = append(f.deleteCalls, id)
	if err, ok := f.deleteErrors[id]; ok {
		return err
	}
	return nil
}

func testPolicy(t *testing.T) (policyConfig, []compiledRule) {
	t.Helper()

	cfg := policyConfig{
		ProtectUnknown: true,
		Rules: []policyRule{
			{Name: "semver", Pattern: "^[0-9]+\\.[0-9]+\\.[0-9]+([-.+].*)?$", Action: "keep"},
			{Name: "edge-pointer", Pattern: "^edge$", Action: "keep"},
			{Name: "nightly-pointer", Pattern: "^nightly$", Action: "keep"},
			{Name: "sha256", Pattern: "^sha256-.*$", Action: "keep"},
			{Name: "edge-sha", Pattern: "^edge-[0-9a-f]{12}$", Action: "delete_after", TTLDays: 30},
			{Name: "nightly-build", Pattern: "^nightly-build-[0-9a-f]{40}$", Action: "delete_after", TTLDays: 21},
			{Name: "e2e", Pattern: "^e2e-[0-9]+-[0-9]+$", Action: "delete_after", TTLDays: 7},
			{Name: "nightly-e2e", Pattern: "^nightly-e2e-[0-9]+-[0-9]+$", Action: "delete_after", TTLDays: 7},
		},
	}

	path := filepath.Join(t.TempDir(), "policy.json")
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	loadedCfg, rules, err := loadPolicy(path)
	if err != nil {
		t.Fatalf("loadPolicy() error = %v", err)
	}
	return loadedCfg, rules
}

func TestRepositoryPolicyEnablesOCIOrphanGrace(t *testing.T) {
	t.Parallel()

	cfg, _, err := loadPolicy("policy.json")
	if err != nil {
		t.Fatalf("loadPolicy(policy.json) error = %v", err)
	}
	if !cfg.OCIGraph.Enabled || cfg.OCIGraph.OrphanTTLDays != 30 {
		t.Fatalf("OCI graph policy = %#v, want enabled with 30-day grace", cfg.OCIGraph)
	}
}

func TestLoadPolicyRejectsInvalidOCIOrphanTTL(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "policy.json")
	raw := `{"protect_unknown":true,"oci_graph":{"enabled":true,"orphan_ttl_days":0},` +
		`"rules":[{"name":"semver","pattern":"^v$","action":"keep"}]}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	if _, _, err := loadPolicy(path); err == nil || !strings.Contains(err.Error(), "orphan_ttl_days must be > 0") {
		t.Fatalf("loadPolicy() error = %v, want invalid orphan TTL", err)
	}
}

func TestRenderSummaryIncludesTable(t *testing.T) {
	report := housekeepingReport{
		Run: runReport{
			Mode:                modeDryRun,
			TimestampUTC:        "2026-03-04T12:00:00Z",
			Owner:               "dc-tec",
			OwnerKind:           ownerKindOrg,
			MaxDeletePerPackage: 100,
		},
		Packages: []packageReport{
			{
				Name:                        "openbao-operator",
				ScannedVersions:             10,
				Candidates:                  2,
				Deleted:                     0,
				KeptProtected:               3,
				KeptUnknown:                 1,
				KeptUnknownUntagged:         1,
				KeptUnknownUnmatchedTag:     0,
				KeptUnknownNoTransientMatch: 0,
			},
		},
	}

	summary := renderSummary(report)
	if !strings.Contains(summary, summaryTableHeader) {
		t.Fatalf("summary missing table header")
	}
	if !strings.Contains(summary, "`openbao-operator`") {
		t.Fatalf("summary missing package row")
	}
}

func TestOwnerScopePath(t *testing.T) {
	got, err := ownerScopePath(ownerKindOrg, "dc-tec")
	if err != nil {
		t.Fatalf("ownerScopePath(org) error = %v", err)
	}
	if got != "orgs/dc-tec" {
		t.Fatalf("got %q, want orgs/dc-tec", got)
	}

	got, err = ownerScopePath(ownerKindUser, "alice")
	if err != nil {
		t.Fatalf("ownerScopePath(user) error = %v", err)
	}
	if got != "users/alice" {
		t.Fatalf("got %q, want users/alice", got)
	}

	_, err = ownerScopePath("team", "dc-tec")
	if err == nil {
		t.Fatalf("expected error for unsupported owner kind")
	}
}

func TestExtractAPIErrorMessageFallback(t *testing.T) {
	resp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader(`{"message":"boom"}`)),
	}
	if msg := extractAPIErrorMessage(resp); msg != "boom" {
		t.Fatalf("message = %q, want boom", msg)
	}

	resp = &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader("plain-text-error")),
	}
	if msg := extractAPIErrorMessage(resp); msg != "plain-text-error" {
		t.Fatalf("message = %q, want plain-text-error", msg)
	}
}

func TestGitHubClientDeleteHandlesNotFoundAsSuccess(t *testing.T) {
	handlerErrors := newHTTPHandlerErrors(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			handlerErrors.Errorf("method = %s, want DELETE", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText := filepath.Base(r.URL.Path)
		id, err := strconv.Atoi(idText)
		if err != nil {
			handlerErrors.Errorf("parse id: %v", err)
			http.Error(w, "invalid package version", http.StatusBadRequest)
			return
		}
		if id == 404 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := &githubPackagesClient{
		baseURL: server.URL,
		token:   "token",
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	if err := client.DeletePackageVersion(
		context.Background(),
		ownerKindOrg,
		"dc-tec",
		"openbao-operator",
		404,
	); err != nil {
		t.Fatalf("DeletePackageVersion(404) error = %v, want nil", err)
	}
}
