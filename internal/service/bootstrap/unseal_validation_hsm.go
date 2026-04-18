package bootstrap

import (
	"context"
	"fmt"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func (m *Manager) validateKMIPUnsealPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	cfg := cluster.Spec.Unseal
	if cfg == nil || cfg.KMIP == nil {
		return nil
	}

	if strings.TrimSpace(cfg.KMIP.ClientCert) == "" || strings.TrimSpace(cfg.KMIP.ClientKey) == "" {
		return providerPrerequisitesError(
			fmt.Errorf("spec.unseal.kmip.clientCert and spec.unseal.kmip.clientKey are required by the KMIP seal"),
		)
	}

	fileRefs, err := kmipSecretFileRefs(cfg.KMIP)
	if err != nil {
		return providerPrerequisitesError(err)
	}
	if len(fileRefs) == 0 {
		return nil
	}

	secret, err := m.credentialsSecret(ctx, cluster, "kmip")
	if err != nil {
		return providerPrerequisitesError(err)
	}

	refByField, err := secretDataForFileRefs(secret.Data, fileRefs, cluster, cfg.CredentialsSecretRef.Name, "kmip credentials")
	if err != nil {
		return providerPrerequisitesError(err)
	}

	if err := validateTransitClientKeyPairPEM(refByField["certificate"], refByField["key"]); err != nil {
		return providerPrerequisitesError(
			fmt.Errorf("spec.unseal.kmip.clientCert and spec.unseal.kmip.clientKey are invalid: %w", err),
		)
	}

	if caData, ok := refByField["caCert"]; ok {
		if err := validateTransitCAPEM(caData); err != nil {
			return providerPrerequisitesError(
				fmt.Errorf("spec.unseal.kmip.caCert is invalid: %w", err),
			)
		}
	}

	return nil
}

func (m *Manager) validatePKCS11UnsealPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	cfg := cluster.Spec.Unseal
	if cfg == nil || cfg.PKCS11 == nil {
		return nil
	}

	hasSlot := strings.TrimSpace(cfg.PKCS11.Slot) != ""
	hasTokenLabel := strings.TrimSpace(cfg.PKCS11.TokenLabel) != ""
	if !hasSlot && !hasTokenLabel {
		return providerPrerequisitesError(
			fmt.Errorf("spec.unseal.pkcs11.slot or spec.unseal.pkcs11.tokenLabel is required by the PKCS#11 seal"),
		)
	}
	if hasSlot && hasTokenLabel {
		return providerPrerequisitesError(
			fmt.Errorf("spec.unseal.pkcs11.slot and spec.unseal.pkcs11.tokenLabel are mutually exclusive"),
		)
	}

	if strings.TrimSpace(cfg.PKCS11.PIN) != "" {
		return nil
	}

	secret, err := m.credentialsSecret(ctx, cluster, "pkcs11")
	if err != nil {
		return providerPrerequisitesError(err)
	}

	pin, ok := secret.Data["BAO_HSM_PIN"]
	if !ok || len(strings.TrimSpace(string(pin))) == 0 {
		return providerPrerequisitesError(
			fmt.Errorf("pkcs11 credentials Secret %s/%s is missing required key %q", cluster.Namespace, cfg.CredentialsSecretRef.Name, "BAO_HSM_PIN"),
		)
	}

	return nil
}
