package rolling

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func rollingRetryToken(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Annotations == nil {
		return false
	}

	value := strings.TrimSpace(cluster.Annotations[constants.AnnotationRetryRollingUpgrade])
	return value != ""
}

func (m *Manager) prepareFailedUpgradeRetry(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster == nil || cluster.Status.Upgrade == nil {
		return false, nil
	}

	if strings.TrimSpace(cluster.Status.Upgrade.LastErrorReason) == "" {
		return false, nil
	}

	if cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion {
		return false, nil
	}

	if !rollingRetryToken(cluster) {
		return false, nil
	}

	logger.Info("Preparing retry for failed rolling upgrade",
		"targetVersion", cluster.Status.Upgrade.TargetVersion,
		"currentPartition", cluster.Status.Upgrade.CurrentPartition)

	if err := m.cleanupStepDownJobForRetry(ctx, logger, cluster); err != nil {
		return false, err
	}
	if err := m.clearRollingRetryAnnotation(ctx, cluster); err != nil {
		return false, err
	}

	// Re-read cluster after metadata patch to avoid stale-resource conflicts
	// when persisting status updates in a concurrent reconcile environment.
	latest := &openbaov1alpha1.OpenBaoCluster{}
	if err := m.client.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, latest); err != nil {
		return false, fmt.Errorf("failed to refresh cluster before retry status patch: %w", err)
	}
	if latest.Status.Upgrade == nil {
		return false, nil
	}

	if err := m.patchRetryStatusMerge(ctx, latest); err != nil {
		return false, fmt.Errorf("failed to clear failed upgrade state for retry: %w", err)
	}
	cluster.Status.Upgrade = latest.Status.Upgrade
	cluster.Annotations = latest.Annotations

	logger.Info("Cleared failed rolling upgrade state and resumed upgrade")
	return true, nil
}

func (m *Manager) patchRetryStatusMerge(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	current := &openbaov1alpha1.OpenBaoCluster{}
	key := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}
	if err := m.client.Get(ctx, key, current); err != nil {
		return fmt.Errorf("failed to refresh cluster before retry status patch: %w", err)
	}
	if current.Status.Upgrade == nil {
		cluster.Status.Upgrade = nil
		return nil
	}

	desired := current.DeepCopy()
	clearUpgradeFailureForRetry(desired)
	if err := m.client.Status().Patch(ctx, desired, client.MergeFrom(current)); err != nil {
		return fmt.Errorf("failed to patch cleared retry status: %w", err)
	}

	cluster.Status.Upgrade = desired.Status.Upgrade
	return nil
}

func (m *Manager) clearRollingRetryAnnotation(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Annotations == nil {
		return nil
	}
	if _, exists := cluster.Annotations[constants.AnnotationRetryRollingUpgrade]; !exists {
		return nil
	}

	original := cluster.DeepCopy()
	delete(cluster.Annotations, constants.AnnotationRetryRollingUpgrade)
	if len(cluster.Annotations) == 0 {
		cluster.Annotations = nil
	}

	if err := m.client.Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to clear %s annotation: %w", constants.AnnotationRetryRollingUpgrade, err)
	}

	return nil
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

func clearUpgradeFailureForRetry(cluster *openbaov1alpha1.OpenBaoCluster) {
	if cluster == nil || cluster.Status.Upgrade == nil {
		return
	}

	cluster.Status.Upgrade.LastErrorReason = ""
	cluster.Status.Upgrade.LastErrorMessage = ""
	cluster.Status.Upgrade.LastErrorAt = nil
	cluster.Status.Upgrade.LastStepDownTime = nil
}
