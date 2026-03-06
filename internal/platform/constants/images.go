package constants

import (
	"fmt"
	"os"
	"strings"
)

// DefaultBackupImage returns the default backup executor image.
// If the cluster specifies an image, it should be used instead.
// The tag is derived from OPERATOR_VERSION env var.
// Default repositories
const (
	// DefaultOpenBaoImageRepository is the default image repository used for OpenBao.
	DefaultOpenBaoImageRepository = "openbao/openbao"

	// EnvOpenBaoImageRepo is the environment variable to override the OpenBao image repository.
	EnvOpenBaoImageRepo = "RELATED_IMAGE_OPENBAO"
)

// DefaultBackupImage returns the default backup executor image.
// If the cluster specifies an image, it should be used instead.
// The tag is derived from OPERATOR_VERSION env var.
func DefaultBackupImage() (string, error) {
	return defaultImage(EnvOperatorBackupImageRepo, DefaultBackupImageRepository)
}

// DefaultUpgradeImage returns the default upgrade executor image.
// If the cluster specifies an image, it should be used instead.
// The tag is derived from OPERATOR_VERSION env var.
func DefaultUpgradeImage() (string, error) {
	return defaultImage(EnvOperatorUpgradeImageRepo, DefaultUpgradeImageRepository)
}

// DefaultInitImage returns the default init container image.
// If the cluster specifies an image, it should be used instead.
// The tag is derived from OPERATOR_VERSION env var.
func DefaultInitImage() (string, error) {
	return defaultImage(EnvOperatorInitImageRepo, DefaultInitImageRepository)
}

// GetOpenBaoImage constructs the OpenBao image reference.
// It uses specVersion for the tag.
func GetOpenBaoImage(specVersion string) string {
	repo := os.Getenv(EnvOpenBaoImageRepo)
	if repo == "" {
		repo = DefaultOpenBaoImageRepository
	}

	// spec.version is expected to be a tag string (e.g. "2.4.4" or "v2.4.4").
	// Do not force a "v" prefix: different registries/projects use different conventions.
	return fmt.Sprintf("%s:%s", repo, strings.TrimSpace(specVersion))
}

// defaultImage constructs an image reference from an env var override or default repo,
// combined with the operator version tag.
func defaultImage(envVar, defaultRepo string) (string, error) {
	repo := strings.TrimSpace(os.Getenv(envVar))
	if repo == "" {
		repo = defaultRepo
	}
	version := os.Getenv(EnvOperatorVersion)
	if version != "" {
		return fmt.Sprintf("%s:%s", repo, version), nil
	}
	// OPERATOR_VERSION must be set in production deployments.
	return "", fmt.Errorf("OPERATOR_VERSION environment variable is required but not set")
}
