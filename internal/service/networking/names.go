package networking

import (
	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

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
