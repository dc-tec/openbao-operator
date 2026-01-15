package constants

import (
	"fmt"
	"os"
	"strings"
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
