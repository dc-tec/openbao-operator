package helpers

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultStorageEmulatorImagesMatchSuitePolicy(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../suites.yaml")
	if err != nil {
		t.Fatalf("read suites.yaml: %v", err)
	}

	var manifest struct {
		Versions struct {
			StorageEmulators struct {
				RustFSImage  string `yaml:"rustfsImage"`
				FakeGCSImage string `yaml:"fakeGCSImage"`
				AzuriteImage string `yaml:"azuriteImage"`
			} `yaml:"storageEmulators"`
		} `yaml:"versions"`
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse suites.yaml: %v", err)
	}

	assertPinnedDigest(t, "RustFS", manifest.Versions.StorageEmulators.RustFSImage)
	assertPinnedDigest(t, "fake-gcs-server", manifest.Versions.StorageEmulators.FakeGCSImage)
	assertPinnedDigest(t, "Azurite", manifest.Versions.StorageEmulators.AzuriteImage)

	if got := DefaultRustFSImage; got != manifest.Versions.StorageEmulators.RustFSImage {
		t.Fatalf("DefaultRustFSImage = %q, want suites.yaml rustfsImage %q", got, manifest.Versions.StorageEmulators.RustFSImage)
	}
	if got := DefaultFakeGCSImage; got != manifest.Versions.StorageEmulators.FakeGCSImage {
		t.Fatalf("DefaultFakeGCSImage = %q, want suites.yaml fakeGCSImage %q", got, manifest.Versions.StorageEmulators.FakeGCSImage)
	}
	if got := DefaultAzuriteImage; got != manifest.Versions.StorageEmulators.AzuriteImage {
		t.Fatalf("DefaultAzuriteImage = %q, want suites.yaml azuriteImage %q", got, manifest.Versions.StorageEmulators.AzuriteImage)
	}
}

func assertPinnedDigest(t *testing.T, name, image string) {
	t.Helper()
	if strings.TrimSpace(image) == "" {
		t.Fatalf("%s image is empty", name)
	}
	if !strings.Contains(image, "@sha256:") {
		t.Fatalf("%s image %q must be digest-pinned", name, image)
	}
}
