package openbaocluster

import (
	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
)

type statusConditionResult = appopenbaocluster.StatusConditionResult

func setACMEIntegrationReadyEvaluatedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	result appopenbaocluster.ACMEIntegrationResult,
) {
	appopenbaocluster.ApplyACMEIntegrationReadyCondition(cluster, result)
}

func setGatewayIntegrationReadyEvaluatedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	result appopenbaocluster.GatewayIntegrationResult,
) {
	appopenbaocluster.ApplyGatewayIntegrationReadyCondition(cluster, result)
}

func setIngressIntegrationReadyEvaluatedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	result appopenbaocluster.IngressIntegrationResult,
) {
	appopenbaocluster.ApplyIngressIntegrationReadyCondition(cluster, result)
}

func setAPIServerNetworkReadyEvaluatedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	result appopenbaocluster.APIServerNetworkResult,
) {
	appopenbaocluster.ApplyAPIServerNetworkReadyCondition(cluster, result)
}

func setTLSReadyEvaluatedCondition(cluster *openbaov1alpha1.OpenBaoCluster, result statusConditionResult) {
	appopenbaocluster.ApplyTLSReadyCondition(cluster, result)
}

func setACMECacheReadyEvaluatedCondition(cluster *openbaov1alpha1.OpenBaoCluster, result statusConditionResult) {
	appopenbaocluster.ApplyACMECacheReadyCondition(cluster, result)
}

func setAuditFileStorageReadyEvaluatedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	result statusConditionResult,
) {
	appopenbaocluster.ApplyAuditFileStorageReadyCondition(cluster, result)
}

func setBackupConfigurationReadyEvaluatedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	result appopenbaocluster.BackupConfigurationResult,
) {
	appopenbaocluster.ApplyBackupConfigurationReadyCondition(cluster, result)
}

func setCloudUnsealIdentityReadyEvaluatedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	result statusConditionResult,
) {
	appopenbaocluster.ApplyCloudUnsealIdentityReadyCondition(cluster, result)
}
