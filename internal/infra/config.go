package infra

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/auth"
	configbuilder "github.com/dc-tec/openbao-operator/internal/config"
)

// usesStaticSeal returns true if the cluster is configured to use the static seal
// (either by default when unseal config is nil, or explicitly when type is "static").
func usesStaticSeal(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster.Spec.Unseal == nil {
		return true
	}
	if cluster.Spec.Unseal.Type == "" {
		return true
	}
	return cluster.Spec.Unseal.Type == "static"
}

// ensureUnsealSecret manages the static auto-unseal Secret for the OpenBaoCluster.
// This function implements a "blind create" pattern: it generates the key in memory
// and attempts to create the Secret, ignoring AlreadyExists errors. This ensures
// the operator never needs GET permission on the unseal key Secret after creation,
// improving security by preventing the operator from reading root keys.
func (m *Manager) ensureUnsealSecret(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	secretName := unsealSecretName(cluster)

	// Generate key in memory
	key, genErr := generateUnsealKey()
	if genErr != nil {
		return fmt.Errorf("failed to generate unseal key for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, genErr)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Type:      corev1.SecretTypeOpaque,
		Immutable: ptr.To(true), // Secure by default: prevent accidental overwrites
		Data: map[string][]byte{
			unsealSecretKey: key,
		},
	}

	// Set OwnerReference for garbage collection when the OpenBaoCluster is deleted.
	if err := controllerutil.SetControllerReference(cluster, secret, m.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference on unseal Secret %s/%s: %w", cluster.Namespace, secretName, err)
	}

	// Attempt CREATE - ignore AlreadyExists errors (blind create pattern)
	if err := m.client.Create(ctx, secret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Secret already exists - this is fine, we don't need to read it
			logger.Info("Unseal Secret already exists; skipping creation", "secret", secretName)
			return nil
		}
		return fmt.Errorf("failed to create unseal Secret %s/%s: %w", cluster.Namespace, secretName, err)
	}

	logger.Info("Created unseal Secret", "secret", secretName)
	return nil
}

// generateUnsealKey generates a 32-byte random key for the static unseal Secret.
func generateUnsealKey() ([]byte, error) {
	// Generate 32 random bytes for the static unseal key.
	// The raw bytes are written directly to the Secret so that the mounted
	// file contains a 32-byte key compatible with OpenBao's static seal.
	raw := make([]byte, unsealKeyBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("failed to read random bytes: %w", err)
	}

	return raw, nil
}

// ensureConfigMap manages the config.hcl ConfigMap for the OpenBaoCluster using Server-Side Apply.
func (m *Manager) ensureConfigMap(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, configContent string) error {
	return m.ensureConfigMapWithName(ctx, cluster, configMapName(cluster), configContent)
}

// ensureConfigMapWithRevision manages a revision-scoped config.hcl ConfigMap for a specific
// StatefulSet revision. This is used for blue/green upgrades where Green must use a
// different retry_join selector than Blue.
func (m *Manager) ensureConfigMapWithRevision(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, revision string, configContent string) error {
	return m.ensureConfigMapWithName(ctx, cluster, configMapNameWithRevision(cluster, revision), configContent)
}

func (m *Manager) ensureConfigMapWithName(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, cmName string, configContent string) error {
	if strings.TrimSpace(cmName) == "" {
		return fmt.Errorf("config ConfigMap name is required")
	}

	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Data: map[string]string{
			configFileName: configContent,
		},
	}

	if err := m.applyResource(ctx, configMap, cluster); err != nil {
		return fmt.Errorf("failed to ensure config ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
	}

	return nil
}

// ensureSelfInitConfigMap creates or updates a separate ConfigMap containing only
// self-initialization stanzas. This ConfigMap is only mounted for pod-0, since
// only the first pod needs to execute initialization requests.
func (m *Manager) ensureSelfInitConfigMap(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	cmName := configInitMapName(cluster)

	// If self-init is not enabled, delete the ConfigMap if it exists
	if cluster.Spec.SelfInit == nil || !cluster.Spec.SelfInit.Enabled {
		configMap := &corev1.ConfigMap{}
		err := m.client.Get(ctx, types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      cmName,
		}, configMap)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil // Already deleted, nothing to do
			}
			return fmt.Errorf("failed to get self-init ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
		}

		logger.Info("Self-init disabled; deleting self-init ConfigMap", "configmap", cmName)
		if err := m.client.Delete(ctx, configMap); err != nil {
			return fmt.Errorf("failed to delete self-init ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
		}
		return nil
	}

	// TIGHTENED: No discovery logic here. Use the struct fields.
	var bootstrapConfig *configbuilder.OperatorBootstrapConfig
	if shouldBootstrapJWTAuth(cluster) {
		if m.oidcIssuer == "" {
			return fmt.Errorf("cannot configure OpenBao JWT auth: OIDC issuer could not be determined at operator startup")
		}
		if len(m.oidcJWTKeys) == 0 {
			return fmt.Errorf("cannot configure OpenBao JWT auth: OIDC JWKS public keys could not be determined at operator startup")
		}

		operatorNS := m.operatorNamespace
		if operatorNS == "" {
			operatorNS = "openbao-operator-system"
		}
		operatorSA := os.Getenv("OPERATOR_SERVICE_ACCOUNT_NAME")
		if operatorSA == "" {
			operatorSA = "openbao-operator-controller"
		}

		bootstrapConfig = &configbuilder.OperatorBootstrapConfig{
			OIDCIssuerURL:   m.oidcIssuer,
			JWTKeysPEM:      m.oidcJWTKeys,
			OperatorNS:      operatorNS,
			OperatorSA:      operatorSA,
			JWTAuthAudience: auth.OpenBaoJWTAudience(),
		}
	}

	// Render self-init stanzas
	initConfig, err := configbuilder.RenderSelfInitHCL(cluster, bootstrapConfig)
	if err != nil {
		return fmt.Errorf("failed to render self-init config.hcl for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	initConfigContent := string(initConfig)
	if len(initConfigContent) == 0 {
		// No self-init requests, delete the ConfigMap if it exists
		configMap := &corev1.ConfigMap{}
		err := m.client.Get(ctx, types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      cmName,
		}, configMap)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil // Already deleted, nothing to do
			}
			return fmt.Errorf("failed to get self-init ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
		}

		logger.Info("No self-init requests; deleting self-init ConfigMap", "configmap", cmName)
		if err := m.client.Delete(ctx, configMap); err != nil {
			return fmt.Errorf("failed to delete self-init ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
		}
		return nil
	}

	// Use SSA to create or update the ConfigMap
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Data: map[string]string{
			configFileName: initConfigContent,
		},
	}

	if err := m.applyResource(ctx, configMap, cluster); err != nil {
		return fmt.Errorf("failed to ensure self-init ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
	}

	return nil
}

func shouldBootstrapJWTAuth(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil {
		return false
	}

	// Bootstrap is opt-in via spec.selfInit.bootstrapJWTAuth
	// Users must explicitly enable bootstrap to have the operator automatically
	// configure JWT auth, OIDC, and operator/backup/upgrade policies and roles.
	if cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.BootstrapJWTAuth {
		return true
	}

	return false
}

// deleteConfigMap removes the config ConfigMap for the OpenBaoCluster.
func (m *Manager) deleteConfigMap(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	configMap := &corev1.ConfigMap{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      configMapName(cluster),
	}, configMap)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if err := m.client.Delete(ctx, configMap); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

// deleteSecrets removes all Secrets associated with the OpenBaoCluster.
func (m *Manager) deleteSecrets(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil {
		return nil
	}

	secretNames := []string{}

	// Only delete operator-owned Secrets. For External/ACME TLS, Secrets are user-managed (or not used),
	// and must not be deleted by the operator.
	{
		mode := cluster.Spec.TLS.Mode
		if mode == "" {
			mode = openbaov1alpha1.TLSModeOperatorManaged
		}

		if cluster.Spec.TLS.Enabled && mode == openbaov1alpha1.TLSModeOperatorManaged {
			secretNames = append(secretNames, tlsServerSecretName(cluster), tlsCASecretName(cluster))
		}
	}

	// Only delete the unseal key Secret when using static unseal (operator-owned).
	{
		staticUnseal := cluster.Spec.Unseal == nil || cluster.Spec.Unseal.Type == "" || cluster.Spec.Unseal.Type == "static"
		if staticUnseal {
			secretNames = append(secretNames, unsealSecretName(cluster))
		}
	}

	// Delete by name without reading Secret contents (avoids requiring Secret "get" permission).
	for _, name := range secretNames {
		if name == "" {
			continue
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: cluster.Namespace,
			},
		}
		if err := m.client.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
