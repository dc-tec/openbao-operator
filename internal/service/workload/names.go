package workload

import (
	"fmt"

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

func unsealSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixUnsealKey
}

func configMapName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixConfigMap
}

func configMapNameWithRevision(cluster *openbaov1alpha1.OpenBaoCluster, rev string) string {
	if rev == "" {
		return configMapName(cluster)
	}
	return fmt.Sprintf("%s%s-%s", cluster.Name, constants.SuffixConfigMap, rev)
}

func configInitMapName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + configInitMapSuffix
}

func tlsServerSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixTLSServer
}

func headlessServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name
}

func statefulSetNameWithRevision(cluster *openbaov1alpha1.OpenBaoCluster, rev string) string {
	if rev == "" {
		return cluster.Name
	}
	return fmt.Sprintf("%s-%s", cluster.Name, rev)
}

func serviceAccountName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Spec.ServiceAccount != nil && cluster.Spec.ServiceAccount.Name != "" {
		return cluster.Spec.ServiceAccount.Name
	}
	return cluster.Name + constants.SuffixServiceAccount
}

func usesStaticSeal(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster.Spec.Unseal == nil {
		return true
	}
	if cluster.Spec.Unseal.Type == "" {
		return true
	}
	return cluster.Spec.Unseal.Type == "static"
}

func int32Ptr(v int32) *int32 {
	return &v
}
