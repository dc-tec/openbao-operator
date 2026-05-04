package restorerequest

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

const (
	reasonServiceClaimsDisabled              = "ServiceClaimsDisabled"
	reasonClaimNotFound                      = "ClaimNotFound"
	reasonClaimReadFailed                    = "ClaimReadFailed"
	reasonRestoreRequestListFailed           = "RestoreRequestListFailed"
	reasonAnotherRestoreRequestActive        = "AnotherRestoreRequestActive"
	reasonClaimDeleting                      = "ClaimDeleting"
	reasonClaimNotMaterializedForSameCluster = "ClaimNotMaterializedForSameCluster"
	reasonLocalClusterNotFound               = "LocalClusterNotFound"
	reasonLocalClusterReadFailed             = "LocalClusterReadFailed"
	reasonLocalClusterDeleting               = "LocalClusterDeleting"
	reasonBackupNotConfigured                = "BackupNotConfigured"
	reasonRestoreExecutionReadFailed         = "RestoreExecutionReadFailed"
	reasonRestoreImageResolutionFailed       = "RestoreImageResolutionFailed"
	reasonRestoreExecutionListFailed         = "RestoreExecutionListFailed"
	reasonAnotherRestoreExecutionActive      = "AnotherRestoreExecutionActive"
	reasonRestoreExecutionNameConflict       = "RestoreExecutionNameConflict"
	reasonRestoreCreateFailed                = "RestoreCreateFailed"
	reasonRestoreRequested                   = "RestoreRequested"
	reasonInvalidRestoreSource               = "InvalidRestoreSource"
	reasonNoSuccessfulBackupAvailable        = "NoSuccessfulBackupAvailable"
	reasonBackupRequestRefRequired           = "BackupRequestRefRequired"
	reasonBackupRequestNotFound              = "BackupRequestNotFound"
	reasonBackupRequestReadFailed            = "BackupRequestReadFailed"
	reasonBackupRequestClaimMismatch         = "BackupRequestClaimMismatch"
	reasonBackupRequestNotSucceeded          = "BackupRequestNotSucceeded"
	reasonBackupRequestClusterUnknown        = "BackupRequestClusterUnknown"
	reasonBackupRequestClusterMismatch       = "BackupRequestClusterMismatch"
	reasonBackupRequestSnapshotMissing       = "BackupRequestSnapshotMissing"
	reasonRestorePending                     = "RestorePending"
	reasonRestoreFailed                      = "RestoreFailed"
	reasonRestoreCompleted                   = "RestoreCompleted"
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

type requestEvaluation struct {
	state          openbaov1alpha1.OpenBaoClusterClaimRestoreRequestState
	reason         string
	clusterRef     *openbaov1alpha1.NamespacedReference
	restoreRef     *openbaov1alpha1.NamespacedReference
	startTime      *metav1.Time
	completionTime *metav1.Time
	snapshotKey    string
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

	request := &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{}
	if err := r.client.Get(ctx, key, request); err != nil {
		if apierrors.IsNotFound(err) {
			return recon.Result{}, nil
		}
		return recon.Result{}, fmt.Errorf("failed to get OpenBaoClusterClaimRestoreRequest: %w", err)
	}

	logger = logger.WithValues("openBaoClusterClaimRestoreRequest", key.String())
	original := request.DeepCopy()

	evaluation := r.reconcileRequestState(ctx, request)
	request.Status.ObservedGeneration = request.Generation
	request.Status.State = evaluation.state
	request.Status.Reason = evaluation.reason
	request.Status.ClusterRef = evaluation.clusterRef
	request.Status.RestoreRef = evaluation.restoreRef
	request.Status.StartTime = evaluation.startTime
	request.Status.CompletionTime = evaluation.completionTime
	request.Status.SnapshotKey = evaluation.snapshotKey
	request.Status.Conditions = nil

	if reflect.DeepEqual(original.Status, request.Status) {
		logger.V(1).Info("OpenBaoClusterClaimRestoreRequest status already up to date")
		return recon.Result{}, nil
	}
	if err := r.client.Status().Patch(ctx, request, client.MergeFrom(original)); err != nil {
		return recon.Result{}, fmt.Errorf("patch OpenBaoClusterClaimRestoreRequest status: %w", err)
	}

	logger.Info("Reconciled OpenBaoClusterClaimRestoreRequest", "state", evaluation.state, "reason", evaluation.reason)
	return recon.Result{}, nil
}

func (r runtimeReconciler) reconcileRequestState(
	ctx context.Context,
	request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest,
) requestEvaluation {
	if request == nil {
		return requestEvaluation{}
	}
	if isTerminalRequestState(request.Status.State) {
		return requestEvaluation{
			state:          request.Status.State,
			reason:         request.Status.Reason,
			clusterRef:     namespacedReferenceCopy(request.Status.ClusterRef),
			restoreRef:     namespacedReferenceCopy(request.Status.RestoreRef),
			startTime:      timeCopy(request.Status.StartTime),
			completionTime: timeCopy(request.Status.CompletionTime),
			snapshotKey:    request.Status.SnapshotKey,
		}
	}
	if !r.enableServiceClaims {
		return requestEvaluation{
			state:  openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked,
			reason: reasonServiceClaimsDisabled,
		}
	}

	claim := &openbaov1alpha1.OpenBaoClusterClaim{}
	if err := r.reader.Get(ctx, types.NamespacedName{Namespace: request.Namespace, Name: request.Spec.ClaimRef.Name}, claim); err != nil {
		if apierrors.IsNotFound(err) {
			return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed, reason: reasonClaimNotFound}
		}
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed, reason: reasonClaimReadFailed}
	}
	if other, err := r.findEarlierActiveRequest(ctx, request); err != nil {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed, reason: reasonRestoreRequestListFailed}
	} else if other != nil {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked, reason: reasonAnotherRestoreRequestActive}
	}
	if !claim.DeletionTimestamp.IsZero() {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked, reason: reasonClaimDeleting}
	}
	if claim.Status.Materialization.Mode != openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster || claim.Status.Materialization.LocalRef == nil {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked, reason: reasonClaimNotMaterializedForSameCluster}
	}

	clusterRef := &openbaov1alpha1.NamespacedReference{
		Namespace: claim.Status.Materialization.LocalRef.Namespace,
		Name:      claim.Status.Materialization.LocalRef.Name,
	}
	localCluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := r.reader.Get(ctx, types.NamespacedName{Namespace: clusterRef.Namespace, Name: clusterRef.Name}, localCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed, reason: reasonLocalClusterNotFound, clusterRef: clusterRef}
		}
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed, reason: reasonLocalClusterReadFailed, clusterRef: clusterRef}
	}
	if !localCluster.DeletionTimestamp.IsZero() {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked, reason: reasonLocalClusterDeleting, clusterRef: clusterRef}
	}
	if localCluster.Spec.Backup == nil {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked, reason: reasonBackupNotConfigured, clusterRef: clusterRef}
	}
	snapshot := r.resolveRestoreSnapshotKey(ctx, request, localCluster)
	if snapshot.failedReason != "" {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed, reason: snapshot.failedReason, clusterRef: clusterRef}
	}
	if snapshot.blockedReason != "" {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked, reason: snapshot.blockedReason, clusterRef: clusterRef}
	}
	snapshotKey := snapshot.key

	restore, err := r.resolveOwnedRestoreExecution(ctx, request)
	if err != nil {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed, reason: reasonRestoreExecutionReadFailed, clusterRef: clusterRef, snapshotKey: snapshotKey}
	}
	if restore != nil {
		return observeRestoreExecution(restore, clusterRef)
	}

	restoreImage, err := resolveRestoreExecutionImage(localCluster)
	if err != nil {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed, reason: reasonRestoreImageResolutionFailed, clusterRef: clusterRef, snapshotKey: snapshotKey}
	}

	if other, err := r.findConflictingActiveRestore(ctx, clusterRef, request.Name); err != nil {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed, reason: reasonRestoreExecutionListFailed, clusterRef: clusterRef, snapshotKey: snapshotKey}
	} else if other != nil {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked, reason: reasonAnotherRestoreExecutionActive, clusterRef: clusterRef, snapshotKey: snapshotKey}
	}

	desired := desiredRestoreExecution(request, localCluster, snapshotKey, restoreImage)
	if err := r.client.Create(ctx, desired); err != nil {
		if apierrors.IsAlreadyExists(err) {
			existing := &openbaov1alpha1.OpenBaoRestore{}
			if getErr := r.reader.Get(ctx, client.ObjectKeyFromObject(desired), existing); getErr != nil {
				return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed, reason: reasonRestoreExecutionReadFailed, clusterRef: clusterRef, snapshotKey: snapshotKey}
			}
			if existing.Labels[constants.LabelOpenBaoClaimRestoreRequest] != request.Name {
				return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked, reason: reasonRestoreExecutionNameConflict, clusterRef: clusterRef, snapshotKey: snapshotKey}
			}
			return observeRestoreExecution(existing, clusterRef)
		}
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed, reason: reasonRestoreCreateFailed, clusterRef: clusterRef, snapshotKey: snapshotKey}
	}

	return requestEvaluation{
		state:       openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending,
		reason:      reasonRestoreRequested,
		clusterRef:  clusterRef,
		restoreRef:  &openbaov1alpha1.NamespacedReference{Namespace: desired.Namespace, Name: desired.Name},
		snapshotKey: snapshotKey,
	}
}

func (r runtimeReconciler) findEarlierActiveRequest(
	ctx context.Context,
	request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest,
) (*openbaov1alpha1.OpenBaoClusterClaimRestoreRequest, error) {
	if request == nil || request.Namespace == "" || request.Spec.ClaimRef.Name == "" {
		return nil, nil
	}

	list := &openbaov1alpha1.OpenBaoClusterClaimRestoreRequestList{}
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

func (r runtimeReconciler) resolveOwnedRestoreExecution(
	ctx context.Context,
	request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest,
) (*openbaov1alpha1.OpenBaoRestore, error) {
	if request == nil || request.Namespace == "" {
		return nil, nil
	}
	if request.Status.RestoreRef != nil &&
		strings.TrimSpace(request.Status.RestoreRef.Namespace) != "" &&
		strings.TrimSpace(request.Status.RestoreRef.Name) != "" {
		restore := &openbaov1alpha1.OpenBaoRestore{}
		key := types.NamespacedName{Namespace: request.Status.RestoreRef.Namespace, Name: request.Status.RestoreRef.Name}
		if err := r.reader.Get(ctx, key, restore); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		if restore.Labels[constants.LabelOpenBaoClaimRestoreRequest] == request.Name {
			return restore, nil
		}
		return nil, nil
	}

	list := &openbaov1alpha1.OpenBaoRestoreList{}
	clusterNamespace := ""
	if request.Status.ClusterRef != nil {
		clusterNamespace = strings.TrimSpace(request.Status.ClusterRef.Namespace)
	}
	if clusterNamespace == "" {
		clusterNamespace = request.Namespace
	}
	if err := r.reader.List(ctx, list, client.InNamespace(clusterNamespace)); err != nil {
		return nil, err
	}

	var owned *openbaov1alpha1.OpenBaoRestore
	for i := range list.Items {
		candidate := &list.Items[i]
		if candidate.Labels[constants.LabelOpenBaoClaimNamespace] != request.Namespace ||
			candidate.Labels[constants.LabelOpenBaoClaimRestoreRequest] != request.Name {
			continue
		}
		if owned == nil || restoreIsEarlier(candidate, owned) {
			owned = candidate
		}
	}
	return owned, nil
}

func (r runtimeReconciler) findConflictingActiveRestore(
	ctx context.Context,
	clusterRef *openbaov1alpha1.NamespacedReference,
	ownedName string,
) (*openbaov1alpha1.OpenBaoRestore, error) {
	if clusterRef == nil || clusterRef.Namespace == "" || clusterRef.Name == "" {
		return nil, nil
	}

	list := &openbaov1alpha1.OpenBaoRestoreList{}
	if err := r.reader.List(ctx, list, client.InNamespace(clusterRef.Namespace)); err != nil {
		return nil, err
	}

	for i := range list.Items {
		candidate := &list.Items[i]
		if !candidate.DeletionTimestamp.IsZero() ||
			candidate.Spec.Cluster != clusterRef.Name ||
			isTerminalExecutionPhase(candidate.Status.Phase) ||
			candidate.Name == ownedName {
			continue
		}
		return candidate, nil
	}
	return nil, nil
}

func desiredRestoreExecution(
	request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest,
	cluster *openbaov1alpha1.OpenBaoCluster,
	snapshotKey string,
	image string,
) *openbaov1alpha1.OpenBaoRestore {
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      request.Name,
			Labels: map[string]string{
				constants.LabelOpenBaoClaimNamespace:      request.Namespace,
				constants.LabelOpenBaoClaimName:           request.Spec.ClaimRef.Name,
				constants.LabelOpenBaoClaimRestoreRequest: request.Name,
			},
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: cluster.Name,
			Source: openbaov1alpha1.RestoreSource{
				Target: cluster.Spec.Backup.Target,
				Key:    snapshotKey,
			},
			Image: image,
		},
	}
	if cluster.Spec.Restore != nil {
		restore.Spec.JWTAuthRole = strings.TrimSpace(cluster.Spec.Restore.JWTAuthRole)
	}
	if restore.Spec.JWTAuthRole == "" && shouldUseRootTokenFallback(cluster) {
		restore.Spec.TokenSecretRef = &corev1.LocalObjectReference{Name: cluster.Name + constants.SuffixRootToken}
	}
	return restore
}

func resolveRestoreExecutionImage(cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if cluster != nil && cluster.Spec.Restore != nil && strings.TrimSpace(cluster.Spec.Restore.Image) != "" {
		return strings.TrimSpace(cluster.Spec.Restore.Image), nil
	}
	if cluster != nil && cluster.Spec.Backup != nil && strings.TrimSpace(cluster.Spec.Backup.Image) != "" {
		return strings.TrimSpace(cluster.Spec.Backup.Image), nil
	}
	return constants.DefaultBackupImage()
}

func shouldUseRootTokenFallback(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil {
		return false
	}
	if portauth.OperatorJWTBootstrapEnabled(cluster) {
		return false
	}
	if cluster.Spec.Restore != nil && strings.TrimSpace(cluster.Spec.Restore.JWTAuthRole) != "" {
		return false
	}
	return cluster.Spec.SelfInit == nil || !cluster.Spec.SelfInit.Enabled
}

func latestSuccessfulBackupKey(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Status.Backup == nil {
		return ""
	}
	return strings.TrimSpace(cluster.Status.Backup.LastBackupName)
}

type restoreSnapshotResolution struct {
	key           string
	blockedReason string
	failedReason  string
}

func (r runtimeReconciler) resolveRestoreSnapshotKey(
	ctx context.Context,
	request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest,
	localCluster *openbaov1alpha1.OpenBaoCluster,
) restoreSnapshotResolution {
	source := request.Spec.Source
	mode := openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeLatestSuccessful
	if source != nil && source.Mode != "" {
		mode = source.Mode
	}

	switch mode {
	case openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeLatestSuccessful:
		if source != nil && source.BackupRequestRef != nil {
			return restoreSnapshotResolution{blockedReason: reasonInvalidRestoreSource}
		}
		snapshotKey := strings.TrimSpace(latestSuccessfulBackupKey(localCluster))
		if snapshotKey == "" {
			return restoreSnapshotResolution{blockedReason: reasonNoSuccessfulBackupAvailable}
		}
		return restoreSnapshotResolution{key: snapshotKey}
	case openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeBackupRequest:
		return r.resolveBackupRequestSnapshotKey(ctx, request, localCluster)
	default:
		return restoreSnapshotResolution{blockedReason: reasonInvalidRestoreSource}
	}
}

func (r runtimeReconciler) resolveBackupRequestSnapshotKey(
	ctx context.Context,
	request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest,
	localCluster *openbaov1alpha1.OpenBaoCluster,
) restoreSnapshotResolution {
	if request == nil || request.Spec.Source == nil || request.Spec.Source.BackupRequestRef == nil ||
		strings.TrimSpace(request.Spec.Source.BackupRequestRef.Name) == "" {
		return restoreSnapshotResolution{blockedReason: reasonBackupRequestRefRequired}
	}

	backupRequestName := strings.TrimSpace(request.Spec.Source.BackupRequestRef.Name)
	backupRequest := &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{}
	key := types.NamespacedName{Namespace: request.Namespace, Name: backupRequestName}
	if err := r.reader.Get(ctx, key, backupRequest); err != nil {
		if apierrors.IsNotFound(err) {
			return restoreSnapshotResolution{blockedReason: reasonBackupRequestNotFound}
		}
		return restoreSnapshotResolution{failedReason: reasonBackupRequestReadFailed}
	}
	if backupRequest.Spec.ClaimRef.Name != request.Spec.ClaimRef.Name {
		return restoreSnapshotResolution{blockedReason: reasonBackupRequestClaimMismatch}
	}
	if backupRequest.Status.State != openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateSucceeded {
		return restoreSnapshotResolution{blockedReason: reasonBackupRequestNotSucceeded}
	}
	if backupRequest.Status.ClusterRef == nil {
		return restoreSnapshotResolution{blockedReason: reasonBackupRequestClusterUnknown}
	}
	if !backupRequestClusterMatches(backupRequest.Status.ClusterRef, localCluster) {
		return restoreSnapshotResolution{blockedReason: reasonBackupRequestClusterMismatch}
	}

	snapshotKey := strings.TrimSpace(backupRequest.Status.SnapshotKey)
	if snapshotKey == "" {
		return restoreSnapshotResolution{blockedReason: reasonBackupRequestSnapshotMissing}
	}
	return restoreSnapshotResolution{key: snapshotKey}
}

func backupRequestClusterMatches(ref *openbaov1alpha1.NamespacedReference, cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return ref != nil &&
		cluster != nil &&
		ref.Namespace == cluster.Namespace &&
		ref.Name == cluster.Name
}

func observeRestoreExecution(
	restore *openbaov1alpha1.OpenBaoRestore,
	clusterRef *openbaov1alpha1.NamespacedReference,
) requestEvaluation {
	if restore == nil {
		return requestEvaluation{state: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending, reason: reasonRestorePending, clusterRef: clusterRef}
	}

	restoreRef := &openbaov1alpha1.NamespacedReference{Namespace: restore.Namespace, Name: restore.Name}
	phase := restore.Status.Phase
	if phase == "" {
		phase = openbaov1alpha1.RestorePhasePending
	}
	snapshotKey := strings.TrimSpace(restore.Status.SnapshotKey)
	if snapshotKey == "" {
		snapshotKey = strings.TrimSpace(restore.Spec.Source.Key)
	}

	switch phase {
	case openbaov1alpha1.RestorePhaseCompleted:
		return requestEvaluation{
			state:          openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateSucceeded,
			reason:         reasonRestoreCompleted,
			clusterRef:     clusterRef,
			restoreRef:     restoreRef,
			startTime:      timeCopy(restore.Status.StartTime),
			completionTime: timeCopy(restore.Status.CompletionTime),
			snapshotKey:    snapshotKey,
		}
	case openbaov1alpha1.RestorePhaseFailed:
		return requestEvaluation{
			state:          openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed,
			reason:         restoreExecutionReason(restore),
			clusterRef:     clusterRef,
			restoreRef:     restoreRef,
			startTime:      timeCopy(restore.Status.StartTime),
			completionTime: timeCopy(restore.Status.CompletionTime),
			snapshotKey:    snapshotKey,
		}
	case openbaov1alpha1.RestorePhaseValidating, openbaov1alpha1.RestorePhaseRunning:
		return requestEvaluation{
			state:       openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning,
			reason:      restoreExecutionReason(restore),
			clusterRef:  clusterRef,
			restoreRef:  restoreRef,
			startTime:   timeCopy(restore.Status.StartTime),
			snapshotKey: snapshotKey,
		}
	default:
		return requestEvaluation{
			state:       openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending,
			reason:      restoreExecutionReason(restore),
			clusterRef:  clusterRef,
			restoreRef:  restoreRef,
			startTime:   timeCopy(restore.Status.StartTime),
			snapshotKey: snapshotKey,
		}
	}
}

func restoreExecutionReason(restore *openbaov1alpha1.OpenBaoRestore) string {
	if restore == nil {
		return ""
	}
	if restore.Status.Phase == openbaov1alpha1.RestorePhaseFailed {
		for i := range restore.Status.Conditions {
			condition := restore.Status.Conditions[i]
			if condition.Status == metav1.ConditionFalse && strings.TrimSpace(condition.Reason) != "" {
				return condition.Reason
			}
		}
		return reasonRestoreFailed
	}
	if restore.Status.Phase == openbaov1alpha1.RestorePhaseCompleted {
		return reasonRestoreCompleted
	}
	if restore.Status.Phase == "" {
		return reasonRestorePending
	}
	return string(restore.Status.Phase)
}

func isTerminalExecutionPhase(phase openbaov1alpha1.RestorePhase) bool {
	switch phase {
	case openbaov1alpha1.RestorePhaseCompleted, openbaov1alpha1.RestorePhaseFailed:
		return true
	default:
		return false
	}
}

func isTerminalRequestState(state openbaov1alpha1.OpenBaoClusterClaimRestoreRequestState) bool {
	switch state {
	case openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateSucceeded,
		openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked,
		openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed:
		return true
	default:
		return false
	}
}

func requestIsEarlier(a, b *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest) bool {
	if a == nil || b == nil {
		return false
	}
	if a.CreationTimestamp.Equal(&b.CreationTimestamp) {
		return a.Name < b.Name
	}
	return a.CreationTimestamp.Time.Before(b.CreationTimestamp.Time)
}

func restoreIsEarlier(a, b *openbaov1alpha1.OpenBaoRestore) bool {
	if a == nil || b == nil {
		return false
	}
	if a.CreationTimestamp.Equal(&b.CreationTimestamp) {
		return a.Name < b.Name
	}
	return a.CreationTimestamp.Time.Before(b.CreationTimestamp.Time)
}

func namespacedReferenceCopy(ref *openbaov1alpha1.NamespacedReference) *openbaov1alpha1.NamespacedReference {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}

func timeCopy(value *metav1.Time) *metav1.Time {
	if value == nil {
		return nil
	}
	return value.DeepCopy()
}
