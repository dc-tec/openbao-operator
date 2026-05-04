package backuprequest

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaoclusterclaim/requestworkflow"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

const (
	reasonBackupRequestListFailed    = "BackupRequestListFailed"
	reasonAnotherBackupRequestActive = "AnotherBackupRequestActive"
	reasonLocalClusterNotFound       = "LocalClusterNotFound"
	reasonLocalClusterReadFailed     = "LocalClusterReadFailed"
	reasonLocalClusterDeleting       = "LocalClusterDeleting"
	reasonTriggerUpdateFailed        = "TriggerUpdateFailed"
	reasonBackupRequested            = "BackupRequested"
	reasonBackupInProgress           = "BackupInProgress"
	reasonBackupFailed               = "BackupFailed"
	reasonBackupCompleted            = "BackupCompleted"
	reasonBackupCompletionPending    = "BackupCompletionPending"
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
	state          openbaov1alpha1.OpenBaoClusterClaimBackupRequestState
	reason         string
	clusterRef     *openbaov1alpha1.NamespacedReference
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

	request := &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{}
	if err := r.client.Get(ctx, key, request); err != nil {
		if apierrors.IsNotFound(err) {
			return recon.Result{}, nil
		}
		return recon.Result{}, fmt.Errorf("failed to get OpenBaoClusterClaimBackupRequest: %w", err)
	}

	logger = logger.WithValues("openBaoClusterClaimBackupRequest", key.String())
	original := request.DeepCopy()

	evaluation := r.reconcileRequestState(ctx, request)
	request.Status.ObservedGeneration = request.Generation
	request.Status.State = evaluation.state
	request.Status.Reason = evaluation.reason
	request.Status.ClusterRef = evaluation.clusterRef
	request.Status.StartTime = evaluation.startTime
	request.Status.CompletionTime = evaluation.completionTime
	request.Status.SnapshotKey = evaluation.snapshotKey
	request.Status.Conditions = nil

	if reflect.DeepEqual(original.Status, request.Status) {
		logger.V(1).Info("OpenBaoClusterClaimBackupRequest status already up to date")
		return recon.Result{}, nil
	}
	if err := r.client.Status().Patch(ctx, request, client.MergeFrom(original)); err != nil {
		return recon.Result{}, fmt.Errorf("patch OpenBaoClusterClaimBackupRequest status: %w", err)
	}

	logger.Info("Reconciled OpenBaoClusterClaimBackupRequest", "state", evaluation.state, "reason", evaluation.reason)
	return recon.Result{}, nil
}

func (r runtimeReconciler) reconcileRequestState(
	ctx context.Context,
	request *openbaov1alpha1.OpenBaoClusterClaimBackupRequest,
) requestEvaluation {
	if request == nil {
		return requestEvaluation{}
	}
	if isTerminalRequestState(request.Status.State) {
		return requestEvaluation{
			state:          request.Status.State,
			reason:         request.Status.Reason,
			clusterRef:     namespacedReferenceCopy(request.Status.ClusterRef),
			startTime:      timeCopy(request.Status.StartTime),
			completionTime: timeCopy(request.Status.CompletionTime),
			snapshotKey:    request.Status.SnapshotKey,
		}
	}
	if !r.enableServiceClaims {
		return requestEvaluation{
			state:  openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateBlocked,
			reason: requestworkflow.ReasonServiceClaimsDisabled,
		}
	}

	claim := &openbaov1alpha1.OpenBaoClusterClaim{}
	if err := r.reader.Get(ctx, types.NamespacedName{Namespace: request.Namespace, Name: request.Spec.ClaimRef.Name}, claim); err != nil {
		if apierrors.IsNotFound(err) {
			return requestEvaluation{
				state:  openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed,
				reason: requestworkflow.ReasonClaimNotFound,
			}
		}
		return requestEvaluation{
			state:  openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed,
			reason: requestworkflow.ReasonClaimReadFailed,
		}
	}
	if other, err := r.findEarlierActiveRequest(ctx, request); err != nil {
		return requestEvaluation{
			state:  openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed,
			reason: reasonBackupRequestListFailed,
		}
	} else if other != nil {
		return requestEvaluation{
			state:  openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateBlocked,
			reason: reasonAnotherBackupRequestActive,
		}
	}
	if !claim.DeletionTimestamp.IsZero() {
		return requestEvaluation{
			state:  openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateBlocked,
			reason: requestworkflow.ReasonClaimDeleting,
		}
	}
	if claim.Status.Materialization.Mode != openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster || claim.Status.Materialization.LocalRef == nil {
		return requestEvaluation{
			state:  openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateBlocked,
			reason: requestworkflow.ReasonClaimNotMaterializedForSameCluster,
		}
	}

	clusterRef := &openbaov1alpha1.NamespacedReference{
		Namespace: claim.Status.Materialization.LocalRef.Namespace,
		Name:      claim.Status.Materialization.LocalRef.Name,
	}
	localCluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := r.reader.Get(ctx, types.NamespacedName{Namespace: clusterRef.Namespace, Name: clusterRef.Name}, localCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return requestEvaluation{
				state:      openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed,
				reason:     reasonLocalClusterNotFound,
				clusterRef: clusterRef,
			}
		}
		return requestEvaluation{
			state:      openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed,
			reason:     reasonLocalClusterReadFailed,
			clusterRef: clusterRef,
		}
	}
	if !localCluster.DeletionTimestamp.IsZero() {
		return requestEvaluation{
			state:      openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateBlocked,
			reason:     reasonLocalClusterDeleting,
			clusterRef: clusterRef,
		}
	}

	triggerToken := backupTriggerToken(request)
	if backupAttemptObserved(localCluster.Status.Backup, triggerToken) {
		return observeTriggeredRequest(localCluster, clusterRef)
	}
	if err := r.ensureTriggerAnnotation(ctx, localCluster, triggerToken); err != nil {
		return requestEvaluation{
			state:      openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed,
			reason:     reasonTriggerUpdateFailed,
			clusterRef: clusterRef,
		}
	}
	return requestEvaluation{
		state:      openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatePending,
		reason:     reasonBackupRequested,
		clusterRef: clusterRef,
	}
}

func (r runtimeReconciler) findEarlierActiveRequest(
	ctx context.Context,
	request *openbaov1alpha1.OpenBaoClusterClaimBackupRequest,
) (*openbaov1alpha1.OpenBaoClusterClaimBackupRequest, error) {
	if request == nil || request.Namespace == "" || request.Spec.ClaimRef.Name == "" {
		return nil, nil
	}

	list := &openbaov1alpha1.OpenBaoClusterClaimBackupRequestList{}
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

func (r runtimeReconciler) ensureTriggerAnnotation(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
	triggerToken string,
) error {
	if cluster == nil || triggerToken == "" {
		return fmt.Errorf("cluster and trigger token are required")
	}

	original := cluster.DeepCopy()
	if cluster.Annotations == nil {
		cluster.Annotations = map[string]string{}
	}
	if cluster.Annotations[constants.AnnotationTriggerBackup] == triggerToken {
		return nil
	}
	cluster.Annotations[constants.AnnotationTriggerBackup] = triggerToken
	return r.client.Patch(ctx, cluster, client.MergeFrom(original))
}

func observeTriggeredRequest(
	localCluster *openbaov1alpha1.OpenBaoCluster,
	clusterRef *openbaov1alpha1.NamespacedReference,
) requestEvaluation {
	startTime := localCluster.Status.Backup.LastAttemptTime.DeepCopy()
	if localClusterBackupInProgress(localCluster) {
		return requestEvaluation{
			state:      openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateRunning,
			reason:     reasonBackupInProgress,
			clusterRef: clusterRef,
			startTime:  startTime,
		}
	}
	if backupAttemptFailed(localCluster.Status.Backup) {
		completion := localCluster.Status.Backup.LastFailureTime.DeepCopy()
		reason := localCluster.Status.Backup.LastFailureReason
		if reason == "" {
			reason = reasonBackupFailed
		}
		return requestEvaluation{
			state:          openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed,
			reason:         reason,
			clusterRef:     clusterRef,
			startTime:      startTime,
			completionTime: completion,
		}
	}
	if backupAttemptSucceeded(localCluster.Status.Backup) {
		completion := localCluster.Status.Backup.LastBackupTime.DeepCopy()
		return requestEvaluation{
			state:          openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateSucceeded,
			reason:         reasonBackupCompleted,
			clusterRef:     clusterRef,
			startTime:      startTime,
			completionTime: completion,
			snapshotKey:    localCluster.Status.Backup.LastBackupName,
		}
	}
	return requestEvaluation{
		state:       openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateRunning,
		reason:      reasonBackupCompletionPending,
		clusterRef:  clusterRef,
		startTime:   startTime,
		snapshotKey: localCluster.Status.Backup.LastBackupName,
	}
}

func backupAttemptObserved(status *openbaov1alpha1.BackupStatus, triggerToken string) bool {
	return status != nil && triggerToken != "" && status.LastHandledManualTrigger == triggerToken && status.LastAttemptTime != nil
}

func backupAttemptSucceeded(status *openbaov1alpha1.BackupStatus) bool {
	return status != nil && status.LastAttemptTime != nil && status.LastBackupTime != nil && !status.LastBackupTime.Before(status.LastAttemptTime)
}

func backupAttemptFailed(status *openbaov1alpha1.BackupStatus) bool {
	return status != nil && status.LastAttemptTime != nil && status.LastFailureTime != nil && !status.LastFailureTime.Before(status.LastAttemptTime)
}

func backupTriggerToken(request *openbaov1alpha1.OpenBaoClusterClaimBackupRequest) string {
	if request == nil {
		return ""
	}
	if request.UID != "" {
		return string(request.UID)
	}
	return types.NamespacedName{Namespace: request.Namespace, Name: request.Name}.String()
}

func localClusterBackupInProgress(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil {
		return false
	}
	if cluster.Status.Phase == openbaov1alpha1.ClusterPhaseBackingUp {
		return true
	}
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionBackingUp))
	return condition != nil && condition.Status == metav1.ConditionTrue
}

func isTerminalRequestState(state openbaov1alpha1.OpenBaoClusterClaimBackupRequestState) bool {
	switch state {
	case openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateSucceeded,
		openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateBlocked,
		openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed:
		return true
	default:
		return false
	}
}

func requestIsEarlier(a, b *openbaov1alpha1.OpenBaoClusterClaimBackupRequest) bool {
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
