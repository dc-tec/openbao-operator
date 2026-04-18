package networking

import (
	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func infraLabels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	return map[string]string{
		constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:    cluster.Name,
		constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster: cluster.Name,
	}
}

func podSelectorLabels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	return podSelectorLabelsWithRevision(cluster, "")
}

func podSelectorLabelsWithRevision(cluster *openbaov1alpha1.OpenBaoCluster, rev string) map[string]string {
	labels := infraLabels(cluster)
	if rev != "" {
		labels[constants.LabelOpenBaoRevision] = rev
	}
	return labels
}

func headlessServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name
}

func tlsServerSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixTLSServer
}

func externalServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + publicServiceSuffix
}

func acmeServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + acmeServiceSuffix
}

func externalServiceNameBlue(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return externalServiceName(cluster) + "-blue"
}

func externalServiceNameGreen(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return externalServiceName(cluster) + "-green"
}

func usesACMEMode(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster.Spec.TLS.Enabled && cluster.Spec.TLS.Mode == openbaov1alpha1.TLSModeACME && cluster.Spec.TLS.ACME != nil
}
