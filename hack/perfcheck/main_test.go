package main

import (
	"reflect"
	"testing"
)

func TestParseScenarioSelection(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{name: "all", input: "all", want: nil},
		{name: "empty means all", input: "", want: nil},
		{name: "single", input: "lifecycle", want: []string{"lifecycle"}},
		{name: "multiple", input: "lifecycle, rolling-upgrade", want: []string{"lifecycle", "rolling-upgrade"}},
		{name: "dedupe", input: "lifecycle,lifecycle", want: []string{"lifecycle"}},
		{name: "unknown deferred to manifest validation", input: "foo", want: []string{"foo"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseScenarioSelection(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseScenarioSelection(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFinalizeOptionsRejectsInvalidTenantChurnCount(t *testing.T) {
	t.Parallel()

	opts := defaultOptions("verify")
	opts.TenantChurnCount = 0

	if _, err := finalizeOptions(opts); err == nil {
		t.Fatalf("expected tenant-churn-count validation error")
	}
}

func TestFinalizeOptionsRejectsInvalidMinimumSuccessfulSamples(t *testing.T) {
	t.Parallel()

	opts := defaultOptions("capture")
	opts.MinimumSuccessfulSamples = -1

	if _, err := finalizeOptions(opts); err == nil {
		t.Fatalf("expected minimum-successful-samples validation error")
	}
}

func TestDefaultRollingUpgradeSourceUsesPreviousStableRelease(t *testing.T) {
	t.Setenv("PERF_UPGRADE_FROM_VERSION", "")
	t.Setenv("PERF_UPGRADE_FROM_IMAGE", "")

	opts := defaultOptions("verify")
	if opts.UpgradeFromVersion != "2.5.5" {
		t.Fatalf("UpgradeFromVersion = %q, want 2.5.5", opts.UpgradeFromVersion)
	}
	if opts.UpgradeFromImage != "openbao/openbao:2.5.5" {
		t.Fatalf("UpgradeFromImage = %q, want openbao/openbao:2.5.5", opts.UpgradeFromImage)
	}
}
