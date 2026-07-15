package statusops

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
)

// PodObserver exposes pod-local health observation used for read-serving state.
type PodObserver interface {
	Health(ctx context.Context) (*portopenbao.HealthStatus, error)
}

// PodObserverFactory constructs pod-local observers for OpenBao pods.
type PodObserverFactory func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (PodObserver, error)

// MembershipRuntime exposes authenticated raft membership reads.
type MembershipRuntime interface {
	ReadRaftConfiguration(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (*portopenbao.RaftConfigurationResponse, error)
	ReadRaftAutopilotState(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (*portopenbao.RaftAutopilotStateResponse, error)
}

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
	podObserverFactory PodObserverFactory,
	membershipRuntime MembershipRuntime,
	cluster *openbaov1alpha1.OpenBaoCluster,
	labelCfg LabelConfig,
) (*StatusState, error) {
	state := &StatusState{}

	// Compute upgrade state from cluster.Status.
	state.RollingUpgradeInProgress = cluster.Status.Upgrade != nil
	if state.RollingUpgradeInProgress && upgrade.UpgradeFailed(cluster.Status.Upgrade) {
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

	if err := gatherPVCState(ctx, reader, cluster, state, labelCfg); err != nil {
		return nil, err
	}

	if err := gatherPodState(ctx, reader, cluster, state, labelCfg); err != nil {
		return nil, err
	}

	gatherReadServingState(ctx, logger, reader, podObserverFactory, cluster, state, labelCfg)
	gatherRaftMembershipState(ctx, logger, membershipRuntime, cluster, state)
	gatherReadReplicaAutopilotHealthState(ctx, logger, membershipRuntime, cluster, state)

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

func gatherPVCState(
	ctx context.Context,
	reader client.Reader,
	cluster *openbaov1alpha1.OpenBaoCluster,
	state *StatusState,
	labelsCfg LabelConfig,
) error {
	var pvcList corev1.PersistentVolumeClaimList
	if err := reader.List(ctx, &pvcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{labelsCfg.OpenBaoClusterKey: cluster.Name}),
	); err != nil {
		return fmt.Errorf("failed to list PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	dataPVCPrefix := constants.VolumeData + "-" + cluster.Name + "-"
	readDataPVCPrefix := constants.VolumeData + "-" + cluster.Name + "-read-"
	storageClasses := map[string]struct{}{}
	readStorageClasses := map[string]struct{}{}
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if strings.HasPrefix(pvc.Name, readDataPVCPrefix) {
			state.ReadReplicaDataPVCCount++
			if pvc.Spec.StorageClassName == nil || strings.TrimSpace(*pvc.Spec.StorageClassName) == "" {
				state.ReadReplicaDataPVCStorageClassUnset = true
				continue
			}

			readStorageClasses[strings.TrimSpace(*pvc.Spec.StorageClassName)] = struct{}{}
			continue
		}
		if !strings.HasPrefix(pvc.Name, dataPVCPrefix) {
			continue
		}

		state.DataPVCCount++
		if pvc.Spec.StorageClassName == nil || strings.TrimSpace(*pvc.Spec.StorageClassName) == "" {
			state.DataPVCStorageClassUnset = true
			continue
		}

		storageClasses[strings.TrimSpace(*pvc.Spec.StorageClassName)] = struct{}{}
	}

	if len(storageClasses) == 0 {
		if len(readStorageClasses) == 0 {
			return nil
		}
	} else {
		state.DataPVCStorageClassNames = make([]string, 0, len(storageClasses))
		for className := range storageClasses {
			state.DataPVCStorageClassNames = append(state.DataPVCStorageClassNames, className)
		}
		sort.Strings(state.DataPVCStorageClassNames)
	}

	if len(readStorageClasses) == 0 {
		return nil
	}

	state.ReadReplicaDataPVCStorageClassNames = make([]string, 0, len(readStorageClasses))
	for className := range readStorageClasses {
		state.ReadReplicaDataPVCStorageClassNames = append(state.ReadReplicaDataPVCStorageClassNames, className)
	}
	sort.Strings(state.ReadReplicaDataPVCStorageClassNames)

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

	// Preserve the active revision across strategy changes so status follows the
	// existing workload rather than assuming the requested strategy renamed it.
	state.ActiveRevision = workloadsvc.BlueGreenActiveRevision(cluster)
	if state.ActiveRevision != "" {
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

	readReplicaDesired := int32(0)
	if cluster.Spec.ReadReplicas != nil {
		readReplicaDesired = cluster.Spec.ReadReplicas.Replicas
	}
	if readReplicaDesired == 0 {
		return nil
	}

	readStatefulSetName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      resourceidentity.ReadReplicaStatefulSetName(cluster),
	}
	readStatefulSet := &appsv1.StatefulSet{}
	if err := reader.Get(ctx, readStatefulSetName, readStatefulSet); err != nil {
		if apierrors.IsNotFound(err) {
			state.ReadReplicaReadyReplicas = 0
			return nil
		}
		return fmt.Errorf("failed to get read StatefulSet %s/%s for status update: %w", cluster.Namespace, readStatefulSetName.Name, err)
	}

	state.ReadReplicaStatefulSet = readStatefulSet
	state.ReadReplicaReadyReplicas = readStatefulSet.Status.ReadyReplicas

	return nil
}

func gatherPodState(
	ctx context.Context,
	reader client.Reader,
	cluster *openbaov1alpha1.OpenBaoCluster,
	state *StatusState,
	labelsCfg LabelConfig,
) error {
	podSelector := resourceidentity.VoterPodSelectorLabels(cluster)
	podSelector[labelsCfg.AppNameKey] = labelsCfg.AppNameValue
	podSelector[labelsCfg.AppManagedByKey] = labelsCfg.AppManagedByValue
	if state.ActiveRevision != "" {
		podSelector[labelsCfg.OpenBaoRevisionKey] = state.ActiveRevision
	} else if upgrade.EffectiveStrategy(cluster) == openbaov1alpha1.UpdateStrategyBlueGreen && cluster.Status.BlueGreen != nil {
		// A RollingUpdate-to-BlueGreen transition keeps the original StatefulSet
		// as Blue. Isolate those pods from Green using their controller revision
		// because the original workload has no OpenBao revision label.
		if controllerRevision := strings.TrimSpace(cluster.Status.BlueGreen.BlueControllerRevision); controllerRevision != "" {
			podSelector[appsv1.ControllerRevisionHashLabelKey] = controllerRevision
		}
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
	if state.ActiveRevision != "" {
		pod0Name = fmt.Sprintf("%s-%s-0", cluster.Name, state.ActiveRevision)
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Name == pod0Name {
			state.Pod0 = pod

			initialized, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelInitialized)
			if err == nil {
				state.Initialized = initialized
				state.InitializedKnown = present
			}

			sealed, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelSealed)
			if err == nil {
				state.Sealed = sealed
				state.SealedKnown = present
			}
		}

		active, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelActive)
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

func gatherReadServingState(
	ctx context.Context,
	logger logr.Logger,
	reader client.Reader,
	podObserverFactory PodObserverFactory,
	cluster *openbaov1alpha1.OpenBaoCluster,
	state *StatusState,
	labelsCfg LabelConfig,
) {
	if cluster == nil || cluster.Spec.ReadReplicas == nil || cluster.Spec.ReadReplicas.Replicas == 0 {
		return
	}
	if state == nil || state.ReadReplicaReadyReplicas == 0 || podObserverFactory == nil {
		return
	}

	readSelector := resourceidentity.ReadReplicaPodSelectorLabels(cluster)
	readSelector[labelsCfg.AppNameKey] = labelsCfg.AppNameValue
	readSelector[labelsCfg.AppManagedByKey] = labelsCfg.AppManagedByValue

	var pods corev1.PodList
	if err := reader.List(ctx, &pods,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(readSelector),
	); err != nil {
		logger.Info("Failed to list read-replica pods for read-serving observation", "error", err)
		return
	}

	observed := 0
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !isPodReady(pod) {
			continue
		}

		observer, err := podObserverFactory(ctx, cluster, pod.Name)
		if err != nil {
			logger.Info("Failed to create pod observer for read-replica pod", "pod", pod.Name, "error", err)
			continue
		}

		health, err := observer.Health(ctx)
		if err != nil {
			logger.Info("Failed to read pod-local health for read-replica pod", "pod", pod.Name, "error", err)
			continue
		}

		observed++
		if health != nil && health.Initialized && !health.Sealed {
			state.ReadServingAvailable = true
		}
	}

	if observed > 0 {
		state.ReadServingKnown = true
	}
}

func gatherRaftMembershipState(
	ctx context.Context,
	logger logr.Logger,
	membershipRuntime MembershipRuntime,
	cluster *openbaov1alpha1.OpenBaoCluster,
	state *StatusState,
) {
	if cluster == nil || cluster.Spec.ReadReplicas == nil || cluster.Spec.ReadReplicas.Replicas == 0 {
		return
	}
	if membershipRuntime == nil || state == nil {
		return
	}

	raftConfig, err := membershipRuntime.ReadRaftConfiguration(ctx, logger, cluster)
	if err != nil {
		logger.Info("Failed to observe Raft membership for read replicas", "error", err)
		return
	}
	if raftConfig == nil {
		return
	}

	prefix := resourceidentity.ReadReplicaStatefulSetName(cluster) + "-"
	count := int32(0)
	for _, server := range raftConfig.Config.Servers {
		if server.Voter {
			continue
		}
		if strings.HasPrefix(server.NodeID, prefix) || strings.Contains(server.Address, prefix) {
			count++
		}
	}

	state.ReadReplicaRegisteredReplicas = count
	state.ReadReplicaMembershipKnown = true
}

func gatherReadReplicaAutopilotHealthState(
	ctx context.Context,
	logger logr.Logger,
	membershipRuntime MembershipRuntime,
	cluster *openbaov1alpha1.OpenBaoCluster,
	state *StatusState,
) {
	if cluster == nil || cluster.Spec.ReadReplicas == nil || cluster.Spec.ReadReplicas.Replicas == 0 {
		return
	}
	if membershipRuntime == nil || state == nil {
		return
	}

	autopilotState, err := membershipRuntime.ReadRaftAutopilotState(ctx, logger, cluster)
	if err != nil {
		logger.Info("Failed to observe Raft Autopilot health for read replicas", "error", err)
		return
	}
	if autopilotState == nil {
		return
	}

	prefix := resourceidentity.ReadReplicaStatefulSetName(cluster) + "-"
	count := int32(0)
	for _, server := range autopilotState.Servers {
		if strings.HasPrefix(server.ID, prefix) || strings.HasPrefix(server.Name, prefix) || strings.Contains(server.Address, prefix) {
			if server.Healthy {
				count++
			}
		}
	}

	state.ReadReplicaHealthyReplicas = count
	state.ReadReplicaAutopilotKnown = true
}

func isPodReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	for i := range pod.Status.Conditions {
		c := pod.Status.Conditions[i]
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
