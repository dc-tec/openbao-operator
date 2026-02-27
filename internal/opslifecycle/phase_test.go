package opslifecycle

import "testing"

func TestPhaseTransitionFields(t *testing.T) {
	t.Parallel()

	base := map[string]string{
		"cluster_namespace": "default",
		"cluster_name":      "openbao",
	}

	fields := phaseTransitionFields("Pending", "Running", base)

	if fields["phase_from"] != "Pending" {
		t.Fatalf("expected phase_from=%q, got %q", "Pending", fields["phase_from"])
	}
	if fields["phase_to"] != "Running" {
		t.Fatalf("expected phase_to=%q, got %q", "Running", fields["phase_to"])
	}
	if fields["cluster_name"] != "openbao" {
		t.Fatalf("expected cluster_name=%q, got %q", "openbao", fields["cluster_name"])
	}

	fields["cluster_name"] = "changed"
	if base["cluster_name"] != "openbao" {
		t.Fatal("expected helper to copy input fields map")
	}
}
