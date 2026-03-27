package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

// handlePhaseIdle transitions from Idle to DeployingGreen when an upgrade is detected.
func (m *Manager) handlePhaseIdle(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, _ string) (phaseOutcome, error) {
	logger.Info("Starting blue/green upgrade",
		"fromVersion", cluster.Status.CurrentVersion,
		"targetVersion", cluster.Spec.Version)
	if cluster.Status.BlueGreen == nil || cluster.Status.BlueGreen.PreUpgradeSnapshotJobName == "" {
		m.emitNormalEvent(cluster, ReasonUpgradeStarted, "Blue/green upgrade started from %s to %s", cluster.Status.CurrentVersion, cluster.Spec.Version)
	}
	logging.LogAuditEvent(logger, logging.EventUpgradeStarted, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"strategy":          string(openbaov1alpha1.UpdateStrategyBlueGreen),
		"from_version":      cluster.Status.CurrentVersion,
		"to_version":        cluster.Spec.Version,
	})

	if cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.GreenRevision == "" &&
		cluster.Status.BlueGreen.PreUpgradeSnapshotJobName == "" {
		cluster.Status.BlueGreen.ManualPromotionRequired = cluster.Spec.Upgrade.BlueGreen != nil &&
			!cluster.Spec.Upgrade.BlueGreen.AutoPromote
	}

	preUpgradeSnapshotEnabled := cluster.Spec.Upgrade.PreUpgradeSnapshot ||
		(cluster.Spec.Upgrade.BlueGreen != nil && cluster.Spec.Upgrade.BlueGreen.PreUpgradeSnapshot)
	if preUpgradeSnapshotEnabled {
		jobName := preUpgradeSnapshotJobName(cluster)
		if cluster.Status.BlueGreen.PreUpgradeSnapshotJobName != jobName {
			_, err := m.ensurePreUpgradeSnapshotJob(ctx, logger, cluster, jobName)
			if err != nil {
				logger.Error(err, "Failed to ensure pre-upgrade snapshot job")
				return phaseOutcome{}, err
			}
			cluster.Status.BlueGreen.PreUpgradeSnapshotJobName = jobName
			m.emitNormalEvent(cluster, upgrade.ReasonPreUpgradeSnapshotJobCreated, "Created pre-upgrade snapshot Job %s", jobName)
			logger.Info("Pre-upgrade snapshot job created", "job", jobName)
			return requeueAfterOutcome(constants.RequeueShort), nil
		}

		jobStatus, err := getJobStatus(ctx, m.client, cluster, jobName)
		if err != nil {
			logger.Error(err, "Failed to check pre-upgrade snapshot job status")
		} else if jobStatus.Exists && jobStatus.Running {
			logger.Info("Waiting for pre-upgrade snapshot to complete", "job", jobName)
			return requeueAfterOutcome(constants.RequeueShort), nil
		} else if jobStatus.Exists && jobStatus.Failed {
			m.emitWarningEvent(cluster, upgrade.ReasonPreUpgradeSnapshotFailed, "Pre-upgrade snapshot Job %s failed", jobName)
			logger.Info("Pre-upgrade snapshot failed", "job", jobName)
			return phaseOutcome{}, fmt.Errorf("pre-upgrade snapshot job failed: %s", jobName)
		}
		m.emitNormalEvent(cluster, upgrade.ReasonPreUpgradeSnapshotCompleted, "Pre-upgrade snapshot completed successfully with Job %s", jobName)
		logger.Info("Pre-upgrade snapshot completed", "job", jobName)
	}

	cluster.Status.BlueGreen.GreenRevision = m.calculateRevision(cluster)
	return advance(openbaov1alpha1.PhaseDeployingGreen), nil
}

// handlePhaseDeployingGreen creates the Green StatefulSet.
// IMPORTANT: Green pods must join the existing Blue cluster as non-voters, not initialize a new cluster.
func (m *Manager) handlePhaseDeployingGreen(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, _ string) (phaseOutcome, error) {
	greenRevision := cluster.Status.BlueGreen.GreenRevision
	blueRevision := cluster.Status.BlueGreen.BlueRevision
	logger = logger.WithValues("greenRevision", greenRevision, "blueRevision", blueRevision)

	bluePods, err := m.getBluePods(ctx, cluster, blueRevision)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to get Blue pods: %w", err)
	}
	if len(bluePods) == 0 {
		logger.Info("No Blue pods found yet, waiting...")
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	bluePodsHaveRevisionLabel := true
	for _, pod := range bluePods {
		rev, present := pod.Labels[constants.LabelOpenBaoRevision]
		if !present || rev != blueRevision {
			bluePodsHaveRevisionLabel = false
			logger.Info("Blue pod missing revision label, InfraManager will update it",
				"pod", pod.Name,
				"expectedRevision", blueRevision,
				"actualRevision", rev)
			break
		}
	}
	if !bluePodsHaveRevisionLabel {
		logger.Info("Blue pods missing revision label; waiting for InfraManager to update StatefulSet")
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	blueReady := false
	for _, pod := range bluePods {
		if isPodReady(&pod) {
			sealed, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelSealed)
			if err == nil && present && !sealed {
				blueReady = true
				break
			}
		}
	}
	if !blueReady {
		logger.Info("Blue pods not ready/unsealed yet, waiting before creating Green StatefulSet")
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	greenStatefulSetName := fmt.Sprintf("%s-%s", cluster.Name, greenRevision)
	greenStatefulSet := &appsv1.StatefulSet{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      greenStatefulSetName,
	}, greenStatefulSet); err != nil {
		if !apierrors.IsNotFound(err) {
			return phaseOutcome{}, fmt.Errorf("failed to get Green StatefulSet: %w", err)
		}
		return m.createGreenStatefulSet(ctx, logger, cluster, blueRevision, greenRevision)
	}

	desiredReplicas := cluster.Spec.Replicas
	if greenStatefulSet.Spec.Replicas != nil {
		desiredReplicas = *greenStatefulSet.Spec.Replicas
	}
	if greenStatefulSet.Status.ReadyReplicas < desiredReplicas {
		logger.Info("Waiting for Green pods to be ready",
			"readyReplicas", greenStatefulSet.Status.ReadyReplicas,
			"desiredReplicas", desiredReplicas)
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to get Green pods: %w", err)
	}
	for _, pod := range greenPods {
		if pod.Status.Phase != corev1.PodRunning {
			logger.Info("Green pod not yet running", "pod", pod.Name, "phase", pod.Status.Phase)
			return requeueAfterOutcome(constants.RequeueShort), nil
		}
	}
	for _, pod := range greenPods {
		sealed, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelSealed)
		if err != nil {
			return phaseOutcome{}, fmt.Errorf("failed to parse sealed label on pod %s: %w", pod.Name, err)
		}
		if !present || sealed {
			logger.Info("Waiting for Green pod to be unsealed", "pod", pod.Name)
			return requeueAfterOutcome(constants.RequeueShort), nil
		}
	}

	return advance(openbaov1alpha1.PhaseJoiningMesh), nil
}

func (m *Manager) createGreenStatefulSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, blueRevision string, greenRevision string) (phaseOutcome, error) {
	if m.infraRuntime == nil {
		return phaseOutcome{}, fmt.Errorf("infra runtime is not configured")
	}

	infraDetails := configbuilder.InfrastructureDetails{
		HeadlessServiceName:   cluster.Name,
		Namespace:             cluster.Namespace,
		APIPort:               constants.PortAPI,
		ClusterPort:           constants.PortCluster,
		TargetRevisionForJoin: blueRevision,
	}
	renderedConfig, err := configbuilder.RenderHCL(cluster, infraDetails)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to render config for Green cluster: %w", err)
	}

	greenImage, greenInitImage, err := m.prepareGreenStatefulSetImages(ctx, logger, cluster)
	if err != nil {
		return phaseOutcome{}, err
	}

	if err := m.infraRuntime.EnsureStatefulSetWithRevision(ctx, logger, cluster, string(renderedConfig), greenImage, greenInitImage, greenRevision, true); err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to create Green StatefulSet: %w", err)
	}

	logger.Info("Created Green StatefulSet", "greenRevision", greenRevision)
	return requeueAfterOutcome(constants.RequeueShort), nil
}

func (m *Manager) prepareGreenStatefulSetImages(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, string, error) {
	greenImage := cluster.Spec.Image
	verifiedGreenDigest, err := m.verifyImageDigest(ctx, logger, cluster, greenImage, constants.ReasonBlueGreenImageVerificationFailed, "Green image verification failed")
	if err != nil {
		return "", "", err
	}
	if verifiedGreenDigest != "" {
		greenImage = verifiedGreenDigest
	}

	initImage, err := resolveInitContainerImage(cluster)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve Green init container image: %w", err)
	}

	verifiedInitContainerDigest, err := m.verifyOperatorImageDigest(ctx, logger, cluster, initImage, constants.ReasonInitContainerImageVerificationFailed, "Green init container image verification failed")
	if err != nil {
		return "", "", err
	}
	if verifiedInitContainerDigest != "" {
		initImage = verifiedInitContainerDigest
	}

	return greenImage, initImage, nil
}

// handlePhaseJoiningMesh joins Green pods to the Raft cluster as non-voters.
func (m *Manager) handlePhaseJoiningMesh(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	step, err := m.runExecutorJobStep(ctx, logger, cluster, ActionJoinGreenNonVoters, "job failure threshold exceeded")
	if err != nil {
		return phaseOutcome{}, err
	}
	if !step.Completed {
		return step.Outcome, nil
	}

	return advance(openbaov1alpha1.PhaseSyncing), nil
}
