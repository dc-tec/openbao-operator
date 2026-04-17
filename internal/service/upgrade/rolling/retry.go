package rolling

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/statusapply"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func (m *Manager) prepareFailedUpgradeRetry(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster == nil || cluster.Status.Upgrade == nil {
		return false, nil
	}

	if !upgrade.UpgradeFailed(cluster.Status.Upgrade) {
		return false, nil
	}

	if cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion {
		return false, nil
	}

	retryRequest := upgrade.RetryRequestValue(cluster)
	if !upgrade.RetryRequestPending(cluster) {
		return false, nil
	}

	logger.Info("Preparing retry for failed rolling upgrade",
		"retryRequest", retryRequest,
		"targetVersion", cluster.Status.Upgrade.TargetVersion,
		"currentPartition", cluster.Status.Upgrade.CurrentPartition)
	m.emitNormalEvent(cluster, upgrade.ReasonRollingRetryRequested, "Rolling upgrade retry requested for target version %s", cluster.Status.Upgrade.TargetVersion)

	if err := m.cleanupStepDownJobForRetry(ctx, logger, cluster); err != nil {
		return false, err
	}
	if err := m.resetTargetPodForRetry(ctx, logger, cluster); err != nil {
		return false, err
	}

	if err := m.patchRetryStatusSSA(ctx, cluster, retryRequest); err != nil {
		return false, fmt.Errorf("failed to clear failed upgrade state for retry: %w", err)
	}
	if cluster.Status.Upgrade == nil {
		return false, nil
	}
	upgrade.MarkRetryRequestHandled(&cluster.Status, retryRequest)

	logger.Info("Cleared failed rolling upgrade state and resumed upgrade")
	m.emitNormalEvent(cluster, upgrade.ReasonRollingRetryAccepted, "Rolling upgrade retry accepted for target version %s", cluster.Status.Upgrade.TargetVersion)
	return true, nil
}

func (m *Manager) patchRetryStatusSSA(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, retryRequest string) error {
	key := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}
	desired, err := statusapply.MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader(ctx, m.reader, m.client, key, func(obj *openbaov1alpha1.OpenBaoCluster) error {
		if obj.Status.Upgrade == nil {
			upgrade.MarkRetryRequestHandled(&obj.Status, retryRequest)
			return nil
		}
		clearUpgradeFailureForRetry(obj)
		upgrade.MarkRetryRequestHandled(&obj.Status, retryRequest)
		return nil
	}, statusapply.OpenBaoClusterAdminOpsStatusApplyOptions{
		ForceOwnership: true,
	})
	if err != nil {
		return fmt.Errorf("failed to apply cleared retry status: %w", err)
	}
	if retryFailureFieldsRemain(desired) {
		return fmt.Errorf("failed to clear failed rolling-upgrade state via SSA")
	}

	cluster.Status.Upgrade = desired.Status.Upgrade
	cluster.Status.UpgradeRequests = desired.Status.UpgradeRequests
	return nil
}

func retryFailureFieldsRemain(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Status.Upgrade == nil {
		return false
	}
	return upgrade.UpgradeFailed(cluster.Status.Upgrade) ||
		upgrade.UpgradeFailureMessage(cluster.Status.Upgrade) != "" ||
		upgrade.UpgradeFailureAt(cluster.Status.Upgrade) != nil ||
		cluster.Status.Upgrade.LastStepDownTime != nil
}

func (m *Manager) cleanupStepDownJobForRetry(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Status.Upgrade == nil {
		return nil
	}
	if cluster.Status.Upgrade.CurrentPartition <= 0 {
		return nil
	}

	targetOrdinal := cluster.Status.Upgrade.CurrentPartition - 1
	targetPod := fmt.Sprintf("%s-%d", cluster.Name, targetOrdinal)
	jobName := upgrade.ExecutorJobName(cluster.Name, upgrade.ExecutorActionRollingStepDownLeader, targetPod, "", "")
	jobKey := types.NamespacedName{Namespace: cluster.Namespace, Name: jobName}

	job := &batchv1.Job{}
	if err := m.client.Get(ctx, jobKey, job); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get step-down Job %s/%s for retry cleanup: %w", cluster.Namespace, jobName, err)
	}

	if err := m.client.Delete(ctx, job); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete step-down Job %s/%s for retry cleanup: %w", cluster.Namespace, jobName, err)
	}

	logger.Info("Deleted stale step-down Job before retry", "job", jobName, "pod", targetPod)
	return nil
}

func (m *Manager) resetTargetPodForRetry(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Status.Upgrade == nil {
		return nil
	}
	if cluster.Status.Upgrade.CurrentPartition <= 0 {
		return nil
	}

	targetOrdinal := cluster.Status.Upgrade.CurrentPartition - 1
	targetPod := fmt.Sprintf("%s-%d", cluster.Name, targetOrdinal)
	podKey := types.NamespacedName{Namespace: cluster.Namespace, Name: targetPod}

	sts := &appsv1.StatefulSet{}
	if err := m.client.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name}, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get StatefulSet %s/%s for retry pod reset: %w", cluster.Namespace, cluster.Name, err)
	}

	pod := &corev1.Pod{}
	if err := m.client.Get(ctx, podKey, pod); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get target pod %s/%s for retry reset: %w", cluster.Namespace, targetPod, err)
	}

	podRevision := ""
	if pod.Labels != nil {
		podRevision = strings.TrimSpace(pod.Labels[appsv1.StatefulSetRevisionLabel])
	}
	desiredRevision := strings.TrimSpace(sts.Status.UpdateRevision)
	podImage := strings.TrimSpace(baoContainerImage(pod.Spec.Containers))
	desiredImage := strings.TrimSpace(baoContainerImage(sts.Spec.Template.Spec.Containers))
	specImage := strings.TrimSpace(cluster.Spec.Image)

	if specImage != "" && desiredImage != specImage {
		logger.Info("Waiting for StatefulSet template to reflect retry target image before resetting failed pod",
			"pod", targetPod,
			"statefulSetImage", desiredImage,
			"specImage", specImage)
		return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf(
			"StatefulSet template still references %q while retry target image is %q",
			desiredImage,
			specImage,
		))
	}

	needsDelete := !isPodReady(pod)
	if desiredRevision != "" && podRevision != desiredRevision {
		needsDelete = true
	}
	if desiredImage != "" && podImage != desiredImage {
		needsDelete = true
	}
	if !needsDelete {
		return nil
	}

	if err := m.client.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete target pod %s/%s for retry reset: %w", cluster.Namespace, targetPod, err)
	}

	logger.Info("Deleted target pod before retry to force a fresh rollout attempt",
		"pod", targetPod,
		"podRevision", podRevision,
		"targetRevision", desiredRevision,
		"podImage", podImage,
		"desiredImage", desiredImage)
	return nil
}

func baoContainerImage(containers []corev1.Container) string {
	for i := range containers {
		if containers[i].Name == constants.ContainerBao {
			return containers[i].Image
		}
	}
	if len(containers) == 0 {
		return ""
	}
	return containers[0].Image
}

func clearUpgradeFailureForRetry(cluster *openbaov1alpha1.OpenBaoCluster) {
	if cluster == nil || cluster.Status.Upgrade == nil {
		return
	}

	now := metav1.Now()
	cluster.Status.Upgrade.Failure = nil
	cluster.Status.Upgrade.LastErrorReason = ""
	cluster.Status.Upgrade.LastErrorMessage = ""
	cluster.Status.Upgrade.LastErrorAt = nil
	cluster.Status.Upgrade.LastStepDownTime = nil
	cluster.Status.Upgrade.StartedAt = &now
}
