// Package kube provides Kubernetes-specific utilities and helpers.
package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/internal/storage"
)

// LoadStorageCredentials loads storage credentials from a Kubernetes Secret.
// If secretRef is nil, returns nil (indicating default credential chain should be used).
// The namespace parameter specifies the namespace where the Secret must exist.
// Cross-namespace references are not allowed for security reasons.
func LoadStorageCredentials(ctx context.Context, c client.Client, secretRef *corev1.LocalObjectReference, namespace string) (*storage.Credentials, error) {
	if secretRef == nil {
		return nil, nil
	}

	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      secretRef.Name,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials Secret %s/%s: %w", namespace, secretRef.Name, err)
	}

	creds := &storage.Credentials{}

	// Load required credentials (if present - they might use workload identity)
	if v, ok := secret.Data[storage.SecretKeyAccessKeyID]; ok {
		creds.AccessKeyID = string(v)
	}
	if v, ok := secret.Data[storage.SecretKeySecretAccessKey]; ok {
		creds.SecretAccessKey = string(v)
	}

	// Validate that if one key is provided, both must be provided
	if (creds.AccessKeyID != "" && creds.SecretAccessKey == "") ||
		(creds.AccessKeyID == "" && creds.SecretAccessKey != "") {
		return nil, fmt.Errorf("credentials Secret %s/%s must contain both %s and %s, or neither",
			namespace, secretRef.Name, storage.SecretKeyAccessKeyID, storage.SecretKeySecretAccessKey)
	}

	// Load optional fields
	if v, ok := secret.Data[storage.SecretKeySessionToken]; ok {
		creds.SessionToken = string(v)
	}
	if v, ok := secret.Data[storage.SecretKeyRegion]; ok {
		creds.Region = string(v)
	}
	if v, ok := secret.Data[storage.SecretKeyCACert]; ok {
		creds.CACert = v
	}

	return creds, nil
}
