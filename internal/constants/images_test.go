package constants

import (
	"testing"
)

func TestDefaultBackupImage_WithVersion(t *testing.T) {
	// Set the version using t.Setenv (auto-cleanup, no error checking needed)
	t.Setenv(EnvOperatorVersion, "v1.2.3")

	// Clear any repo override
	t.Setenv(EnvOperatorBackupImageRepo, "")

	got, err := DefaultBackupImage()
	if err != nil {
		t.Fatalf("DefaultBackupImage() unexpected error: %v", err)
	}
	want := DefaultBackupImageRepository + ":v1.2.3"
	if got != want {
		t.Errorf("DefaultBackupImage() = %v, want %v", got, want)
	}
}

func TestDefaultBackupImage_WithCustomRepo(t *testing.T) {
	t.Setenv(EnvOperatorVersion, "v1.2.3")
	t.Setenv(EnvOperatorBackupImageRepo, "my-registry.io/custom-backup")

	got, err := DefaultBackupImage()
	if err != nil {
		t.Fatalf("DefaultBackupImage() unexpected error: %v", err)
	}
	want := "my-registry.io/custom-backup:v1.2.3"
	if got != want {
		t.Errorf("DefaultBackupImage() = %v, want %v", got, want)
	}
}

func TestDefaultUpgradeImage_WithVersion(t *testing.T) {
	t.Setenv(EnvOperatorVersion, "v2.0.0")
	t.Setenv(EnvOperatorUpgradeImageRepo, "")

	got, err := DefaultUpgradeImage()
	if err != nil {
		t.Fatalf("DefaultUpgradeImage() unexpected error: %v", err)
	}
	want := DefaultUpgradeImageRepository + ":v2.0.0"
	if got != want {
		t.Errorf("DefaultUpgradeImage() = %v, want %v", got, want)
	}
}

func TestDefaultInitImage_WithVersion(t *testing.T) {
	t.Setenv(EnvOperatorVersion, "v3.0.0")
	t.Setenv(EnvOperatorInitImageRepo, "")

	got, err := DefaultInitImage()
	if err != nil {
		t.Fatalf("DefaultInitImage() unexpected error: %v", err)
	}
	want := DefaultInitImageRepository + ":v3.0.0"
	if got != want {
		t.Errorf("DefaultInitImage() = %v, want %v", got, want)
	}
}

func TestDefaultImage_ErrorsWithoutVersion(t *testing.T) {
	// Ensure OPERATOR_VERSION is not set
	t.Setenv(EnvOperatorVersion, "")

	// Test that each default image function returns error when version is missing.
	testCases := []struct {
		name string
		fn   func() (string, error)
	}{
		{"DefaultBackupImage", DefaultBackupImage},
		{"DefaultUpgradeImage", DefaultUpgradeImage},
		{"DefaultInitImage", DefaultInitImage},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.fn()
			if err == nil {
				t.Errorf("%s() should have returned error when OPERATOR_VERSION is not set, got %v", tc.name, got)
			}
		})
	}
}
