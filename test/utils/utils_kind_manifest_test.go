package utils

import (
	"strings"
	"testing"
)

func TestSelectManifestDigestForPlatform(t *testing.T) {
	t.Parallel()

	const manifestList = `{
		"schemaVersion": 2,
		"manifests": [
			{
				"digest": "sha256:amd64digest",
				"platform": {"os": "linux", "architecture": "amd64"}
			},
			{
				"digest": "sha256:arm64digest",
				"platform": {"os": "linux", "architecture": "arm64"}
			}
		]
	}`

	got, err := selectManifestDigestForPlatform(manifestList, "linux", "amd64")
	if err != nil {
		t.Fatalf("selectManifestDigestForPlatform returned error: %v", err)
	}
	if got != "sha256:amd64digest" {
		t.Fatalf("expected amd64 digest, got %q", got)
	}
}

func TestSelectManifestDigestForPlatformMissingPlatform(t *testing.T) {
	t.Parallel()

	const manifestList = `{
		"schemaVersion": 2,
		"manifests": [
			{
				"digest": "sha256:arm64digest",
				"platform": {"os": "linux", "architecture": "arm64"}
			}
		]
	}`

	_, err := selectManifestDigestForPlatform(manifestList, "linux", "amd64")
	if err == nil {
		t.Fatalf("expected error when platform digest is missing")
	}
	if !strings.Contains(err.Error(), "no linux/amd64 manifest found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSelectManifestDigestForPlatformInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := selectManifestDigestForPlatform("not-json", "linux", "amd64")
	if err == nil {
		t.Fatalf("expected parse error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse manifest list") {
		t.Fatalf("unexpected error: %v", err)
	}
}
