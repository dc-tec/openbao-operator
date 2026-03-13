package openbao

import (
	"fmt"
	"path"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const (
	// ACMESharedCacheMountPath is the shared filesystem mount used for ACME account and certificate state.
	ACMESharedCacheMountPath = "/bao/acme-cache"

	acmeCacheSubdir = "certmagic"
)

// UsesACMEMode reports whether the cluster is configured to use OpenBao's native ACME client.
func UsesACMEMode(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil && cluster.Spec.TLS.Enabled && cluster.Spec.TLS.Mode == openbaov1alpha1.TLSModeACME
}

// RequiresSharedACMECache reports whether more than one OpenBao Pod can serve the same ACME hostname.
func RequiresSharedACMECache(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if !UsesACMEMode(cluster) {
		return false
	}
	if cluster.Spec.Replicas > 1 {
		return true
	}
	return cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen
}

// HasACMESharedCache reports whether the cluster config includes a shared ACME cache.
func HasACMESharedCache(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return UsesACMEMode(cluster) &&
		cluster.Spec.TLS.ACME != nil &&
		cluster.Spec.TLS.ACME.SharedCache != nil
}

// UsesManagedACMESharedCache reports whether the operator should create a managed RWX PVC.
func UsesManagedACMESharedCache(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return HasACMESharedCache(cluster) &&
		cluster.Spec.TLS.ACME.SharedCache.Mode == openbaov1alpha1.ACMESharedCacheModeManagedPVC
}

// UsesExistingACMESharedCache reports whether the operator should mount a pre-existing RWX PVC.
func UsesExistingACMESharedCache(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return HasACMESharedCache(cluster) &&
		cluster.Spec.TLS.ACME.SharedCache.Mode == openbaov1alpha1.ACMESharedCacheModeExistingPVC
}

// ManagedACMESharedCachePVCName returns the operator-managed PVC name for the cluster.
func ManagedACMESharedCachePVCName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil {
		return ""
	}
	return fmt.Sprintf("%s-acme-cache", strings.TrimSpace(cluster.Name))
}

// ACMESharedCacheClaimName resolves the PVC claim name when shared cache is configured.
func ACMESharedCacheClaimName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if !HasACMESharedCache(cluster) {
		return ""
	}
	if UsesExistingACMESharedCache(cluster) {
		return strings.TrimSpace(cluster.Spec.TLS.ACME.SharedCache.ExistingClaimName)
	}
	return ManagedACMESharedCachePVCName(cluster)
}

// ACMESharedCachePath returns the effective cache path rendered into OpenBao config.
func ACMESharedCachePath(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if HasACMESharedCache(cluster) {
		return path.Join(ACMESharedCacheMountPath, acmeCacheSubdir)
	}
	return path.Join(constants.PathData, acmeCacheSubdir)
}
