package workload

import (
	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const metricsScrapeProfileAllNodes = "AllNodes"

func metricsOnlyListenerEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil ||
		cluster.Spec.Observability == nil ||
		cluster.Spec.Observability.Metrics == nil ||
		!cluster.Spec.Observability.Metrics.Enabled {
		return false
	}
	listener := cluster.Spec.Observability.Metrics.MetricsOnlyListener
	if listener != nil && listener.Enabled != nil {
		return *listener.Enabled
	}
	return cluster.Spec.Observability.Metrics.ScrapeProfile == metricsScrapeProfileAllNodes
}

func metricsOnlyListenerPort(cluster *openbaov1alpha1.OpenBaoCluster) int32 {
	if cluster != nil &&
		cluster.Spec.Observability != nil &&
		cluster.Spec.Observability.Metrics != nil &&
		cluster.Spec.Observability.Metrics.MetricsOnlyListener != nil {
		port := cluster.Spec.Observability.Metrics.MetricsOnlyListener.Port
		if port > 0 {
			return port
		}
	}
	return constants.PortMetrics
}
