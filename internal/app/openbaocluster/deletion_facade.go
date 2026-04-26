package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/deletionops"
)

// DeletionDependencies defines dependencies for OpenBaoCluster deletion orchestration.
type DeletionDependencies = deletionops.Dependencies

// HandleDeletion applies deletion policy side effects.
func HandleDeletion(ctx context.Context, logger logr.Logger, deps DeletionDependencies, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if len(deps.RetentionSecrets) == 0 {
		deps.RetentionSecrets = deletionops.DefaultRetentionSecrets(cluster)
	}
	return deletionops.Handle(ctx, logger, deps, cluster)
}

// OrphanRetentionSecrets removes owner references from retention secrets.
func OrphanRetentionSecrets(ctx context.Context, logger logr.Logger, kubeClient client.Client, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return deletionops.OrphanRetentionSecrets(ctx, logger, kubeClient, cluster, deletionops.DefaultRetentionSecrets(cluster))
}

// HasOwnerReference reports whether object has an owner reference with uid.
func HasOwnerReference(obj metav1.Object, uid types.UID) bool {
	return deletionops.HasOwnerReference(obj, uid)
}

// RemoveOwnerReferences removes all owner references from a Secret.
func RemoveOwnerReferences(ctx context.Context, logger logr.Logger, kubeClient client.Client, secret *corev1.Secret) error {
	return deletionops.RemoveOwnerReferences(ctx, logger, kubeClient, secret)
}
