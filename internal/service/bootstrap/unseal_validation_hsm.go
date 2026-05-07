package bootstrap

import (
	"context"
	"fmt"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
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

	if err := validatePKCS11RuntimeEnvMappings(cfg.PKCS11); err != nil {
		return providerPrerequisitesError(err)
	}

	needsCredentialsSecret := strings.TrimSpace(cfg.PKCS11.PIN) == "" || pkcs11RuntimeNeedsCredentialsSecret(cfg.PKCS11.Runtime)
	if !needsCredentialsSecret {
		return nil
	}

	secret, err := m.credentialsSecret(ctx, cluster, portopenbao.SealTypePKCS11)
	if err != nil {
		return providerPrerequisitesError(err)
	}

	if strings.TrimSpace(cfg.PKCS11.PIN) == "" {
		pin, ok := secret.Data[portopenbao.EnvBaoHSMPIN]
		if !ok || len(strings.TrimSpace(string(pin))) == 0 {
			return providerPrerequisitesError(
				fmt.Errorf("pkcs11 credentials Secret %s/%s is missing required key %q", cluster.Namespace, cfg.CredentialsSecretRef.Name, portopenbao.EnvBaoHSMPIN),
			)
		}
	}

	if err := validatePKCS11RuntimeSecretKeys(secret.Data, cluster.Namespace, cfg.CredentialsSecretRef.Name, cfg.PKCS11.Runtime); err != nil {
		return providerPrerequisitesError(
			err,
		)
	}

	return nil
}

func pkcs11RuntimeNeedsCredentialsSecret(runtime *openbaov1alpha1.PKCS11RuntimeConfig) bool {
	return len(portopenbao.PKCS11RuntimeMappings(runtime)) > 0
}

func validatePKCS11RuntimeEnvMappings(cfg *openbaov1alpha1.PKCS11SealConfig) error {
	if cfg == nil || cfg.Runtime == nil {
		return nil
	}

	mappings := portopenbao.PKCS11RuntimeMappings(cfg.Runtime)
	seen := make(map[string]string, len(mappings))
	for _, env := range mappings {
		fieldPath := fmt.Sprintf("spec.unseal.pkcs11.%s[%s]", env.Kind, env.Name)
		if err := validatePKCS11RuntimeEnvName(env.Name); err != nil {
			return fmt.Errorf("%s is invalid: %w", fieldPath, err)
		}
		if strings.TrimSpace(env.SecretKey) == "" {
			return fmt.Errorf("%s.secretKey is required", fieldPath)
		}
		if previous, ok := seen[env.Name]; ok {
			return fmt.Errorf("%s duplicates %s", fieldPath, previous)
		}
		seen[env.Name] = string(env.Kind)
	}

	return nil
}

func validatePKCS11RuntimeEnvName(name string) error {
	if !portopenbao.IsValidEnvVarName(name) {
		return fmt.Errorf("environment variable name must match ^[A-Za-z_][A-Za-z0-9_]*$")
	}
	if portopenbao.IsPKCS11SealOwnedEnvVar(name) {
		return fmt.Errorf("environment variable %q is managed by spec.unseal.pkcs11", name)
	}
	return nil
}

func validatePKCS11RuntimeSecretKeys(secretData map[string][]byte, namespace, secretName string, runtime *openbaov1alpha1.PKCS11RuntimeConfig) error {
	if runtime == nil {
		return nil
	}

	mappings := portopenbao.PKCS11RuntimeMappings(runtime)
	required := make([]string, 0, len(mappings))
	for _, env := range mappings {
		required = append(required, env.SecretKey)
	}
	if len(required) == 0 {
		return nil
	}

	return requireSecretKeys(secretData, namespace, secretName, "pkcs11 credentials", required...)
}
