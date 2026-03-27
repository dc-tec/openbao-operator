package openbaocluster

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

type statusConditionResult struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

type conditionContractEntry struct {
	reason string
	status metav1.ConditionStatus
}

type conditionContract map[string]metav1.ConditionStatus

func newConditionContract(entries ...conditionContractEntry) conditionContract {
	contract := make(conditionContract, len(entries))
	for _, entry := range entries {
		contract[entry.reason] = entry.status
	}
	return contract
}

func (c conditionContract) allows(result statusConditionResult) bool {
	wantStatus, ok := c[result.Reason]
	return ok && wantStatus == result.Status
}

var acmeIntegrationReadyConditionContract = newConditionContract(
	conditionContractEntry{reason: ReasonACMEIntegrationReady, status: metav1.ConditionTrue},
	conditionContractEntry{reason: ReasonGatewayAPIMissing, status: metav1.ConditionFalse},
	conditionContractEntry{reason: ReasonACMEGatewayNotConfiguredForPassthrough, status: metav1.ConditionFalse},
	conditionContractEntry{reason: ReasonACMEDomainNotResolvable, status: metav1.ConditionFalse},
	conditionContractEntry{reason: ReasonPrerequisitesMissing, status: metav1.ConditionFalse},
	conditionContractEntry{reason: reasonUnknown, status: metav1.ConditionUnknown},
)

var gatewayIntegrationReadyConditionContract = newConditionContract(
	conditionContractEntry{reason: ReasonGatewayIntegrationReady, status: metav1.ConditionTrue},
	conditionContractEntry{reason: ReasonGatewayAPIMissing, status: metav1.ConditionFalse},
	conditionContractEntry{reason: ReasonGatewayReferenceMissing, status: metav1.ConditionFalse},
	conditionContractEntry{reason: ReasonGatewayClassMissing, status: metav1.ConditionFalse},
	conditionContractEntry{reason: ReasonGatewayListenerIncompatible, status: metav1.ConditionFalse},
	conditionContractEntry{reason: ReasonGatewayClassNotAccepted, status: metav1.ConditionFalse},
	conditionContractEntry{reason: ReasonGatewayVersionUnsupported, status: metav1.ConditionFalse},
	conditionContractEntry{reason: ReasonGatewayFeatureUnsupported, status: metav1.ConditionFalse},
	conditionContractEntry{reason: ReasonGatewayNotProgrammed, status: metav1.ConditionFalse},
	conditionContractEntry{reason: ReasonGatewayClassPending, status: metav1.ConditionUnknown},
	conditionContractEntry{reason: ReasonGatewayCapabilitiesUnknown, status: metav1.ConditionUnknown},
	conditionContractEntry{reason: ReasonGatewayProgrammingPending, status: metav1.ConditionUnknown},
	conditionContractEntry{reason: reasonUnknown, status: metav1.ConditionUnknown},
)

var apiServerNetworkReadyConditionContract = newConditionContract(
	conditionContractEntry{reason: ReasonAPIServerNetworkReady, status: metav1.ConditionTrue},
	conditionContractEntry{reason: ReasonAPIServerEndpointIPsRecommended, status: metav1.ConditionUnknown},
	conditionContractEntry{reason: ReasonAPIServerNetworkConfigurationInvalid, status: metav1.ConditionFalse},
	conditionContractEntry{reason: reasonUnknown, status: metav1.ConditionUnknown},
)

var backupConfigurationReadyConditionContract = newConditionContract(
	conditionContractEntry{reason: reasonReady, status: metav1.ConditionTrue},
	conditionContractEntry{reason: constants.ReasonAmbientIdentityAssumed, status: metav1.ConditionTrue},
	conditionContractEntry{reason: constants.ReasonWorkloadIdentityConfigured, status: metav1.ConditionTrue},
	conditionContractEntry{reason: constants.ReasonAuthenticationRequired, status: metav1.ConditionFalse},
	conditionContractEntry{reason: constants.ReasonTokenSecretMissing, status: metav1.ConditionFalse},
	conditionContractEntry{reason: constants.ReasonCredentialsSecretMissing, status: metav1.ConditionFalse},
	conditionContractEntry{reason: constants.ReasonNetworkEgressRulesRequired, status: metav1.ConditionFalse},
	conditionContractEntry{reason: reasonUnknown, status: metav1.ConditionUnknown},
)

var cloudUnsealIdentityReadyConditionContract = newConditionContract(
	conditionContractEntry{reason: reasonReady, status: metav1.ConditionTrue},
	conditionContractEntry{reason: constants.ReasonAmbientIdentityAssumed, status: metav1.ConditionTrue},
	conditionContractEntry{reason: constants.ReasonWorkloadIdentityConfigured, status: metav1.ConditionTrue},
	conditionContractEntry{reason: constants.ReasonCredentialsSecretMissing, status: metav1.ConditionFalse},
	conditionContractEntry{reason: constants.ReasonPrerequisitesMissing, status: metav1.ConditionFalse},
	conditionContractEntry{reason: reasonUnknown, status: metav1.ConditionUnknown},
)

func applyConditionContract(
	conditions *[]metav1.Condition,
	conditionType openbaov1alpha1.ConditionType,
	generation int64,
	result statusConditionResult,
	contract conditionContract,
) {
	normalized := normalizeConditionContractResult(conditionType, result, contract)
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               string(conditionType),
		Status:             normalized.Status,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             normalized.Reason,
		Message:            normalized.Message,
	})
}

func normalizeConditionContractResult(
	conditionType openbaov1alpha1.ConditionType,
	result statusConditionResult,
	contract conditionContract,
) statusConditionResult {
	normalized := statusConditionResult{
		Status:  result.Status,
		Reason:  strings.TrimSpace(result.Reason),
		Message: strings.TrimSpace(result.Message),
	}
	if contract.allows(normalized) {
		return normalized
	}

	message := fmt.Sprintf(
		"Controller rejected unexpected %s result: status=%s reason=%q",
		conditionType,
		normalized.Status,
		normalized.Reason,
	)
	if normalized.Message != "" {
		message += ": " + normalized.Message
	}

	return statusConditionResult{
		Status:  metav1.ConditionUnknown,
		Reason:  reasonUnknown,
		Message: message,
	}
}

func setACMEIntegrationReadyEvaluatedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	result appopenbaocluster.ACMEIntegrationResult,
) {
	applyConditionContract(
		&cluster.Status.Conditions,
		openbaov1alpha1.ConditionACMEIntegrationReady,
		cluster.Generation,
		statusConditionResult{Status: result.Status, Reason: result.Reason, Message: result.Message},
		acmeIntegrationReadyConditionContract,
	)
}

func setGatewayIntegrationReadyEvaluatedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	result appopenbaocluster.GatewayIntegrationResult,
) {
	applyConditionContract(
		&cluster.Status.Conditions,
		openbaov1alpha1.ConditionGatewayIntegrationReady,
		cluster.Generation,
		statusConditionResult{Status: result.Status, Reason: result.Reason, Message: result.Message},
		gatewayIntegrationReadyConditionContract,
	)
}

func setAPIServerNetworkReadyEvaluatedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	result appopenbaocluster.APIServerNetworkResult,
) {
	applyConditionContract(
		&cluster.Status.Conditions,
		openbaov1alpha1.ConditionAPIServerNetworkReady,
		cluster.Generation,
		statusConditionResult{Status: result.Status, Reason: result.Reason, Message: result.Message},
		apiServerNetworkReadyConditionContract,
	)
}

func setBackupConfigurationReadyEvaluatedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	result appopenbaocluster.BackupConfigurationResult,
) {
	applyConditionContract(
		&cluster.Status.Conditions,
		openbaov1alpha1.ConditionBackupConfigurationReady,
		cluster.Generation,
		statusConditionResult{Status: result.Status, Reason: result.Reason, Message: result.Message},
		backupConfigurationReadyConditionContract,
	)
}

func setCloudUnsealIdentityReadyEvaluatedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	result statusConditionResult,
) {
	applyConditionContract(
		&cluster.Status.Conditions,
		openbaov1alpha1.ConditionCloudUnsealIdentityReady,
		cluster.Generation,
		result,
		cloudUnsealIdentityReadyConditionContract,
	)
}
