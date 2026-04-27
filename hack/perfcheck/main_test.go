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
