package openbaocluster

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

func setClusterConditionResult(
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
	result statusConditionResult,
) {
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(conditionType),
		Status:             result.Status,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             result.Reason,
		Message:            result.Message,
	})
}

func setPausedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
	status metav1.ConditionStatus,
	message string,
) {
	setClusterConditionResult(cluster, conditionType, statusConditionResult{
		Status:  status,
		Reason:  reasonPaused,
		Message: message,
	})
}

func setProfileNotSetCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
	status metav1.ConditionStatus,
	message string,
) {
	setClusterConditionResult(cluster, conditionType, statusConditionResult{
		Status:  status,
		Reason:  ReasonProfileNotSet,
		Message: message,
	})
}
