package workload

import (
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func usesDeclarativeOCIPluginDownload(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Spec.Configuration == nil || cluster.Spec.Configuration.Plugin == nil {
		return false
	}
	autoDownload := cluster.Spec.Configuration.Plugin.AutoDownload
	if autoDownload == nil || !*autoDownload {
		return false
	}
	for _, plugin := range cluster.Spec.Plugins {
		if strings.TrimSpace(plugin.Image) != "" {
			return true
		}
	}
	return false
}
