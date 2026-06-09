package restore

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/hardenedcontract"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
	"github.com/dc-tec/openbao-operator/internal/service/workloadidentity"
)

// validateCluster validates that the target cluster exists and checks hardened profile requirements.
// Returns (cluster, result, error) where result is non-nil if validation failed and should return early.
func (m *Manager) validateCluster(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (*openbaov1alpha1.OpenBaoCluster, *ctrl.Result, error) {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := m.reader.Get(ctx, types.NamespacedName{
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
	if cluster.Spec.Profile == openbaov1alpha1.ProfileHardened {
		if cluster.Spec.Network != nil && !hardenedcontract.EgressRulesExplicit(cluster.Spec.Network.EgressRules) {
			result, err := m.failRestore(ctx, logger, restore,
				"Hardened profile requires spec.network.egressRules entries to be port-scoped and target explicit non-wildcard peers")
			return nil, &result, err
		}
		if violation := hardenedcontract.EvaluateStorageTarget("Restore", restore.Spec.Source.Target); violation != nil {
			result, err := m.failRestore(ctx, logger, restore, violation.Message)
			return nil, &result, err
		}
	}

	return cluster, nil, nil
}

// acquireOperationLock acquires the cluster operation lock for the restore operation.
// Returns (lockBefore, forceAcquired, result, error) where result is non-nil if lock acquisition failed and should return early.
func (m *Manager) acquireOperationLock(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) (*openbaov1alpha1.OperationLockStatus, bool, *ctrl.Result, error) {
	lock := restoreOperationLock(restore)
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
	if err := opslifecycle.AcquireWithReader(ctx, m.reader, m.client, cluster, lock, opslifecycle.AcquireOptions{
		Message: restoreLockMessage(restore),
		Force:   forceAcquire,
	}); err != nil {
		if opslifecycle.IsLockHeld(err) {
			fields := map[string]string{
				"cluster_namespace": restore.Namespace,
				"cluster_name":      restore.Spec.Cluster,
				"restore_name":      restore.Name,
				"operation":         string(openbaov1alpha1.ClusterOperationRestore),
				"holder":            lock.Holder,
			}
			opslifecycle.AddHeldAuditFields(fields, err)
			logging.LogAuditEvent(logger, logging.EventOperationLockBlocked, fields)
			m.emitWarningEvent(restore, ReasonOperationLockBlocked, "Restore is waiting for the cluster operation lock: %v", err)
			original := restore.DeepCopy()
			restore.Status.Message = restoreWaitingForOperationLockStatusMessage(err)
			if statusErr := m.patchStatus(ctx, restore, original); statusErr != nil {
				return nil, false, nil, fmt.Errorf("failed to patch restore status after lock contention: %w", statusErr)
			}
			result := ctrl.Result{RequeueAfter: opslifecycle.RequeueDelay(opslifecycle.RetryClassLockContention)}
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
			"holder":             lock.Holder,
			"replaced_operation": string(lockBefore.Operation),
			"replaced_holder":    lockBefore.Holder,
		})
	} else if !lock.IsHeldBy(lockBefore) {
		logging.LogAuditEvent(logger, logging.EventOperationLockAcquired, map[string]string{
			"cluster_namespace": restore.Namespace,
			"cluster_name":      restore.Spec.Cluster,
			"restore_name":      restore.Name,
			"operation":         string(openbaov1alpha1.ClusterOperationRestore),
			"holder":            lock.Holder,
		})
	}

	return lockBefore, forceAcquire, nil, nil
}

// handleLockOverride records an event and sets a condition when a lock override occurs.
func (m *Manager) handleLockOverride(restore *openbaov1alpha1.OpenBaoRestore, lockBefore *openbaov1alpha1.OperationLockStatus) {
	if m.recorder != nil {
		m.recorder.Eventf(restore, nil, corev1.EventTypeWarning, ReasonOperationLockOverride, ReasonOperationLockOverride,
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
	if !restore.Spec.Force {
		if !cluster.Status.Initialized {
			result, err := m.failRestore(ctx, logger, restore, "target cluster is not initialized (use force: true to override)")
			return &result, err
		}

		upgradingCond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
		if upgradingCond != nil && upgradingCond.Status == metav1.ConditionTrue {
			result, err := m.failRestore(ctx, logger, restore, "cannot restore while cluster is upgrading")
			return &result, err
		}
	}

	if result, err := m.waitForSteadyReadReplicasScaledDown(ctx, restore, cluster); result != nil || err != nil {
		return result, err
	}

	return nil, nil
}

func (m *Manager) waitForSteadyReadReplicasScaledDown(ctx context.Context, restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) (*ctrl.Result, error) {
	if cluster == nil || cluster.Spec.ReadReplicas == nil || cluster.Spec.ReadReplicas.Replicas == 0 {
		return nil, nil
	}

	readStatefulSet := &appsv1.StatefulSet{}
	key := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      resourceidentity.ReadReplicaStatefulSetName(cluster),
	}
	if err := m.reader.Get(ctx, key, readStatefulSet); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get steady read-replica StatefulSet %s/%s: %w", key.Namespace, key.Name, err)
	}

	if readReplicaStatefulSetScaledDown(readStatefulSet) {
		return nil, nil
	}

	original := restore.DeepCopy()
	restore.Status.Message = fmt.Sprintf(
		"Waiting for steady read replicas to scale down before restore starts: statefulSet=%s specReplicas=%d statusReplicas=%d readyReplicas=%d",
		readStatefulSet.Name,
		derefReplicas(readStatefulSet.Spec.Replicas),
		readStatefulSet.Status.Replicas,
		readStatefulSet.Status.ReadyReplicas,
	)
	if err := m.patchStatus(ctx, restore, original); err != nil {
		return nil, fmt.Errorf("failed to patch restore status while waiting for steady read replicas to scale down: %w", err)
	}

	result := ctrl.Result{RequeueAfter: restoreRequeueImmediately}
	return &result, nil
}

// validateExecutionReadiness validates restore auth, storage, and hardened-profile
// egress prerequisites the operator can verify before creating a restore Job.
func (m *Manager) validateExecutionReadiness(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) (*ctrl.Result, error) {
	readiness, err := workloadidentity.EvaluateRestoreReadiness(ctx, m.client, restore, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate restore prerequisites: %w", err)
	}

	if err := m.patchRestoreConfigurationCondition(ctx, restore, readiness.Status, readiness.Reason, readiness.Message); err != nil {
		return nil, err
	}

	if readiness.Status != metav1.ConditionTrue {
		result, err := m.failRestore(ctx, logger, restore, readiness.Message)
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

// ensureRestoreServiceAccount creates the ServiceAccount for restore jobs using Server-Side Apply.
func (m *Manager) ensureRestoreServiceAccount(
	ctx context.Context,
	_ logr.Logger,
	restore *openbaov1alpha1.OpenBaoRestore,
	cluster *openbaov1alpha1.OpenBaoCluster,
) error {
	return EnsureRestoreServiceAccount(ctx, m.client, m.scheme, cluster, restore.Spec.Source.Target)
}

// ensureRestoreRBAC creates RBAC for the restore service account using Server-Side Apply.
// The restore job needs permission to list pods for leader discovery.
func (m *Manager) ensureRestoreRBAC(ctx context.Context, _ logr.Logger, _ *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return EnsureRestoreRBAC(ctx, m.client, m.scheme, cluster)
}

func readReplicaStatefulSetScaledDown(sts *appsv1.StatefulSet) bool {
	if sts == nil {
		return true
	}
	if sts.Status.ObservedGeneration < sts.Generation {
		return false
	}
	if derefReplicas(sts.Spec.Replicas) != 0 {
		return false
	}
	return sts.Status.Replicas == 0 && sts.Status.ReadyReplicas == 0 && sts.Status.CurrentReplicas == 0
}

func derefReplicas(replicas *int32) int32 {
	if replicas == nil {
		return 0
	}
	return *replicas
}
