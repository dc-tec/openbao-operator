package security

import (
	"context"
	"fmt"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/root"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const trustedRootConfigMapKey = "trusted_root.json"

// loadTrustedRoot loads the trusted root material for keyless verification.
func (v *ImageVerifier) loadTrustedRoot(ctx context.Context) (root.TrustedMaterial, error) {
	if v.trustedRootConfig != nil {
		configMapName := strings.TrimSpace(v.trustedRootConfig.ConfigMapName)
		configMapNamespace := strings.TrimSpace(v.trustedRootConfig.ConfigMapNamespace)
		if configMapName != "" || configMapNamespace != "" {
			return v.loadTrustedRootFromConfigMap(ctx, configMapNamespace, configMapName)
		}
	}

	v.logger.V(1).Info("Using embedded trusted_root.json")
	trustedRoot, err := root.NewTrustedRootFromJSON(embeddedTrustedRootJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded trusted_root.json: %w", err)
	}
	return trustedRoot, nil
}

func (v *ImageVerifier) loadTrustedRootFromConfigMap(ctx context.Context, namespace, name string) (root.TrustedMaterial, error) {
	if namespace == "" || name == "" {
		return nil, fmt.Errorf("trusted root ConfigMap requires both namespace and name")
	}
	if v.client == nil {
		return nil, fmt.Errorf("trusted root ConfigMap %s/%s is configured but Kubernetes client is not available", namespace, name)
	}

	configMap := &corev1.ConfigMap{}
	if err := v.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, configMap); err != nil {
		return nil, fmt.Errorf("failed to load trusted root ConfigMap %s/%s: %w", namespace, name, err)
	}

	trustedRootJSON, ok := configMap.Data[trustedRootConfigMapKey]
	if !ok || strings.TrimSpace(trustedRootJSON) == "" {
		return nil, fmt.Errorf("trusted root ConfigMap %s/%s is missing required key %q", namespace, name, trustedRootConfigMapKey)
	}

	v.logger.Info("Loading trusted root from ConfigMap", "configmap", name, "namespace", namespace)
	trustedRoot, err := root.NewTrustedRootFromJSON([]byte(trustedRootJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s from ConfigMap %s/%s: %w", trustedRootConfigMapKey, namespace, name, err)
	}
	return trustedRoot, nil
}
