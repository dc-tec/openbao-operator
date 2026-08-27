package restore

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceownership"
	portworkload "github.com/dc-tec/openbao-operator/internal/port/workload"
)

func restoreRestartToken(restore *openbaov1alpha1.OpenBaoRestore) string {
	if restore == nil {
		return ""
	}
	if restore.UID != "" {
		return string(restore.UID)
	}
	return restore.Name
}

func clusterRestoreMatches(cluster *openbaov1alpha1.OpenBaoCluster, restore *openbaov1alpha1.OpenBaoRestore) bool {
	if cluster == nil || cluster.Status.Restore == nil || restore == nil {
		return false
	}
	return cluster.Status.Restore.Name == restore.Name && cluster.Status.Restore.UID == restoreRestartToken(restore)
}

func clusterRestoreRestartCompleted(cluster *openbaov1alpha1.OpenBaoCluster, restore *openbaov1alpha1.OpenBaoRestore) bool {
	return clusterRestoreMatches(cluster, restore) && cluster.Status.Restore.RestartCompletedAt != nil
}

func (m *Manager) requestPostRestoreRestart(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
	restore *openbaov1alpha1.OpenBaoRestore,
) (bool, error) {
	if clusterRestoreMatches(cluster, restore) {
		return false, nil
	}
	if m.adminOpsMutator == nil {
		return false, fmt.Errorf("adminops status mutator is required")
	}

	err := m.adminOpsMutator(
		ctx,
		cluster,
		func(obj *openbaov1alpha1.OpenBaoCluster) error {
			obj.Status.Restore = &openbaov1alpha1.ClusterRestoreStatus{
				Name: restore.Name,
				UID:  restoreRestartToken(restore),
			}
			return nil
		},
		false,
	)
	if err != nil {
		return false, fmt.Errorf("failed to request post-restore workload restart: %w", err)
	}

	return true, nil
}

func (m *Manager) markPostRestoreRestartCompleted(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
	restore *openbaov1alpha1.OpenBaoRestore,
) (bool, error) {
	if clusterRestoreRestartCompleted(cluster, restore) {
		return false, nil
	}
	if m.adminOpsMutator == nil {
		return false, fmt.Errorf("adminops status mutator is required")
	}

	err := m.adminOpsMutator(
		ctx,
		cluster,
		func(obj *openbaov1alpha1.OpenBaoCluster) error {
			if !clusterRestoreMatches(obj, restore) {
				return fmt.Errorf("cluster restore rollout no longer matches OpenBaoRestore %s/%s", restore.Namespace, restore.Name)
			}
			completedAt := metav1.Now()
			obj.Status.Restore.RestartCompletedAt = &completedAt
			return nil
		},
		false,
	)
	if err != nil {
		return false, fmt.Errorf("failed to record post-restore workload restart completion: %w", err)
	}

	return true, nil
}

func (m *Manager) postRestoreVoterRestartComplete(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
	restore *openbaov1alpha1.OpenBaoRestore,
) (bool, string, error) {
	statefulSetName := portworkload.StableVoterStatefulSetName(cluster)
	statefulSet := &appsv1.StatefulSet{}
	if err := m.reader.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      statefulSetName,
	}, statefulSet); err != nil {
		if apierrors.IsNotFound(err) {
			return false, fmt.Sprintf("Waiting for voter StatefulSet %s/%s before post-restore restart", cluster.Namespace, statefulSetName), nil
		}
		return false, "", fmt.Errorf("failed to get voter StatefulSet during post-restore restart: %w", err)
	}
	if err := resourceownership.RequireOwnerProof("verify post-restore voter restart", statefulSet, cluster); err != nil {
		return false, "", err
	}

	desired := cluster.Spec.Replicas
	appliedToken := strings.TrimSpace(statefulSet.Spec.Template.Annotations[constants.AnnotationRestoreRevision])
	targetToken := restoreRestartToken(restore)
	complete := appliedToken == targetToken &&
		statefulSet.Status.ObservedGeneration >= statefulSet.Generation &&
		statefulSet.Status.Replicas == desired &&
		statefulSet.Status.ReadyReplicas == desired &&
		statefulSet.Status.UpdatedReplicas == desired &&
		statefulSet.Status.CurrentReplicas == desired &&
		statefulSet.Status.UpdateRevision != "" &&
		statefulSet.Status.CurrentRevision == statefulSet.Status.UpdateRevision
	if complete {
		return true, "", nil
	}

	return false, fmt.Sprintf(
		"Waiting for voter Pods to restart after snapshot application: statefulSet=%s desiredReplicas=%d readyReplicas=%d updatedReplicas=%d currentReplicas=%d observedGeneration=%d generation=%d restoreRevisionApplied=%t currentRevision=%q updateRevision=%q",
		statefulSetName,
		desired,
		statefulSet.Status.ReadyReplicas,
		statefulSet.Status.UpdatedReplicas,
		statefulSet.Status.CurrentReplicas,
		statefulSet.Status.ObservedGeneration,
		statefulSet.Generation,
		appliedToken == targetToken,
		statefulSet.Status.CurrentRevision,
		statefulSet.Status.UpdateRevision,
	), nil
}

func (m *Manager) patchRestoreProgressMessage(ctx context.Context, restore *openbaov1alpha1.OpenBaoRestore, message string) error {
	if restore.Status.Message == message {
		return nil
	}
	original := restore.DeepCopy()
	restore.Status.Message = message
	if err := m.patchStatus(ctx, restore, original); err != nil {
		return fmt.Errorf("failed to patch restore progress status: %w", err)
	}
	return nil
}
