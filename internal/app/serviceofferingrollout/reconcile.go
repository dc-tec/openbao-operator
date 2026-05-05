package serviceofferingrollout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaoclusterclaim/requestworkflow"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

const (
	reasonOfferingNotFound                  = "OfferingNotFound"
	reasonOfferingReadFailed                = "OfferingReadFailed"
	reasonTargetRevisionNotCurrent          = "TargetRevisionNotCurrent"
	reasonTargetRevisionNotFound            = "TargetRevisionNotFound"
	reasonTargetRevisionReadFailed          = "TargetRevisionReadFailed"
	reasonClaimSelectorInvalid              = "ClaimSelectorInvalid"
	reasonClaimListFailed                   = "ClaimListFailed"
	reasonUpgradeRequestListFailed          = "UpgradeRequestListFailed"
	reasonUpgradeRequestCreateFailed        = "UpgradeRequestCreateFailed"
	reasonUnsupportedRolloutMode            = "UnsupportedRolloutMode"
	reasonNoEligibleClaims                  = "NoEligibleClaims"
	reasonAlreadyApplied                    = "AlreadyApplied"
	reasonAnotherUpgradeRequestActive       = "AnotherUpgradeRequestActive"
	reasonWaitingForRolloutSlot             = "WaitingForRolloutSlot"
	reasonUpgradeRequestCreated             = "UpgradeRequestCreated"
	reasonRolloutInProgress                 = "RolloutInProgress"
	reasonClaimUpgradeRequestBlocked        = "ClaimUpgradeRequestBlocked"
	reasonClaimUpgradeRequestFailed         = "ClaimUpgradeRequestFailed"
	reasonRolloutProgressing                = "RolloutProgressing"
	reasonRolloutCompleted                  = "RolloutCompleted"
	defaultRolloutMaxConcurrent       int32 = 1
)

type Reconciler interface {
	Reconcile(ctx context.Context, key types.NamespacedName, logger logr.Logger) (recon.Result, error)
}

type Runtime struct {
	Client              client.Client
	Reader              client.Reader
	Recorder            events.EventRecorder
	EnableServiceClaims bool
}

type runtimeReconciler struct {
	client              client.Client
	reader              client.Reader
	recorder            events.EventRecorder
	enableServiceClaims bool
}

type rolloutEvaluation struct {
	status openbaov1alpha1.OpenBaoServiceOfferingRolloutStatus
	err    error
}

type upgradeRequestInventory struct {
	ownedByClaim  map[types.NamespacedName]*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest
	activeByClaim map[types.NamespacedName]*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest
}

func NewReconciler(deps Runtime) Reconciler {
	reader := deps.Reader
	if reader == nil {
		reader = deps.Client
	}
	return runtimeReconciler{
		client:              deps.Client,
		reader:              reader,
		recorder:            deps.Recorder,
		enableServiceClaims: deps.EnableServiceClaims,
	}
}

func (r runtimeReconciler) Reconcile(ctx context.Context, key types.NamespacedName, logger logr.Logger) (recon.Result, error) {
	if r.client == nil {
		return recon.Result{}, fmt.Errorf("client is required")
	}

	rollout := &openbaov1alpha1.OpenBaoServiceOfferingRollout{}
	if err := r.client.Get(ctx, key, rollout); err != nil {
		if apierrors.IsNotFound(err) {
			return recon.Result{}, nil
		}
		return recon.Result{}, fmt.Errorf("failed to get OpenBaoServiceOfferingRollout: %w", err)
	}

	logger = logger.WithValues("openBaoServiceOfferingRollout", key.Name)
	original := rollout.DeepCopy()
	evaluation := r.evaluate(ctx, rollout)
	rollout.Status = evaluation.status

	if !reflect.DeepEqual(original.Status, rollout.Status) {
		if err := r.client.Status().Patch(ctx, rollout, client.MergeFrom(original)); err != nil {
			return recon.Result{}, fmt.Errorf("patch OpenBaoServiceOfferingRollout status: %w", err)
		}
		r.emitRolloutEvents(original, rollout)
		logger.Info("Reconciled OpenBaoServiceOfferingRollout", "state", rollout.Status.State, "reason", rollout.Status.Reason)
	}

	return requeueForStatus(rollout.Status), evaluation.err
}

func (r runtimeReconciler) emitRolloutEvents(
	original *openbaov1alpha1.OpenBaoServiceOfferingRollout,
	rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout,
) {
	if original == nil || rollout == nil {
		return
	}
	oldState := string(original.Status.State)
	newState := string(rollout.Status.State)
	oldReason := original.Status.Reason
	newReason := rollout.Status.Reason
	if requestworkflow.StateTransitionChanged(oldState, oldReason, newState, newReason) {
		requestworkflow.EmitEvent(
			r.recorder,
			rollout,
			requestworkflow.EventTypeForState(newState),
			requestworkflow.EventReason(newState, newReason, "OfferingRollout"),
			rolloutEventNote(rollout, fmt.Sprintf("Service offering rollout is %s", rollout.Status.State)),
		)
		return
	}
	if rolloutProgressChanged(original.Status, rollout.Status) {
		requestworkflow.EmitEvent(
			r.recorder,
			rollout,
			requestworkflow.EventTypeForState(newState),
			reasonRolloutProgressing,
			rolloutEventNote(rollout, "Service offering rollout progressed"),
		)
	}
}

func rolloutProgressChanged(
	oldStatus openbaov1alpha1.OpenBaoServiceOfferingRolloutStatus,
	newStatus openbaov1alpha1.OpenBaoServiceOfferingRolloutStatus,
) bool {
	return oldStatus.Total != newStatus.Total ||
		oldStatus.Pending != newStatus.Pending ||
		oldStatus.Running != newStatus.Running ||
		oldStatus.Succeeded != newStatus.Succeeded ||
		oldStatus.Blocked != newStatus.Blocked ||
		oldStatus.Failed != newStatus.Failed
}

func rolloutEventNote(rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout, prefix string) string {
	note := prefix
	if rollout.Status.Reason != "" {
		note = fmt.Sprintf("%s: %s", note, rollout.Status.Reason)
	}
	return fmt.Sprintf(
		"%s (total %d, pending %d, running %d, succeeded %d, blocked %d, failed %d)",
		note,
		rollout.Status.Total,
		rollout.Status.Pending,
		rollout.Status.Running,
		rollout.Status.Succeeded,
		rollout.Status.Blocked,
		rollout.Status.Failed,
	)
}

func (r runtimeReconciler) evaluate(
	ctx context.Context,
	rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout,
) rolloutEvaluation {
	status := openbaov1alpha1.OpenBaoServiceOfferingRolloutStatus{
		ObservedGeneration: rollout.Generation,
		State:              openbaov1alpha1.OpenBaoServiceOfferingRolloutStatePending,
	}
	if !r.enableServiceClaims {
		status.State = openbaov1alpha1.OpenBaoServiceOfferingRolloutStateBlocked
		status.Reason = requestworkflow.ReasonServiceClaimsDisabled
		return rolloutEvaluation{status: status}
	}
	if rolloutMode(rollout) != openbaov1alpha1.OpenBaoServiceOfferingRolloutModeInPlaceOnly {
		status.State = openbaov1alpha1.OpenBaoServiceOfferingRolloutStateBlocked
		status.Reason = reasonUnsupportedRolloutMode
		return rolloutEvaluation{status: status}
	}

	offering := &openbaov1alpha1.OpenBaoServiceOffering{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: rollout.Spec.OfferingRef.Name}, offering); err != nil {
		if apierrors.IsNotFound(err) {
			status.State = openbaov1alpha1.OpenBaoServiceOfferingRolloutStateBlocked
			status.Reason = reasonOfferingNotFound
			return rolloutEvaluation{status: status}
		}
		status.State = openbaov1alpha1.OpenBaoServiceOfferingRolloutStateFailed
		status.Reason = reasonOfferingReadFailed
		return rolloutEvaluation{status: status, err: fmt.Errorf("get OpenBaoServiceOffering %s: %w", rollout.Spec.OfferingRef.Name, err)}
	}
	if offering.Spec.CurrentRevisionRef.Name != rollout.Spec.TargetRevisionRef.Name {
		status.State = openbaov1alpha1.OpenBaoServiceOfferingRolloutStateBlocked
		status.Reason = reasonTargetRevisionNotCurrent
		return rolloutEvaluation{status: status}
	}

	targetProfile := &openbaov1alpha1.OpenBaoServiceProfile{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: rollout.Spec.TargetRevisionRef.Name}, targetProfile); err != nil {
		if apierrors.IsNotFound(err) {
			status.State = openbaov1alpha1.OpenBaoServiceOfferingRolloutStateBlocked
			status.Reason = reasonTargetRevisionNotFound
			return rolloutEvaluation{status: status}
		}
		status.State = openbaov1alpha1.OpenBaoServiceOfferingRolloutStateFailed
		status.Reason = reasonTargetRevisionReadFailed
		return rolloutEvaluation{status: status, err: fmt.Errorf("get OpenBaoServiceProfile %s: %w", rollout.Spec.TargetRevisionRef.Name, err)}
	}
	status.TargetRevisionRef = &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
		Name: targetProfile.Name,
		UID:  string(targetProfile.UID),
	}

	claimSelector, err := rolloutClaimSelector(rollout)
	if err != nil {
		status.State = openbaov1alpha1.OpenBaoServiceOfferingRolloutStateBlocked
		status.Reason = reasonClaimSelectorInvalid
		return rolloutEvaluation{status: status}
	}

	claims, err := r.selectedClaims(ctx, rollout, claimSelector)
	if err != nil {
		status.State = openbaov1alpha1.OpenBaoServiceOfferingRolloutStateFailed
		status.Reason = reasonClaimListFailed
		return rolloutEvaluation{status: status, err: err}
	}
	requests, err := r.upgradeRequestInventory(ctx, rollout)
	if err != nil {
		status.State = openbaov1alpha1.OpenBaoServiceOfferingRolloutStateFailed
		status.Reason = reasonUpgradeRequestListFailed
		return rolloutEvaluation{status: status, err: err}
	}

	return r.evaluateClaims(ctx, rollout, claims, requests, targetProfile, status)
}

func (r runtimeReconciler) selectedClaims(
	ctx context.Context,
	rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout,
	selector labels.Selector,
) ([]openbaov1alpha1.OpenBaoClusterClaim, error) {
	list := &openbaov1alpha1.OpenBaoClusterClaimList{}
	if err := r.reader.List(ctx, list); err != nil {
		return nil, fmt.Errorf("list OpenBaoClusterClaim for OpenBaoServiceOfferingRollout %s: %w", rollout.Name, err)
	}

	namespaceSet := rolloutNamespaceSet(rollout)
	claims := make([]openbaov1alpha1.OpenBaoClusterClaim, 0, len(list.Items))
	for i := range list.Items {
		claim := list.Items[i]
		if !claim.DeletionTimestamp.IsZero() ||
			!namespaceSelected(namespaceSet, claim.Namespace) ||
			!selector.Matches(labels.Set(claim.Labels)) ||
			claim.Status.Applied.ServiceOfferingRef == nil ||
			claim.Status.Applied.ServiceOfferingRef.Name != rollout.Spec.OfferingRef.Name {
			continue
		}
		claims = append(claims, claim)
	}
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].Namespace == claims[j].Namespace {
			return claims[i].Name < claims[j].Name
		}
		return claims[i].Namespace < claims[j].Namespace
	})
	return claims, nil
}

func (r runtimeReconciler) upgradeRequestInventory(
	ctx context.Context,
	rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout,
) (upgradeRequestInventory, error) {
	list := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestList{}
	if err := r.reader.List(ctx, list); err != nil {
		return upgradeRequestInventory{}, fmt.Errorf("list OpenBaoClusterClaimUpgradeRequest for OpenBaoServiceOfferingRollout %s: %w", rollout.Name, err)
	}

	inventory := upgradeRequestInventory{
		ownedByClaim:  make(map[types.NamespacedName]*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest),
		activeByClaim: make(map[types.NamespacedName]*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest),
	}
	for i := range list.Items {
		request := &list.Items[i]
		if request.Spec.ClaimRef.Name == "" {
			continue
		}
		claimKey := types.NamespacedName{Namespace: request.Namespace, Name: request.Spec.ClaimRef.Name}
		if requestIsActive(request) {
			current, ok := inventory.activeByClaim[claimKey]
			if !ok || requestIsEarlier(request, current) {
				inventory.activeByClaim[claimKey] = request.DeepCopy()
			}
		}
		if rolloutOwnsRequest(rollout, request) {
			current, ok := inventory.ownedByClaim[claimKey]
			if !ok || requestIsEarlier(request, current) {
				inventory.ownedByClaim[claimKey] = request.DeepCopy()
			}
		}
	}
	return inventory, nil
}

func (r runtimeReconciler) evaluateClaims(
	ctx context.Context,
	rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout,
	claims []openbaov1alpha1.OpenBaoClusterClaim,
	requests upgradeRequestInventory,
	targetProfile *openbaov1alpha1.OpenBaoServiceProfile,
	status openbaov1alpha1.OpenBaoServiceOfferingRolloutStatus,
) rolloutEvaluation {
	maxConcurrent := rolloutMaxConcurrent(rollout)
	active := activeRequestCount(requests.ownedByClaim)
	status.Total = int32(len(claims))
	status.Claims = make([]openbaov1alpha1.OpenBaoServiceOfferingRolloutClaimStatus, 0, len(claims))

	for i := range claims {
		claim := &claims[i]
		claimKey := types.NamespacedName{Namespace: claim.Namespace, Name: claim.Name}
		request := requests.ownedByClaim[claimKey]
		claimStatus := openbaov1alpha1.OpenBaoServiceOfferingRolloutClaimStatus{
			Namespace: claim.Namespace,
			Name:      claim.Name,
		}
		if claimAppliedTarget(claim, targetProfile) {
			claimStatus.State = openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded
			claimStatus.Reason = reasonAlreadyApplied
			if request != nil {
				claimStatus.RequestRef = requestRef(request)
			}
			status.Succeeded++
			status.Claims = append(status.Claims, claimStatus)
			continue
		}
		if request != nil {
			claimStatus.RequestRef = requestRef(request)
			claimStatus.State = normalizedRequestState(request.Status.State)
			claimStatus.Reason = request.Status.Reason
			incrementRolloutCount(&status, claimStatus.State)
			status.Claims = append(status.Claims, claimStatus)
			continue
		}
		if activeRequest := requests.activeByClaim[claimKey]; activeRequest != nil {
			claimStatus.RequestRef = requestRef(activeRequest)
			claimStatus.State = openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending
			claimStatus.Reason = reasonAnotherUpgradeRequestActive
			status.Pending++
			status.Claims = append(status.Claims, claimStatus)
			continue
		}
		if active >= maxConcurrent {
			claimStatus.State = openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending
			claimStatus.Reason = reasonWaitingForRolloutSlot
			status.Pending++
			status.Claims = append(status.Claims, claimStatus)
			continue
		}

		created, err := r.createUpgradeRequest(ctx, rollout, claim)
		if err != nil {
			claimStatus.State = openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed
			claimStatus.Reason = reasonUpgradeRequestCreateFailed
			status.Failed++
			status.Claims = append(status.Claims, claimStatus)
			status.State = openbaov1alpha1.OpenBaoServiceOfferingRolloutStateFailed
			status.Reason = reasonUpgradeRequestCreateFailed
			return rolloutEvaluation{status: status, err: err}
		}
		active++
		claimStatus.RequestRef = requestRef(created)
		claimStatus.State = openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending
		claimStatus.Reason = reasonUpgradeRequestCreated
		status.Pending++
		status.Claims = append(status.Claims, claimStatus)
	}

	status.State, status.Reason = aggregateRolloutState(status)
	return rolloutEvaluation{status: status}
}

func (r runtimeReconciler) createUpgradeRequest(
	ctx context.Context,
	rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) (*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest, error) {
	request := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: claim.Namespace,
			Name:      rolloutRequestName(rollout.Name, claim.Namespace, claim.Name),
			Labels: map[string]string{
				constants.LabelOpenBaoServiceOfferingRollout: rollout.Name,
				constants.LabelOpenBaoClaimNamespace:         claim.Namespace,
				constants.LabelOpenBaoClaimName:              claim.Name,
			},
			Annotations: map[string]string{
				constants.AnnotationServiceOfferingRolloutUID: string(rollout.UID),
			},
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
			Target: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
				ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: rollout.Spec.OfferingRef.Name},
			},
		},
	}
	if err := r.client.Create(ctx, request); err != nil {
		if apierrors.IsAlreadyExists(err) {
			existing := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{}
			if getErr := r.reader.Get(ctx, client.ObjectKeyFromObject(request), existing); getErr == nil && rolloutOwnsRequest(rollout, existing) {
				return existing, nil
			}
		}
		return nil, fmt.Errorf("create OpenBaoClusterClaimUpgradeRequest %s/%s for rollout %s: %w", request.Namespace, request.Name, rollout.Name, err)
	}
	return request, nil
}

func rolloutClaimSelector(rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout) (labels.Selector, error) {
	if rollout.Spec.Selector == nil || rollout.Spec.Selector.ClaimSelector == nil {
		return labels.Everything(), nil
	}
	return metav1.LabelSelectorAsSelector(rollout.Spec.Selector.ClaimSelector)
}

func rolloutNamespaceSet(rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout) map[string]struct{} {
	if rollout.Spec.Selector == nil || len(rollout.Spec.Selector.Namespaces) == 0 {
		return nil
	}
	namespaces := make(map[string]struct{}, len(rollout.Spec.Selector.Namespaces))
	for _, namespace := range rollout.Spec.Selector.Namespaces {
		if namespace == "" {
			continue
		}
		namespaces[namespace] = struct{}{}
	}
	return namespaces
}

func namespaceSelected(namespaces map[string]struct{}, namespace string) bool {
	if len(namespaces) == 0 {
		return true
	}
	_, ok := namespaces[namespace]
	return ok
}

func rolloutMode(rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout) openbaov1alpha1.OpenBaoServiceOfferingRolloutMode {
	if rollout.Spec.Strategy == nil || rollout.Spec.Strategy.Mode == "" {
		return openbaov1alpha1.OpenBaoServiceOfferingRolloutModeInPlaceOnly
	}
	return rollout.Spec.Strategy.Mode
}

func rolloutMaxConcurrent(rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout) int32 {
	if rollout.Spec.Strategy == nil || rollout.Spec.Strategy.MaxConcurrent == nil || *rollout.Spec.Strategy.MaxConcurrent < 1 {
		return defaultRolloutMaxConcurrent
	}
	return *rollout.Spec.Strategy.MaxConcurrent
}

func activeRequestCount(requests map[types.NamespacedName]*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) int32 {
	var active int32
	for _, request := range requests {
		if requestIsActive(request) {
			active++
		}
	}
	return active
}

func rolloutOwnsRequest(
	rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout,
	request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest,
) bool {
	if rollout == nil || request == nil || request.Labels[constants.LabelOpenBaoServiceOfferingRollout] != rollout.Name {
		return false
	}
	if rollout.UID == "" {
		return true
	}
	return request.Annotations[constants.AnnotationServiceOfferingRolloutUID] == string(rollout.UID)
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

func requestIsActive(request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) bool {
	if request == nil {
		return false
	}
	switch normalizedRequestState(request.Status.State) {
	case openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut:
		return true
	default:
		return false
	}
}

func normalizedRequestState(
	state openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState,
) openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState {
	if state == "" {
		return openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending
	}
	return state
}

func incrementRolloutCount(
	status *openbaov1alpha1.OpenBaoServiceOfferingRolloutStatus,
	state openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState,
) {
	switch normalizedRequestState(state) {
	case openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending:
		status.Pending++
	case openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut:
		status.Running++
	case openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded:
		status.Succeeded++
	case openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked:
		status.Blocked++
	case openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed:
		status.Failed++
	}
}

func aggregateRolloutState(status openbaov1alpha1.OpenBaoServiceOfferingRolloutStatus) (
	openbaov1alpha1.OpenBaoServiceOfferingRolloutState,
	string,
) {
	if status.Total == 0 {
		return openbaov1alpha1.OpenBaoServiceOfferingRolloutStateSucceeded, reasonNoEligibleClaims
	}
	if status.Failed > 0 {
		return openbaov1alpha1.OpenBaoServiceOfferingRolloutStateFailed, reasonClaimUpgradeRequestFailed
	}
	if status.Blocked > 0 {
		return openbaov1alpha1.OpenBaoServiceOfferingRolloutStateBlocked, reasonClaimUpgradeRequestBlocked
	}
	if status.Pending > 0 || status.Running > 0 {
		return openbaov1alpha1.OpenBaoServiceOfferingRolloutStateRunning, reasonRolloutInProgress
	}
	return openbaov1alpha1.OpenBaoServiceOfferingRolloutStateSucceeded, reasonRolloutCompleted
}

func requeueForStatus(status openbaov1alpha1.OpenBaoServiceOfferingRolloutStatus) recon.Result {
	if status.Pending > 0 || status.Running > 0 {
		return recon.Result{RequeueAfter: constants.RequeueShort}
	}
	return recon.Result{}
}

func claimAppliedTarget(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	targetProfile *openbaov1alpha1.OpenBaoServiceProfile,
) bool {
	return claim != nil &&
		targetProfile != nil &&
		claim.Status.Applied.ServiceProfileRef != nil &&
		claim.Status.Applied.ServiceProfileRef.Name == targetProfile.Name &&
		claim.Status.Applied.ServiceProfileRef.UID == string(targetProfile.UID)
}

func requestRef(request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) *openbaov1alpha1.NamespacedReference {
	if request == nil {
		return nil
	}
	return &openbaov1alpha1.NamespacedReference{Namespace: request.Namespace, Name: request.Name}
}

func rolloutRequestName(rolloutName, namespace, claimName string) string {
	sum := sha256.Sum256([]byte(namespace + "/" + claimName))
	suffix := hex.EncodeToString(sum[:])[:10]
	prefix := strings.Trim(rolloutName, "-.")
	if prefix == "" {
		prefix = "rollout"
	}
	maxPrefixLen := 253 - 1 - len(suffix)
	if len(prefix) > maxPrefixLen {
		prefix = strings.TrimRight(prefix[:maxPrefixLen], "-.")
	}
	if prefix == "" {
		prefix = "rollout"
	}
	return fmt.Sprintf("%s-%s", prefix, suffix)
}
