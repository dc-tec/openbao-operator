package openbaoclusterclaim

import (
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/connectionpublishing"
)

func desiredStatusSummary(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	acceptance result,
	contract result,
	materialization result,
	ownership result,
	localCluster *openbaov1alpha1.OpenBaoCluster,
	publication connectionpublishing.PublicationResult,
	activeWorkflows activeClaimWorkflows,
) *openbaov1alpha1.OpenBaoClusterClaimStatusSummary {
	if claim == nil {
		return nil
	}

	if reason, message, sourceRef, ok := activeMaintenanceStatus(activeWorkflows); ok {
		severity := openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo
		if activeWorkflows.RestoreRequest != nil || activeWorkflows.RestoreExecution != nil {
			severity = openbaov1alpha1.OpenBaoClusterClaimStatusSeverityWarning
			message = activeRestoreSummaryMessage(activeWorkflows.RestoreRequest, activeWorkflows.RestoreExecution)
		}
		return newClaimStatusSummary(
			severity,
			reason,
			message,
			sourceRef,
		)
	}

	switch claim.Status.Phase {
	case openbaov1alpha1.OpenBaoClusterClaimPhaseReady:
		return nil
	case openbaov1alpha1.OpenBaoClusterClaimPhaseDeleting:
		return newClaimStatusSummary(
			openbaov1alpha1.OpenBaoClusterClaimStatusSeverityWarning,
			reasonDeleting,
			"Service instance is being retired.",
			materializationSourceRef(claim),
		)
	}

	if summary := summaryFromResult(
		acceptance,
		tenantSourceRef(claim),
		openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo,
	); summary != nil {
		return summary
	}
	if summary := summaryFromResult(
		contract,
		serviceContractSourceRef(claim),
		openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo,
	); summary != nil {
		return summary
	}
	if summary := summaryFromResult(
		materialization,
		materializationSourceRef(claim),
		openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo,
	); summary != nil {
		return summary
	}
	if summary := summaryFromResult(
		ownership,
		materializationSourceRef(claim),
		openbaov1alpha1.OpenBaoClusterClaimStatusSeverityError,
	); summary != nil {
		return summary
	}
	if summary := summaryFromPublication(claim, publication); summary != nil {
		return summary
	}
	if summary := summaryFromBackupRequest(activeWorkflows.BackupRequest, localCluster); summary != nil {
		return summary
	}

	if localCluster != nil {
		if localClusterBackupInProgress(localCluster) {
			return newClaimStatusSummary(
				openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo,
				string(openbaov1alpha1.ClusterPhaseBackingUp),
				"Service instance remains available while a backup operation is active.",
				localClusterSourceRef(localCluster),
			)
		}
		if reason, message, ok := localClusterBackupFailure(localCluster); ok {
			return newClaimStatusSummary(
				openbaov1alpha1.OpenBaoClusterClaimStatusSeverityWarning,
				reason,
				message,
				localClusterSourceRef(localCluster),
			)
		}
		switch localCluster.Status.Phase {
		case openbaov1alpha1.ClusterPhaseFailed:
			return newClaimStatusSummary(
				openbaov1alpha1.OpenBaoClusterClaimStatusSeverityError,
				string(openbaov1alpha1.ReasonInvalid),
				"Service instance is unavailable because the local OpenBaoCluster has failed.",
				localClusterSourceRef(localCluster),
			)
		case openbaov1alpha1.ClusterPhaseRunning:
			return nil
		default:
			return newClaimStatusSummary(
				openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo,
				string(openbaov1alpha1.ReasonPending),
				"Service instance provisioning is still in progress.",
				localClusterSourceRef(localCluster),
			)
		}
	}

	if claim.Status.Phase == openbaov1alpha1.OpenBaoClusterClaimPhaseFailed {
		return newClaimStatusSummary(
			openbaov1alpha1.OpenBaoClusterClaimStatusSeverityError,
			string(openbaov1alpha1.ReasonInvalid),
			"Service instance is unavailable and requires operator action.",
			materializationSourceRef(claim),
		)
	}
	if claim.Status.Phase == openbaov1alpha1.OpenBaoClusterClaimPhasePending || claim.Status.Phase == openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning {
		return newClaimStatusSummary(
			openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo,
			string(openbaov1alpha1.ReasonPending),
			"Service instance is waiting for same-cluster materialization.",
			materializationSourceRef(claim),
		)
	}

	return nil
}

func summaryFromBackupRequest(
	request *openbaov1alpha1.OpenBaoClusterClaimBackupRequest,
	localCluster *openbaov1alpha1.OpenBaoCluster,
) *openbaov1alpha1.OpenBaoClusterClaimStatusSummary {
	if request == nil || isTerminalClaimBackupRequestState(request.Status.State) {
		return nil
	}
	if localClusterBackupInProgress(localCluster) {
		return nil
	}

	state := request.Status.State
	if state == "" {
		state = openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatePending
	}
	message := "Manual backup request is queued for this service instance."
	if state == openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateRunning {
		message = "Manual backup request is active for this service instance."
	}
	return newClaimStatusSummary(
		openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo,
		string(state),
		message,
		backupRequestSourceRef(request),
	)
}

func activeRestoreSummaryMessage(
	request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest,
	restore *openbaov1alpha1.OpenBaoRestore,
) string {
	if restore != nil && strings.TrimSpace(restore.Status.Message) != "" {
		return restore.Status.Message
	}
	if request == nil {
		return "A restore workflow is active for this service instance."
	}
	switch request.Status.State {
	case openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning:
		return "Restore request is active for this service instance."
	default:
		return "Restore request is queued for this service instance."
	}
}

func summaryFromResult(
	current result,
	sourceRef *openbaov1alpha1.TypedObjectReference,
	pendingSeverity openbaov1alpha1.OpenBaoClusterClaimStatusSeverity,
) *openbaov1alpha1.OpenBaoClusterClaimStatusSummary {
	if current.Valid {
		return nil
	}
	severity := pendingSeverity
	if current.Reason == openbaov1alpha1.ReasonInvalid || current.Reason == openbaov1alpha1.ReasonFeatureDisabled {
		severity = openbaov1alpha1.OpenBaoClusterClaimStatusSeverityError
	}
	return newClaimStatusSummary(severity, string(current.Reason), current.Message, sourceRef)
}

func summaryFromPublication(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	publication connectionpublishing.PublicationResult,
) *openbaov1alpha1.OpenBaoClusterClaimStatusSummary {
	if publication.Publishable {
		return nil
	}
	if strings.TrimSpace(publication.Message) == "" && publication.Reason == "" {
		return nil
	}
	severity := openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo
	if publication.Reason == openbaov1alpha1.ReasonInvalid {
		severity = openbaov1alpha1.OpenBaoClusterClaimStatusSeverityError
	}
	return newClaimStatusSummary(
		severity,
		string(publication.Reason),
		publication.Message,
		connectionSecretSourceRef(claim),
	)
}

func newClaimStatusSummary(
	severity openbaov1alpha1.OpenBaoClusterClaimStatusSeverity,
	reason string,
	message string,
	sourceRef *openbaov1alpha1.TypedObjectReference,
) *openbaov1alpha1.OpenBaoClusterClaimStatusSummary {
	if severity == "" && strings.TrimSpace(reason) == "" && strings.TrimSpace(message) == "" && sourceRef == nil {
		return nil
	}
	return &openbaov1alpha1.OpenBaoClusterClaimStatusSummary{
		Severity: severity,
		Reason:   reason,
		Message:  message,
		SourceRef: func() *openbaov1alpha1.TypedObjectReference {
			if sourceRef == nil {
				return nil
			}
			copy := *sourceRef
			return &copy
		}(),
	}
}

func tenantSourceRef(claim *openbaov1alpha1.OpenBaoClusterClaim) *openbaov1alpha1.TypedObjectReference {
	if claim == nil || strings.TrimSpace(claim.Spec.TenantRef.Name) == "" {
		return claimSourceRef(claim)
	}
	return &openbaov1alpha1.TypedObjectReference{
		Kind:      kindOpenBaoTenant,
		Namespace: claim.Namespace,
		Name:      claim.Spec.TenantRef.Name,
	}
}

func serviceContractSourceRef(claim *openbaov1alpha1.OpenBaoClusterClaim) *openbaov1alpha1.TypedObjectReference {
	if claim == nil {
		return nil
	}
	if name := strings.TrimSpace(claim.Spec.ServiceProfileRef.Name); name != "" {
		return &openbaov1alpha1.TypedObjectReference{Kind: kindOpenBaoServiceProfile, Name: name}
	}
	if name := strings.TrimSpace(localReferenceName(claim.Spec.ServiceOfferingRef)); name != "" {
		return &openbaov1alpha1.TypedObjectReference{Kind: kindOpenBaoServiceOffering, Name: name}
	}
	return claimSourceRef(claim)
}

func materializationSourceRef(claim *openbaov1alpha1.OpenBaoClusterClaim) *openbaov1alpha1.TypedObjectReference {
	if claim == nil {
		return nil
	}
	if claim.Status.Materialization.LocalRef != nil {
		return &openbaov1alpha1.TypedObjectReference{
			Kind:      kindOpenBaoCluster,
			Namespace: claim.Status.Materialization.LocalRef.Namespace,
			Name:      claim.Status.Materialization.LocalRef.Name,
		}
	}
	return claimSourceRef(claim)
}

func connectionSecretSourceRef(claim *openbaov1alpha1.OpenBaoClusterClaim) *openbaov1alpha1.TypedObjectReference {
	if claim == nil || strings.TrimSpace(claim.Name) == "" {
		return claimSourceRef(claim)
	}
	return &openbaov1alpha1.TypedObjectReference{
		Kind:      kindSecret,
		Namespace: claim.Namespace,
		Name:      connectionpublishing.SecretName(claim.Name),
	}
}

func upgradeRequestSourceRef(request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) *openbaov1alpha1.TypedObjectReference {
	if request == nil {
		return nil
	}
	return &openbaov1alpha1.TypedObjectReference{
		Kind:      kindOpenBaoClusterClaimUpgradeRequest,
		Namespace: request.Namespace,
		Name:      request.Name,
	}
}

func restoreSourceRef(restore *openbaov1alpha1.OpenBaoRestore) *openbaov1alpha1.TypedObjectReference {
	if restore == nil {
		return nil
	}
	return &openbaov1alpha1.TypedObjectReference{
		Kind:      kindOpenBaoRestore,
		Namespace: restore.Namespace,
		Name:      restore.Name,
	}
}

func localClusterSourceRef(cluster *openbaov1alpha1.OpenBaoCluster) *openbaov1alpha1.TypedObjectReference {
	if cluster == nil {
		return nil
	}
	return &openbaov1alpha1.TypedObjectReference{
		Kind:      kindOpenBaoCluster,
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
}

func claimSourceRef(claim *openbaov1alpha1.OpenBaoClusterClaim) *openbaov1alpha1.TypedObjectReference {
	if claim == nil {
		return nil
	}
	return &openbaov1alpha1.TypedObjectReference{
		Kind:      kindOpenBaoClusterClaim,
		Namespace: claim.Namespace,
		Name:      claim.Name,
	}
}
