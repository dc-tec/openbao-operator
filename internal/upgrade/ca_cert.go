package upgrade

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var ErrCACertMissing = errors.New("ca.crt missing from secret")

// ReadCACertSecret returns the CA certificate from the given Secret.
// It expects the key "ca.crt" to be present.
func ReadCACertSecret(ctx context.Context, c client.Client, secretRef types.NamespacedName) ([]byte, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, secretRef, secret); err != nil {
		return nil, err
	}
	caCert, ok := secret.Data["ca.crt"]
	if !ok {
		return nil, ErrCACertMissing
	}
	return caCert, nil
}
