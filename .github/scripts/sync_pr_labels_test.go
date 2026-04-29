package main

import "testing"

func TestPathMatches(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{name: "double star subtree", pattern: "docs/**/*", path: "docs/foo/bar.md", want: true},
		{name: "double star suffix", pattern: "**/*.md", path: "website/pages/index.md", want: true},
		{name: "single star segment", pattern: "hack/helm*/**/*", path: "hack/helmvalues/example.yaml", want: true},
		{name: "embedded wildcard", pattern: "*role*.yaml", path: "clusterrolebinding.yaml", want: true},
		{name: "exact file", pattern: "README.md", path: "README.md", want: true},
		{name: "negative", pattern: "config/rbac/**/*", path: "config/crd/foo.yaml", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := pathMatches(tc.pattern, tc.path); got != tc.want {
				t.Fatalf("pathMatches(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
			}
		})
	}
}

func TestMatchingContentLabels(t *testing.T) {
	t.Parallel()

	syncer := &labelSyncer{
		labelConfig: map[string][]labelRule{
			"documentation": {{
				ChangedFiles: []globRule{{Any: []string{"docs/**/*", "**/*.md"}}},
			}},
			"controller": {{
				ChangedFiles: []globRule{{Any: []string{"internal/controller/**/*"}}},
			}},
		},
	}

	files := []pullRequestFile{
		{Filename: "docs/reference/api.md"},
		{Filename: "internal/controller/openbaocluster/controller.go"},
	}

	got := syncer.matchingContentLabels(files)
	want := []string{"controller", "documentation"}
	if len(got) != len(want) {
		t.Fatalf("matchingContentLabels length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("matchingContentLabels[%d] = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestLabelerConfigMatchesCIRoutingLabels(t *testing.T) {
	t.Parallel()

	cfg, err := loadLabelConfig("../labeler.yml")
	if err != nil {
		t.Fatalf("loadLabelConfig() error = %v", err)
	}
	syncer := &labelSyncer{labelConfig: cfg}

	cases := []struct {
		name       string
		path       string
		wantLabels []string
	}{
		{
			name:       "e2e manifest routes all semantic e2e lanes",
			path:       "test/e2e/suites.yaml",
			wantLabels: []string{"backup", "security", "tests", "upgrades"},
		},
		{
			name:       "shared e2e helpers route all semantic e2e lanes",
			path:       "test/e2e/helpers/images.go",
			wantLabels: []string{"backup", "security", "tests", "upgrades"},
		},
		{
			name:       "backup restore e2e routes backup lane",
			path:       "test/e2e/backup_restore_test.go",
			wantLabels: []string{"backup", "restore", "tests"},
		},
		{
			name:       "upgrade e2e routes upgrade lane",
			path:       "test/e2e/Upgrade_Strategies_test.go",
			wantLabels: []string{"tests", "upgrades"},
		},
		{
			name:       "restore service routes backup lane",
			path:       "internal/service/restore/manager.go",
			wantLabels: []string{"backup", "restore"},
		},
		{
			name:       "provisioner app routes provisioner lane",
			path:       "internal/app/provisioner/provisioner.go",
			wantLabels: []string{"provisioner"},
		},
		{
			name:       "cert service routes hardened security lane",
			path:       "internal/service/certs/manager.go",
			wantLabels: []string{"certs", "security"},
		},
		{
			name:       "labeler changes are devops changes",
			path:       ".github/labeler.yml",
			wantLabels: []string{"devops"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := syncer.matchingContentLabels([]pullRequestFile{{Filename: tc.path}})
			for _, want := range tc.wantLabels {
				if !containsLabel(got, want) {
					t.Fatalf("labels for %q = %v, want %q", tc.path, got, want)
				}
			}
		})
	}
}

func containsLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}

func TestCalculateSizeLabel(t *testing.T) {
	t.Parallel()

	xs := 10
	s := 100
	m := 500
	l := 1000

	syncer := &labelSyncer{
		sizeConfig: sizeConfig{
			Thresholds: []sizeThreshold{
				{Label: "size/XS", Max: &xs},
				{Label: "size/S", Max: &s},
				{Label: "size/M", Max: &m},
				{Label: "size/L", Max: &l},
				{Label: "size/XL"},
			},
			Ignore: []string{"go.mod", "go.sum", "*_test.go"},
		},
	}

	files := []pullRequestFile{
		{Filename: "go.mod", Additions: 50, Deletions: 10},
		{Filename: "internal/controller/foo_test.go", Additions: 40, Deletions: 20},
		{Filename: "internal/controller/controller.go", Additions: 180, Deletions: 40},
	}

	if got := syncer.calculateSizeLabel(files); got != "size/M" {
		t.Fatalf("calculateSizeLabel() = %q, want %q", got, "size/M")
	}
}
