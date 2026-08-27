package restore

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
	"github.com/dc-tec/openbao-operator/internal/service/workloadidentity"
)

// handleRunning manages the restore job and checks for completion.
func (m *Manager) handleRunning(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
	// Get target cluster for job configuration
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := m.reader.Get(ctx, types.NamespacedName{
		Namespace: restore.Namespace,
		Name:      restore.Spec.Cluster,
	}, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get target cluster: %w", err)
	}

	// Check if job already exists
	jobName := restoreJobName(restore)
	job, err := opslifecycle.ReadManagedJob(ctx, m.reader, types.NamespacedName{
		Namespace: restore.Namespace,
		Name:      jobName,
	}, restore, openbaov1alpha1.GroupVersion.WithKind("OpenBaoRestore"), "observe restore")

	if apierrors.IsNotFound(err) {
		done, err := m.renewRunningRestoreLock(ctx, logger, restore, cluster)
		if err != nil {
			return ctrl.Result{}, err
		}
		if done {
			return ctrl.Result{}, nil
		}
		return m.createRestoreJob(ctx, logger, restore, cluster, jobName)
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get restore job: %w", err)
	}

	// Check job status
	if job.Status.Succeeded > 0 {
		if err := m.handleSucceededRestoreJob(ctx, logger, restore, cluster); err != nil {
			return ctrl.Result{}, err
		}
		if restore.Status.Phase == openbaov1alpha1.RestorePhaseCompleted {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: restoreRequeueImmediately}, nil
	}

	done, err := m.renewRunningRestoreLock(ctx, logger, restore, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	if done {
		return ctrl.Result{}, nil
	}

	if job.Status.Failed > 0 {
		return m.failRestore(ctx, logger, restore, restoreJobFailedStatusMessage(job, workloadidentity.FailureHint(restore.Spec.Source.Target, restoreServiceAccountName(cluster))))
	}

	// Job still running
	original := restore.DeepCopy()
	restore.Status.Message = restoreJobRunningStatusMessage(jobName)
	if err := m.patchStatus(ctx, restore, original); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch restore status while job is running: %w", err)
	}

	return ctrl.Result{RequeueAfter: restoreRequeueJobPoll}, nil
}

func (m *Manager) renewRunningRestoreLock(
	ctx context.Context,
	logger logr.Logger,
	restore *openbaov1alpha1.OpenBaoRestore,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (bool, error) {
	lock := restoreOperationLock(restore)
	lockHeldByUs := lock.IsHeldBy(cluster.Status.OperationLock)
	if err := opslifecycle.AcquireWithReader(ctx, m.reader, m.client, cluster, lock, opslifecycle.AcquireOptions{
		Message: restoreLockMessage(restore),
	}); err != nil {
		if opslifecycle.IsLockHeld(err) {
			logging.LogAuditEvent(logger, logging.EventRestoreLockLost, map[string]string{
				"cluster_namespace": restore.Namespace,
				"cluster_name":      restore.Spec.Cluster,
				"restore_name":      restore.Name,
			})
			m.emitWarningEvent(restore, ReasonOperationLockLost, "Restore lost the cluster operation lock while running")
			_, failErr := m.failRestore(
				ctx,
				logger,
				restore,
				"Restore stopped because another operation took the cluster operation lock while the restore Job was running. Check concurrent backup or upgrade activity, then create a new OpenBaoRestore to retry.",
			)
			return true, failErr
		}
		return false, fmt.Errorf("failed to renew cluster operation lock: %w", err)
	}
	if !lockHeldByUs {
		logging.LogAuditEvent(logger, logging.EventOperationLockAcquired, map[string]string{
			"cluster_namespace": restore.Namespace,
			"cluster_name":      restore.Spec.Cluster,
			"restore_name":      restore.Name,
			"operation":         string(openbaov1alpha1.ClusterOperationRestore),
			"holder":            lock.Holder,
		})
	}

	return false, nil
}

func (m *Manager) handleSucceededRestoreJob(
	ctx context.Context,
	logger logr.Logger,
	restore *openbaov1alpha1.OpenBaoRestore,
	cluster *openbaov1alpha1.OpenBaoCluster,
) error {
	if !clusterRestoreRestartCompleted(cluster, restore) {
		done, err := m.renewRunningRestoreLock(ctx, logger, restore, cluster)
		if err != nil {
			return err
		}
		if done {
			return nil
		}

		requested, err := m.requestPostRestoreRestart(ctx, cluster, restore)
		if err != nil {
			return err
		}
		if requested {
			m.emitNormalEvent(restore, ReasonRestoreRestartRequested, "Requested voter Pod restart after snapshot application")
		}

		complete, message, err := m.postRestoreVoterRestartComplete(ctx, cluster, restore)
		if err != nil {
			return err
		}
		if !complete {
			return m.patchRestoreProgressMessage(ctx, restore, message)
		}

		marked, err := m.markPostRestoreRestartCompleted(ctx, cluster, restore)
		if err != nil {
			return err
		}
		if marked {
			m.emitNormalEvent(restore, ReasonRestoreRestartCompleted, "Voter Pods completed the post-restore restart")
		}
	}

	if err := m.releaseClusterLock(ctx, logger, restore); err != nil {
		return fmt.Errorf("failed to release cluster operation lock after post-restore voter restart: %w", err)
	}

	if !shouldWaitForSteadyReadReplicaRestore(cluster) {
		return m.completeRestore(ctx, logger, restore, "Restore completed successfully after voter Pods restarted")
	}

	if steadyReadReplicaRestoreComplete(cluster) {
		return m.completeRestore(ctx, logger, restore, "Restore completed successfully")
	}

	readyReplicas := int32(0)
	registeredReplicas := int32(0)
	if cluster.Status.ReadReplicas != nil {
		readyReplicas = cluster.Status.ReadReplicas.ReadyReplicas
		registeredReplicas = cluster.Status.ReadReplicas.RegisteredReplicas
	}

	original := restore.DeepCopy()
	restore.Status.Message = fmt.Sprintf(
		"Waiting for steady read replicas to restore before marking restore complete: desiredReadReplicas=%d readyReadReplicas=%d registeredReadReplicas=%d readReplicasReady=%t readServingAvailable=%t raftMembershipReady=%t",
		cluster.Spec.ReadReplicas.Replicas,
		readyReplicas,
		registeredReplicas,
		restoreConditionTrue(cluster, openbaov1alpha1.ConditionReadReplicasReady),
		restoreConditionTrue(cluster, openbaov1alpha1.ConditionReadServingAvailable),
		restoreConditionTrue(cluster, openbaov1alpha1.ConditionRaftMembershipReady),
	)
	if err := m.patchStatus(ctx, restore, original); err != nil {
		return fmt.Errorf("failed to patch restore status while waiting for steady read replicas to restore: %w", err)
	}

	return nil
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
					restore.Status.Message = fmt.Sprintf("Waiting for restore executor image verification before creating the restore Job: %v", err)
					if statusErr := m.patchStatus(ctx, restore, original); statusErr != nil {
						return ctrl.Result{}, fmt.Errorf("failed to patch restore status after transient image verification failure: %w", statusErr)
					}
					return ctrl.Result{RequeueAfter: restoreRequeueJobPoll}, nil
				}
				return m.failRestore(
					ctx,
					logger,
					restore,
					fmt.Sprintf("Restore executor image verification failed: %v. Check image verification configuration or use failurePolicy=Warn if that is intended, then create a new OpenBaoRestore to retry.", err),
				)
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

	if err := opslifecycle.PrepareManagedJobOwner(job, restore, m.scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to prepare restore Job ownership: %w", err)
	}

	if err := m.client.Create(ctx, job); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if _, readErr := opslifecycle.ReadManagedJob(
				ctx,
				m.reader,
				types.NamespacedName{Namespace: restore.Namespace, Name: jobName},
				restore,
				openbaov1alpha1.GroupVersion.WithKind("OpenBaoRestore"),
				"use existing restore",
			); readErr != nil {
				return ctrl.Result{}, fmt.Errorf("restore Job create collided with an untrusted existing Job: %w", readErr)
			}
			logger.V(1).Info("Restore job already exists after create attempt; ownership verified", "job", jobName)
			return ctrl.Result{RequeueAfter: restoreRequeueJobCheck}, nil
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
	if message, ok := workloadidentity.IdentityConfigurationEventMessage(restore.Spec.Source.Target, restoreServiceAccountName(cluster)); ok {
		m.emitNormalEvent(restore, ReasonRestoreIdentityConfiguration, "%s", message)
	}
	m.emitNormalEvent(restore, ReasonRestoreJobCreated, "Created restore Job %s", jobName)
	original := restore.DeepCopy()
	restore.Status.Message = restoreJobRunningStatusMessage(jobName)
	if err := m.patchStatus(ctx, restore, original); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch restore status after job creation: %w", err)
	}

	// Requeue to check job status.
	return ctrl.Result{RequeueAfter: restoreRequeueJobCheck}, nil
}
