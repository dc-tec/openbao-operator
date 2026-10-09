package workload

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/kubebao/openbao-operator/api/v1alpha1"
	"github.com/kubebao/openbao-operator/internal/platform/resourceidentity"
	"github.com/kubebao/openbao-operator/internal/platform/resourceownership"
)

// staticUnsealSecretName returns the operator-generated static key Secret when
// it already belongs to the cluster. Earlier releases ignored
// spec.unseal.credentialsSecretRef for static seals; honoring it on such a
// cluster would mount a different key and leave OpenBao sealed. An empty result
// keeps the referenced Secret.
func (m *Manager) staticUnsealSecretName(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if !usesStaticSeal(cluster) || cluster.Spec.Unseal == nil || cluster.Spec.Unseal.CredentialsSecretRef == nil {
		return "", nil
	}

	name := resourceidentity.UnsealSecretName(cluster)
	metadata := &metav1.PartialObjectMetadata{}
	metadata.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	if err := m.reader.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: name}, metadata); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to check generated unseal Secret %s/%s: %w", cluster.Namespace, name, err)
	}
	if !resourceownership.HasOwnerProof(metadata, cluster) {
		return "", nil
	}

	logger.V(1).Info("Keeping the operator-generated static unseal key; spec.unseal.credentialsSecretRef is ignored", "secret", name)
	return name, nil
}

// overrideStaticUnsealSecret points the static unseal volume at secretName.
func overrideStaticUnsealSecret(volumes []corev1.Volume, secretName string) {
	if secretName == "" {
		return
	}
	for i := range volumes {
		if volumes[i].Name == unsealVolumeName && volumes[i].Secret != nil {
			volumes[i].Secret.SecretName = secretName
		}
	}
}
