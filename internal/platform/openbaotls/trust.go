package openbaotls

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// LoadClusterTrustBundle resolves the effective trust bundle for internal
// cluster clients. A nil CA bundle means the client should rely on system roots.
func LoadClusterTrustBundle(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) ([]byte, error) {
	source, err := portopenbao.ResolveClientTrustBundle(cluster)
	if err != nil {
		return nil, err
	}
	if source.UseSystemRoots {
		return nil, nil
	}

	secret := &corev1.Secret{}
	secretRef := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      source.SecretName,
	}
	if err := c.Get(ctx, secretRef, secret); err != nil {
		if apierrors.IsForbidden(err) {
			return nil, operatorerrors.WrapTransientKubernetesAPI(
				fmt.Errorf("forbidden to get trust bundle secret %s/%s: %w", secretRef.Namespace, secretRef.Name, err),
			)
		}
		return nil, err
	}

	caCert, ok := secret.Data[source.SecretKey]
	if !ok {
		return nil, fmt.Errorf("trust bundle key %q missing from secret %s/%s", source.SecretKey, secretRef.Namespace, secretRef.Name)
	}
	return caCert, nil
}
