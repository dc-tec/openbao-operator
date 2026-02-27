// Package restore provides restore management for OpenBao clusters.
// It handles restoring snapshots from object storage to an OpenBao cluster.
package restore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"github.com/dc-tec/openbao-operator/internal/logging"
	observability "github.com/dc-tec/openbao-operator/internal/observability"
	"github.com/dc-tec/openbao-operator/internal/operationlock"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	"github.com/dc-tec/openbao-operator/internal/security"
)

const (
	// RestoreJobNamePrefix is the prefix for restore job names.
	RestoreJobNamePrefix = constants.PrefixRestoreJob
	// RestoreJobTTLSeconds is the TTL for completed/failed restore jobs.
	RestoreJobTTLSeconds = 3600 // 1 hour
	// RestoreServiceAccountSuffix is appended to cluster name for the restore SA.
	RestoreServiceAccountSuffix = constants.SuffixRestoreServiceAccount
	// RestoreConditionType is the condition type for restore operations.
	RestoreConditionType = constants.RestoreConditionType // This will need to be added to conditions.go if missed

	restoreRequeueImmediately = 1 * time.Second
)

// Manager orchestrates restore operations for OpenBao clusters.
type Manager struct {
	client                client.Client
	scheme                *runtime.Scheme
	recorder              events.EventRecorder
	operatorImageVerifier imageverify.Verifier
	Platform              string
}

// NewManager creates a new restore Manager.
func NewManager(c client.Client, scheme *runtime.Scheme, recorder events.EventRecorder, operatorImageVerifier imageverify.Verifier, platform string) *Manager {
	return &Manager{
		client:                c,
		scheme:                scheme,
		recorder:              recorder,
		operatorImageVerifier: operatorImageVerifier,
		Platform:              platform,
	}
}

// Reconcile processes an OpenBaoRestore resource through its lifecycle.
// Returns (result, error) where result.RequeueAfter indicates if reconciliation should be rescheduled.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
	if restore.DeletionTimestamp != nil {
		return m.handleDeletion(ctx, logger, restore)
	}
	if err := m.ensureFinalizer(ctx, restore); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure restore finalizer: %w", err)
	}

	// Initialize status if not set
	if restore.Status.Phase == "" {
		restore.Status.Phase = openbaov1alpha1.RestorePhasePending
	}

	switch restore.Status.Phase {
	case openbaov1alpha1.RestorePhasePending:
		return m.handlePending(ctx, logger, restore)
	case openbaov1alpha1.RestorePhaseValidating:
		return m.handleValidating(ctx, logger, restore)
	case openbaov1alpha1.RestorePhaseRunning:
		return m.handleRunning(ctx, logger, restore)
	case openbaov1alpha1.RestorePhaseCompleted, openbaov1alpha1.RestorePhaseFailed:
		// Terminal states: ensure lock cleanup eventually succeeds.
		return m.ensureTerminalLockReleased(ctx, logger, restore)
	default:
		logger.Info("Unknown restore phase", "phase", restore.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (m *Manager) patchStatus(ctx context.Context, restore *openbaov1alpha1.OpenBaoRestore, original *openbaov1alpha1.OpenBaoRestore) error {
	return m.client.Status().Patch(ctx, restore, client.MergeFrom(original))
}

func (m *Manager) ensureTerminalLockReleased(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
	if err := m.releaseClusterLock(ctx, logger, restore); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to release cluster operation lock for terminal restore %s/%s: %w", restore.Namespace, restore.Name, err)
	}
	return ctrl.Result{}, nil
}

// handlePending transitions from Pending to Validating phase.
func (m *Manager) handlePending(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
	logger.Info("Starting restore validation", "cluster", restore.Spec.Cluster)

	// Record start time
	original := restore.DeepCopy()
	now := metav1.Now()
	restore.Status.StartTime = &now
	restore.Status.Phase = openbaov1alpha1.RestorePhaseValidating
	restore.Status.SnapshotKey = restore.Spec.Source.Key
	restore.Status.Message = "Validating restore preconditions"

	if err := m.patchStatus(ctx, restore, original); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch restore status: %w", err)
	}
	logging.LogAuditEvent(logger, logging.EventRestorePhaseTransition, map[string]string{
		"cluster_namespace": restore.Namespace,
		"cluster_name":      restore.Spec.Cluster,
		"restore_name":      restore.Name,
		"phase_from":        string(openbaov1alpha1.RestorePhasePending),
		"phase_to":          string(openbaov1alpha1.RestorePhaseValidating),
	})

	observability.NewRestoreMetrics(restore.Namespace, restore.Spec.Cluster).RecordStarted()

	return ctrl.Result{RequeueAfter: restoreRequeueImmediately}, nil
}

// handleValidating validates preconditions and transitions to Running.
func (m *Manager) handleValidating(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
	// Validate and get target cluster
	cluster, result, err := m.validateCluster(ctx, logger, restore)
	if result != nil || err != nil {
		if result != nil {
			return *result, err
		}
		return ctrl.Result{}, err
	}

	// Acquire operation lock
	lockBefore, forceAcquired, result, err := m.acquireOperationLock(ctx, logger, restore, cluster)
	if result != nil || err != nil {
		if result != nil {
			return *result, err
		}
		return ctrl.Result{}, err
	}

	// Handle lock override event if needed
	if forceAcquired && lockBefore != nil {
		original := restore.DeepCopy()
		m.handleLockOverride(restore, lockBefore)
		if err := m.patchStatus(ctx, restore, original); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch restore status after lock override: %w", err)
		}
	}

	// Validate cluster state
	if result, err := m.validateClusterState(ctx, logger, restore, cluster); result != nil || err != nil {
		if result != nil {
			return *result, err
		}
		return ctrl.Result{}, err
	}

	// Validate authentication
	if result, err := m.validateAuthentication(ctx, logger, restore, cluster); result != nil || err != nil {
		if result != nil {
			return *result, err
		}
		return ctrl.Result{}, err
	}

	// Ensure restore resources (ServiceAccount and RBAC)
	if err := m.ensureRestoreResources(ctx, logger, restore, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// Transition to Running phase
	original := restore.DeepCopy()
	restore.Status.Phase = openbaov1alpha1.RestorePhaseRunning
	restore.Status.Message = "Creating restore job"

	if err := m.patchStatus(ctx, restore, original); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch restore status: %w", err)
	}
	logging.LogAuditEvent(logger, logging.EventRestorePhaseTransition, map[string]string{
		"cluster_namespace": restore.Namespace,
		"cluster_name":      restore.Spec.Cluster,
		"restore_name":      restore.Name,
		"phase_from":        string(openbaov1alpha1.RestorePhaseValidating),
		"phase_to":          string(openbaov1alpha1.RestorePhaseRunning),
	})

	logger.Info("Restore validation passed, transitioning to Running phase")
	return ctrl.Result{RequeueAfter: restoreRequeueImmediately}, nil
}

// validateCluster validates that the target cluster exists and checks hardened profile requirements.
// Returns (cluster, result, error) where result is non-nil if validation failed and should return early.
func (m *Manager) validateCluster(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (*openbaov1alpha1.OpenBaoCluster, *ctrl.Result, error) {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: restore.Namespace,
		Name:      restore.Spec.Cluster,
	}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			result, err := m.failRestore(ctx, logger, restore, fmt.Sprintf("target cluster %q not found", restore.Spec.Cluster))
			return nil, &result, err
		}
		return nil, nil, fmt.Errorf("failed to get target cluster: %w", err)
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileHardened &&
		(cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) == 0) {
		result, err := m.failRestore(ctx, logger, restore,
			"Hardened profile requires explicit spec.network.egressRules so restore Jobs can reach the object storage endpoint")
		return nil, &result, err
	}

	return cluster, nil, nil
}

// acquireOperationLock acquires the cluster operation lock for the restore operation.
// Returns (lockBefore, forceAcquired, result, error) where result is non-nil if lock acquisition failed and should return early.
func (m *Manager) acquireOperationLock(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) (*openbaov1alpha1.OperationLockStatus, bool, *ctrl.Result, error) {
	lockHolder := fmt.Sprintf("%s/%s", constants.ControllerNameOpenBaoRestore, restore.Name)
	lockMessage := fmt.Sprintf("restore %s/%s", restore.Namespace, restore.Name)
	forceAcquire := false

	if restore.Spec.OverrideOperationLock {
		if !restore.Spec.Force {
			result, err := m.failRestore(ctx, logger, restore, "overrideOperationLock requires force: true")
			return nil, false, &result, err
		}
		if cluster.Status.OperationLock != nil && cluster.Status.OperationLock.Operation != openbaov1alpha1.ClusterOperationRestore {
			forceAcquire = true
		}
	}

	lockBefore := cluster.Status.OperationLock
	if err := operationlock.Acquire(ctx, m.client, cluster, operationlock.AcquireOptions{
		Holder:    lockHolder,
		Operation: openbaov1alpha1.ClusterOperationRestore,
		Message:   lockMessage,
		Force:     forceAcquire,
	}); err != nil {
		if errors.Is(err, operationlock.ErrLockHeld) {
			fields := map[string]string{
				"cluster_namespace": restore.Namespace,
				"cluster_name":      restore.Spec.Cluster,
				"restore_name":      restore.Name,
				"operation":         string(openbaov1alpha1.ClusterOperationRestore),
				"holder":            lockHolder,
			}
			var heldErr *operationlock.HeldError
			if errors.As(err, &heldErr) {
				fields["held_by_operation"] = string(heldErr.Operation)
				fields["held_by_holder"] = heldErr.Holder
			}
			logging.LogAuditEvent(logger, logging.EventOperationLockBlocked, fields)
			original := restore.DeepCopy()
			var held *operationlock.HeldError
			if errors.As(err, &held) {
				restore.Status.Message = fmt.Sprintf("Waiting for cluster operation lock: operation=%s holder=%s", held.Operation, held.Holder)
			} else {
				restore.Status.Message = "Waiting for cluster operation lock"
			}
			if statusErr := m.patchStatus(ctx, restore, original); statusErr != nil {
				return nil, false, nil, fmt.Errorf("failed to patch restore status after lock contention: %w", statusErr)
			}
			result := ctrl.Result{RequeueAfter: constants.RequeueShort}
			return nil, false, &result, nil
		}
		return nil, false, nil, fmt.Errorf("failed to acquire cluster operation lock: %w", err)
	}

	if forceAcquire && lockBefore != nil {
		logging.LogAuditEvent(logger, logging.EventOperationLockForceAcquired, map[string]string{
			"cluster_namespace":  restore.Namespace,
			"cluster_name":       restore.Spec.Cluster,
			"restore_name":       restore.Name,
			"operation":          string(openbaov1alpha1.ClusterOperationRestore),
			"holder":             lockHolder,
			"replaced_operation": string(lockBefore.Operation),
			"replaced_holder":    lockBefore.Holder,
		})
	} else if lockBefore == nil || lockBefore.Operation != openbaov1alpha1.ClusterOperationRestore || lockBefore.Holder != lockHolder {
		logging.LogAuditEvent(logger, logging.EventOperationLockAcquired, map[string]string{
			"cluster_namespace": restore.Namespace,
			"cluster_name":      restore.Spec.Cluster,
			"restore_name":      restore.Name,
			"operation":         string(openbaov1alpha1.ClusterOperationRestore),
			"holder":            lockHolder,
		})
	}

	return lockBefore, forceAcquire, nil, nil
}

// handleLockOverride records an event and sets a condition when a lock override occurs.
func (m *Manager) handleLockOverride(restore *openbaov1alpha1.OpenBaoRestore, lockBefore *openbaov1alpha1.OperationLockStatus) {
	if m.recorder != nil {
		m.recorder.Eventf(restore, nil, corev1.EventTypeWarning, "OperationLockOverride", "OperationLockOverride",
			"OverrideOperationLock used; cleared existing lock operation=%s holder=%s", lockBefore.Operation, lockBefore.Holder)
	}
	meta.SetStatusCondition(&restore.Status.Conditions, metav1.Condition{
		Type:               constants.ConditionTypeOperationLockOverride,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: restore.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             constants.ReasonOperationLockOverridden,
		Message:            fmt.Sprintf("Cleared existing lock operation=%s holder=%s", lockBefore.Operation, lockBefore.Holder),
	})
}

// validateClusterState validates that the cluster is in a valid state for restore.
// Returns (result, error) where result is non-nil if validation failed and should return early.
func (m *Manager) validateClusterState(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) (*ctrl.Result, error) {
	if restore.Spec.Force {
		return nil, nil
	}

	if !cluster.Status.Initialized {
		result, err := m.failRestore(ctx, logger, restore, "target cluster is not initialized (use force: true to override)")
		return &result, err
	}

	upgradingCond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
	if upgradingCond != nil && upgradingCond.Status == metav1.ConditionTrue {
		result, err := m.failRestore(ctx, logger, restore, "cannot restore while cluster is upgrading")
		return &result, err
	}

	return nil, nil
}

// validateAuthentication validates that authentication is configured for the restore operation.
// Returns (result, error) where result is non-nil if validation failed and should return early.
func (m *Manager) validateAuthentication(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) (*ctrl.Result, error) {
	hasJWTAuth := effectiveRestoreJWTRole(restore, cluster) != ""
	hasTokenSecret := restore.Spec.TokenSecretRef != nil && restore.Spec.TokenSecretRef.Name != ""

	if !hasJWTAuth && !hasTokenSecret {
		result, err := m.failRestore(ctx, logger, restore,
			"authentication is required: either jwtAuthRole or tokenSecretRef must be set in the restore spec")
		return &result, err
	}

	return nil, nil
}

// ensureRestoreResources ensures the ServiceAccount and RBAC resources exist for the restore operation.
func (m *Manager) ensureRestoreResources(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if err := m.ensureRestoreServiceAccount(ctx, logger, restore, cluster); err != nil {
		return fmt.Errorf("failed to ensure restore service account: %w", err)
	}

	if err := m.ensureRestoreRBAC(ctx, logger, restore, cluster); err != nil {
		return fmt.Errorf("failed to ensure restore RBAC: %w", err)
	}

	return nil
}

// handleRunning manages the restore job and checks for completion.
func (m *Manager) handleRunning(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
	// Get target cluster for job configuration
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: restore.Namespace,
		Name:      restore.Spec.Cluster,
	}, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get target cluster: %w", err)
	}

	lockHolder := fmt.Sprintf("%s/%s", constants.ControllerNameOpenBaoRestore, restore.Name)
	lockMessage := fmt.Sprintf("restore %s/%s", restore.Namespace, restore.Name)
	lockHeldByUs := cluster.Status.OperationLock != nil &&
		cluster.Status.OperationLock.Operation == openbaov1alpha1.ClusterOperationRestore &&
		cluster.Status.OperationLock.Holder == lockHolder
	if err := operationlock.Acquire(ctx, m.client, cluster, operationlock.AcquireOptions{
		Holder:    lockHolder,
		Operation: openbaov1alpha1.ClusterOperationRestore,
		Message:   lockMessage,
	}); err != nil {
		if errors.Is(err, operationlock.ErrLockHeld) {
			logging.LogAuditEvent(logger, logging.EventRestoreLockLost, map[string]string{
				"cluster_namespace": restore.Namespace,
				"cluster_name":      restore.Spec.Cluster,
				"restore_name":      restore.Name,
			})
			return m.failRestore(ctx, logger, restore, "cluster operation lock was taken by another operation while restore was running")
		}
		return ctrl.Result{}, fmt.Errorf("failed to renew cluster operation lock: %w", err)
	}
	if !lockHeldByUs {
		logging.LogAuditEvent(logger, logging.EventOperationLockAcquired, map[string]string{
			"cluster_namespace": restore.Namespace,
			"cluster_name":      restore.Spec.Cluster,
			"restore_name":      restore.Name,
			"operation":         string(openbaov1alpha1.ClusterOperationRestore),
			"holder":            lockHolder,
		})
	}

	// Check if job already exists
	jobName := restoreJobName(restore)
	job := &batchv1.Job{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: restore.Namespace,
		Name:      jobName,
	}, job)

	if apierrors.IsNotFound(err) {
		return m.createRestoreJob(ctx, logger, restore, cluster, jobName)
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get restore job: %w", err)
	}

	// Check job status
	if job.Status.Succeeded > 0 {
		if err := m.completeRestore(ctx, logger, restore, "Restore completed successfully"); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if job.Status.Failed > 0 {
		// Get failure message from job conditions
		message := "Restore job failed"
		for _, cond := range job.Status.Conditions {
			if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
				if cond.Message != "" {
					message = fmt.Sprintf("Restore job failed: %s", cond.Message)
				}
				break
			}
		}
		return m.failRestore(ctx, logger, restore, message)
	}

	// Job still running
	original := restore.DeepCopy()
	restore.Status.Message = "Restore job in progress"
	if err := m.patchStatus(ctx, restore, original); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch restore status while job is running: %w", err)
	}

	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (m *Manager) createRestoreJob(
	ctx context.Context,
	logger logr.Logger,
	restore *openbaov1alpha1.OpenBaoRestore,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
) (ctrl.Result, error) {
	executorImage, err := getRestoreExecutorImage(restore, cluster)
	if err != nil {
		return m.failRestore(ctx, logger, restore, fmt.Sprintf("failed to determine restore executor image: %v", err))
	}

	verifiedExecutorDigest := ""
	if executorImage != "" && security.IsOperatorImageVerificationEnabled(cluster) {
		verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
		defer cancel()

		digest, err := security.VerifyOperatorImageForCluster(verifyCtx, logger, m.operatorImageVerifier, cluster, executorImage)
		if err != nil {
			failurePolicy := ""
			if cluster.Spec.OperatorImageVerification != nil {
				failurePolicy = cluster.Spec.OperatorImageVerification.FailurePolicy
			}
			if failurePolicy == "" {
				failurePolicy = constants.ImageVerificationFailurePolicyBlock
			}
			if failurePolicy == constants.ImageVerificationFailurePolicyBlock {
				if operatorerrors.IsTransient(err) {
					original := restore.DeepCopy()
					restore.Status.Message = fmt.Sprintf("Waiting for restore executor image verification: %v", err)
					if statusErr := m.patchStatus(ctx, restore, original); statusErr != nil {
						return ctrl.Result{}, fmt.Errorf("failed to patch restore status after transient image verification failure: %w", statusErr)
					}
					return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
				}
				return m.failRestore(ctx, logger, restore, fmt.Sprintf("restore executor image verification failed: %v", err))
			}
			logger.Error(err, "Restore executor image verification failed but proceeding due to Warn policy", "image", executorImage)
		} else {
			verifiedExecutorDigest = digest
			logger.Info("Restore executor image verified successfully", "digest", digest)
		}
	}

	job, err := m.buildRestoreJob(restore, cluster, verifiedExecutorDigest)
	if err != nil {
		return m.failRestore(ctx, logger, restore, fmt.Sprintf("failed to build restore job: %v", err))
	}

	// Set owner reference.
	if err := controllerutil.SetControllerReference(restore, job, m.scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := m.client.Create(ctx, job); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.V(1).Info("Restore job already exists after create attempt; proceeding", "job", jobName)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to create restore job: %w", err)
	}

	logger.Info("Created restore job", "job", jobName)
	logging.LogAuditEvent(logger, logging.EventRestoreJobCreated, map[string]string{
		"cluster_namespace": restore.Namespace,
		"cluster_name":      restore.Spec.Cluster,
		"restore_name":      restore.Name,
		"job":               jobName,
	})
	original := restore.DeepCopy()
	restore.Status.Message = "Restore job running"
	if err := m.patchStatus(ctx, restore, original); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch restore status after job creation: %w", err)
	}

	// Requeue to check job status.
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// failRestore transitions the restore to Failed phase.
//
//nolint:unparam // ctrl.Result is always zero value but required by controller-runtime interface
func (m *Manager) failRestore(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, message string) (ctrl.Result, error) {
	original := restore.DeepCopy()
	now := metav1.Now()
	restore.Status.Phase = openbaov1alpha1.RestorePhaseFailed
	restore.Status.CompletionTime = &now
	restore.Status.Message = message

	meta.SetStatusCondition(&restore.Status.Conditions, metav1.Condition{
		Type:               string(RestoreConditionType),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: restore.Generation,
		Reason:             ReasonRestoreFailed,
		Message:            message,
		LastTransitionTime: now,
	})

	if err := m.patchStatus(ctx, restore, original); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch restore status: %w", err)
	}
	logging.LogAuditEvent(logger, logging.EventRestoreFailed, map[string]string{
		"cluster_namespace": restore.Namespace,
		"cluster_name":      restore.Spec.Cluster,
		"restore_name":      restore.Name,
	})

	durationSeconds := 0.0
	if restore.Status.StartTime != nil {
		durationSeconds = now.Time.Sub(restore.Status.StartTime.Time).Seconds()
	}
	observability.NewRestoreMetrics(restore.Namespace, restore.Spec.Cluster).RecordFailureWithDuration(durationSeconds)

	if err := m.releaseClusterLock(ctx, logger, restore); err != nil {
		logger.Error(err, "Failed to release cluster operation lock after restore failure")
	}

	return ctrl.Result{}, nil
}

// completeRestore transitions the restore to Completed phase.
func (m *Manager) completeRestore(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, message string) error {
	original := restore.DeepCopy()
	now := metav1.Now()
	restore.Status.Phase = openbaov1alpha1.RestorePhaseCompleted
	restore.Status.CompletionTime = &now
	restore.Status.Message = message

	meta.SetStatusCondition(&restore.Status.Conditions, metav1.Condition{
		Type:               string(RestoreConditionType),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: restore.Generation,
		Reason:             ReasonRestoreSucceeded,
		Message:            message,
		LastTransitionTime: now,
	})

	if err := m.patchStatus(ctx, restore, original); err != nil {
		return fmt.Errorf("failed to patch restore status: %w", err)
	}
	logging.LogAuditEvent(logger, logging.EventRestoreCompleted, map[string]string{
		"cluster_namespace": restore.Namespace,
		"cluster_name":      restore.Spec.Cluster,
		"restore_name":      restore.Name,
	})

	durationSeconds := 0.0
	if restore.Status.StartTime != nil {
		durationSeconds = now.Time.Sub(restore.Status.StartTime.Time).Seconds()
	}
	observability.NewRestoreMetrics(restore.Namespace, restore.Spec.Cluster).RecordSuccess(durationSeconds)

	if err := m.releaseClusterLock(ctx, logger, restore); err != nil {
		logger.Error(err, "Failed to release cluster operation lock after restore completion")
	}

	return nil
}

func (m *Manager) ensureFinalizer(ctx context.Context, restore *openbaov1alpha1.OpenBaoRestore) error {
	if controllerutil.ContainsFinalizer(restore, openbaov1alpha1.OpenBaoRestoreFinalizer) {
		return nil
	}

	original := restore.DeepCopy()
	controllerutil.AddFinalizer(restore, openbaov1alpha1.OpenBaoRestoreFinalizer)
	if err := m.client.Patch(ctx, restore, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to add finalizer: %w", err)
	}
	return nil
}

func (m *Manager) handleDeletion(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(restore, openbaov1alpha1.OpenBaoRestoreFinalizer) {
		return ctrl.Result{}, nil
	}

	if err := m.releaseClusterLock(ctx, logger, restore); err != nil {
		logger.Error(err, "Failed to release cluster operation lock during restore deletion")
	}

	original := restore.DeepCopy()
	controllerutil.RemoveFinalizer(restore, openbaov1alpha1.OpenBaoRestoreFinalizer)
	if err := m.client.Patch(ctx, restore, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
	}

	return ctrl.Result{}, nil
}

func (m *Manager) releaseClusterLock(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) error {
	if restore.Spec.Cluster == "" {
		return nil
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: restore.Namespace,
		Name:      restore.Spec.Cluster,
	}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get target cluster for lock release: %w", err)
	}

	holder := fmt.Sprintf("%s/%s", constants.ControllerNameOpenBaoRestore, restore.Name)
	if err := operationlock.Release(ctx, m.client, cluster, holder, openbaov1alpha1.ClusterOperationRestore); err != nil {
		if errors.Is(err, operationlock.ErrLockHeld) {
			return nil
		}
		return err
	}
	logging.LogAuditEvent(logger, logging.EventOperationLockReleased, map[string]string{
		"cluster_namespace": restore.Namespace,
		"cluster_name":      restore.Spec.Cluster,
		"restore_name":      restore.Name,
		"operation":         string(openbaov1alpha1.ClusterOperationRestore),
		"holder":            holder,
	})

	logger.V(1).Info("Released cluster operation lock for restore", "cluster", cluster.Name)
	return nil
}

// restoreJobName returns the name for the restore job.
func restoreJobName(restore *openbaov1alpha1.OpenBaoRestore) string {
	return fmt.Sprintf("%s%s", RestoreJobNamePrefix, restore.Name)
}

// ensureRestoreServiceAccount creates the ServiceAccount for restore jobs using Server-Side Apply.
func (m *Manager) ensureRestoreServiceAccount(ctx context.Context, _ logr.Logger, _ *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return EnsureRestoreServiceAccount(ctx, m.client, m.scheme, cluster)
}

// ensureRestoreRBAC creates RBAC for the restore service account using Server-Side Apply.
// The restore job needs permission to list pods for leader discovery.
func (m *Manager) ensureRestoreRBAC(ctx context.Context, _ logr.Logger, _ *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return EnsureRestoreRBAC(ctx, m.client, m.scheme, cluster)
}
