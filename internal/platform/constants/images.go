package constants

import (
	"fmt"
	"os"
	"strings"
)

// DefaultBackupImage returns the default backup executor image.
// If the cluster specifies an image, it should be used instead.
// A complete image override takes precedence over the repository and OPERATOR_VERSION tag.
// Default repositories
const (
	// DefaultOpenBaoImageRepository is the default image repository used for OpenBao.
	DefaultOpenBaoImageRepository = "openbao/openbao"

	// EnvOpenBaoImageRepo is the environment variable to override the OpenBao image repository.
	EnvOpenBaoImageRepo = "RELATED_IMAGE_OPENBAO"
)

// DefaultBackupImage returns the default backup executor image.
// If the cluster specifies an image, it should be used instead.
// A complete image override takes precedence over the repository and OPERATOR_VERSION tag.
func DefaultBackupImage() (string, error) {
	return defaultImage(EnvOperatorBackupImage, EnvOperatorBackupImageRepo, DefaultBackupImageRepository, "backup")
}

// DefaultUpgradeImage returns the default upgrade executor image.
// If the cluster specifies an image, it should be used instead.
// A complete image override takes precedence over the repository and OPERATOR_VERSION tag.
func DefaultUpgradeImage() (string, error) {
	return defaultImage(EnvOperatorUpgradeImage, EnvOperatorUpgradeImageRepo, DefaultUpgradeImageRepository, "upgrade")
}

// DefaultInitImage returns the default init container image.
// If the cluster specifies an image, it should be used instead.
// A complete image override takes precedence over the repository and OPERATOR_VERSION tag.
func DefaultInitImage() (string, error) {
	return defaultImage(EnvOperatorInitImage, EnvOperatorInitImageRepo, DefaultInitImageRepository, "initContainer")
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

// defaultImage prefers a complete image reference, then combines the repository and operator version.
func defaultImage(imageEnv, envVar, defaultRepo, fieldPath string) (string, error) {
	if image := strings.TrimSpace(os.Getenv(imageEnv)); image != "" {
		return image, nil
	}
	repo := strings.TrimSpace(os.Getenv(envVar))
	if repo == "" {
		repo = defaultRepo
	}
	version := os.Getenv(EnvOperatorVersion)
	if version != "" {
		return fmt.Sprintf("%s:%s", repo, version), nil
	}
	// OPERATOR_VERSION must be set in production deployments.
	return "", fmt.Errorf("OPERATOR_VERSION environment variable is required when spec.%s.image is not set explicitly", fieldPath)
}
