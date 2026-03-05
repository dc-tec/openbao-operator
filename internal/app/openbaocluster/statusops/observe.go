package statusops

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
	inframanager "github.com/dc-tec/openbao-operator/internal/infra"
)

// LabelConfig supplies labels used during status observation.
type LabelConfig struct {
	AppInstanceKey       string
	AppManagedByKey      string
	AppManagedByValue    string
	OpenBaoClusterKey    string
	OpenBaoComponentKey  string
	BackupComponentValue string
	AppNameKey           string
	AppNameValue         string
	OpenBaoRevisionKey   string
}

// GatherState performs API calls to observe cluster state for status computation.
func GatherState(
	ctx context.Context,
	logger logr.Logger,
	reader client.Reader,
	cluster *openbaov1alpha1.OpenBaoCluster,
	labelCfg LabelConfig,
) (*StatusState, error) {
	state := &StatusState{}

	// Compute upgrade state from cluster.Status.
	state.RollingUpgradeInProgress = cluster.Status.Upgrade != nil
	if state.RollingUpgradeInProgress && cluster.Status.Upgrade.LastErrorReason != "" {
		state.UpgradeFailed = true
	}

	state.BlueGreenInProgress = cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.Phase != "" &&
		cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle

	state.UpgradeInProgress = (state.RollingUpgradeInProgress && !state.UpgradeFailed) || state.BlueGreenInProgress

	if err := gatherBackupState(ctx, reader, cluster, state, labelCfg); err != nil {
		return nil, err
	}

	if err := gatherStatefulSetState(ctx, logger, reader, cluster, state); err != nil {
		return nil, err
	}

	if err := gatherPodState(ctx, reader, cluster, state, labelCfg); err != nil {
		return nil, err
	}

	return state, nil
}

func gatherBackupState(
	ctx context.Context,
	reader client.Reader,
	cluster *openbaov1alpha1.OpenBaoCluster,
	state *StatusState,
	labelsCfg LabelConfig,
) error {
	jobList := &batchv1.JobList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		labelsCfg.AppInstanceKey:      cluster.Name,
		labelsCfg.AppManagedByKey:     labelsCfg.AppManagedByValue,
		labelsCfg.OpenBaoClusterKey:   cluster.Name,
		labelsCfg.OpenBaoComponentKey: labelsCfg.BackupComponentValue,
	})

	if err := reader.List(ctx, jobList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return fmt.Errorf("failed to list backup Jobs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	selectedActiveJobName := ""
	for i := range jobList.Items {
		job := &jobList.Items[i]
		if job.Status.Succeeded != 0 || job.Status.Failed != 0 {
			continue
		}
		if selectedActiveJobName == "" || job.Name < selectedActiveJobName {
			selectedActiveJobName = job.Name
		}
	}
	if selectedActiveJobName != "" {
		state.BackupJobName = selectedActiveJobName
		state.BackupInProgress = true
	}

	return nil
}

func gatherStatefulSetState(
	ctx context.Context,
	logger logr.Logger,
	reader client.Reader,
	cluster *openbaov1alpha1.OpenBaoCluster,
	state *StatusState,
) error {
	statefulSetName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}

	// Compute active revision for blue/green deployments.
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen {
		state.ActiveRevision = inframanager.BlueGreenActiveRevision(cluster)
		statefulSetName.Name = fmt.Sprintf("%s-%s", cluster.Name, state.ActiveRevision)
	}

	statefulSet := &appsv1.StatefulSet{}
	err := reader.Get(ctx, statefulSetName, statefulSet)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get StatefulSet %s/%s for status update: %w", cluster.Namespace, statefulSetName.Name, err)
		}
		// StatefulSet not found: cluster is initializing.
		state.ReadyReplicas = 0
		state.Available = false
		return nil
	}

	state.StatefulSet = statefulSet
	state.ReadyReplicas = statefulSet.Status.ReadyReplicas
	desiredReplicas := cluster.Spec.Replicas
	state.Available = state.ReadyReplicas == desiredReplicas && state.ReadyReplicas > 0

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

func gatherPodState(
	ctx context.Context,
	reader client.Reader,
	cluster *openbaov1alpha1.OpenBaoCluster,
	state *StatusState,
	labelsCfg LabelConfig,
) error {
	podSelector := map[string]string{
		labelsCfg.AppInstanceKey:  cluster.Name,
		labelsCfg.AppNameKey:      labelsCfg.AppNameValue,
		labelsCfg.AppManagedByKey: labelsCfg.AppManagedByValue,
	}
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen && state.ActiveRevision != "" {
		podSelector[labelsCfg.OpenBaoRevisionKey] = state.ActiveRevision
	}

	var pods corev1.PodList
	if err := reader.List(ctx, &pods,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(podSelector),
	); err != nil {
		return fmt.Errorf("failed to list pods for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	state.Pods = pods.Items

	pod0Name := fmt.Sprintf("%s-0", cluster.Name)
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen && state.ActiveRevision != "" {
		pod0Name = fmt.Sprintf("%s-%s-0", cluster.Name, state.ActiveRevision)
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Name == pod0Name {
			state.Pod0 = pod

			initialized, present, err := constants.ParseBoolLabel(pod.Labels, constants.LabelOpenBaoInitialized)
			if err == nil {
				state.Initialized = initialized
				state.InitializedKnown = present
			}

			sealed, present, err := constants.ParseBoolLabel(pod.Labels, constants.LabelOpenBaoSealed)
			if err == nil {
				state.Sealed = sealed
				state.SealedKnown = present
			}
		}

		active, present, err := constants.ParseBoolLabel(pod.Labels, constants.LabelOpenBaoActive)
		if err == nil && present && active {
			state.LeaderCount++
			if state.LeaderCount == 1 {
				state.LeaderName = pod.Name
			} else {
				state.LeaderName = ""
			}
		}
	}

	return nil
}
