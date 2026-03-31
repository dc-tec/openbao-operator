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
		tc := tc
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
