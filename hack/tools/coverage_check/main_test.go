package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseCoverageProfileScopesToInternalPackages(t *testing.T) {
	profile := `mode: set
github.com/dc-tec/openbao-operator/internal/app/app.go:1.1,2.1 4 1
github.com/dc-tec/openbao-operator/internal/service/service.go:3.1,4.1 6 0
github.com/dc-tec/openbao-operator/hack/tools/tool.go:1.1,2.1 100 1
github.com/dc-tec/openbao-operator/cmd/controller/main.go:1.1,2.1 100 1
github.com/dc-tec/openbao-operator/test/e2e/suite.go:1.1,2.1 100 1
github.com/dc-tec/openbao-operator/api/v1alpha1/types.go:1.1,2.1 100 1
`

	report, err := parseCoverageProfile(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("parseCoverageProfile() error = %v", err)
	}
	if report.Internal != (coverageStats{Covered: 4, Total: 10}) {
		t.Fatalf("internal stats = %#v, want covered=4 total=10", report.Internal)
	}
	if len(report.Layers) != 2 {
		t.Fatalf("layer count = %d, want 2", len(report.Layers))
	}
	if report.Layers["app"] != (coverageStats{Covered: 4, Total: 4}) {
		t.Fatalf("app stats = %#v, want covered=4 total=4", report.Layers["app"])
	}
	if report.Layers["service"] != (coverageStats{Covered: 0, Total: 6}) {
		t.Fatalf("service stats = %#v, want covered=0 total=6", report.Layers["service"])
	}
}

func TestParseCoverageProfileMergesDuplicateBlocks(t *testing.T) {
	profile := `mode: count
github.com/dc-tec/openbao-operator/internal/app/app.go:1.1,2.1 4 0
github.com/dc-tec/openbao-operator/internal/app/app.go:1.1,2.1 4 3
`

	report, err := parseCoverageProfile(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("parseCoverageProfile() error = %v", err)
	}
	if report.Internal != (coverageStats{Covered: 4, Total: 4}) {
		t.Fatalf("internal stats = %#v, want covered=4 total=4", report.Internal)
	}
}

func TestParseCoverageProfileRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    string
	}{
		{name: "empty", profile: "", want: "profile is empty"},
		{name: "mode", profile: "mode: invalid\n", want: "unsupported mode"},
		{name: "line", profile: "mode: set\nnot-a-profile-line\n", want: "invalid coverage syntax"},
		{
			name:    "no internal statements",
			profile: "mode: set\ngithub.com/dc-tec/openbao-operator/hack/tool.go:1.1,2.1 1 1\n",
			want:    "no internal package statements",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCoverageProfile(strings.NewReader(tt.profile))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parseCoverageProfile() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestInternalLayer(t *testing.T) {
	tests := []struct {
		fileName string
		want     string
		ok       bool
	}{
		{fileName: "github.com/dc-tec/openbao-operator/internal/service/manager.go", want: "service", ok: true},
		{fileName: "internal/platform/status.go", want: "platform", ok: true},
		{fileName: "internal/version.go", want: "root", ok: true},
		{fileName: "github.com/dc-tec/openbao-operator/hack/internal/tool.go", want: "", ok: false},
		{fileName: "github.com/example/project/cmd/main.go", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.fileName, func(t *testing.T) {
			got, ok := internalLayer(tt.fileName)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("internalLayer(%q) = (%q, %t), want (%q, %t)", tt.fileName, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestVerifyMinimum(t *testing.T) {
	report := coverageReport{Internal: coverageStats{Covered: 6832, Total: 10000}}
	if err := verifyMinimum(report, 68.0); err != nil {
		t.Fatalf("verifyMinimum() unexpected error = %v", err)
	}
	if err := verifyMinimum(report, 68.5); err == nil {
		t.Fatal("verifyMinimum() error = nil, want threshold failure")
	}
}

func TestPrintCoverageReport(t *testing.T) {
	report := coverageReport{
		Internal: coverageStats{Covered: 7, Total: 10},
		Layers: map[string]coverageStats{
			"service": {Covered: 3, Total: 6},
			"app":     {Covered: 4, Total: 4},
		},
	}

	var out bytes.Buffer
	if err := printCoverageReport(&out, report, 68.0); err != nil {
		t.Fatalf("printCoverageReport() error = %v", err)
	}
	want := `internal coverage: 70.00% (7/10 statements)
  app          100.00% (4/4)
  service      50.00% (3/6)
required minimum: 68.00%
coverage gate: pass
`
	if out.String() != want {
		t.Fatalf("printCoverageReport() =\n%s\nwant:\n%s", out.String(), want)
	}
}
