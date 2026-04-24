package openbaoclusterclaimupgraderequest

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/service/claimcontract"
)

const (
	reasonServiceClaimsDisabled                 = "ServiceClaimsDisabled"
	reasonClaimNotFound                         = "ClaimNotFound"
	reasonClaimReadFailed                       = "ClaimReadFailed"
	reasonUpgradeRequestListFailed              = "UpgradeRequestListFailed"
	reasonAnotherUpgradeRequestActive           = "AnotherUpgradeRequestActive"
	reasonClaimDeleting                         = "ClaimDeleting"
	reasonClaimNotMaterializedForSameCluster    = "ClaimNotMaterializedForSameCluster"
	reasonClaimHasNoAppliedRevision             = "ClaimHasNoAppliedRevision"
	reasonCurrentCatalogResolutionFailed        = "CurrentCatalogResolutionFailed"
	reasonCurrentContractInvalid                = "CurrentContractInvalid"
	reasonTargetResolutionFailed                = "TargetResolutionFailed"
	reasonTargetNotFound                        = "TargetNotFound"
	reasonTargetCatalogResolutionFailed         = "TargetCatalogResolutionFailed"
	reasonTargetContractInvalid                 = "TargetContractInvalid"
	reasonAlreadyApplied                        = "AlreadyApplied"
	reasonClaimUpdateFailed                     = "ClaimUpdateFailed"
	reasonRolloutRequested                      = "RolloutRequested"
	reasonLocalClusterPending                   = "LocalClusterPending"
	reasonClaimRolloutBlocked                   = "ClaimRolloutBlocked"
	reasonClaimRolloutFailed                    = "ClaimRolloutFailed"
	reasonAppliedRevisionPending                = "AppliedRevisionPending"
	reasonClaimRolloutInProgress                = "ClaimRolloutInProgress"
	reasonLocalClusterReadFailed                = "LocalClusterReadFailed"
	reasonLocalClusterFailed                    = "LocalClusterFailed"
	reasonLocalClusterReconciling               = "LocalClusterReconciling"
	reasonUpgradeInProgress                     = "UpgradeInProgress"
	reasonLocalClusterNotReady                  = "LocalClusterNotReady"
	reasonClaimNotReadyYet                      = "ClaimNotReadyYet"
	reasonUpgradeApplied                        = "UpgradeApplied"
	reasonClassificationInputsInvalid           = "ClassificationInputsInvalid"
	reasonBootstrapChangeRequiresReprovision    = "BootstrapChangeRequiresReprovision"
	reasonBackupLocationChangeRequiresMigration = "BackupLocationChangeRequiresMigration"
	reasonBackupExecutionIdentityChanged        = "BackupExecutionIdentityChanged"
	reasonReplacementWorkflowRequired           = "ReplacementWorkflowRequired"
	reasonInPlaceSupported                      = "InPlaceSupported"
	reasonEquivalentServiceShape                = "EquivalentServiceShape"
	conditionTypeServiceAvailable               = "ServiceAvailable"
	conditionTypeMaintenanceActive              = "MaintenanceActive"
)

type Reconciler interface {
	Reconcile(ctx context.Context, key types.NamespacedName, logger logr.Logger) (recon.Result, error)
}

type Runtime struct {
	Client              client.Client
	Reader              client.Reader
	EnableServiceClaims bool
}

type runtimeReconciler struct {
	client              client.Client
	reader              client.Reader
	enableServiceClaims bool
}

func NewReconciler(deps Runtime) Reconciler {
	reader := deps.Reader
	if reader == nil {
		reader = deps.Client
	}
	return runtimeReconciler{client: deps.Client, reader: reader, enableServiceClaims: deps.EnableServiceClaims}
}

func (r runtimeReconciler) Reconcile(ctx context.Context, key types.NamespacedName, logger logr.Logger) (recon.Result, error) {
	if r.client == nil {
		return recon.Result{}, fmt.Errorf("client is required")
	}

	request := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{}
	if err := r.client.Get(ctx, key, request); err != nil {
		if apierrors.IsNotFound(err) {
			return recon.Result{}, nil
		}
		return recon.Result{}, fmt.Errorf("failed to get OpenBaoClusterClaimUpgradeRequest: %w", err)
	}

	logger = logger.WithValues("openBaoClusterClaimUpgradeRequest", key.String())
	original := request.DeepCopy()

	state, reason, current, target, classification := r.reconcileRequestState(ctx, request)
	request.Status.ObservedGeneration = request.Generation
	request.Status.State = state
	request.Status.Reason = reason
	request.Status.Current = current
	request.Status.Target = target
	request.Status.Classification = classification
	request.Status.Conditions = nil

	if reflect.DeepEqual(original.Status, request.Status) {
		logger.V(1).Info("OpenBaoClusterClaimUpgradeRequest status already up to date")
		return requeueForState(request.Status.State), nil
	}
	if err := r.client.Status().Patch(ctx, request, client.MergeFrom(original)); err != nil {
		return recon.Result{}, fmt.Errorf("patch OpenBaoClusterClaimUpgradeRequest status: %w", err)
	}

	logger.Info("Reconciled OpenBaoClusterClaimUpgradeRequest", "state", state, "reason", reason)
	return requeueForState(state), nil
}

func requeueForState(openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState) recon.Result {
	return recon.Result{}
}

func (r runtimeReconciler) reconcileRequestState(
	ctx context.Context,
	request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest,
) (
	openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState,
	string,
	*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus,
	*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus,
	*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestClassificationStatus,
) {
	if request == nil {
		return "", "", nil, nil, nil
	}
	if isTerminalRequestState(request.Status.State) {
		return request.Status.State,
			request.Status.Reason,
			revisionStatusCopy(request.Status.Current),
			revisionStatusCopy(request.Status.Target),
			classificationStatusCopy(request.Status.Classification)
	}
	if !r.enableServiceClaims {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked,
			reasonServiceClaimsDisabled, nil, nil,
			classificationStatus(openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked, reasonServiceClaimsDisabled)
	}

	claim := &openbaov1alpha1.OpenBaoClusterClaim{}
	if err := r.reader.Get(ctx, types.NamespacedName{Namespace: request.Namespace, Name: request.Spec.ClaimRef.Name}, claim); err != nil {
		if apierrors.IsNotFound(err) {
			return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
				reasonClaimNotFound, nil, nil, nil
		}
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
			reasonClaimReadFailed, nil, nil, nil
	}

	currentStatus := currentRevisionStatus(claim)
	if other, err := r.findEarlierActiveRequest(ctx, request); err != nil {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
			reasonUpgradeRequestListFailed, currentStatus, nil, nil
	} else if other != nil {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked,
			reasonAnotherUpgradeRequestActive, currentStatus, nil,
			classificationStatus(openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked, reasonAnotherUpgradeRequestActive)
	}
	if !claim.DeletionTimestamp.IsZero() {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked,
			reasonClaimDeleting, currentStatus, nil,
			classificationStatus(openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked, reasonClaimDeleting)
	}
	if claim.Status.Materialization.Mode != openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster || claim.Status.Materialization.LocalRef == nil {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked,
			reasonClaimNotMaterializedForSameCluster, currentStatus, nil,
			classificationStatus(openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked, reasonClaimNotMaterializedForSameCluster)
	}
	if claim.Status.Applied.ServiceProfileRef == nil {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked,
			reasonClaimHasNoAppliedRevision, currentStatus, nil,
			classificationStatus(openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked, reasonClaimHasNoAppliedRevision)
	}

	currentCatalog, err := r.resolveCatalogBundle(ctx, claim.Status.Applied.ServiceProfileRef.Name)
	if err != nil {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
			reasonCurrentCatalogResolutionFailed, currentStatus, nil, nil
	}
	currentApproved, currentValidation := claimcontract.BindApprovedServiceContract(claimForAppliedRevision(claim), currentCatalog)
	if !currentValidation.Valid || currentApproved == nil {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
			reasonCurrentContractInvalid, currentStatus, nil, nil
	}
	currentStatus = revisionStatusFromContract(claim.Status.Applied.ServiceOfferingRef, claim.Status.Applied.ServiceProfileRef, currentApproved)

	resolvedOffering, targetProfile, err := r.resolveTarget(ctx, request)
	if err != nil {
		reason := reasonTargetResolutionFailed
		if apierrors.IsNotFound(err) {
			reason = reasonTargetNotFound
		}
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed, reason, currentStatus, nil, nil
	}
	desiredClaim := claim.DeepCopy()
	if resolvedOffering != nil {
		desiredClaim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: resolvedOffering.Name}
	} else {
		desiredClaim.Spec.ServiceOfferingRef = nil
	}
	desiredClaim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: targetProfile.Name}

	targetCatalog, err := r.resolveCatalogBundle(ctx, targetProfile.Name)
	if err != nil {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
			reasonTargetCatalogResolutionFailed, currentStatus, nil, nil
	}
	targetApproved, targetValidation := claimcontract.BindApprovedServiceContract(desiredClaim, targetCatalog)
	if !targetValidation.Valid || targetApproved == nil {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
			reasonTargetContractInvalid, currentStatus, nil, nil
	}
	targetStatus := revisionStatusFromResolvedTarget(resolvedOffering, targetProfile, targetApproved)

	classificationClass, classificationReason := classifyUpgrade(currentApproved, currentCatalog, targetApproved, targetCatalog)
	classification := classificationStatus(classificationClass, classificationReason)
	targetAlreadyApplied := appliedRevisionMatchesTarget(claim, targetStatus)
	if targetAlreadyApplied {
		if request.Status.State == openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut ||
			request.Status.State == openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded ||
			claimUpgradeRequestToken(claim) == upgradeRequestToken(request) {
			return r.observeInPlaceRollout(ctx, claim, currentStatus, targetStatus, classification)
		}
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked,
			reasonAlreadyApplied, currentStatus, targetStatus,
			classificationStatus(openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked, reasonAlreadyApplied)
	}
	switch classificationClass {
	case openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace:
		if !claimSpecMatchesTarget(claim, resolvedOffering, targetProfile) {
			if err := r.promoteClaimTarget(ctx, request, claim, resolvedOffering, targetProfile); err != nil {
				return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
					reasonClaimUpdateFailed, currentStatus, targetStatus, classification
			}
			return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut,
				reasonRolloutRequested, currentStatus, targetStatus, classification
		}
		return r.observeInPlaceRollout(ctx, claim, currentStatus, targetStatus, classification)
	case openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassReplacementRequired:
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateReplacementRequired, classificationReason, currentStatus, targetStatus, classification
	default:
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked, classificationReason, currentStatus, targetStatus, classification
	}
}

func (r runtimeReconciler) findEarlierActiveRequest(
	ctx context.Context,
	request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest,
) (*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest, error) {
	if request == nil || request.Namespace == "" || request.Spec.ClaimRef.Name == "" {
		return nil, nil
	}

	list := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestList{}
	if err := r.reader.List(ctx, list, client.InNamespace(request.Namespace)); err != nil {
		return nil, err
	}

	for i := range list.Items {
		candidate := &list.Items[i]
		if candidate.Name == request.Name || candidate.Spec.ClaimRef.Name != request.Spec.ClaimRef.Name || isTerminalRequestState(candidate.Status.State) {
			continue
		}
		if requestIsEarlier(candidate, request) {
			return candidate, nil
		}
	}
	return nil, nil
}

func isTerminalRequestState(state openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState) bool {
	switch state {
	case openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateReplacementRequired,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed:
		return true
	default:
		return false
	}
}

func requestIsEarlier(a, b *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) bool {
	if a == nil || b == nil {
		return false
	}
	if a.CreationTimestamp.Equal(&b.CreationTimestamp) {
		return a.Name < b.Name
	}
	return a.CreationTimestamp.Time.Before(b.CreationTimestamp.Time)
}

func claimForAppliedRevision(claim *openbaov1alpha1.OpenBaoClusterClaim) *openbaov1alpha1.OpenBaoClusterClaim {
	if claim == nil {
		return nil
	}

	current := claim.DeepCopy()
	current.Spec.ServiceOfferingRef = localReferenceCopy(claim.Status.Applied.ServiceOfferingRef)
	if claim.Status.Applied.ServiceProfileRef != nil {
		current.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: claim.Status.Applied.ServiceProfileRef.Name}
	}
	return current
}

func claimSpecMatchesTarget(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	offering *openbaov1alpha1.OpenBaoServiceOffering,
	profile *openbaov1alpha1.OpenBaoServiceProfile,
) bool {
	if claim == nil || profile == nil {
		return false
	}
	if claim.Spec.ServiceProfileRef.Name != profile.Name {
		return false
	}
	if offering == nil {
		return claim.Spec.ServiceOfferingRef == nil
	}
	return claim.Spec.ServiceOfferingRef != nil && claim.Spec.ServiceOfferingRef.Name == offering.Name
}

func (r runtimeReconciler) promoteClaimTarget(
	ctx context.Context,
	request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	offering *openbaov1alpha1.OpenBaoServiceOffering,
	profile *openbaov1alpha1.OpenBaoServiceProfile,
) error {
	if claim == nil || profile == nil {
		return fmt.Errorf("claim and target profile are required")
	}

	original := claim.DeepCopy()
	if offering != nil {
		claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: offering.Name}
	} else {
		claim.Spec.ServiceOfferingRef = nil
	}
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: profile.Name}
	if token := upgradeRequestToken(request); token != "" {
		if claim.Annotations == nil {
			claim.Annotations = map[string]string{}
		}
		claim.Annotations[constants.AnnotationClaimUpgradeRequest] = token
	}
	if reflect.DeepEqual(original.Spec, claim.Spec) &&
		reflect.DeepEqual(original.Annotations, claim.Annotations) {
		return nil
	}
	if err := r.client.Patch(ctx, claim, client.MergeFrom(original)); err != nil {
		return err
	}
	return nil
}

func (r runtimeReconciler) observeInPlaceRollout(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	currentStatus *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus,
	targetStatus *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus,
	classification *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestClassificationStatus,
) (
	openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState,
	string,
	*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus,
	*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus,
	*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestClassificationStatus,
) {
	if claim == nil || claim.Status.Materialization.LocalRef == nil {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut,
			reasonLocalClusterPending, currentStatus, targetStatus, classification
	}
	if !appliedRevisionMatchesTarget(claim, targetStatus) {
		switch claim.Status.Rollout.State {
		case openbaov1alpha1.OpenBaoClusterClaimRolloutStateBlocked:
			return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
				reasonClaimRolloutBlocked, currentStatus, targetStatus, classification
		case openbaov1alpha1.OpenBaoClusterClaimRolloutStateFailed:
			return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
				reasonClaimRolloutFailed, currentStatus, targetStatus, classification
		default:
			return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut,
				reasonAppliedRevisionPending, currentStatus, targetStatus, classification
		}
	}
	switch claim.Status.Rollout.State {
	case openbaov1alpha1.OpenBaoClusterClaimRolloutStateBlocked:
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
			reasonClaimRolloutBlocked, currentStatus, targetStatus, classification
	case openbaov1alpha1.OpenBaoClusterClaimRolloutStateFailed:
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
			reasonClaimRolloutFailed, currentStatus, targetStatus, classification
	case openbaov1alpha1.OpenBaoClusterClaimRolloutStatePending,
		openbaov1alpha1.OpenBaoClusterClaimRolloutStateRendering,
		openbaov1alpha1.OpenBaoClusterClaimRolloutStateRollingOut:
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut,
			reasonClaimRolloutInProgress, currentStatus, targetStatus, classification
	}

	localCluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := r.reader.Get(ctx, types.NamespacedName{
		Namespace: claim.Status.Materialization.LocalRef.Namespace,
		Name:      claim.Status.Materialization.LocalRef.Name,
	}, localCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut,
				reasonLocalClusterPending, currentStatus, targetStatus, classification
		}
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
			reasonLocalClusterReadFailed, currentStatus, targetStatus, classification
	}

	if localCluster.Status.Phase == openbaov1alpha1.ClusterPhaseFailed {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
			reasonLocalClusterFailed, currentStatus, targetStatus, classification
	}
	if localCluster.Status.ObservedGeneration < localCluster.Generation {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut,
			reasonLocalClusterReconciling, currentStatus, targetStatus, classification
	}
	if localCluster.Status.Upgrade != nil || localCluster.Status.Phase == openbaov1alpha1.ClusterPhaseUpgrading {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut,
			reasonUpgradeInProgress, currentStatus, targetStatus, classification
	}
	if localCluster.Status.Phase != openbaov1alpha1.ClusterPhaseRunning {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut,
			reasonLocalClusterNotReady, currentStatus, targetStatus, classification
	}
	if !claimServiceReadyForUpgradeCompletion(claim) {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut,
			reasonClaimNotReadyYet, currentStatus, targetStatus, classification
	}

	return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded,
		reasonUpgradeApplied, currentStatus, targetStatus, classification
}

func claimServiceReadyForUpgradeCompletion(claim *openbaov1alpha1.OpenBaoClusterClaim) bool {
	if claim == nil {
		return false
	}
	switch claim.Status.Phase {
	case openbaov1alpha1.OpenBaoClusterClaimPhaseReady:
		return true
	case openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded:
		return claimServiceConditionStatus(claim, conditionTypeServiceAvailable) == metav1.ConditionTrue &&
			claimServiceConditionStatus(claim, conditionTypeMaintenanceActive) == metav1.ConditionTrue
	default:
		return false
	}
}

func claimServiceConditionStatus(claim *openbaov1alpha1.OpenBaoClusterClaim, conditionType string) metav1.ConditionStatus {
	if claim == nil {
		return metav1.ConditionUnknown
	}
	for i := range claim.Status.Conditions {
		if claim.Status.Conditions[i].Type == conditionType {
			return claim.Status.Conditions[i].Status
		}
	}
	return metav1.ConditionUnknown
}

func (r runtimeReconciler) resolveTarget(
	ctx context.Context,
	request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest,
) (*openbaov1alpha1.OpenBaoServiceOffering, *openbaov1alpha1.OpenBaoServiceProfile, error) {
	if request.Spec.Target.ServiceProfileRef != nil {
		profile := &openbaov1alpha1.OpenBaoServiceProfile{}
		if err := r.reader.Get(ctx, types.NamespacedName{Name: request.Spec.Target.ServiceProfileRef.Name}, profile); err != nil {
			return nil, nil, err
		}
		return nil, profile, nil
	}

	offering := &openbaov1alpha1.OpenBaoServiceOffering{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: request.Spec.Target.ServiceOfferingRef.Name}, offering); err != nil {
		return nil, nil, err
	}
	profile := &openbaov1alpha1.OpenBaoServiceProfile{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: offering.Spec.CurrentRevisionRef.Name}, profile); err != nil {
		return nil, nil, err
	}
	return offering, profile, nil
}

func (r runtimeReconciler) resolveCatalogBundle(ctx context.Context, serviceProfileName string) (*claimcontract.CatalogBundle, error) {
	serviceProfile := &openbaov1alpha1.OpenBaoServiceProfile{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: serviceProfileName}, serviceProfile); err != nil {
		return nil, err
	}
	catalog := &claimcontract.CatalogBundle{ServiceProfile: serviceProfile}

	catalog.ExposureClass = &openbaov1alpha1.OpenBaoExposureClass{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: serviceProfile.Spec.Exposure.ClassRef.Name}, catalog.ExposureClass); err != nil {
		return nil, err
	}
	catalog.BackupProfile = &openbaov1alpha1.OpenBaoBackupProfile{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: serviceProfile.Spec.Backup.ProfileRef.Name}, catalog.BackupProfile); err != nil {
		return nil, err
	}

	if err := r.resolveImplementationProfiles(ctx, serviceProfile, catalog); err != nil {
		return nil, err
	}
	if err := r.resolveBackupCatalogObjects(ctx, catalog); err != nil {
		return nil, err
	}
	if err := r.resolveExposureCatalogObjects(ctx, catalog); err != nil {
		return nil, err
	}
	return catalog, nil
}

func (r runtimeReconciler) resolveImplementationProfiles(
	ctx context.Context,
	serviceProfile *openbaov1alpha1.OpenBaoServiceProfile,
	catalog *claimcontract.CatalogBundle,
) error {
	var err error

	catalog.BootstrapProfile, err = r.resolveBootstrapProfile(ctx, serviceProfile.Spec.Bootstrap.ProfileRef)
	if err != nil {
		return err
	}
	catalog.StorageProfile, err = r.resolveStorageProfile(ctx, serviceProfile.Spec.Storage.ProfileRef)
	if err != nil {
		return err
	}
	catalog.UnsealProfile, err = r.resolveUnsealProfile(ctx, serviceProfile.Spec.Unseal)
	if err != nil {
		return err
	}
	catalog.RuntimeProfile, err = r.resolveRuntimeProfile(ctx, serviceProfile.Spec.Runtime)
	if err != nil {
		return err
	}
	catalog.ObservabilityProfile, err = r.resolveObservabilityProfile(ctx, serviceProfile.Spec.Observability)
	if err != nil {
		return err
	}
	catalog.NetworkProfile, err = r.resolveNetworkProfile(ctx, serviceProfile.Spec.Network)
	if err != nil {
		return err
	}
	catalog.UpgradePolicy, err = r.resolveUpgradePolicy(ctx, serviceProfile.Spec.Lifecycle.PolicyRef)
	return err
}

func (r runtimeReconciler) resolveBootstrapProfile(
	ctx context.Context,
	ref *openbaov1alpha1.LocalReference,
) (*openbaov1alpha1.OpenBaoBootstrapProfile, error) {
	if ref == nil {
		return nil, nil
	}
	profile := &openbaov1alpha1.OpenBaoBootstrapProfile{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: ref.Name}, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (r runtimeReconciler) resolveStorageProfile(
	ctx context.Context,
	ref *openbaov1alpha1.LocalReference,
) (*openbaov1alpha1.OpenBaoStorageProfile, error) {
	if ref == nil {
		return nil, nil
	}
	profile := &openbaov1alpha1.OpenBaoStorageProfile{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: ref.Name}, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (r runtimeReconciler) resolveUnsealProfile(
	ctx context.Context,
	spec *openbaov1alpha1.OpenBaoServiceProfileUnsealSpec,
) (*openbaov1alpha1.OpenBaoUnsealProfile, error) {
	if spec == nil || spec.ProfileRef == nil {
		return nil, nil
	}
	profile := &openbaov1alpha1.OpenBaoUnsealProfile{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: spec.ProfileRef.Name}, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (r runtimeReconciler) resolveRuntimeProfile(
	ctx context.Context,
	spec *openbaov1alpha1.OpenBaoServiceProfileRuntimeSpec,
) (*openbaov1alpha1.OpenBaoRuntimeProfile, error) {
	if spec == nil || spec.ProfileRef == nil {
		return nil, nil
	}
	profile := &openbaov1alpha1.OpenBaoRuntimeProfile{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: spec.ProfileRef.Name}, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (r runtimeReconciler) resolveObservabilityProfile(
	ctx context.Context,
	spec *openbaov1alpha1.OpenBaoServiceProfileObservabilitySpec,
) (*openbaov1alpha1.OpenBaoObservabilityProfile, error) {
	if spec == nil || spec.ProfileRef == nil {
		return nil, nil
	}
	profile := &openbaov1alpha1.OpenBaoObservabilityProfile{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: spec.ProfileRef.Name}, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (r runtimeReconciler) resolveNetworkProfile(
	ctx context.Context,
	spec *openbaov1alpha1.OpenBaoServiceProfileNetworkSpec,
) (*openbaov1alpha1.OpenBaoNetworkProfile, error) {
	if spec == nil || spec.ProfileRef == nil {
		return nil, nil
	}
	profile := &openbaov1alpha1.OpenBaoNetworkProfile{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: spec.ProfileRef.Name}, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (r runtimeReconciler) resolveUpgradePolicy(
	ctx context.Context,
	ref *openbaov1alpha1.LocalReference,
) (*openbaov1alpha1.OpenBaoUpgradePolicy, error) {
	if ref == nil {
		return nil, nil
	}
	policy := &openbaov1alpha1.OpenBaoUpgradePolicy{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: ref.Name}, policy); err != nil {
		return nil, err
	}
	return policy, nil
}

func (r runtimeReconciler) resolveBackupCatalogObjects(ctx context.Context, catalog *claimcontract.CatalogBundle) error {
	if catalog.BackupProfile.Spec.TargetRef == nil {
		return nil
	}

	catalog.BackupTarget = &openbaov1alpha1.OpenBaoBackupTarget{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: catalog.BackupProfile.Spec.TargetRef.Name}, catalog.BackupTarget); err != nil {
		return err
	}
	catalog.BackupBackend = &openbaov1alpha1.OpenBaoBackupBackend{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: catalog.BackupTarget.Spec.BackendRef.Name}, catalog.BackupBackend); err != nil {
		return err
	}
	if catalog.BackupTarget.Spec.AuthProfileRef != nil {
		catalog.BackupAuth = &openbaov1alpha1.OpenBaoBackupAuthProfile{}
		if err := r.reader.Get(ctx, types.NamespacedName{Name: catalog.BackupTarget.Spec.AuthProfileRef.Name}, catalog.BackupAuth); err != nil {
			return err
		}
	}
	if catalog.BackupTarget.Spec.TransportProfileRef != nil {
		catalog.TransferProfile = &openbaov1alpha1.OpenBaoTransferProfile{}
		if err := r.reader.Get(ctx, types.NamespacedName{Name: catalog.BackupTarget.Spec.TransportProfileRef.Name}, catalog.TransferProfile); err != nil {
			return err
		}
	}
	return nil
}

func (r runtimeReconciler) resolveExposureCatalogObjects(ctx context.Context, catalog *claimcontract.CatalogBundle) error {
	if catalog.ExposureClass.Spec.EntrypointRef != nil {
		catalog.Entrypoint = &openbaov1alpha1.OpenBaoEntrypoint{}
		if err := r.reader.Get(ctx, types.NamespacedName{Name: catalog.ExposureClass.Spec.EntrypointRef.Name}, catalog.Entrypoint); err != nil {
			return err
		}
	}
	if catalog.ExposureClass.Spec.IngressPolicyRef != nil {
		catalog.IngressPolicy = &openbaov1alpha1.OpenBaoIngressPolicy{}
		if err := r.reader.Get(ctx, types.NamespacedName{Name: catalog.ExposureClass.Spec.IngressPolicyRef.Name}, catalog.IngressPolicy); err != nil {
			return err
		}
	}
	return nil
}

func classifyUpgrade(
	currentApproved *claimcontract.ApprovedServiceContract,
	currentCatalog *claimcontract.CatalogBundle,
	targetApproved *claimcontract.ApprovedServiceContract,
	targetCatalog *claimcontract.CatalogBundle,
) (openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClass, string) {
	if currentApproved == nil || targetApproved == nil || currentCatalog == nil || targetCatalog == nil {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked, reasonClassificationInputsInvalid
	}
	if currentApproved.Bootstrap.Mode != targetApproved.Bootstrap.Mode || localRefName(currentApproved.Bootstrap.ProfileRef) != localRefName(targetApproved.Bootstrap.ProfileRef) {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked, reasonBootstrapChangeRequiresReprovision
	}
	if currentApproved.Backup.Parameters.Location != targetApproved.Backup.Parameters.Location || currentApproved.Backup.Parameters.Partition != targetApproved.Backup.Parameters.Partition {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked, reasonBackupLocationChangeRequiresMigration
	}
	if backupTargetName(currentCatalog) != backupTargetName(targetCatalog) || backupBackendName(currentCatalog) != backupBackendName(targetCatalog) || backupAuthName(currentCatalog) != backupAuthName(targetCatalog) || transferProfileName(currentCatalog) != transferProfileName(targetCatalog) {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked, reasonBackupExecutionIdentityChanged
	}
	if currentApproved.Cluster.Voters != targetApproved.Cluster.Voters ||
		currentApproved.Cluster.ReadReplicas != targetApproved.Cluster.ReadReplicas ||
		currentApproved.Cluster.SecurityProfile != targetApproved.Cluster.SecurityProfile ||
		!reflect.DeepEqual(currentApproved.Storage, targetApproved.Storage) ||
		!reflect.DeepEqual(currentApproved.Unseal, targetApproved.Unseal) ||
		!reflect.DeepEqual(currentApproved.Runtime, targetApproved.Runtime) ||
		!reflect.DeepEqual(currentApproved.Observability, targetApproved.Observability) ||
		!reflect.DeepEqual(currentApproved.Network, targetApproved.Network) ||
		currentApproved.Exposure.ClassRef.Name != targetApproved.Exposure.ClassRef.Name {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassReplacementRequired, reasonReplacementWorkflowRequired
	}
	if currentApproved.Cluster.Version != targetApproved.Cluster.Version ||
		!reflect.DeepEqual(currentApproved.Lifecycle, targetApproved.Lifecycle) ||
		backupProfileSchedule(currentCatalog) != backupProfileSchedule(targetCatalog) ||
		!reflect.DeepEqual(backupProfileRetention(currentCatalog), backupProfileRetention(targetCatalog)) {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace, reasonInPlaceSupported
	}
	return openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace, reasonEquivalentServiceShape
}

func currentRevisionStatus(claim *openbaov1alpha1.OpenBaoClusterClaim) *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus {
	if claim == nil {
		return nil
	}
	return &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus{
		ServiceOfferingRef: localReferenceCopy(claim.Status.Applied.ServiceOfferingRef),
		ServiceProfileRef:  boundRevisionCopy(claim.Status.Applied.ServiceProfileRef),
		ApprovedContract:   contractIdentityCopy(claim.Status.Applied.ApprovedContract),
		RenderedContract:   contractIdentityCopy(claim.Status.Applied.RenderedContract),
	}
}

func revisionStatusFromResolvedTarget(
	offering *openbaov1alpha1.OpenBaoServiceOffering,
	profile *openbaov1alpha1.OpenBaoServiceProfile,
	approved *claimcontract.ApprovedServiceContract,
) *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus {
	var offeringRef *openbaov1alpha1.LocalReference
	if offering != nil {
		offeringRef = &openbaov1alpha1.LocalReference{Name: offering.Name}
	}
	status := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus{
		ServiceOfferingRef: offeringRef,
	}
	if profile != nil {
		status.ServiceProfileRef = &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{Name: profile.Name, UID: string(profile.UID)}
	}
	if approved != nil {
		status.ApprovedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(approved))
	}
	return status
}

func appliedRevisionMatchesTarget(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	target *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus,
) bool {
	if claim == nil || target == nil || target.ServiceProfileRef == nil || claim.Status.Applied.ServiceProfileRef == nil {
		return false
	}
	if claim.Status.Applied.ServiceProfileRef.Name != target.ServiceProfileRef.Name ||
		claim.Status.Applied.ServiceProfileRef.UID != target.ServiceProfileRef.UID {
		return false
	}
	if localRefName(claim.Status.Applied.ServiceOfferingRef) != localRefName(target.ServiceOfferingRef) {
		return false
	}
	return true
}

func revisionStatusFromContract(
	offering *openbaov1alpha1.LocalReference,
	profile *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference,
	approved *claimcontract.ApprovedServiceContract,
) *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus {
	status := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus{
		ServiceOfferingRef: localReferenceCopy(offering),
		ApprovedContract:   claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(approved)),
	}
	status.ServiceProfileRef = boundRevisionCopy(profile)
	return status
}

func classificationStatus(
	class openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClass,
	reason string,
) *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestClassificationStatus {
	return &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestClassificationStatus{Class: class, Reason: reason}
}

func revisionStatusCopy(
	status *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus,
) *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus {
	if status == nil {
		return nil
	}
	return &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus{
		ServiceOfferingRef: localReferenceCopy(status.ServiceOfferingRef),
		ServiceProfileRef:  boundRevisionCopy(status.ServiceProfileRef),
		ApprovedContract:   contractIdentityCopy(status.ApprovedContract),
		RenderedContract:   contractIdentityCopy(status.RenderedContract),
	}
}

func classificationStatusCopy(
	status *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestClassificationStatus,
) *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestClassificationStatus {
	if status == nil {
		return nil
	}
	copy := *status
	return &copy
}

func localReferenceCopy(ref *openbaov1alpha1.LocalReference) *openbaov1alpha1.LocalReference {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}

func boundRevisionCopy(ref *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference) *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}

func contractIdentityCopy(ref *openbaov1alpha1.OpenBaoClusterClaimContractIdentityStatus) *openbaov1alpha1.OpenBaoClusterClaimContractIdentityStatus {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}

func localRefName(ref *openbaov1alpha1.LocalReference) string {
	if ref == nil {
		return ""
	}
	return ref.Name
}

func upgradeRequestToken(request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) string {
	if request == nil {
		return ""
	}
	if request.UID != "" {
		return string(request.UID)
	}
	return types.NamespacedName{Namespace: request.Namespace, Name: request.Name}.String()
}

func claimUpgradeRequestToken(claim *openbaov1alpha1.OpenBaoClusterClaim) string {
	if claim == nil || claim.Annotations == nil {
		return ""
	}
	return claim.Annotations[constants.AnnotationClaimUpgradeRequest]
}

func backupTargetName(catalog *claimcontract.CatalogBundle) string {
	if catalog == nil || catalog.BackupTarget == nil {
		return ""
	}
	return catalog.BackupTarget.Name
}

func backupBackendName(catalog *claimcontract.CatalogBundle) string {
	if catalog == nil || catalog.BackupBackend == nil {
		return ""
	}
	return catalog.BackupBackend.Name
}

func backupAuthName(catalog *claimcontract.CatalogBundle) string {
	if catalog == nil || catalog.BackupAuth == nil {
		return ""
	}
	return catalog.BackupAuth.Name
}

func transferProfileName(catalog *claimcontract.CatalogBundle) string {
	if catalog == nil || catalog.TransferProfile == nil {
		return ""
	}
	return catalog.TransferProfile.Name
}

func backupProfileSchedule(catalog *claimcontract.CatalogBundle) string {
	if catalog == nil || catalog.BackupProfile == nil {
		return ""
	}
	return catalog.BackupProfile.Spec.Schedule
}

func backupProfileRetention(catalog *claimcontract.CatalogBundle) *openbaov1alpha1.BackupRetention {
	if catalog == nil || catalog.BackupProfile == nil || catalog.BackupProfile.Spec.Retention == nil {
		return nil
	}
	copy := *catalog.BackupProfile.Spec.Retention
	return &copy
}
