package upgrade

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/openbaotls"
)

var ErrCACertMissing = errors.New("ca.crt missing from secret")

// ReadCACertSecret returns the CA certificate from the given Secret.
// It expects the key "ca.crt" to be present.
func ReadCACertSecret(ctx context.Context, c client.Client, secretRef types.NamespacedName) ([]byte, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, secretRef, secret); err != nil {
		// In multi-tenant mode, Secret access is restricted via allowlist Roles that
		// may be reconciled slightly after the OpenBaoCluster itself. Treat Forbidden
		// as transient to avoid long controller-runtime backoff loops.
		if apierrors.IsForbidden(err) {
			return nil, operatorerrors.WrapTransientKubernetesAPI(
				fmt.Errorf("forbidden to get CA Secret %s/%s: %w", secretRef.Namespace, secretRef.Name, err),
			)
		}
		return nil, err
	}
	caCert, ok := secret.Data["ca.crt"]
	if !ok {
		return nil, ErrCACertMissing
	}
	return caCert, nil
}

// LoadClusterCACert resolves the effective trust bundle for internal cluster
// clients. A nil CA bundle means the client should rely on system roots.
func LoadClusterCACert(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) ([]byte, error) {
	return openbaotls.LoadClusterTrustBundle(ctx, c, cluster)
}
