package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/backup"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
)

// ensurePreUpgradeSnapshotJob creates or checks the status of the pre-upgrade snapshot Job.
func (m *Manager) ensurePreUpgradeSnapshotJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
) (*JobResult, error) {
	if cluster.Spec.Backup == nil || cluster.Spec.Backup.Target.Endpoint == "" {
		return nil, fmt.Errorf("backup configuration required for pre-upgrade snapshot")
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileHardened &&
		(cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) == 0) {
		return nil, operatorerrors.WithReason(
			constants.ReasonNetworkEgressRulesRequired,
			operatorerrors.WrapPermanentConfig(fmt.Errorf(
				"hardened profile with pre-upgrade snapshots enabled requires explicit spec.network.egressRules so snapshot Jobs can reach the object storage endpoint",
			)),
		)
	}

	// ExecutorImage defaults to constants.DefaultBackupImage() when not specified
	if err := backup.EnsureBackupServiceAccount(ctx, m.client, m.scheme, cluster); err != nil {
		return nil, fmt.Errorf("failed to ensure backup ServiceAccount for snapshot job: %w", err)
	}
	if err := backup.EnsureBackupRBAC(ctx, m.client, m.scheme, cluster); err != nil {
		return nil, fmt.Errorf("failed to ensure backup RBAC for snapshot job: %w", err)
	}

	return ensureJob(ctx, m.client, m.scheme, logger, cluster, jobName, func(jobName string) (*batchv1.Job, error) {
		verifiedExecutorDigest, err := m.verifyImageDigest(
			ctx,
			logger,
			cluster,
			cluster.Spec.Backup.ExecutorImage,
			constants.ReasonBlueGreenSnapshotImageVerificationFailed,
			"Pre-upgrade snapshot executor image verification failed",
		)
		if err != nil {
			return nil, err
		}
		return m.buildSnapshotJob(cluster, jobName, "pre-upgrade", verifiedExecutorDigest)
	}, "component", ComponentUpgradeSnapshot, "phase", "pre-upgrade")
}

// buildSnapshotJob creates a backup Job spec for upgrade snapshots.
func (m *Manager) buildSnapshotJob(cluster *openbaov1alpha1.OpenBaoCluster, jobName, phase string, verifiedExecutorDigest string) (*batchv1.Job, error) {
	// For Blue/Green upgrades, target the Blue (current active) StatefulSet.
	// The pre-upgrade snapshot must capture the data from the currently running pods.
	statefulSetName := cluster.Name
	if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueRevision != "" {
		statefulSetName = fmt.Sprintf("%s-%s", cluster.Name, cluster.Status.BlueGreen.BlueRevision)
	}

	job, err := backup.BuildJob(cluster, backup.JobOptions{
		JobName:                jobName,
		JobType:                backup.JobTypePreUpgrade,
		VerifiedExecutorDigest: verifiedExecutorDigest,
		FilenamePrefix:         phase,
		ClientConfig:           m.clientConfig,
		Platform:               m.Platform,
		TargetStatefulSetName:  statefulSetName,
	})

	if err != nil {
		return nil, err
	}

	// Keep labels/annotations stable for the upgrade snapshot use-case.
	// We build the Job via the backup builder (shared logic), but expose it as an
	// "upgrade-snapshot" component so it remains distinguishable from scheduled backups.
	if job.Labels == nil {
		job.Labels = map[string]string{}
	}
	job.Labels[constants.LabelOpenBaoComponent] = ComponentUpgradeSnapshot
	delete(job.Labels, constants.LabelOpenBaoBackupType)

	if job.Spec.Template.Labels == nil {
		job.Spec.Template.Labels = map[string]string{}
	}
	job.Spec.Template.Labels[constants.LabelOpenBaoComponent] = ComponentUpgradeSnapshot
	delete(job.Spec.Template.Labels, constants.LabelOpenBaoBackupType)

	if job.Annotations == nil {
		job.Annotations = map[string]string{}
	}
	job.Annotations[AnnotationSnapshotPhase] = phase

	return job, nil
}
