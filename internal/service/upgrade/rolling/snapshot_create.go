package rolling

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	portbackup "github.com/dc-tec/openbao-operator/internal/port/backup"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	snapshothelpers "github.com/dc-tec/openbao-operator/internal/service/upgrade/snapshot"
)

func (m *Manager) createPreUpgradeBackupJob(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	jobName := m.backupJobName(cluster)
	logger.Info("Creating pre-upgrade backup job", "job", jobName)

	if err := m.ensurePreUpgradeBackupRuntime(ctx, cluster); err != nil {
		return false, err
	}

	verifiedExecutorDigest, err := m.resolvePreUpgradeBackupExecutorDigest(ctx, logger, cluster)
	if err != nil {
		return false, err
	}

	job, err := m.buildPreUpgradeBackupJob(cluster, jobName, verifiedExecutorDigest)
	if err != nil {
		return false, err
	}

	if err := m.client.Create(ctx, job); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if _, readErr := opslifecycle.ReadManagedJob(
				ctx,
				m.jobReader(),
				client.ObjectKey{Namespace: cluster.Namespace, Name: jobName},
				cluster,
				openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster"),
				"use existing pre-upgrade backup",
			); readErr != nil {
				return false, fmt.Errorf("pre-upgrade backup Job create collided with an untrusted existing Job: %w", readErr)
			}
			logger.V(1).Info("Pre-upgrade backup job already exists after create attempt; ownership verified", "job", jobName)
			return false, nil
		}
		return false, fmt.Errorf("failed to create backup job: %w", err)
	}

	logger.Info("Pre-upgrade backup job created", "job", jobName)
	logging.LogAuditEvent(logger, logging.EventPreUpgradeSnapshotJobCreated, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"job":               jobName,
	})
	m.emitNormalEvent(cluster, upgrade.ReasonPreUpgradeSnapshotJobCreated, "Created pre-upgrade snapshot Job %s", jobName)
	return false, nil
}

func (m *Manager) ensurePreUpgradeBackupRuntime(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if err := snapshothelpers.EnsureRuntime(ctx, m.backupRuntime, cluster); err != nil {
		return operatorerrors.WithReason(upgrade.ReasonPreUpgradeBackupFailed, err)
	}
	return nil
}

func (m *Manager) resolvePreUpgradeBackupExecutorDigest(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (string, error) {
	digest, err := snapshothelpers.ResolvePreUpgradeSnapshotExecutorDigest(
		ctx,
		logger,
		m.operatorImageVerifier,
		cluster,
		constants.ReasonPreUpgradeBackupImageVerificationFailed,
		"pre-upgrade backup executor image verification failed",
	)
	if err != nil {
		return "", operatorerrors.WithReason(
			upgrade.ReasonPreUpgradeBackupFailed,
			err,
		)
	}
	return digest, nil
}

func (m *Manager) buildPreUpgradeBackupJob(
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
	verifiedExecutorDigest string,
) (*batchv1.Job, error) {
	job, err := m.backupRuntime.BuildPreUpgradeJob(cluster, portbackup.JobBuildOptions{
		JobName:                jobName,
		FilenamePrefix:         constants.BackupTypePreUpgrade,
		VerifiedExecutorDigest: verifiedExecutorDigest,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build backup job: %w", err)
	}

	if err := opslifecycle.PrepareManagedJobOwner(job, cluster, m.scheme); err != nil {
		return nil, fmt.Errorf("failed to prepare backup Job ownership: %w", err)
	}

	return job, nil
}
