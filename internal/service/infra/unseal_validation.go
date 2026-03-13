package infra

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

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

func (m *Manager) validateAWSKMSUnsealPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	cfg := cluster.Spec.Unseal
	if cfg == nil || cfg.AWSKMS == nil || cfg.CredentialsSecretRef == nil {
		return nil
	}

	secret, err := m.credentialsSecret(ctx, cluster, "awskms")
	if err != nil {
		return providerPrerequisitesError(err)
	}

	if err := requireSecretKeysTogether(secret.Data, cluster.Namespace, cfg.CredentialsSecretRef.Name, "awskms credentials", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"); err != nil {
		return providerPrerequisitesError(err)
	}

	return nil
}

func (m *Manager) validateAzureKeyVaultUnsealPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	cfg := cluster.Spec.Unseal
	if cfg == nil || cfg.AzureKeyVault == nil || cfg.CredentialsSecretRef == nil {
		return nil
	}

	secret, err := m.credentialsSecret(ctx, cluster, "azurekeyvault")
	if err != nil {
		return providerPrerequisitesError(err)
	}

	if err := requireSecretKeys(secret.Data, cluster.Namespace, cfg.CredentialsSecretRef.Name, "azurekeyvault credentials", "AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET"); err != nil {
		return providerPrerequisitesError(err)
	}

	return nil
}

func (m *Manager) validateOCIKMSUnsealPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	cfg := cluster.Spec.Unseal
	if cfg == nil || cfg.OCIKMS == nil {
		return nil
	}

	usesAPIKey := cfg.OCIKMS.AuthTypeAPIKey != nil && *cfg.OCIKMS.AuthTypeAPIKey
	if cfg.CredentialsSecretRef != nil && !usesAPIKey {
		return providerPrerequisitesError(
			fmt.Errorf("ocikms credentials Secret requires spec.unseal.ocikms.authTypeAPIKey=true"),
		)
	}
	if !usesAPIKey || cfg.CredentialsSecretRef == nil {
		return nil
	}

	secret, err := m.credentialsSecret(ctx, cluster, "ocikms")
	if err != nil {
		return providerPrerequisitesError(err)
	}

	configData := secret.Data["config"]
	if len(strings.TrimSpace(string(configData))) == 0 {
		return providerPrerequisitesError(
			fmt.Errorf("ocikms credentials Secret %s/%s is missing required key %q", cluster.Namespace, cfg.CredentialsSecretRef.Name, "config"),
		)
	}

	configFields, err := parseOCIKMSDefaultProfile(configData)
	if err != nil {
		return providerPrerequisitesError(
			fmt.Errorf("ocikms credentials Secret %s/%s has invalid OCI SDK config in key %q: %w", cluster.Namespace, cfg.CredentialsSecretRef.Name, "config", err),
		)
	}

	for _, key := range []string{"user", "fingerprint", "tenancy", "region", "key_file"} {
		if strings.TrimSpace(configFields[key]) == "" {
			return providerPrerequisitesError(
				fmt.Errorf("ocikms credentials Secret %s/%s OCI SDK config must define %q in profile [DEFAULT]", cluster.Namespace, cfg.CredentialsSecretRef.Name, key),
			)
		}
	}

	keyFileKey, ok := mountedSealCredentialsKey(configFields["key_file"])
	if !ok {
		return providerPrerequisitesError(
			fmt.Errorf("ocikms credentials Secret %s/%s OCI SDK config key_file must reference a file under %s", cluster.Namespace, cfg.CredentialsSecretRef.Name, sealCredsVolumeMountPath),
		)
	}
	if len(strings.TrimSpace(string(secret.Data[keyFileKey]))) == 0 {
		return providerPrerequisitesError(
			fmt.Errorf("ocikms credentials Secret %s/%s is missing required key %q referenced by OCI SDK config key_file", cluster.Namespace, cfg.CredentialsSecretRef.Name, keyFileKey),
		)
	}

	return nil
}

func (m *Manager) validateGCPCKMSUnsealPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	cfg := cluster.Spec.Unseal
	if cfg == nil || cfg.GCPCloudKMS == nil {
		return nil
	}

	credentialsPath := strings.TrimSpace(cfg.GCPCloudKMS.Credentials)
	if credentialsPath == "" {
		return nil
	}

	key, usesMountedSecret := mountedSealCredentialsKey(credentialsPath)
	if !usesMountedSecret {
		return nil
	}

	secret, err := m.credentialsSecret(ctx, cluster, "gcpckms")
	if err != nil {
		return providerPrerequisitesError(err)
	}

	data, ok := secret.Data[key]
	if !ok || len(strings.TrimSpace(string(data))) == 0 {
		return providerPrerequisitesError(
			fmt.Errorf("gcpckms credentials Secret %s/%s is missing required key %q", cluster.Namespace, cfg.CredentialsSecretRef.Name, key),
		)
	}

	if !json.Valid(data) {
		return providerPrerequisitesError(
			fmt.Errorf("spec.unseal.gcpCloudKMS.credentials (%s) must contain valid JSON credentials", key),
		)
	}

	return nil
}

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

func (m *Manager) validateTransitUnsealPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	cfg := cluster.Spec.Unseal
	if cfg == nil || cfg.Transit == nil {
		return nil
	}

	if err := validateTransitAddress(cfg.Transit.Address); err != nil {
		return transitPrerequisitesError(err)
	}
	if err := validateTransitClientTLSPair(cfg.Transit); err != nil {
		return transitPrerequisitesError(err)
	}

	fileRefs, err := transitTLSSecretFileRefs(cfg.Transit)
	if err != nil {
		return transitPrerequisitesError(err)
	}

	requiredSecretKeys := transitRequiredSecretKeys(cluster)
	if cfg.CredentialsSecretRef == nil {
		if len(requiredSecretKeys) == 0 {
			return nil
		}
		return transitPrerequisitesError(
			fmt.Errorf(
				"transit unseal requires spec.unseal.credentialsSecretRef because the configuration references Secret-backed credential keys %s",
				strings.Join(requiredSecretKeys, ", "),
			),
		)
	}

	var secret corev1.Secret
	secretName := types.NamespacedName{Namespace: cluster.Namespace, Name: cfg.CredentialsSecretRef.Name}
	if err := m.reader.Get(ctx, secretName, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return transitPrerequisitesError(fmt.Errorf("transit credentials Secret %s/%s not found", cluster.Namespace, cfg.CredentialsSecretRef.Name))
		}
		return fmt.Errorf("failed to read transit credentials Secret %s/%s: %w", cluster.Namespace, cfg.CredentialsSecretRef.Name, err)
	}

	missingKeys := make([]string, 0, len(requiredSecretKeys))
	for _, key := range requiredSecretKeys {
		if _, ok := secret.Data[key]; ok {
			continue
		}
		missingKeys = append(missingKeys, key)
	}
	if len(missingKeys) == 0 {
		if err := validateTransitSecretData(cfg.Transit, secret.Data, fileRefs); err != nil {
			return transitPrerequisitesError(err)
		}
		return nil
	}

	return transitPrerequisitesError(
		fmt.Errorf(
			"transit credentials Secret %s/%s is missing required keys %s",
			cluster.Namespace,
			cfg.CredentialsSecretRef.Name,
			strings.Join(missingKeys, ", "),
		),
	)
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

func validateTransitAddress(address string) error {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		return fmt.Errorf("spec.unseal.transit.address must be a valid absolute URL: %w", err)
	}
	if strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
		return fmt.Errorf("spec.unseal.transit.address must be a valid absolute URL")
	}
	return nil
}

func validateTransitClientTLSPair(cfg *openbaov1alpha1.TransitSealConfig) error {
	if cfg == nil {
		return nil
	}

	hasClientCert := strings.TrimSpace(cfg.TLSClientCert) != ""
	hasClientKey := strings.TrimSpace(cfg.TLSClientKey) != ""
	if hasClientCert == hasClientKey {
		return nil
	}

	return fmt.Errorf("spec.unseal.transit.tlsClientCert and spec.unseal.transit.tlsClientKey must be set together")
}

type transitTLSSecretFileRef struct {
	fieldName string
	key       string
}

type secretFileRef = transitTLSSecretFileRef

func transitTLSSecretFileRefs(cfg *openbaov1alpha1.TransitSealConfig) ([]transitTLSSecretFileRef, error) {
	if cfg == nil {
		return nil, nil
	}

	out := make([]transitTLSSecretFileRef, 0, 3)
	for _, candidate := range []struct {
		fieldName string
		path      string
	}{
		{fieldName: "tlsCACert", path: cfg.TLSCACert},
		{fieldName: "tlsClientCert", path: cfg.TLSClientCert},
		{fieldName: "tlsClientKey", path: cfg.TLSClientKey},
	} {
		if strings.TrimSpace(candidate.path) == "" {
			continue
		}

		key, ok := mountedSealCredentialsKey(candidate.path)
		if !ok {
			return nil, fmt.Errorf(
				"spec.unseal.transit.%s must reference a file under %s",
				candidate.fieldName,
				sealCredsVolumeMountPath,
			)
		}

		out = append(out, transitTLSSecretFileRef{
			fieldName: candidate.fieldName,
			key:       key,
		})
	}

	return out, nil
}

func validateTransitSecretData(
	cfg *openbaov1alpha1.TransitSealConfig,
	secretData map[string][]byte,
	fileRefs []transitTLSSecretFileRef,
) error {
	if cfg == nil {
		return nil
	}

	if strings.TrimSpace(cfg.Token) == "" && len(strings.TrimSpace(string(secretData["token"]))) == 0 {
		return fmt.Errorf("transit credentials Secret key %q must contain a non-empty token", "token")
	}

	refByField := make(map[string]string, len(fileRefs))
	for _, ref := range fileRefs {
		refByField[ref.fieldName] = ref.key
	}

	if key, ok := refByField["tlsCACert"]; ok {
		if err := validateTransitCAPEM(secretData[key]); err != nil {
			return fmt.Errorf("spec.unseal.transit.tlsCACert (%s) is invalid: %w", key, err)
		}
	}

	clientCertKey, hasClientCert := refByField["tlsClientCert"]
	clientKeyKey, hasClientKey := refByField["tlsClientKey"]
	if hasClientCert && hasClientKey {
		if err := validateTransitClientKeyPairPEM(secretData[clientCertKey], secretData[clientKeyKey]); err != nil {
			return fmt.Errorf(
				"spec.unseal.transit.tlsClientCert (%s) and spec.unseal.transit.tlsClientKey (%s) are invalid: %w",
				clientCertKey,
				clientKeyKey,
				err,
			)
		}
	}

	return nil
}

func kmipSecretFileRefs(cfg *openbaov1alpha1.KMIPSealConfig) ([]secretFileRef, error) {
	if cfg == nil {
		return nil, nil
	}

	return secretFileRefsForPaths([]struct {
		fieldName string
		path      string
	}{
		{fieldName: "certificate", path: cfg.ClientCert},
		{fieldName: "key", path: cfg.ClientKey},
		{fieldName: "caCert", path: cfg.CACert},
	}, "spec.unseal.kmip")
}

func secretFileRefsForPaths(candidates []struct {
	fieldName string
	path      string
}, fieldPrefix string) ([]secretFileRef, error) {
	out := make([]secretFileRef, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.path) == "" {
			continue
		}

		key, ok := mountedSealCredentialsKey(candidate.path)
		if !ok {
			if strings.HasPrefix(strings.TrimSpace(candidate.path), sealCredsVolumeMountPath) {
				return nil, fmt.Errorf("%s.%s must reference a file under %s", fieldPrefix, candidate.fieldName, sealCredsVolumeMountPath)
			}
			continue
		}

		out = append(out, secretFileRef{
			fieldName: candidate.fieldName,
			key:       key,
		})
	}

	return out, nil
}

func secretDataForFileRefs(
	secretData map[string][]byte,
	fileRefs []secretFileRef,
	cluster *openbaov1alpha1.OpenBaoCluster,
	secretName string,
	secretDescription string,
) (map[string][]byte, error) {
	out := make(map[string][]byte, len(fileRefs))
	for _, ref := range fileRefs {
		data, ok := secretData[ref.key]
		if !ok || len(strings.TrimSpace(string(data))) == 0 {
			return nil, fmt.Errorf(
				"%s Secret %s/%s is missing required key %q",
				secretDescription,
				cluster.Namespace,
				secretName,
				ref.key,
			)
		}
		out[ref.fieldName] = data
	}
	return out, nil
}

func requireSecretKeys(secretData map[string][]byte, namespace, secretName, description string, keys ...string) error {
	missing := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(strings.TrimSpace(string(secretData[key]))) == 0 {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%s Secret %s/%s is missing required keys %s", description, namespace, secretName, strings.Join(missing, ", "))
}

func requireSecretKeysTogether(secretData map[string][]byte, namespace, secretName, description, firstKey, secondKey string) error {
	first := strings.TrimSpace(string(secretData[firstKey]))
	second := strings.TrimSpace(string(secretData[secondKey]))
	switch {
	case first != "" && second != "":
		return nil
	case first == "" && second == "":
		return fmt.Errorf("%s Secret %s/%s must contain both %s and %s", description, namespace, secretName, firstKey, secondKey)
	default:
		return fmt.Errorf("%s Secret %s/%s must contain both %s and %s", description, namespace, secretName, firstKey, secondKey)
	}
}

func parseOCIKMSDefaultProfile(data []byte) (map[string]string, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("config is empty")
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	inDefaultProfile := false
	sawDefaultProfile := false
	fields := map[string]string{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			profile := strings.TrimSpace(line[1 : len(line)-1])
			inDefaultProfile = strings.EqualFold(profile, "DEFAULT")
			if inDefaultProfile {
				sawDefaultProfile = true
			}
			continue
		}

		if !inDefaultProfile {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid line %q in profile [DEFAULT]", line)
		}
		fields[strings.TrimSpace(strings.ToLower(key))] = strings.TrimSpace(value)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !sawDefaultProfile {
		return nil, fmt.Errorf("missing profile [DEFAULT]")
	}

	return fields, nil
}

func (m *Manager) credentialsSecret(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, provider string) (*corev1.Secret, error) {
	if cluster == nil || cluster.Spec.Unseal == nil || cluster.Spec.Unseal.CredentialsSecretRef == nil {
		return nil, fmt.Errorf("%s unseal requires spec.unseal.credentialsSecretRef for Secret-backed credentials", provider)
	}

	var secret corev1.Secret
	secretName := types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Spec.Unseal.CredentialsSecretRef.Name}
	if err := m.reader.Get(ctx, secretName, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%s credentials Secret %s/%s not found", provider, cluster.Namespace, cluster.Spec.Unseal.CredentialsSecretRef.Name)
		}
		return nil, fmt.Errorf("failed to read %s credentials Secret %s/%s: %w", provider, cluster.Namespace, cluster.Spec.Unseal.CredentialsSecretRef.Name, err)
	}

	return &secret, nil
}

func validateTransitCAPEM(data []byte) error {
	if len(strings.TrimSpace(string(data))) == 0 {
		return fmt.Errorf("PEM bundle is empty")
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return fmt.Errorf("expected one or more PEM certificates")
	}
	return nil
}

func validateTransitClientKeyPairPEM(certPEM, keyPEM []byte) error {
	if err := validateTransitCAPEM(certPEM); err != nil {
		return fmt.Errorf("client certificate is invalid: %w", err)
	}
	if len(strings.TrimSpace(string(keyPEM))) == 0 {
		return fmt.Errorf("client private key is empty")
	}
	if block, _ := pem.Decode(keyPEM); block == nil {
		return fmt.Errorf("client private key is not valid PEM")
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return fmt.Errorf("client certificate and private key do not form a valid key pair: %w", err)
	}
	return nil
}

func transitRequiredSecretKeys(cluster *openbaov1alpha1.OpenBaoCluster) []string {
	if cluster == nil || cluster.Spec.Unseal == nil || cluster.Spec.Unseal.Transit == nil {
		return nil
	}

	required := make([]string, 0, 4)
	if strings.TrimSpace(cluster.Spec.Unseal.Transit.Token) == "" {
		required = append(required, "token")
	}

	for _, filePath := range []string{
		cluster.Spec.Unseal.Transit.TLSCACert,
		cluster.Spec.Unseal.Transit.TLSClientCert,
		cluster.Spec.Unseal.Transit.TLSClientKey,
	} {
		key, ok := mountedSealCredentialsKey(filePath)
		if !ok {
			continue
		}
		required = append(required, key)
	}

	slices.Sort(required)
	return slices.Compact(required)
}
