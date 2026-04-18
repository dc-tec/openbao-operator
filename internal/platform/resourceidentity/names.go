package resourceidentity

import (
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const configInitMapSuffix = "-config-init"

func Labels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	return map[string]string{
		constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:    cluster.Name,
		constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster: cluster.Name,
	}
}

func PodSelectorLabels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	return PodSelectorLabelsWithRevision(cluster, "")
}

func PodSelectorLabelsWithRevision(cluster *openbaov1alpha1.OpenBaoCluster, revision string) map[string]string {
	labels := Labels(cluster)
	if revision != "" {
		labels[constants.LabelOpenBaoRevision] = revision
	}
	return labels
}

func UnsealSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixUnsealKey
}

func ConfigMapName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixConfigMap
}

func ConfigMapNameWithRevision(cluster *openbaov1alpha1.OpenBaoCluster, revision string) string {
	if revision == "" {
		return ConfigMapName(cluster)
	}
	return fmt.Sprintf("%s%s-%s", cluster.Name, constants.SuffixConfigMap, revision)
}

func ConfigInitMapName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + configInitMapSuffix
}

func TLSServerSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixTLSServer
}

func HeadlessServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name
}

func ServiceAccountName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Spec.ServiceAccount != nil && cluster.Spec.ServiceAccount.Name != "" {
		return cluster.Spec.ServiceAccount.Name
	}
	return cluster.Name + constants.SuffixServiceAccount
}
