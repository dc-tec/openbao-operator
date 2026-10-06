package restore

import (
	"context"
	"fmt"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// RecoveryClient contains the authenticated operations required by Resume.
type RecoveryClient interface {
	ReadRaftConfiguration(context.Context) (*portopenbao.RaftConfigurationResponse, error)
	StepDownLeader(context.Context) error
}

// RecoveryClientFactory connects directly to a named Pod using operator credentials.
type RecoveryClientFactory func(context.Context, *api.OpenBaoCluster, string) (RecoveryClient, error)

// restartAcknowledgedTarget keeps the hold and lock until every recorded Pod has
// a ready replacement. The administrator remains responsible for process fencing.
func (m *Manager) restartAcknowledgedTarget(ctx context.Context, request *api.OpenBaoRestore, cluster *api.OpenBaoCluster) (bool, error) {
	restart := request.Status.Restart

	if restart != nil && restart.CompletedAt != nil {
		return true, nil
	}

	if cluster.DeletionTimestamp != nil || cluster.Spec.Paused ||
		cluster.Annotations[constants.AnnotationRestoreHold] != restoreExecutionOperationID(request) ||
		!restoreOperationLock(request).IsHeldBy(cluster.Status.OperationLock) {
		return false, recoveryBlocked("managed Resume requires the original held, locked, unpaused target; use Abandon for administrator recovery")
	}
	pods, err := m.recoveryPods(ctx, cluster)
	if err != nil {
		return false, err
	}
	if ready, err := checkRecoveryPods(pods, restart); !ready || err != nil {
		return false, err
	}

	leader, err := m.recoveryLeaderForTarget(ctx, cluster, pods)
	if err != nil {
		return false, err
	}

	if restart == nil {
		return false, m.recordRecoveryRestart(ctx, request, pods)
	}

	// Prefer remaining standbys. A leader is restarted only after step-down,
	// except in a single-voter cluster where the restart interrupts service.
	var candidate *corev1.Pod
	replaced := 0
	for i, original := range restart.Pods {
		if pods[i].UID != original.UID {
			replaced++
		}
		if pods[i].UID == original.UID && (candidate == nil || candidate.Name == leader) {
			candidate = &pods[i]
		}
	}

	if candidate == nil {
		if restart.CompletedAt != nil {
			return true, nil
		}
		before := request.DeepCopy()
		now := metav1.Now()
		request.Status.Restart.CompletedAt = &now
		return false, m.patchStatus(ctx, request, before)
	}

	if candidate.Name == leader && cluster.Spec.Replicas > 1 {
		bao, err := m.recoveryClientFor(ctx, cluster, candidate.Name)
		if err != nil {
			return false, err
		}
		err = bao.StepDownLeader(ctx)
		return false, &recoveryIssue{reason: ReasonRecoveryRestarting,
			message: fmt.Sprintf("Waiting for leader %s to step down; %d/%d Pods replaced and ready", candidate.Name, replaced, len(pods)), cause: err}
	}

	return m.restartRecoveryPod(ctx, candidate, replaced, len(pods))
}

func (m *Manager) restartRecoveryPod(ctx context.Context, candidate *corev1.Pod, replaced, total int) (bool, error) {
	// Maintenance prevents the termination hook from removing this Raft member.
	// Keep normal management held so its maintenance reconciler cannot undo this.
	before := candidate.DeepCopy()

	if candidate.Annotations == nil {
		candidate.Annotations = make(map[string]string)
	}
	candidate.Annotations[constants.AnnotationMaintenance] = "true"

	if err := m.client.Patch(ctx, candidate, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); err != nil {
		return false, err
	}
	err := m.client.Delete(ctx, candidate, client.Preconditions{UID: &candidate.UID, ResourceVersion: &candidate.ResourceVersion})
	return false, &recoveryIssue{reason: ReasonRecoveryRestarting,
		message: fmt.Sprintf("Restarting Pod %s; %d/%d Pods replaced and ready", candidate.Name, replaced, total), cause: client.IgnoreNotFound(err)}
}

func (m *Manager) recoveryPods(ctx context.Context, cluster *api.OpenBaoCluster) ([]corev1.Pod, error) {
	type pool struct {
		name     string
		replicas int32
	}
	pools := []pool{{restoreTargetStatefulSetName(cluster), cluster.Spec.Replicas}}

	if cluster.Spec.ReadReplicas != nil && cluster.Spec.ReadReplicas.Replicas > 0 {
		pools = append(pools, pool{resourceidentity.ReadReplicaStatefulSetName(cluster), cluster.Spec.ReadReplicas.Replicas})
	}
	var pods []corev1.Pod
	for _, pool := range pools {
		if pool.replicas < 1 {
			return nil, recoveryBlocked("managed Resume requires a nonempty voter pool")
		}
		sts := &appsv1.StatefulSet{}
		if err := m.reader.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: pool.name}, sts); err != nil {
			return nil, err
		}
		if !metav1.IsControlledBy(sts, cluster) || sts.DeletionTimestamp != nil || sts.Spec.Replicas == nil || *sts.Spec.Replicas != pool.replicas {
			return nil, recoveryBlocked("workload %s is not the stable target pool", pool.name)
		}
		for i := int32(0); i < pool.replicas; i++ {
			pod := &corev1.Pod{}
			if err := m.reader.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: fmt.Sprintf("%s-%d", pool.name, i)}, pod); err != nil {
				if apierrors.IsNotFound(err) {
					return nil, &recoveryIssue{reason: ReasonRecoveryWaitingForPod,
						message: fmt.Sprintf("Waiting for workload Pod %s-%d to exist; inspect its StatefulSet and scheduling events", pool.name, i)}
				}
				return nil, err
			}
			if pod.UID == "" || !metav1.IsControlledBy(pod, sts) {
				return nil, recoveryBlocked("workload Pod %s is not owned by the target StatefulSet", pod.Name)
			}
			pods = append(pods, *pod)
		}
	}

	return pods, nil
}

func recoveryLeader(cluster *api.OpenBaoCluster, pods []corev1.Pod, membership *portopenbao.RaftConfigurationResponse) (string, error) {
	if membership == nil {
		return "", fmt.Errorf("target returned no Raft membership")
	}
	leader := ""
	for i := range pods {
		index := slices.IndexFunc(membership.Config.Servers, func(server portopenbao.RaftServer) bool {
			return server.NodeID == pods[i].Name
		})
		if index < 0 || membership.Config.Servers[index].Voter != (i < int(cluster.Spec.Replicas)) {
			return "", fmt.Errorf("workload Pod %s does not have its expected Raft membership", pods[i].Name)
		}
		if membership.Config.Servers[index].Leader {
			if leader != "" || i >= int(cluster.Spec.Replicas) {
				return "", fmt.Errorf("target Raft membership has an invalid leader")
			}
			leader = pods[i].Name
		}
	}

	if leader == "" {
		return "", fmt.Errorf("target Raft membership has no workload leader")
	}
	return leader, nil
}

func checkRecoveryPods(pods []corev1.Pod, restart *api.RestoreRestartStatus) (bool, error) {
	replaced := 0
	if restart != nil {
		if len(pods) != len(restart.Pods) {
			return false, recoveryBlocked("workload topology changed during Resume; restore the topology or use Abandon")
		}
		for i, original := range restart.Pods {
			owner := metav1.GetControllerOf(&pods[i])
			if pods[i].Name != original.Name || owner.UID != original.StatefulSetUID {
				return false, recoveryBlocked("workload identity changed during Resume; use Abandon")
			}
			if pods[i].UID != original.UID {
				replaced++
			}
		}
	}
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil || !slices.ContainsFunc(pod.Status.Conditions, func(c corev1.PodCondition) bool {
			return c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue
		}) {
			message := "Waiting for Pod " + pod.Name + " to become Ready; inspect Pod events, unseal access, and readiness probes"
			if restart != nil {
				message += fmt.Sprintf(" (%d/%d Pods replaced)", replaced, len(pods))
			}
			return false, &recoveryIssue{reason: ReasonRecoveryWaitingForPod, message: message}
		}
	}

	return true, nil
}

// recoveryLeaderForTarget checks health, restored operator access, and membership.
func (m *Manager) recoveryLeaderForTarget(ctx context.Context, cluster *api.OpenBaoCluster, pods []corev1.Pod) (string, error) {
	if _, err := m.targetHealth(ctx, cluster); err != nil {
		return "", &recoveryIssue{reason: ReasonRecoveryHealthUnavailable,
			message: "Cannot confirm every voter is initialized, unsealed, and healthy; inspect voter health and TLS trust", cause: err}
	}

	if m.recoveryClientFor == nil {
		return "", recoveryBlocked("authenticated restore recovery client is required")
	}

	// Read through a voter to verify restored operator access and current membership.
	bao, err := m.recoveryClientFor(ctx, cluster, pods[0].Name)
	if err != nil {
		return "", &recoveryIssue{reason: ReasonRecoveryAccessUnavailable,
			message: "Cannot authenticate to voter " + pods[0].Name + "; repair restored operator JWT and TLS trust", cause: err}
	}
	membership, err := bao.ReadRaftConfiguration(ctx)
	if err != nil {
		return "", &recoveryIssue{reason: ReasonRecoveryAccessUnavailable,
			message: "Cannot read Raft membership through voter " + pods[0].Name + "; check operator authentication, permissions, and connectivity", cause: err}
	}
	leader, err := recoveryLeader(cluster, pods, membership)
	if err != nil {
		return "", &recoveryIssue{reason: ReasonRecoveryMembershipChanged, message: err.Error() + "; inspect Raft membership before continuing"}
	}

	return leader, nil
}

// recoveryBlocked reports an operator-defined Resume precondition in status. It
// keeps the regular recovery poll instead of controller error backoff.
func recoveryBlocked(format string, args ...any) error {
	return &recoveryIssue{reason: ReasonRecoveryBlocked, message: fmt.Sprintf(format, args...)}
}

func (m *Manager) recordRecoveryRestart(ctx context.Context, request *api.OpenBaoRestore, pods []corev1.Pod) error {
	before := request.DeepCopy()
	request.Status.Restart = &api.RestoreRestartStatus{}
	for i := range pods {
		pod := &pods[i]
		request.Status.Restart.Pods = append(request.Status.Restart.Pods, api.RestoreRestartPod{
			Name: pod.Name, UID: pod.UID, StatefulSetUID: metav1.GetControllerOf(pod).UID,
		})
	}
	request.Status.Message = "Administrator Resume accepted; restarting voters and read replicas while the management hold remains"
	return m.patchStatus(ctx, request, before)
}
