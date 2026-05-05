package openbaoclusterclaim

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/claimcontract"
	"github.com/dc-tec/openbao-operator/internal/service/connectionpublishing"
)

type result struct {
	Valid   bool
	Reason  openbaov1alpha1.ConditionReason
	Message string
}

func claimPhase(
	contract result,
	rendered result,
	localClusterResolution result,
	localResolved result,
	ownership result,
	localCluster *openbaov1alpha1.OpenBaoCluster,
	publication connectionpublishing.PublicationResult,
	activeWorkflows activeClaimWorkflows,
) openbaov1alpha1.OpenBaoClusterClaimPhase {
	if !contract.Valid {
		if contract.Reason == openbaov1alpha1.ReasonInvalid {
			return openbaov1alpha1.OpenBaoClusterClaimPhaseFailed
		}
		return openbaov1alpha1.OpenBaoClusterClaimPhasePending
	}
	if localResolved.Valid {
		if !rendered.Valid || !localClusterResolution.Valid {
			if rendered.Reason == openbaov1alpha1.ReasonInvalid ||
				rendered.Reason == openbaov1alpha1.ReasonFeatureDisabled ||
				localClusterResolution.Reason == openbaov1alpha1.ReasonInvalid ||
				localClusterResolution.Reason == openbaov1alpha1.ReasonFeatureDisabled {
				return openbaov1alpha1.OpenBaoClusterClaimPhaseFailed
			}
			return openbaov1alpha1.OpenBaoClusterClaimPhasePending
		}
		if !ownership.Valid {
			return openbaov1alpha1.OpenBaoClusterClaimPhaseFailed
		}
		if localCluster == nil {
			return openbaov1alpha1.OpenBaoClusterClaimPhasePending
		}
		if !localCluster.DeletionTimestamp.IsZero() {
			return openbaov1alpha1.OpenBaoClusterClaimPhaseDeleting
		}
		switch localCluster.Status.Phase {
		case openbaov1alpha1.ClusterPhaseRunning:
			if publication.Publishable {
				if maintenanceActive(activeWorkflows) {
					return openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded
				}
				if localClusterBackupDegraded(localCluster) {
					return openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded
				}
				return openbaov1alpha1.OpenBaoClusterClaimPhaseReady
			}
			if publication.Reason == openbaov1alpha1.ReasonInvalid {
				return openbaov1alpha1.OpenBaoClusterClaimPhaseFailed
			}
			return openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning
		case openbaov1alpha1.ClusterPhaseBackingUp:
			if publication.Publishable {
				return openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded
			}
			if publication.Reason == openbaov1alpha1.ReasonInvalid {
				return openbaov1alpha1.OpenBaoClusterClaimPhaseFailed
			}
			return openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning
		case openbaov1alpha1.ClusterPhaseFailed:
			return openbaov1alpha1.OpenBaoClusterClaimPhaseFailed
		default:
			return openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning
		}
	}
	return openbaov1alpha1.OpenBaoClusterClaimPhasePending
}

func (r runtimeReconciler) claimsEnabled() bool {
	return r.enableServiceClaims
}

func shouldRequeuePendingClaimState(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	acceptance result,
	catalogResolution result,
	contractResolution result,
	localResolved result,
	bootstrapResolution result,
	renderedResolution result,
	localClusterResolution result,
	publication connectionpublishing.PublicationResult,
	localCluster *openbaov1alpha1.OpenBaoCluster,
) bool {
	if claim == nil || !claim.DeletionTimestamp.IsZero() {
		return false
	}
	if hasPendingDependency(acceptance) || hasPendingDependency(catalogResolution) || hasPendingDependency(contractResolution) {
		return true
	}
	if !localResolved.Valid {
		return hasPendingDependency(localResolved)
	}
	if hasPendingDependency(bootstrapResolution) || hasPendingDependency(renderedResolution) || hasPendingDependency(localClusterResolution) {
		return true
	}
	if publication.ShouldRequeue {
		return true
	}
	if localCluster != nil &&
		localCluster.Status.Phase == openbaov1alpha1.ClusterPhaseRunning &&
		!publication.Publishable &&
		publication.Reason == openbaov1alpha1.ReasonPending {
		return true
	}

	return false
}

func hasPendingDependency(validation result) bool {
	if validation.Valid {
		return false
	}
	switch validation.Reason {
	case openbaov1alpha1.ReasonPending, openbaov1alpha1.ReasonPlacementPending:
		return true
	default:
		return false
	}
}

func validateMaterializedServiceSelectionChange(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	activeUpgradeRequest *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest,
) result {
	if claim == nil {
		return result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoClusterClaim is required to validate post-materialization service selection changes.",
		}
	}
	if claim.Status.Materialization.Mode == "" {
		return result{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "OpenBaoClusterClaim has not materialized yet.",
		}
	}
	applied := claim.Status.Applied.ServiceProfileRef
	if applied == nil || applied.Name == "" {
		return result{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "OpenBaoClusterClaim has not recorded an applied service-profile revision yet.",
		}
	}
	appliedOffering := claim.Status.Applied.ServiceOfferingRef
	desiredOffering := strings.TrimSpace(localReferenceName(claim.Spec.ServiceOfferingRef))
	if applied.Name == claim.Spec.ServiceProfileRef.Name &&
		localReferenceName(appliedOffering) == desiredOffering {
		return result{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "OpenBaoClusterClaim service selection matches the applied revision.",
		}
	}
	if upgradeRequestAllowsMaterializedSelectorChange(claim, activeUpgradeRequest) {
		return result{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "OpenBaoClusterClaim service selection change is gated by an active in-place upgrade request.",
		}
	}
	return result{
		Valid:   false,
		Reason:  openbaov1alpha1.ReasonInvalid,
		Message: "OpenBaoClusterClaim service selectors are immutable after materialization until explicit rollout support is implemented.",
	}
}

func upgradeRequestAllowsMaterializedSelectorChange(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest,
) bool {
	if claim == nil || request == nil {
		return false
	}
	if request.Spec.ClaimRef.Name != claim.Name || request.Namespace != claim.Namespace || isTerminalUpgradeRequestState(request.Status.State) {
		return false
	}
	if request.Status.Classification != nil &&
		request.Status.Classification.Class != openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace {
		return false
	}

	desiredProfile := strings.TrimSpace(claim.Spec.ServiceProfileRef.Name)
	desiredOffering := strings.TrimSpace(localReferenceName(claim.Spec.ServiceOfferingRef))
	if desiredProfile == "" {
		return false
	}

	if target := request.Status.Target; target != nil {
		if target.ServiceProfileRef == nil || strings.TrimSpace(target.ServiceProfileRef.Name) != desiredProfile {
			return false
		}
		return strings.TrimSpace(localReferenceName(target.ServiceOfferingRef)) == desiredOffering
	}

	if request.Spec.Target.ServiceProfileRef != nil {
		return strings.TrimSpace(request.Spec.Target.ServiceProfileRef.Name) == desiredProfile && desiredOffering == ""
	}
	if request.Spec.Target.ServiceOfferingRef != nil {
		return strings.TrimSpace(request.Spec.Target.ServiceOfferingRef.Name) == desiredOffering
	}
	return false
}

func isTerminalUpgradeRequestState(state openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState) bool {
	switch state {
	case openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed:
		return true
	default:
		return false
	}
}

func desiredRolloutStatus(
	approved result,
	rendered result,
	localClusterResolution result,
	localResolved result,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) openbaov1alpha1.OpenBaoClusterClaimRolloutStatus {
	if !approved.Valid || claim == nil || claim.Spec.ServiceProfileRef.Name == "" {
		return openbaov1alpha1.OpenBaoClusterClaimRolloutStatus{}
	}
	if localResolved.Valid && !rendered.Valid {
		state := openbaov1alpha1.OpenBaoClusterClaimRolloutStateRendering
		if rendered.Reason == openbaov1alpha1.ReasonInvalid || rendered.Reason == openbaov1alpha1.ReasonFeatureDisabled {
			state = openbaov1alpha1.OpenBaoClusterClaimRolloutStateBlocked
		}
		return openbaov1alpha1.OpenBaoClusterClaimRolloutStatus{
			State:  state,
			Reason: string(rendered.Reason),
		}
	}
	if localResolved.Valid && !localClusterResolution.Valid {
		state := openbaov1alpha1.OpenBaoClusterClaimRolloutStateRendering
		if localClusterResolution.Reason == openbaov1alpha1.ReasonInvalid || localClusterResolution.Reason == openbaov1alpha1.ReasonFeatureDisabled {
			state = openbaov1alpha1.OpenBaoClusterClaimRolloutStateBlocked
		}
		return openbaov1alpha1.OpenBaoClusterClaimRolloutStatus{
			State:  state,
			Reason: string(localClusterResolution.Reason),
		}
	}

	return openbaov1alpha1.OpenBaoClusterClaimRolloutStatus{
		State: openbaov1alpha1.OpenBaoClusterClaimRolloutStateIdle,
	}
}

func desiredAppliedStatus(
	current openbaov1alpha1.OpenBaoClusterClaimAppliedStatus,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	approved *claimcontract.ApprovedServiceContract,
	rendered *claimcontract.RenderedExecutionContract,
	approvedResult result,
	renderedResult result,
) openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	if !approvedResult.Valid || approved == nil {
		return current
	}

	applied := claimcontract.AppliedStatus(approved)
	applied.ServiceOfferingRef = copyLocalReference(current.ServiceOfferingRef)
	if claim != nil {
		applied.ServiceOfferingRef = copyLocalReference(claim.Spec.ServiceOfferingRef)
	}
	applied.ApprovedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(approved))
	if renderedResult.Valid && rendered != nil {
		applied.RenderedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(rendered))
		applied.RenderedDependencies = claimcontract.AppliedRenderedDependencies(rendered)
		return applied
	}
	applied.RenderedContract = copyContractIdentityStatus(current.RenderedContract)
	applied.RenderedDependencies = copyRenderedDependencyStatus(current.RenderedDependencies)
	return applied
}

func copyContractIdentityStatus(status *openbaov1alpha1.OpenBaoClusterClaimContractIdentityStatus) *openbaov1alpha1.OpenBaoClusterClaimContractIdentityStatus {
	if status == nil {
		return nil
	}
	copy := *status
	return &copy
}

func copyLocalReference(ref *openbaov1alpha1.LocalReference) *openbaov1alpha1.LocalReference {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}

func copyNamespacedReference(ref *openbaov1alpha1.NamespacedReference) *openbaov1alpha1.NamespacedReference {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}

func localReferenceName(ref *openbaov1alpha1.LocalReference) string {
	if ref == nil {
		return ""
	}
	return strings.TrimSpace(ref.Name)
}

func copyRenderedDependencyStatus(
	status *openbaov1alpha1.OpenBaoClusterClaimRenderedDependencyStatus,
) *openbaov1alpha1.OpenBaoClusterClaimRenderedDependencyStatus {
	if status == nil {
		return nil
	}
	copy := *status
	copy.EntrypointRef = claimcontractBoundRevisionCopy(status.EntrypointRef)
	copy.IngressPolicyRef = claimcontractBoundRevisionCopy(status.IngressPolicyRef)
	copy.BackupTargetRef = claimcontractBoundRevisionCopy(status.BackupTargetRef)
	copy.BackupBackendRef = claimcontractBoundRevisionCopy(status.BackupBackendRef)
	copy.BackupAuthProfileRef = claimcontractBoundRevisionCopy(status.BackupAuthProfileRef)
	copy.TransferProfileRef = claimcontractBoundRevisionCopy(status.TransferProfileRef)
	copy.BootstrapProjectionIdentity = copyContractIdentityStatus(status.BootstrapProjectionIdentity)
	copy.Identity = copyContractIdentityStatus(status.Identity)
	return &copy
}

func claimcontractBoundRevisionCopy(
	ref *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference,
) *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}

func desiredMaterializationStatus(
	localTarget *openbaov1alpha1.NamespacedReference,
	localResolved result,
	rendered result,
	currentLocalRef *openbaov1alpha1.NamespacedReference,
	currentApplied openbaov1alpha1.OpenBaoClusterClaimAppliedStatus,
) openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus {
	if localResolved.Valid && localTarget != nil {
		status := openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
			Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
		}
		switch {
		case currentLocalRef != nil && currentLocalRef.Namespace != "" && currentLocalRef.Name != "":
			status.LocalRef = copyNamespacedReference(currentLocalRef)
		case rendered.Valid || hasAppliedRenderedContractStatus(currentApplied):
			status.LocalRef = &openbaov1alpha1.NamespacedReference{
				Namespace: localTarget.Namespace,
				Name:      localTarget.Name,
			}
		}
		return status
	}

	return openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{}
}

func hasAppliedRenderedContractStatus(status openbaov1alpha1.OpenBaoClusterClaimAppliedStatus) bool {
	return status.RenderedContract != nil && status.RenderedContract.IdentityHash != ""
}

func resolvedMaterializationResult(local result, localCluster result) result {
	if local.Valid {
		return localCluster
	}
	return local
}

func controllerCondition(enabled bool, generation int64) metav1.Condition {
	condition := metav1.Condition{
		Type:               conditionTypeControllerActive,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
	}

	if enabled {
		condition.Status = metav1.ConditionTrue
		condition.Reason = reasonNotImplemented
		condition.Message = "OpenBaoClusterClaim controller is active."
		return condition
	}

	condition.Status = metav1.ConditionFalse
	condition.Reason = string(openbaov1alpha1.ReasonFeatureDisabled)
	condition.Message = "OpenBaoClusterClaim controller is disabled until " + constants.EnvOperatorEnableServiceClaims + " is enabled."
	return condition
}

func acceptanceCondition(validation result, generation int64) metav1.Condition {
	return resultCondition(conditionTypeAccepted, validation, generation)
}

func serviceContractCondition(validation result, generation int64) metav1.Condition {
	return resultCondition(conditionTypeServiceContract, validation, generation)
}

func materializationCondition(validation result, generation int64) metav1.Condition {
	return resultCondition(conditionTypeMaterialization, validation, generation)
}

func ownershipCondition(validation result, generation int64) metav1.Condition {
	return resultCondition(conditionTypeOwnershipReady, validation, generation)
}

func resultCondition(conditionType string, validation result, generation int64) metav1.Condition {
	condition := metav1.Condition{
		Type:               conditionType,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(validation.Reason),
		Message:            validation.Message,
	}
	if validation.Valid {
		condition.Status = metav1.ConditionTrue
		return condition
	}
	condition.Status = metav1.ConditionFalse
	return condition
}

func connectionCondition(publication connectionpublishing.PublicationResult, generation int64) metav1.Condition {
	condition := metav1.Condition{
		Type:               conditionTypeConnectionPublished,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(publication.Reason),
		Message:            publication.Message,
	}
	if publication.Publishable {
		condition.Status = metav1.ConditionTrue
		return condition
	}
	condition.Status = metav1.ConditionFalse
	return condition
}

func serviceAvailabilityCondition(
	phase openbaov1alpha1.OpenBaoClusterClaimPhase,
	publication connectionpublishing.PublicationResult,
	localResolved result,
	localCluster *openbaov1alpha1.OpenBaoCluster,
	activeWorkflows activeClaimWorkflows,
	generation int64,
) metav1.Condition {
	condition := metav1.Condition{
		Type:               conditionTypeServiceAvailable,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
	}

	switch phase {
	case openbaov1alpha1.OpenBaoClusterClaimPhaseReady:
		condition.Status = metav1.ConditionTrue
		condition.Reason = string(openbaov1alpha1.ReasonReady)
		condition.Message = "Service instance is available."
		return condition
	case openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded:
		condition.Status = metav1.ConditionTrue
		if reason, message, _, ok := activeMaintenanceStatus(activeWorkflows); ok {
			condition.Reason = reason
			condition.Message = message
			return condition
		}
		if localClusterBackupInProgress(localCluster) {
			condition.Reason = string(openbaov1alpha1.ClusterPhaseBackingUp)
			condition.Message = "Service instance remains available while a backup operation is active."
			return condition
		}
		if reason, message, ok := localClusterBackupFailure(localCluster); ok {
			condition.Reason = reason
			condition.Message = message
			return condition
		}
		condition.Reason = string(openbaov1alpha1.ReasonReady)
		condition.Message = "Service instance remains available, but it is not in the steady state."
		return condition
	case openbaov1alpha1.OpenBaoClusterClaimPhaseDeleting:
		condition.Status = metav1.ConditionFalse
		condition.Reason = reasonDeleting
		condition.Message = "Service instance is being retired."
		return condition
	case openbaov1alpha1.OpenBaoClusterClaimPhaseFailed:
		condition.Status = metav1.ConditionFalse
		switch {
		case publication.Reason == openbaov1alpha1.ReasonInvalid && strings.TrimSpace(publication.Message) != "":
			condition.Reason = string(publication.Reason)
			condition.Message = publication.Message
		case localResolved.Valid && localCluster != nil && localCluster.Status.Phase == openbaov1alpha1.ClusterPhaseFailed:
			condition.Reason = string(openbaov1alpha1.ReasonInvalid)
			condition.Message = "Service instance is unavailable because the local concrete workload has failed."
		default:
			condition.Reason = string(openbaov1alpha1.ReasonInvalid)
			condition.Message = "Service instance is unavailable and requires operator action."
		}
		return condition
	default:
		condition.Status = metav1.ConditionFalse
		if strings.TrimSpace(publication.Message) != "" {
			condition.Reason = string(publication.Reason)
			condition.Message = publication.Message
			return condition
		}
		if localResolved.Valid {
			condition.Reason = string(openbaov1alpha1.ReasonPending)
			condition.Message = "Service instance provisioning is still in progress."
			return condition
		}
		condition.Reason = string(openbaov1alpha1.ReasonPending)
		condition.Message = "Service instance is waiting for same-cluster materialization."
		return condition
	}
}

func maintenanceActiveCondition(activeWorkflows activeClaimWorkflows, generation int64) metav1.Condition {
	condition := metav1.Condition{
		Type:               conditionTypeMaintenanceActive,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
	}

	if reason, message, _, ok := activeMaintenanceStatus(activeWorkflows); ok {
		condition.Status = metav1.ConditionTrue
		condition.Reason = reason
		condition.Message = message
		return condition
	}

	condition.Status = metav1.ConditionFalse
	condition.Reason = reasonIdle
	condition.Message = "No maintenance workflow is active for this service instance."
	return condition
}

func maintenanceActive(activeWorkflows activeClaimWorkflows) bool {
	_, _, _, ok := activeMaintenanceStatus(activeWorkflows)
	return ok
}

func activeMaintenanceStatus(activeWorkflows activeClaimWorkflows) (reason string, message string, sourceRef *openbaov1alpha1.TypedObjectReference, ok bool) {
	if activeWorkflows.UpgradeRequest == nil || isTerminalUpgradeRequestState(activeWorkflows.UpgradeRequest.Status.State) {
		return activeRestoreStatus(activeWorkflows)
	}
	state := activeWorkflows.UpgradeRequest.Status.State
	if state == "" {
		state = openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending
	}
	return string(state), "Service instance remains available while an upgrade workflow is active.", upgradeRequestSourceRef(activeWorkflows.UpgradeRequest), true
}

func activeRestoreStatus(activeWorkflows activeClaimWorkflows) (reason string, message string, sourceRef *openbaov1alpha1.TypedObjectReference, ok bool) {
	if activeWorkflows.RestoreRequest != nil && !isTerminalClaimRestoreRequestState(activeWorkflows.RestoreRequest.Status.State) {
		state := activeWorkflows.RestoreRequest.Status.State
		if state == "" {
			state = openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending
		}
		return string(state), "Service instance remains available while a restore workflow is active.", claimRestoreRequestSourceRef(activeWorkflows.RestoreRequest), true
	}
	if activeWorkflows.RestoreExecution == nil || isTerminalRestorePhase(activeWorkflows.RestoreExecution.Status.Phase) {
		return "", "", nil, false
	}
	state := activeWorkflows.RestoreExecution.Status.Phase
	if state == "" {
		state = openbaov1alpha1.RestorePhasePending
	}
	return string(state), "Service instance remains available while a restore workflow is active.", restoreSourceRef(activeWorkflows.RestoreExecution), true
}
