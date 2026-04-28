package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempE2EFile(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("package e2e\n"), 0o644); err != nil {
		t.Fatalf("write temp e2e file: %v", err)
	}
	return filepath.ToSlash(path)
}

func TestValidateManifestAcceptsCatalogMatchedSuite(t *testing.T) {
	t.Parallel()

	file := writeTempE2EFile(t, "Cluster_Lifecycle_test.go")

	facts := buildSuiteFacts([]catalogCase{
		{
			File:         file,
			Path:         []string{"Cluster Lifecycle", "creates a cluster"},
			CaseLabel:    "cluster-lifecycle-create",
			DomainLabels: []string{"lifecycle", "cluster"},
			Coverage:     []string{"lifecycle"},
		},
	})

	err := validateManifest(manifest{
		Version: 1,
		Suites: []manifestSuite{
			{
				ID:        "cluster-lifecycle",
				Title:     "Cluster Lifecycle",
				Owner:     "core",
				RiskTier:  "critical",
				Isolation: "shared-cluster",
				Files:     []string{file},
				Labels:    []string{"cluster", "lifecycle"},
				Coverage:  []string{"lifecycle"},
				CI: suiteCI{
					Lanes:       []string{"core"},
					PullRequest: "changed-paths",
				},
				Nightly: suiteNightly{
					Policy: "primary-version-full",
				},
			},
		},
	}, facts, options{})
	if err != nil {
		t.Fatalf("validateManifest() error = %v", err)
	}
}

func TestValidateManifestRejectsLabelDrift(t *testing.T) {
	t.Parallel()

	file := writeTempE2EFile(t, "Security_Guardrails_test.go")

	facts := buildSuiteFacts([]catalogCase{
		{
			File:         file,
			Path:         []string{"Security Guardrails", "checks RBAC"},
			DomainLabels: []string{"security", "rbac"},
		},
	})

	err := validateManifest(manifest{
		Version: 1,
		Suites: []manifestSuite{
			{
				ID:        "security-guardrails",
				Title:     "Security Guardrails",
				Owner:     "security",
				RiskTier:  "critical",
				Isolation: "global-mutator",
				Files:     []string{file},
				Labels:    []string{"security"},
				CI: suiteCI{
					Lanes:       []string{"security"},
					PullRequest: "changed-paths",
				},
				Nightly: suiteNightly{
					Policy: "primary-version-full",
				},
			},
		},
	}, facts, options{})
	if err == nil {
		t.Fatalf("validateManifest() error = nil, want label drift")
	}
	if !strings.Contains(err.Error(), "labels") {
		t.Fatalf("error = %q, want labels message", err)
	}
}

func TestValidateManifestRejectsDuplicateFileOwnership(t *testing.T) {
	t.Parallel()

	file := writeTempE2EFile(t, "Operator_Manager_test.go")

	facts := buildSuiteFacts([]catalogCase{
		{
			File:         file,
			Path:         []string{"Manager", "starts"},
			DomainLabels: []string{"manager"},
		},
	})

	base := manifestSuite{
		Title:     "Manager",
		Owner:     "manager",
		RiskTier:  "critical",
		Isolation: "shared-cluster",
		Files:     []string{file},
		Labels:    []string{"manager"},
		CI: suiteCI{
			Lanes:       []string{"core"},
			PullRequest: "changed-paths",
		},
		Nightly: suiteNightly{
			Policy: "primary-version-full",
		},
	}
	first := base
	first.ID = "operator-manager"
	second := base
	second.ID = "operator-manager-copy"

	err := validateManifest(manifest{
		Version: 1,
		Suites:  []manifestSuite{first, second},
	}, facts, options{})
	if err == nil {
		t.Fatalf("validateManifest() error = nil, want duplicate file ownership")
	}
	if !strings.Contains(err.Error(), "already owned") {
		t.Fatalf("error = %q, want duplicate ownership message", err)
	}
}

func TestValidateManifestCanRequireExplicitCaseIDs(t *testing.T) {
	t.Parallel()

	file := writeTempE2EFile(t, "backup_restore_test.go")

	facts := buildSuiteFacts([]catalogCase{
		{
			File:         file,
			Path:         []string{"DR: Storage Providers Backup & Restore", "backs up"},
			DomainLabels: []string{"dr", "backup"},
		},
	})

	err := validateManifest(manifest{
		Version: 1,
		Suites: []manifestSuite{
			{
				ID:        "backup-restore",
				Title:     "DR: Storage Providers Backup & Restore",
				Owner:     "backup-restore",
				RiskTier:  "critical",
				Isolation: "global-mutator",
				Files:     []string{file},
				Labels:    []string{"backup", "dr"},
				CI: suiteCI{
					Lanes:       []string{"backup-restore"},
					PullRequest: "changed-paths",
				},
				Nightly: suiteNightly{
					Policy: "primary-version-full",
				},
			},
		},
	}, facts, options{RequireCaseIDs: true})
	if err == nil {
		t.Fatalf("validateManifest() error = nil, want missing case IDs")
	}
	if !strings.Contains(err.Error(), "explicit case IDs") {
		t.Fatalf("error = %q, want explicit case IDs message", err)
	}
}

func TestLoadCatalogRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cases.json")
	if err := os.WriteFile(path, []byte(`{not-json}`), 0o644); err != nil {
		t.Fatalf("write malformed catalog: %v", err)
	}

	_, err := loadCatalog(path)
	if err == nil {
		t.Fatalf("loadCatalog() error = nil, want parse failure")
	}
	if !strings.Contains(err.Error(), "parse catalog") {
		t.Fatalf("error = %q, want parse catalog message", err)
	}
}
