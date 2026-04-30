package security

import (
	"context"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portsecurity "github.com/dc-tec/openbao-operator/internal/port/security"
	"github.com/go-logr/logr"
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
