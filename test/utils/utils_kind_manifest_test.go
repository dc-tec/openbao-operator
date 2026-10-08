package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadImageToKindClusterWithName(t *testing.T) {
	t.Chdir(".") // Restore the working directory changed by Run after this test.
	image := "ghcr.io/dc-tec/openbao-init@sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name      string
		image     string
		kindArgs  string
		nodes     string
		failNode  string
		wantError string
		wantPulls []string
	}{
		{
			name:     "load local tags without a registry pull",
			image:    "openbao-init:dev",
			kindArgs: "load docker-image openbao-init:dev --name digest-test",
		},
		{
			name:      "pull the exact digest on every node",
			nodes:     "kind-control-plane\nkind-worker\n",
			wantPulls: []string{"kind-control-plane", "kind-worker"},
		},
		{
			name:      "reject an empty cluster",
			wantError: "no nodes found",
		},
		{
			name:      "report a failed pull without importing an archive",
			nodes:     "kind-control-plane\nkind-worker\n",
			failNode:  "kind-control-plane",
			wantError: `pull image "` + image + `" into Kind node "kind-control-plane"`,
			wantPulls: []string{"kind-control-plane"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			imageName := tc.image
			if imageName == "" {
				imageName = image
			}
			kindArgs := tc.kindArgs
			if kindArgs == "" {
				kindArgs = "get nodes --name digest-test"
			}
			dir := t.TempDir()
			trace := filepath.Join(dir, "pulls")
			kindPath := filepath.Join(dir, "kind")
			kindScript := `#!/bin/sh
[ "$*" = "$TEST_KIND_ARGS" ] || exit 91
printf '%s' "$TEST_KIND_NODES"
`
			dockerScript := `#!/bin/sh
printf '%s\n' "$*" >> "$TEST_PULL_TRACE"
[ "$2" != "$TEST_FAIL_NODE" ]
`
			for name, script := range map[string]string{kindPath: kindScript, filepath.Join(dir, "docker"): dockerScript} {
				if err := os.WriteFile(name, []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("KIND", kindPath)
			t.Setenv("KIND_CLUSTER", "digest-test")
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("TEST_KIND_NODES", tc.nodes)
			t.Setenv("TEST_KIND_ARGS", kindArgs)
			t.Setenv("TEST_PULL_TRACE", trace)
			t.Setenv("TEST_FAIL_NODE", tc.failNode)

			err := LoadImageToKindClusterWithName(imageName)
			if tc.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
			var expected strings.Builder
			for _, node := range tc.wantPulls {
				expected.WriteString("exec " + node + " crictl pull " + imageName + "\n")
			}
			got, readErr := os.ReadFile(trace)
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			if string(got) != expected.String() {
				t.Fatalf("pull commands = %q, want %q", got, expected.String())
			}
		})
	}
}

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
