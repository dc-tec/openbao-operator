package openbaocluster

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
	"github.com/dc-tec/openbao-operator/internal/constants"
	controllerutil "github.com/dc-tec/openbao-operator/internal/controller"
	inframanager "github.com/dc-tec/openbao-operator/internal/infra"
)

func (r *OpenBaoClusterReconciler) handleDeletion(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	policy := cluster.Spec.DeletionPolicy
	if policy == "" {
		policy = openbaov1alpha1.DeletionPolicyRetain
	}

	logger.Info("Applying DeletionPolicy for OpenBaoCluster", "deletionPolicy", string(policy))

	// CRITICAL: When DeletionPolicy is Retain, we must orphan critical secrets BEFORE
	// the finalizer is removed. Otherwise, Kubernetes GC will delete them via OwnerReferences
	// even though they contain data needed to decrypt PVCs.
	if policy == openbaov1alpha1.DeletionPolicyRetain {
		if err := r.orphanRetentionSecrets(ctx, logger, cluster); err != nil {
			return fmt.Errorf("failed to orphan retention secrets: %w", err)
		}
	}

	// Clear per-cluster metrics to avoid leaving stale series after deletion.
	clusterMetrics := controllerutil.NewClusterMetrics(cluster.Namespace, cluster.Name)
	clusterMetrics.Clear()

	infra := inframanager.NewManagerWithReader(r.Client, r.APIReader, r.Scheme, r.OperatorNamespace, r.OIDCIssuer, r.OIDCJWTKeys)
	if err := infra.Cleanup(ctx, logger, cluster, policy); err != nil {
		return err
	}

	// Backup deletion for DeletionPolicyDeleteAll will be implemented alongside the BackupManager
	// data path. For now, backups are left untouched even when DeleteAll is specified.

	return nil
}

// orphanRetentionSecrets removes OwnerReferences from secrets that must be retained
// to preserve encrypted data recoverability. Without the unseal key, PVC data is unreadable.
//
// Secrets orphaned:
//   - <cluster>-unseal-key: Required to decrypt storage data
//   - <cluster>-root-token: Required for initial access after restore
func (r *OpenBaoClusterReconciler) orphanRetentionSecrets(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	retentionSecrets := []string{
		cluster.Name + constants.SuffixUnsealKey,
		cluster.Name + constants.SuffixRootToken,
	}

	for _, secretName := range retentionSecrets {
		secret := &corev1.Secret{}
		key := types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      secretName,
		}

		if err := r.Get(ctx, key, secret); err != nil {
			if client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to get secret %s: %w", secretName, err)
			}
			// Secret doesn't exist, nothing to orphan
			logger.V(1).Info("Retention secret not found, skipping orphan", "secret", secretName)
			continue
		}

		// Check if secret has OwnerReferences pointing to this cluster
		if !hasOwnerReference(secret, cluster.UID) {
			logger.V(1).Info("Retention secret already orphaned", "secret", secretName)
			continue
		}

		// Remove all OwnerReferences to prevent GC from deleting this secret
		if err := r.removeOwnerReferences(ctx, logger, secret); err != nil {
			return fmt.Errorf("failed to orphan secret %s: %w", secretName, err)
		}

		logger.Info("Orphaned retention secret to preserve data recoverability",
			"secret", secretName,
			"cluster_namespace", cluster.Namespace,
			"cluster_name", cluster.Name)
	}

	return nil
}

// hasOwnerReference checks if the object has an OwnerReference with the given UID.
func hasOwnerReference(obj metav1.Object, uid types.UID) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == uid {
			return true
		}
	}
	return false
}

// removeOwnerReferences patches the object to remove all OwnerReferences.
// This "orphans" the object so Kubernetes GC won't delete it when the parent is removed.
func (r *OpenBaoClusterReconciler) removeOwnerReferences(ctx context.Context, logger logr.Logger, secret *corev1.Secret) error {
	// Use a JSON Patch to remove ownerReferences field entirely
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

	if err := r.Patch(ctx, secret, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
		return fmt.Errorf("failed to patch secret: %w", err)
	}

	logger.V(1).Info("Removed ownerReferences from secret", "secret", secret.Name)
	return nil
}
