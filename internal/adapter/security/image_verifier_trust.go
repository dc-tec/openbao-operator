package security

import (
	"context"
	"fmt"

	"github.com/sigstore/sigstore-go/pkg/root"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// loadTrustedRoot loads the trusted root material for keyless verification.
func (v *ImageVerifier) loadTrustedRoot(ctx context.Context) (root.TrustedMaterial, error) {
	if v.trustedRootConfig != nil && v.trustedRootConfig.ConfigMapName != "" && v.trustedRootConfig.ConfigMapNamespace != "" {
		if v.client != nil {
			configMap := &corev1.ConfigMap{}
			err := v.client.Get(ctx, types.NamespacedName{
				Namespace: v.trustedRootConfig.ConfigMapNamespace,
				Name:      v.trustedRootConfig.ConfigMapName,
			}, configMap)
			if err == nil {
				if trustedRootJSON, ok := configMap.Data["trusted_root.json"]; ok {
					v.logger.Info("Loading trusted root from ConfigMap",
						"configmap", v.trustedRootConfig.ConfigMapName,
						"namespace", v.trustedRootConfig.ConfigMapNamespace)
					trustedRoot, err := root.NewTrustedRootFromJSON([]byte(trustedRootJSON))
					if err != nil {
						return nil, fmt.Errorf("failed to parse trusted_root.json from ConfigMap %s/%s: %w",
							v.trustedRootConfig.ConfigMapNamespace, v.trustedRootConfig.ConfigMapName, err)
					}
					return trustedRoot, nil
				}
				v.logger.Info("ConfigMap found but missing 'trusted_root.json' key, falling back to embedded",
					"configmap", v.trustedRootConfig.ConfigMapName,
					"namespace", v.trustedRootConfig.ConfigMapNamespace)
			} else {
				v.logger.V(1).Info("Failed to load ConfigMap, falling back to embedded trusted root",
					"configmap", v.trustedRootConfig.ConfigMapName,
					"namespace", v.trustedRootConfig.ConfigMapNamespace,
					"error", err)
			}
		}
	}

	v.logger.V(1).Info("Using embedded trusted_root.json")
	trustedRoot, err := root.NewTrustedRootFromJSON(embeddedTrustedRootJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded trusted_root.json: %w", err)
	}
	return trustedRoot, nil
}
