package security

import (
	"context"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portsecurity "github.com/dc-tec/openbao-operator/internal/port/security"
	"github.com/go-logr/logr"
)

const (
	defaultGitHubOIDCIssuerRegExp = "^https://token\\.actions\\.githubusercontent\\.com$"
	openBaoReleaseSubjectRegExp   = "^https://github\\.com/openbao/openbao/\\.github/workflows/release\\.yml@refs/tags/v?[0-9A-Za-z][0-9A-Za-z._+-]*$"
	operatorSubjectRegExp         = "^https://github\\.com/dc-tec/openbao-operator/\\.github/workflows/(release\\.yml@refs/tags/.+|publish-edge\\.yml@refs/heads/main|publish-nightly\\.yml@refs/heads/main|reusable-build\\.yml@refs/heads/main)$"
)

func VerifyImageForCluster(ctx context.Context, logger logr.Logger, verifier imageverify.Verifier, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	return portsecurity.VerifyImageForCluster(ctx, logger, verifier, cluster, imageRef)
}

func VerifyOperatorImageForCluster(ctx context.Context, logger logr.Logger, verifier imageverify.Verifier, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	return portsecurity.VerifyOperatorImageForCluster(ctx, logger, verifier, cluster, imageRef)
}

func IsMainImageVerificationEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return portsecurity.IsMainImageVerificationEnabled(cluster)
}

func IsOperatorImageVerificationEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return portsecurity.IsOperatorImageVerificationEnabled(cluster)
}

func hasStrictKeylessConfig(config imageverify.VerifyConfig) bool {
	return strings.TrimSpace(config.Issuer) != "" && strings.TrimSpace(config.Subject) != ""
}

func hasRegexKeylessConfig(config imageverify.VerifyConfig) bool {
	return strings.TrimSpace(config.IssuerRegExp) != "" && strings.TrimSpace(config.SubjectRegExp) != ""
}

func hasKeylessConfig(config imageverify.VerifyConfig) bool {
	return hasStrictKeylessConfig(config) || hasRegexKeylessConfig(config)
}
