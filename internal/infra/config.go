package infra

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	configbuilder "github.com/openbao/operator/internal/config"
)

// ensureUnsealSecret manages the static auto-unseal Secret for the OpenBaoCluster.
func (m *Manager) ensureUnsealSecret(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	secretName := unsealSecretName(cluster)

	secret := &corev1.Secret{}
	getErr := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      secretName,
	}, secret)
	if getErr != nil {
		if !apierrors.IsNotFound(getErr) {
			return fmt.Errorf("failed to get unseal Secret %s/%s: %w", cluster.Namespace, secretName, getErr)
		}

		logger.Info("Unseal Secret not found; generating new static auto-unseal key", "secret", secretName)

		key, genErr := generateUnsealKey()
		if genErr != nil {
			return fmt.Errorf("failed to generate unseal key for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, genErr)
		}

		secret = &corev1.Secret{
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

		if err := m.client.Create(ctx, secret); err != nil {
			return fmt.Errorf("failed to create unseal Secret %s/%s: %w", cluster.Namespace, secretName, err)
		}

		return nil
	}

	// Existing Secret Logic: STRICT Validation Only
	// Since this is a new operator, any Unseal Secret that isn't exactly 32 raw bytes
	// is effectively corrupted or user error. We do not attempt migration or auto-rotation.
	existingKey, ok := secret.Data[unsealSecretKey]

	// If key is missing or wrong length, fail hard. Do NOT auto-rotate or migrate.
	if !ok || len(existingKey) != unsealKeyBytes {
		return fmt.Errorf("unseal Secret %s/%s is invalid: expected exactly 32 raw bytes (got %d); manual intervention required", cluster.Namespace, secretName, len(existingKey))
	}

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

// ensureConfigMap manages the config.hcl ConfigMap for the OpenBaoCluster.
func (m *Manager) ensureConfigMap(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, configContent string) error {
	cmName := configMapName(cluster)

	configMap := &corev1.ConfigMap{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cmName,
	}, configMap)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get config ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
		}

		logger.Info("ConfigMap not found; creating new config.hcl ConfigMap", "configmap", cmName)

		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmName,
				Namespace: cluster.Namespace,
				Labels:    infraLabels(cluster),
			},
			Data: map[string]string{
				configFileName: configContent,
			},
		}

		// Set OwnerReference for garbage collection when the OpenBaoCluster is deleted.
		if err := controllerutil.SetControllerReference(cluster, configMap, m.scheme); err != nil {
			return fmt.Errorf("failed to set owner reference on config ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
		}

		if err := m.client.Create(ctx, configMap); err != nil {
			return fmt.Errorf("failed to create config ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
		}

		return nil
	}

	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	existing := configMap.Data[configFileName]
	if existing == configContent {
		return nil
	}

	logger.Info("Updating existing config ConfigMap", "configmap", cmName)
	configMap.Data[configFileName] = configContent

	if configMap.Labels == nil {
		configMap.Labels = infraLabels(cluster)
	} else {
		for k, v := range infraLabels(cluster) {
			configMap.Labels[k] = v
		}
	}

	if err := m.client.Update(ctx, configMap); err != nil {
		return fmt.Errorf("failed to update config ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
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

	// Render self-init stanzas
	initConfig, err := configbuilder.RenderSelfInitHCL(cluster)
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

	configMap := &corev1.ConfigMap{}
	err = m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cmName,
	}, configMap)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get self-init ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
		}

		logger.Info("Self-init ConfigMap not found; creating", "configmap", cmName)

		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmName,
				Namespace: cluster.Namespace,
				Labels:    infraLabels(cluster),
			},
			Data: map[string]string{
				configFileName: initConfigContent,
			},
		}

		// Set OwnerReference for garbage collection when the OpenBaoCluster is deleted.
		if err := controllerutil.SetControllerReference(cluster, configMap, m.scheme); err != nil {
			return fmt.Errorf("failed to set owner reference on self-init ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
		}

		if err := m.client.Create(ctx, configMap); err != nil {
			return fmt.Errorf("failed to create self-init ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
		}

		return nil
	}

	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	existing := configMap.Data[configFileName]
	if existing == initConfigContent {
		return nil
	}

	logger.Info("Updating self-init ConfigMap", "configmap", cmName)
	configMap.Data[configFileName] = initConfigContent

	if configMap.Labels == nil {
		configMap.Labels = infraLabels(cluster)
	} else {
		for k, v := range infraLabels(cluster) {
			configMap.Labels[k] = v
		}
	}

	if err := m.client.Update(ctx, configMap); err != nil {
		return fmt.Errorf("failed to update self-init ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
	}

	return nil
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
	secretNames := []string{
		unsealSecretName(cluster),
		tlsServerSecretName(cluster),
		tlsCASecretName(cluster),
	}

	for _, name := range secretNames {
		secret := &corev1.Secret{}
		err := m.client.Get(ctx, types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      name,
		}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}

		if err := m.client.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
