package deletionops

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
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
		removed, found, err := RemoveClusterOwnerReference(ctx, logger, kubeClient, cluster, secretName)
		if err != nil {
			return fmt.Errorf("failed to orphan secret %s: %w", secretName, err)
		}
		if !found {
			logger.V(1).Info("Retention secret not found, skipping orphan", "secret", secretName)
			continue
		}
		if !removed {
			logger.V(1).Info("Retention secret already orphaned", "secret", secretName)
			continue
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

// HasClusterOwnerReference checks whether obj has the exact OpenBaoCluster owner reference.
func HasClusterOwnerReference(obj metav1.Object, cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if obj == nil || cluster == nil {
		return false
	}
	for _, ref := range obj.GetOwnerReferences() {
		if isClusterOwnerReference(ref, cluster) {
			return true
		}
	}
	return false
}

func isClusterOwnerReference(ref metav1.OwnerReference, cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return ref.APIVersion == openbaov1alpha1.GroupVersion.String() &&
		ref.Kind == "OpenBaoCluster" &&
		ref.Name == cluster.Name &&
		ref.UID == cluster.UID
}

// RemoveClusterOwnerReference removes only the matching OpenBaoCluster owner
// reference. It re-reads and retries on resource-version conflicts so concurrent
// metadata updates and unrelated owner references are preserved.
func RemoveClusterOwnerReference(
	ctx context.Context,
	logger logr.Logger,
	kubeClient client.Client,
	cluster *openbaov1alpha1.OpenBaoCluster,
	secretName string,
) (removed bool, found bool, err error) {
	if cluster == nil {
		return false, false, fmt.Errorf("cluster is required")
	}
	key := client.ObjectKey{Namespace: cluster.Namespace, Name: secretName}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		secret := &corev1.Secret{}
		if getErr := kubeClient.Get(ctx, key, secret); getErr != nil {
			if apierrors.IsNotFound(getErr) {
				found = false
				return nil
			}
			return getErr
		}
		found = true

		ownerReferences := secret.GetOwnerReferences()
		if !HasClusterOwnerReference(secret, cluster) {
			return nil
		}

		preserved := make([]metav1.OwnerReference, 0, len(ownerReferences)-1)
		for _, ref := range ownerReferences {
			if !isClusterOwnerReference(ref, cluster) {
				preserved = append(preserved, ref)
			}
		}
		secret.SetOwnerReferences(preserved)
		if updateErr := kubeClient.Update(ctx, secret); updateErr != nil {
			return updateErr
		}
		removed = true
		return nil
	})
	if err != nil {
		return false, found, fmt.Errorf("failed to update secret owner references: %w", err)
	}

	if removed {
		logger.V(1).Info("Removed OpenBaoCluster ownerReference from secret", "secret", secretName, "cluster", cluster.Name)
	}
	return removed, found, nil
}
