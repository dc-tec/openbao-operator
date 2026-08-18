package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portbackup "github.com/dc-tec/openbao-operator/internal/port/backup"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	snapshothelpers "github.com/dc-tec/openbao-operator/internal/service/upgrade/snapshot"
)

// ensurePreUpgradeSnapshotJob creates or checks the status of the pre-upgrade snapshot Job.
func (m *Manager) ensurePreUpgradeSnapshotJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
) (*JobResult, error) {
	if err := snapshothelpers.ValidatePreUpgradeSnapshotPrerequisites(ctx, m.client, cluster, snapshothelpers.ValidationOptions{
		MissingBackupMessage:  "backup configuration required for pre-upgrade snapshot",
		RequireEndpoint:       true,
		RequireTokenSecret:    false,
		NetworkErrorMessage:   "hardened profile with pre-upgrade snapshots enabled requires explicit spec.network.egressRules so snapshot Jobs can reach the object storage endpoint",
		AuthenticationMessage: "backup authentication is required: set spec.backup.jwtAuthRole or spec.backup.tokenSecretRef (or enable spec.selfInit.oidc.enabled=true)",
	}); err != nil {
		return nil, operatorerrors.WithReason(
			upgrade.ReasonPreUpgradeBackupFailed,
			operatorerrors.WrapPermanentConfig(err),
		)
	}

	// Image defaults to constants.DefaultBackupImage() when not specified
	if err := snapshothelpers.EnsureRuntime(ctx, m.backupRuntime, cluster); err != nil {
		return nil, err
	}

	return ensureJob(ctx, m.client, m.reader, m.scheme, logger, cluster, jobName, func(jobName string) (*batchv1.Job, error) {
		verifiedExecutorDigest, err := snapshothelpers.ResolvePreUpgradeSnapshotExecutorDigest(
			ctx,
			logger,
			m.operatorImageVerifier,
			cluster,
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

	job, err := m.backupRuntime.BuildPreUpgradeJob(cluster, portbackup.JobBuildOptions{
		JobName:                jobName,
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

func podSnapshotsFromPods(pods []corev1.Pod) ([]podSnapshot, error) {
	snapshots := make([]podSnapshot, 0, len(pods))
	for i := range pods {
		pod := &pods[i]

		sealed, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelSealed)
		if err != nil {
			return nil, fmt.Errorf("failed to parse sealed label on pod %s: %w", pod.Name, err)
		}

		active := false
		isActive, isActivePresent, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelActive)
		if err == nil && isActivePresent && isActive {
			active = true
		}

		snapshots = append(snapshots, podSnapshot{
			Ready:    isPodReady(pod),
			Unsealed: present && !sealed,
			Active:   active,
			Deleting: pod.DeletionTimestamp != nil,
		})
	}
	return snapshots, nil
}
