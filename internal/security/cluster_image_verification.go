package security

import (
	"context"
	"fmt"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/interfaces"
	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/name"
)

const (
	defaultGitHubOIDCIssuer = "https://token.actions.githubusercontent.com"

	openBaoReleaseSubjectPrefix       = "https://github.com/openbao/openbao/.github/workflows/release.yml@refs/tags/"
	operatorReleaseSubjectPrefix      = "https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml@refs/tags/"
	openBaoOfficialRepository         = "openbao/openbao"
	operatorInitOfficialRepository    = "ghcr.io/dc-tec/openbao-init"
	operatorBackupOfficialRepository  = "ghcr.io/dc-tec/openbao-backup"
	operatorUpgradeOfficialRepository = "ghcr.io/dc-tec/openbao-upgrade"
)

// VerifyImageForCluster verifies an image reference using the cluster's ImageVerification configuration.
// It returns an image digest reference (e.g., "repo@sha256:...") when verification is enabled and succeeds.
// When image verification is disabled, it returns an empty digest and a nil error.
func VerifyImageForCluster(ctx context.Context, logger logr.Logger, verifier interfaces.ImageVerifier, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	if cluster == nil {
		return "", fmt.Errorf("cluster is required")
	}
	if cluster.Spec.ImageVerification == nil || !cluster.Spec.ImageVerification.Enabled {
		return "", nil
	}
	if imageRef == "" {
		return "", fmt.Errorf("image reference is required")
	}
	if verifier == nil {
		return "", fmt.Errorf("image verifier is required")
	}

	config := interfaces.VerifyConfig{
		PublicKey:        strings.TrimSpace(cluster.Spec.ImageVerification.PublicKey),
		Issuer:           strings.TrimSpace(cluster.Spec.ImageVerification.Issuer),
		Subject:          strings.TrimSpace(cluster.Spec.ImageVerification.Subject),
		IgnoreTlog:       cluster.Spec.ImageVerification.IgnoreTlog,
		ImagePullSecrets: cluster.Spec.ImageVerification.ImagePullSecrets,
		Namespace:        cluster.Namespace,
	}

	applyOfficialKeylessDefaults(&config, imageRef, false)

	// Validate that either PublicKey OR (Issuer and Subject) are provided after
	// applying any official keyless defaults.
	if !hasKeylessConfig(config) && config.PublicKey == "" {
		return "", fmt.Errorf("image verification is enabled but neither public key nor keyless configuration (issuer and subject) is provided")
	}

	digest, err := verifier.Verify(ctx, imageRef, config)
	if err != nil {
		return "", err
	}

	return digest, nil
}

// VerifyOperatorImageForCluster verifies an operator-managed helper image (init container,
// backup/upgrade/restore executors) using the cluster's OperatorImageVerification config.
// Unlike VerifyImageForCluster, this function does NOT fall back to ImageVerification.
// If OperatorImageVerification is not configured, verification is skipped for helper images.
func VerifyOperatorImageForCluster(ctx context.Context, logger logr.Logger, verifier interfaces.ImageVerifier, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	if cluster == nil {
		return "", fmt.Errorf("cluster is required")
	}
	if imageRef == "" {
		return "", fmt.Errorf("image reference is required")
	}

	// Use OperatorImageVerification only - no fallback to ImageVerification
	// This prevents confusing failures when the main image and helper images have different signers
	verificationConfig := cluster.Spec.OperatorImageVerification
	if verificationConfig == nil || !verificationConfig.Enabled {
		return "", nil
	}

	if verifier == nil {
		return "", fmt.Errorf("image verifier is required")
	}

	config := interfaces.VerifyConfig{
		PublicKey:        strings.TrimSpace(verificationConfig.PublicKey),
		Issuer:           strings.TrimSpace(verificationConfig.Issuer),
		Subject:          strings.TrimSpace(verificationConfig.Subject),
		IgnoreTlog:       verificationConfig.IgnoreTlog,
		ImagePullSecrets: verificationConfig.ImagePullSecrets,
		Namespace:        cluster.Namespace,
	}

	applyOfficialKeylessDefaults(&config, imageRef, true)

	// Validate that either PublicKey OR (Issuer and Subject) are provided after
	// applying any official keyless defaults.
	if !hasKeylessConfig(config) && config.PublicKey == "" {
		return "", fmt.Errorf("operator image verification is enabled but neither public key nor keyless configuration (issuer and subject) is provided")
	}

	digest, err := verifier.Verify(ctx, imageRef, config)
	if err != nil {
		return "", err
	}

	return digest, nil
}

func hasKeylessConfig(config interfaces.VerifyConfig) bool {
	return strings.TrimSpace(config.Issuer) != "" && strings.TrimSpace(config.Subject) != ""
}

func applyOfficialKeylessDefaults(config *interfaces.VerifyConfig, imageRef string, isOperatorImage bool) {
	if config == nil {
		return
	}
	if strings.TrimSpace(config.PublicKey) != "" {
		return
	}
	// Only default keyless identity when both fields are omitted.
	if strings.TrimSpace(config.Issuer) != "" || strings.TrimSpace(config.Subject) != "" {
		return
	}

	repo, tag, ok := imageRepositoryAndTag(imageRef)
	if !ok {
		return
	}

	subject := defaultSubjectForImage(repo, tag, isOperatorImage)
	if subject == "" {
		return
	}

	config.Issuer = defaultGitHubOIDCIssuer
	config.Subject = subject
}

func imageRepositoryAndTag(imageRef string) (string, string, bool) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", "", false
	}

	tagRef, ok := ref.(name.Tag)
	if !ok {
		return "", "", false
	}

	repository := normalizeRepository(tagRef.Context().Name())
	tag := strings.TrimSpace(tagRef.TagStr())
	if repository == "" || tag == "" {
		return "", "", false
	}

	return repository, tag, true
}

func normalizeRepository(repository string) string {
	repository = strings.TrimSpace(repository)
	repository = strings.TrimPrefix(repository, "index.docker.io/")
	repository = strings.TrimPrefix(repository, "docker.io/")
	return repository
}

func defaultSubjectForImage(repository, tag string, isOperatorImage bool) string {
	if !isOperatorImage {
		if repository != openBaoOfficialRepository {
			return ""
		}

		tag = strings.TrimSpace(tag)
		if tag == "" {
			return ""
		}
		if !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}

		return openBaoReleaseSubjectPrefix + tag
	}

	switch repository {
	case operatorInitOfficialRepository, operatorBackupOfficialRepository, operatorUpgradeOfficialRepository:
		return operatorReleaseSubjectPrefix + tag
	default:
		return ""
	}
}
