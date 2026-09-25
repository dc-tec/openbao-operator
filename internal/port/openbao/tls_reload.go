package openbao

import (
	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	platformsemver "github.com/dc-tec/openbao-operator/internal/platform/semver"
)

// NativeTLSAutoReloadInterval retains the wrapper's 10-second polling interval.
const NativeTLSAutoReloadInterval = "10s"

// UsesNativeTLSAutoReload reports whether OpenBao can reload mounted TLS files itself.
// OpenBao 2.6 ignores the listener setting, so older versions retain the wrapper watcher.
func UsesNativeTLSAutoReload(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || !cluster.Spec.TLS.Enabled || cluster.Spec.TLS.Mode == openbaov1alpha1.TLSModeACME {
		return false
	}

	supported, err := platformsemver.AtLeast(cluster.Spec.Version, 2, 7, 0)
	return err == nil && supported
}
