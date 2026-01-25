package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	openbaolabels "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/revision"
)

// clusterState holds the observed state used for status computation.
// This separates API calls from condition logic to reduce cyclomatic complexity.
type clusterState struct {
	// StatefulSet observed state
	StatefulSet   *appsv1.StatefulSet
	ReadyReplicas int32
	Available     bool
	StatusStale   bool // StatefulSet status may lag behind reality

	// Pod state
	Pods             []corev1.Pod
	Pod0             *corev1.Pod
	LeaderName       string
	LeaderCount      int
	Initialized      bool
	InitializedKnown bool
	Sealed           bool
	SealedKnown      bool

	// Backup state
	BackupJobName    string
	BackupInProgress bool

	// Upgrade state (computed from cluster.Status)
	UpgradeFailed            bool
	UpgradeInProgress        bool
	RollingUpgradeInProgress bool
	BlueGreenInProgress      bool

	// Active revision for blue/green deployments
	ActiveRevision string
}

// gatherClusterState performs all API calls to observe cluster state.
// This separates data gathering from condition computation.
func (r *OpenBaoClusterReconciler) gatherClusterState(
	ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster,
) (*clusterState, error) {
	state := &clusterState{}

	// Compute upgrade state from cluster.Status
	state.RollingUpgradeInProgress = cluster.Status.Upgrade != nil
	if state.RollingUpgradeInProgress && cluster.Status.Upgrade.LastErrorReason != "" {
		state.UpgradeFailed = true
	}

	state.BlueGreenInProgress = cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.Phase != "" &&
		cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle

	state.UpgradeInProgress = (state.RollingUpgradeInProgress && !state.UpgradeFailed) || state.BlueGreenInProgress

	// Find active backup job
	if err := r.gatherBackupState(ctx, cluster, state); err != nil {
		return nil, err
	}

	// Get StatefulSet state
	if err := r.gatherStatefulSetState(ctx, logger, cluster, state); err != nil {
		return nil, err
	}

	// Get pod state
	if err := r.gatherPodState(ctx, cluster, state); err != nil {
		return nil, err
	}

	return state, nil
}

func (r *OpenBaoClusterReconciler) gatherBackupState(
	ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState,
) error {
	jobList := &batchv1.JobList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		constants.LabelAppInstance:      cluster.Name,
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:   cluster.Name,
		constants.LabelOpenBaoComponent: constants.ComponentBackup,
	})

	if err := r.List(ctx, jobList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return fmt.Errorf("failed to list backup Jobs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	for i := range jobList.Items {
		job := &jobList.Items[i]
		if job.Status.Succeeded == 0 && job.Status.Failed == 0 {
			state.BackupJobName = job.Name
			state.BackupInProgress = true
			break
		}
	}

	return nil
}

func (r *OpenBaoClusterReconciler) gatherStatefulSetState(
	ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState,
) error {
	statefulSetName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}

	// Compute active revision for blue/green deployments
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen {
		state.ActiveRevision = revision.OpenBaoClusterRevision(cluster.Spec.Version, cluster.Spec.Image, cluster.Spec.Replicas)
		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueRevision != "" {
			state.ActiveRevision = cluster.Status.BlueGreen.BlueRevision
			if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseDemotingBlue ||
				cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseCleanup {
				if cluster.Status.BlueGreen.GreenRevision != "" {
					state.ActiveRevision = cluster.Status.BlueGreen.GreenRevision
				}
			}
		}
		statefulSetName.Name = fmt.Sprintf("%s-%s", cluster.Name, state.ActiveRevision)
	}

	statefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, statefulSetName, statefulSet)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get StatefulSet %s/%s for status update: %w", cluster.Namespace, statefulSetName.Name, err)
		}
		// StatefulSet not found - cluster is initializing
		state.ReadyReplicas = 0
		state.Available = false
		return nil
	}

	state.StatefulSet = statefulSet
	state.ReadyReplicas = statefulSet.Status.ReadyReplicas
	desiredReplicas := cluster.Spec.Replicas
	state.Available = state.ReadyReplicas == desiredReplicas && state.ReadyReplicas > 0

	// Check if StatefulSet status may be stale
	statefulSetStillScaling := statefulSet.Status.ObservedGeneration < statefulSet.Generation ||
		statefulSet.Status.Replicas < desiredReplicas

	statusPotentiallyStale := statefulSet.Status.Replicas == desiredReplicas &&
		statefulSet.Status.ReadyReplicas < statefulSet.Status.Replicas &&
		statefulSet.Status.ReadyReplicas < desiredReplicas

	state.StatusStale = statefulSetStillScaling || statusPotentiallyStale

	logger.Info("StatefulSet status read for ReadyReplicas calculation",
		"statefulSetReadyReplicas", statefulSet.Status.ReadyReplicas,
		"statefulSetReplicas", statefulSet.Status.Replicas,
		"statefulSetCurrentReplicas", statefulSet.Status.CurrentReplicas,
		"statefulSetUpdatedReplicas", statefulSet.Status.UpdatedReplicas,
		"statefulSetObservedGeneration", statefulSet.Status.ObservedGeneration,
		"statefulSetGeneration", statefulSet.Generation,
		"desiredReplicas", desiredReplicas,
		"calculatedReadyReplicas", state.ReadyReplicas,
		"available", state.Available,
		"statusStale", state.StatusStale)

	return nil
}

func (r *OpenBaoClusterReconciler) gatherPodState(
	ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState,
) error {
	podSelector := map[string]string{
		constants.LabelAppInstance:  cluster.Name,
		constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
		constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
	}
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen && state.ActiveRevision != "" {
		podSelector[constants.LabelOpenBaoRevision] = state.ActiveRevision
	}

	var pods corev1.PodList
	if err := r.List(ctx, &pods,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(podSelector),
	); err != nil {
		return fmt.Errorf("failed to list pods for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	state.Pods = pods.Items

	// Find pod0 and leader
	pod0Name := fmt.Sprintf("%s-0", cluster.Name)
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen && state.ActiveRevision != "" {
		pod0Name = fmt.Sprintf("%s-%s-0", cluster.Name, state.ActiveRevision)
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Name == pod0Name {
			state.Pod0 = pod

			// Parse initialized label
			initialized, present, err := openbaolabels.ParseBoolLabel(pod.Labels, openbaolabels.LabelInitialized)
			if err == nil {
				state.Initialized = initialized
				state.InitializedKnown = present
			}

			// Parse sealed label
			sealed, present, err := openbaolabels.ParseBoolLabel(pod.Labels, openbaolabels.LabelSealed)
			if err == nil {
				state.Sealed = sealed
				state.SealedKnown = present
			}
		}

		// Check for active leader
		active, present, err := openbaolabels.ParseBoolLabel(pod.Labels, openbaolabels.LabelActive)
		if err == nil && present && active {
			state.LeaderCount++
			state.LeaderName = pod.Name
		}
	}

	return nil
}
