package bootstrap

import (
	"context"
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

const reasonPrerequisitesMissing = "PrerequisitesMissing"

func (m *Manager) validateUnsealPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Spec.Unseal == nil {
		return nil
	}
	if err := validateUnsealCredentialsSecretRef(cluster); err != nil {
		return providerPrerequisitesError(err)
	}

	switch cluster.Spec.Unseal.Type {
	case "", portopenbao.SealTypeStatic:
		return nil
	case unsealTypeTransit:
		return m.validateTransitUnsealPrerequisites(ctx, cluster)
	case portopenbao.SealTypeAWSKMS:
		return m.validateAWSKMSUnsealPrerequisites(ctx, cluster)
	case portopenbao.SealTypeAzureKeyVault:
		return m.validateAzureKeyVaultUnsealPrerequisites(ctx, cluster)
	case portopenbao.SealTypeGCPCKMS:
		return m.validateGCPCKMSUnsealPrerequisites(ctx, cluster)
	case portopenbao.SealTypeKMIP:
		return m.validateKMIPUnsealPrerequisites(ctx, cluster)
	case portopenbao.SealTypeKMSPlugin:
		return m.validateKMSPluginUnsealPrerequisites(ctx, cluster)
	case portopenbao.SealTypeOCIKMS:
		return m.validateOCIKMSUnsealPrerequisites(ctx, cluster)
	case portopenbao.SealTypePKCS11:
		return m.validatePKCS11UnsealPrerequisites(ctx, cluster)
	default:
		return nil
	}
}

func transitPrerequisitesError(err error) error {
	if err == nil {
		return nil
	}
	return operatorerrors.WithReason(reasonPrerequisitesMissing, operatorerrors.WrapPermanentPrerequisitesMissing(err))
}

func providerPrerequisitesError(err error) error {
	return transitPrerequisitesError(err)
}

func missingSecretKeyError(namespace, secretName, provider, key string) error {
	return fmt.Errorf("%s credentials Secret %s/%s is missing required key %q", provider, namespace, secretName, key)
}

func (m *Manager) validateKMSPluginUnsealPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Spec.Unseal == nil || cluster.Spec.Unseal.CredentialsSecretRef == nil {
		return nil
	}
	if _, err := m.credentialsSecret(ctx, cluster, "kms plugin"); err != nil {
		return providerPrerequisitesError(err)
	}
	return nil
}
