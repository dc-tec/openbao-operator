package upgrade

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

const ReasonUpgradeStrategyTransitionBlocked = "UpgradeStrategyTransitionBlocked"

// StrategyTransitionManager accepts upgrade strategy changes only after the
// current workload and every long-running operation have reached steady state.
type StrategyTransitionManager struct {
	reader client.Reader
}

// NewStrategyTransitionManager creates the strategy transition safety manager.
func NewStrategyTransitionManager(reader client.Reader) *StrategyTransitionManager {
	return &StrategyTransitionManager{reader: reader}
}

// Reconcile initializes strategy tracking and accepts safe idle transitions.
func (m *StrategyTransitionManager) Reconcile(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (recon.Result, error) {
	if cluster == nil || m.reader == nil {
		return recon.Result{}, nil
	}

	desired := DesiredStrategy(cluster)
	if cluster.Status.AcceptedUpgradeStrategy == "" {
		inferred, err := m.inferCurrentStrategy(ctx, cluster)
		if err != nil {
			return recon.Result{}, operatorerrors.WithReason(
				ReasonUpgradeStrategyTransitionBlocked,
				operatorerrors.WrapTransientKubernetesAPI(err),
			)
		}
		cluster.Status.AcceptedUpgradeStrategy = inferred
		logger.Info("Initialized accepted upgrade strategy", "acceptedStrategy", inferred)
	}

	accepted := cluster.Status.AcceptedUpgradeStrategy
	if desired == accepted {
		return recon.Result{}, nil
	}

	// Keep the previously accepted manager running until any active operation
	// completes. EffectiveStrategy ensures the requested strategy cannot take
	// over mid-operation.
	if hasActiveClusterOperation(cluster) {
		logger.Info(
			"Deferring requested upgrade strategy change until the active operation completes",
			"acceptedStrategy", accepted,
			"requestedStrategy", desired,
		)
		return recon.Result{}, nil
	}

	if err := validateIdleStrategyTransitionStatus(cluster, desired); err != nil {
		return recon.Result{}, blockedStrategyTransition(err)
	}
	activeStatefulSet, err := m.validateSteadyWorkload(ctx, cluster)
	if err != nil {
		return recon.Result{}, blockedStrategyTransition(err)
	}
	if desired == openbaov1alpha1.UpdateStrategyBlueGreen {
		if err := validateBlueGreenTransitionPrerequisites(cluster); err != nil {
			return recon.Result{}, blockedStrategyTransition(err)
		}
	}

	normalizeStrategyTransitionStatus(cluster, desired, activeStatefulSet)
	cluster.Status.AcceptedUpgradeStrategy = desired
	logger.Info(
		"Accepted idle upgrade strategy change",
		"previousStrategy", accepted,
		"acceptedStrategy", desired,
		"statefulSet", activeStatefulSet.Name,
	)

	return recon.Result{RequeueAfter: constants.RequeueShort}, nil
}

func blockedStrategyTransition(err error) error {
	return operatorerrors.WithReason(
		ReasonUpgradeStrategyTransitionBlocked,
		operatorerrors.WrapTransientClusterState(err),
	)
}

func (m *StrategyTransitionManager) inferCurrentStrategy(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (openbaov1alpha1.UpdateStrategyType, error) {
	if cluster.Status.BlueGreen != nil {
		return openbaov1alpha1.UpdateStrategyBlueGreen, nil
	}

	statefulSets, err := m.voterStatefulSets(ctx, cluster)
	if err != nil {
		return "", err
	}
	if len(statefulSets) == 0 {
		return DesiredStrategy(cluster), nil
	}

	foundRolling := false
	foundBlueGreen := false
	for i := range statefulSets {
		if statefulSets[i].Name == cluster.Name {
			foundRolling = true
		} else {
			foundBlueGreen = true
		}
	}
	if foundRolling && foundBlueGreen {
		return "", fmt.Errorf("cannot infer current upgrade strategy while both rolling and revisioned voter StatefulSets exist")
	}
	if foundBlueGreen {
		return openbaov1alpha1.UpdateStrategyBlueGreen, nil
	}
	return openbaov1alpha1.UpdateStrategyRollingUpdate, nil
}

func hasActiveClusterOperation(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil {
		return false
	}
	if cluster.Status.OperationLock != nil || cluster.Status.Upgrade != nil {
		return true
	}
	if cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.Phase != "" &&
		cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
		return true
	}
	return cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active
}

func validateIdleStrategyTransitionStatus(
	cluster *openbaov1alpha1.OpenBaoCluster,
	desired openbaov1alpha1.UpdateStrategyType,
) error {
	if !cluster.Status.Initialized {
		return fmt.Errorf("finish cluster initialization before changing spec.upgrade.strategy")
	}
	if cluster.Status.Phase != openbaov1alpha1.ClusterPhaseRunning {
		return fmt.Errorf("wait for status.phase=Running before changing spec.upgrade.strategy; current phase is %q", cluster.Status.Phase)
	}
	if strings.TrimSpace(cluster.Status.CurrentVersion) != strings.TrimSpace(cluster.Spec.Version) {
		return fmt.Errorf(
			"finish or recover the current version transition before changing spec.upgrade.strategy: status.currentVersion=%q spec.version=%q",
			cluster.Status.CurrentVersion,
			cluster.Spec.Version,
		)
	}
	if cluster.Status.ReadyReplicas != cluster.Spec.Replicas {
		return fmt.Errorf(
			"wait for all voter replicas to become ready before changing spec.upgrade.strategy: ready=%d desired=%d",
			cluster.Status.ReadyReplicas,
			cluster.Spec.Replicas,
		)
	}
	if available := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionAvailable)); available == nil || available.Status != metav1.ConditionTrue {
		return fmt.Errorf("wait for the Available condition to become True before changing spec.upgrade.strategy")
	}
	if degraded := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionDegraded)); degraded != nil && degraded.Status == metav1.ConditionTrue && degraded.Reason != ReasonUpgradeStrategyTransitionBlocked {
		return fmt.Errorf("resolve the Degraded condition before changing spec.upgrade.strategy")
	}
	if cluster.Status.Workload != nil && cluster.Status.Workload.LastError != nil {
		return fmt.Errorf("resolve the workload controller error before changing spec.upgrade.strategy")
	}
	if cluster.Status.Upgrade != nil {
		return fmt.Errorf("finish or recover the rolling upgrade before changing spec.upgrade.strategy")
	}
	if cluster.Status.OperationLock != nil {
		return fmt.Errorf(
			"wait for the %s operation lock held by %q to clear before changing spec.upgrade.strategy",
			cluster.Status.OperationLock.Operation,
			cluster.Status.OperationLock.Holder,
		)
	}
	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active {
		return fmt.Errorf("recover the cluster from safe mode before changing spec.upgrade.strategy")
	}
	if err := validateIdleBlueGreenStatus(cluster.Status.BlueGreen); err != nil {
		return err
	}
	if RetryRequestPending(cluster) || PromoteRequestPending(cluster) || RollbackRequestPending(cluster) {
		return fmt.Errorf("finish or clear pending spec.upgrade.requests before changing spec.upgrade.strategy")
	}
	if cluster.Spec.ReadReplicas != nil && cluster.Spec.ReadReplicas.Replicas > 0 {
		if cluster.Status.ReadReplicas == nil {
			return fmt.Errorf("wait for read replica status before changing spec.upgrade.strategy")
		}
		if cluster.Status.ReadReplicas.ReadyReplicas != cluster.Spec.ReadReplicas.Replicas {
			return fmt.Errorf(
				"wait for read replicas to become ready before changing spec.upgrade.strategy: ready=%d desired=%d",
				cluster.Status.ReadReplicas.ReadyReplicas,
				cluster.Spec.ReadReplicas.Replicas,
			)
		}
	}
	if desired != openbaov1alpha1.UpdateStrategyRollingUpdate && desired != openbaov1alpha1.UpdateStrategyBlueGreen {
		return fmt.Errorf("unsupported requested upgrade strategy %q", desired)
	}
	return nil
}

func validateIdleBlueGreenStatus(status *openbaov1alpha1.BlueGreenStatus) error {
	if status == nil {
		return nil
	}
	if status.Phase != "" && status.Phase != openbaov1alpha1.PhaseIdle {
		return fmt.Errorf("finish or recover the blue/green phase %q before changing spec.upgrade.strategy", status.Phase)
	}
	if strings.TrimSpace(status.GreenRevision) != "" {
		return fmt.Errorf("remove the Green revision before changing spec.upgrade.strategy")
	}
	if status.StartTime != nil || status.RollbackStartTime != nil || status.ManualPromotionRequired {
		return fmt.Errorf("finish the blue/green promotion or rollback before changing spec.upgrade.strategy")
	}
	if status.JobFailureCount != 0 || strings.TrimSpace(status.LastJobFailure) != "" {
		return fmt.Errorf("recover the blue/green job failure state before changing spec.upgrade.strategy")
	}
	return nil
}

func validateBlueGreenTransitionPrerequisites(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Spec.Upgrade == nil {
		return fmt.Errorf("spec.upgrade is required when switching to BlueGreen")
	}
	if strings.TrimSpace(resolveUpgradeJWTAuthRole(cluster)) == "" {
		return fmt.Errorf(
			"switching to BlueGreen requires spec.upgrade.jwtAuthRole or the default role from spec.selfInit.oidc bootstrap",
		)
	}
	if _, err := resolveUpgradeExecutorImage(cluster, ""); err != nil {
		return fmt.Errorf("resolve BlueGreen upgrade executor image: %w", err)
	}
	return nil
}

func (m *StrategyTransitionManager) validateSteadyWorkload(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (*appsv1.StatefulSet, error) {
	statefulSets, err := m.voterStatefulSets(ctx, cluster)
	if err != nil {
		return nil, err
	}
	activeName := StableVoterStatefulSetName(cluster)
	var active *appsv1.StatefulSet
	for i := range statefulSets {
		if statefulSets[i].Name == activeName {
			active = &statefulSets[i]
			continue
		}
		return nil, fmt.Errorf(
			"remove non-active voter StatefulSet %s before changing spec.upgrade.strategy",
			statefulSets[i].Name,
		)
	}
	if active == nil {
		return nil, fmt.Errorf("active voter StatefulSet %s/%s was not found", cluster.Namespace, activeName)
	}
	if err := validateStatefulSetSteady(cluster, active); err != nil {
		return nil, err
	}
	if err := m.validateVoterPodsSteady(ctx, cluster, activeName, active.Status.CurrentRevision); err != nil {
		return nil, err
	}
	if err := m.validateNoPVCResizePending(ctx, cluster); err != nil {
		return nil, err
	}
	return active, nil
}

func (m *StrategyTransitionManager) voterStatefulSets(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
) ([]appsv1.StatefulSet, error) {
	list := &appsv1.StatefulSetList{}
	if err := m.reader.List(ctx, list, client.InNamespace(cluster.Namespace), client.MatchingLabels(resourceidentity.Labels(cluster))); err != nil {
		return nil, fmt.Errorf("list managed StatefulSets: %w", err)
	}

	readName := resourceidentity.ReadReplicaStatefulSetName(cluster)
	statefulSets := make([]appsv1.StatefulSet, 0, len(list.Items))
	for i := range list.Items {
		if list.Items[i].Name == readName {
			continue
		}
		statefulSets = append(statefulSets, list.Items[i])
	}
	return statefulSets, nil
}

func validateStatefulSetSteady(cluster *openbaov1alpha1.OpenBaoCluster, sts *appsv1.StatefulSet) error {
	desired := cluster.Spec.Replicas
	if sts.Status.ObservedGeneration < sts.Generation ||
		sts.Spec.Replicas == nil || *sts.Spec.Replicas != desired ||
		sts.Status.Replicas != desired ||
		sts.Status.CurrentReplicas != desired ||
		sts.Status.ReadyReplicas != desired {
		return fmt.Errorf("wait for voter StatefulSet %s to finish scaling or restarting before changing spec.upgrade.strategy", sts.Name)
	}
	if strings.TrimSpace(sts.Status.CurrentRevision) == "" {
		return fmt.Errorf("wait for voter StatefulSet %s to report its current revision before changing spec.upgrade.strategy", sts.Name)
	}
	if sts.Spec.UpdateStrategy.Type != appsv1.OnDeleteStatefulSetStrategyType &&
		(sts.Status.UpdatedReplicas != desired || sts.Status.CurrentRevision != sts.Status.UpdateRevision) {
		return fmt.Errorf("wait for voter StatefulSet %s revisions to converge before changing spec.upgrade.strategy", sts.Name)
	}
	if sts.Spec.UpdateStrategy.RollingUpdate != nil &&
		sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil &&
		*sts.Spec.UpdateStrategy.RollingUpdate.Partition != 0 {
		return fmt.Errorf("wait for voter StatefulSet %s rolling partition to reach zero before changing spec.upgrade.strategy", sts.Name)
	}
	return nil
}

func (m *StrategyTransitionManager) validateVoterPodsSteady(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
	activeStatefulSetName string,
	currentControllerRevision string,
) error {
	pods := &corev1.PodList{}
	if err := m.reader.List(
		ctx,
		pods,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(resourceidentity.VoterPodSelectorLabels(cluster)),
	); err != nil {
		return fmt.Errorf("list voter pods: %w", err)
	}
	if len(pods.Items) != int(cluster.Spec.Replicas) {
		return fmt.Errorf(
			"wait for exactly %d active voter pods before changing spec.upgrade.strategy; found %d",
			cluster.Spec.Replicas,
			len(pods.Items),
		)
	}
	expectedPodNames := make(map[string]struct{}, cluster.Spec.Replicas)
	for ordinal := int32(0); ordinal < cluster.Spec.Replicas; ordinal++ {
		expectedPodNames[fmt.Sprintf("%s-%d", activeStatefulSetName, ordinal)] = struct{}{}
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if _, expected := expectedPodNames[pod.Name]; !expected {
			return fmt.Errorf("remove non-active voter pod %s before changing spec.upgrade.strategy", pod.Name)
		}
		if pod.DeletionTimestamp != nil || !podReady(pod) {
			return fmt.Errorf("wait for voter pod %s to become Ready before changing spec.upgrade.strategy", pod.Name)
		}
		if revision := strings.TrimSpace(pod.Labels[appsv1.ControllerRevisionHashLabelKey]); revision != strings.TrimSpace(currentControllerRevision) {
			return fmt.Errorf("wait for voter pod %s to converge on current controller revision %q before changing spec.upgrade.strategy", pod.Name, currentControllerRevision)
		}
	}
	return nil
}

func (m *StrategyTransitionManager) validateNoPVCResizePending(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
) error {
	pvcs := &corev1.PersistentVolumeClaimList{}
	if err := m.reader.List(ctx, pvcs, client.InNamespace(cluster.Namespace), client.MatchingLabels(resourceidentity.Labels(cluster))); err != nil {
		return fmt.Errorf("list managed PVCs: %w", err)
	}
	for i := range pvcs.Items {
		for _, condition := range pvcs.Items[i].Status.Conditions {
			if condition.Type == corev1.PersistentVolumeClaimFileSystemResizePending && condition.Status == corev1.ConditionTrue {
				return fmt.Errorf("finish the filesystem resize for PVC %s before changing spec.upgrade.strategy", pvcs.Items[i].Name)
			}
		}
	}
	return nil
}

func normalizeStrategyTransitionStatus(
	cluster *openbaov1alpha1.OpenBaoCluster,
	desired openbaov1alpha1.UpdateStrategyType,
	activeStatefulSet *appsv1.StatefulSet,
) {
	cluster.Status.Upgrade = nil
	if desired == openbaov1alpha1.UpdateStrategyBlueGreen {
		if cluster.Status.BlueGreen == nil {
			cluster.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{}
		}
		cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseIdle
		cluster.Status.BlueGreen.BlueRevision = strings.TrimSpace(activeStatefulSet.Spec.Template.Labels[constants.LabelOpenBaoRevision])
		cluster.Status.BlueGreen.BlueControllerRevision = strings.TrimSpace(activeStatefulSet.Status.CurrentRevision)
		cluster.Status.BlueGreen.BlueImage = strings.TrimSpace(containerImage(activeStatefulSet.Spec.Template.Spec.Containers, constants.ContainerBao))
	}
	core.ResetBlueGreenTransientState(cluster.Status.BlueGreen)
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func containerImage(containers []corev1.Container, name string) string {
	for i := range containers {
		if containers[i].Name == name {
			return containers[i].Image
		}
	}
	return ""
}
