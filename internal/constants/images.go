package constants

import (
	"fmt"
	"os"
	"strings"
)

// DefaultBackupImage returns the default backup executor image.
// If the cluster specifies an image, it should be used instead.
// The tag is derived from OPERATOR_VERSION env var.
func DefaultBackupImage() string {
	return defaultImage(EnvOperatorBackupImageRepo, DefaultBackupImageRepository)
}

// DefaultUpgradeImage returns the default upgrade executor image.
// If the cluster specifies an image, it should be used instead.
// The tag is derived from OPERATOR_VERSION env var.
func DefaultUpgradeImage() string {
	return defaultImage(EnvOperatorUpgradeImageRepo, DefaultUpgradeImageRepository)
}

// DefaultInitImage returns the default init container image.
// If the cluster specifies an image, it should be used instead.
// The tag is derived from OPERATOR_VERSION env var.
func DefaultInitImage() string {
	return defaultImage(EnvOperatorInitImageRepo, DefaultInitImageRepository)
}

// defaultImage constructs an image reference from an env var override or default repo,
// combined with the operator version tag.
func defaultImage(envVar, defaultRepo string) string {
	repo := strings.TrimSpace(os.Getenv(envVar))
	if repo == "" {
		repo = defaultRepo
	}
	version := os.Getenv(EnvOperatorVersion)
	if version != "" {
		return fmt.Sprintf("%s:%s", repo, version)
	}
	// OPERATOR_VERSION must be set in production deployments.
	// If you see this panic, ensure the operator Deployment sets the OPERATOR_VERSION env var.
	panic("OPERATOR_VERSION environment variable is required but not set. " +
		"This is a deployment configuration error. " +
		"Ensure the operator Deployment sets OPERATOR_VERSION to the release version (e.g., v0.1.0).")
}
