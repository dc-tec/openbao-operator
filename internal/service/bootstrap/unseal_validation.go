package bootstrap

import (
	"context"
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

const reasonPrerequisitesMissing = "PrerequisitesMissing"

func (m *Manager) validateUnsealPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Spec.Unseal == nil {
		return nil
	}

	switch cluster.Spec.Unseal.Type {
	case "", "static":
		return nil
	case unsealTypeTransit:
		return m.validateTransitUnsealPrerequisites(ctx, cluster)
	case "awskms":
		return m.validateAWSKMSUnsealPrerequisites(ctx, cluster)
	case "azurekeyvault":
		return m.validateAzureKeyVaultUnsealPrerequisites(ctx, cluster)
	case "gcpckms":
		return m.validateGCPCKMSUnsealPrerequisites(ctx, cluster)
	case "kmip":
		return m.validateKMIPUnsealPrerequisites(ctx, cluster)
	case "ocikms":
		return m.validateOCIKMSUnsealPrerequisites(ctx, cluster)
	case "pkcs11":
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
