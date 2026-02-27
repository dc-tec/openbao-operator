package deletionops

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/logging"
)

// OrphanRetentionSecrets removes owner references from secrets required for
// recoverability when retention policy is used.
func OrphanRetentionSecrets(
	ctx context.Context,
	logger logr.Logger,
	kubeClient client.Client,
	cluster *openbaov1alpha1.OpenBaoCluster,
	retentionSecretNames []string,
) error {
	if len(retentionSecretNames) == 0 {
		return fmt.Errorf("retention secret names are required")
	}

	for _, secretName := range retentionSecretNames {
		secret := &corev1.Secret{}
		key := types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      secretName,
		}

		if err := kubeClient.Get(ctx, key, secret); err != nil {
			if client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to get secret %s: %w", secretName, err)
			}
			logger.V(1).Info("Retention secret not found, skipping orphan", "secret", secretName)
			continue
		}

		if !HasOwnerReference(secret, cluster.UID) {
			logger.V(1).Info("Retention secret already orphaned", "secret", secretName)
			continue
		}

		if err := RemoveOwnerReferences(ctx, logger, kubeClient, secret); err != nil {
			return fmt.Errorf("failed to orphan secret %s: %w", secretName, err)
		}

		logger.Info("Orphaned retention secret to preserve data recoverability",
			"secret", secretName,
			"cluster_namespace", cluster.Namespace,
			"cluster_name", cluster.Name)
		logging.LogAuditEvent(logger, logging.EventRetentionSecretOrphaned, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"secret_name":       secretName,
			"deletion_policy":   string(cluster.Spec.DeletionPolicy),
		})
	}

	return nil
}

// HasOwnerReference checks whether object has an owner reference with the provided UID.
func HasOwnerReference(obj metav1.Object, uid types.UID) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == uid {
			return true
		}
	}
	return false
}

// RemoveOwnerReferences patches the secret to remove all owner references.
func RemoveOwnerReferences(ctx context.Context, logger logr.Logger, kubeClient client.Client, secret *corev1.Secret) error {
	patch := []map[string]interface{}{
		{
			"op":   "remove",
			"path": "/metadata/ownerReferences",
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	if err := kubeClient.Patch(ctx, secret, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
		return fmt.Errorf("failed to patch secret: %w", err)
	}

	logger.V(1).Info("Removed ownerReferences from secret", "secret", secret.Name)
	return nil
}
