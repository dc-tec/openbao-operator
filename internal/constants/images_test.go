package constants

import (
	"testing"
)

func TestDefaultBackupImage_WithVersion(t *testing.T) {
	// Set the version using t.Setenv (auto-cleanup, no error checking needed)
	t.Setenv(EnvOperatorVersion, "v1.2.3")

	// Clear any repo override
	t.Setenv(EnvOperatorBackupImageRepo, "")

	got := DefaultBackupImage()
	want := DefaultBackupImageRepository + ":v1.2.3"
	if got != want {
		t.Errorf("DefaultBackupImage() = %v, want %v", got, want)
	}
}

func TestDefaultBackupImage_WithCustomRepo(t *testing.T) {
	t.Setenv(EnvOperatorVersion, "v1.2.3")
	t.Setenv(EnvOperatorBackupImageRepo, "my-registry.io/custom-backup")

	got := DefaultBackupImage()
	want := "my-registry.io/custom-backup:v1.2.3"
	if got != want {
		t.Errorf("DefaultBackupImage() = %v, want %v", got, want)
	}
}

func TestDefaultUpgradeImage_WithVersion(t *testing.T) {
	t.Setenv(EnvOperatorVersion, "v2.0.0")
	t.Setenv(EnvOperatorUpgradeImageRepo, "")

	got := DefaultUpgradeImage()
	want := DefaultUpgradeImageRepository + ":v2.0.0"
	if got != want {
		t.Errorf("DefaultUpgradeImage() = %v, want %v", got, want)
	}
}

func TestDefaultInitImage_WithVersion(t *testing.T) {
	t.Setenv(EnvOperatorVersion, "v3.0.0")
	t.Setenv(EnvOperatorInitImageRepo, "")

	got := DefaultInitImage()
	want := DefaultInitImageRepository + ":v3.0.0"
	if got != want {
		t.Errorf("DefaultInitImage() = %v, want %v", got, want)
	}
}

func TestDefaultImage_PanicsWithoutVersion(t *testing.T) {
	// Ensure OPERATOR_VERSION is not set
	t.Setenv(EnvOperatorVersion, "")

	// Test that each default image function panics when version is missing.
	testCases := []struct {
		name string
		fn   func() string
	}{
		{"DefaultBackupImage", DefaultBackupImage},
		{"DefaultUpgradeImage", DefaultUpgradeImage},
		{"DefaultInitImage", DefaultInitImage},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("%s() should have panicked when OPERATOR_VERSION is not set", tc.name)
				}
			}()

			// This should panic
			_ = tc.fn()
		})
	}
}
