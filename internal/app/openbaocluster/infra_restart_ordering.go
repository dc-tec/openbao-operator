package openbaocluster

import (
	"strings"

	appsv1 "k8s.io/api/apps/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
)

func (r *infraReconciler) applyReadFirstRestartOrdering(
	cluster *openbaov1alpha1.OpenBaoCluster,
	spec *workloadsvc.StatefulSetSpec,
	readSpec *workloadsvc.StatefulSetSpec,
	currentSTS *appsv1.StatefulSet,
	currentSTSFound bool,
	readCurrentSTS *appsv1.StatefulSet,
	readCurrentSTSFound bool,
) bool {
	desiredRestartAt := clusterRestartAt(cluster)
	if desiredRestartAt == "" || spec == nil {
		return false
	}

	spec.RestartAt = stringPtr(desiredRestartAt)
	if readSpec != nil {
		readSpec.RestartAt = stringPtr(desiredRestartAt)
	}

	if cluster == nil || !cluster.Status.Initialized {
		return false
	}
	if readSpec == nil || readSpec.SkipReconciliation || readSpec.Replicas == 0 || cluster.Spec.ReadReplicas == nil {
		return false
	}
	if !currentSTSFound {
		return false
	}
	if statefulSetRestartAtSettled(readCurrentSTS, readCurrentSTSFound, desiredRestartAt, readSpec.Replicas) {
		return false
	}

	currentVoterRestartAt := currentStatefulSetRestartAt(currentSTS)
	spec.RestartAt = stringPtr(currentVoterRestartAt)
	return currentVoterRestartAt != desiredRestartAt
}

func clusterRestartAt(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil {
		return ""
	}
	if cluster.Spec.Runtime != nil {
		return strings.TrimSpace(cluster.Spec.Runtime.RestartAt)
	}
	return ""
}

func currentStatefulSetRestartAt(sts *appsv1.StatefulSet) string {
	if sts == nil || sts.Spec.Template.Annotations == nil {
		return ""
	}
	return strings.TrimSpace(sts.Spec.Template.Annotations[constants.AnnotationRestartAt])
}

func statefulSetRestartAtSettled(sts *appsv1.StatefulSet, found bool, desiredRestartAt string, replicas int32) bool {
	if !found || sts == nil {
		return false
	}
	if currentStatefulSetRestartAt(sts) != strings.TrimSpace(desiredRestartAt) {
		return false
	}
	if sts.Status.ObservedGeneration < sts.Generation {
		return false
	}
	if sts.Status.ReadyReplicas != replicas {
		return false
	}
	if sts.Status.UpdatedReplicas != replicas {
		return false
	}
	if sts.Status.CurrentReplicas != replicas {
		return false
	}
	if sts.Status.CurrentRevision != "" && sts.Status.UpdateRevision != "" && sts.Status.CurrentRevision != sts.Status.UpdateRevision {
		return false
	}
	return true
}

func stringPtr(value string) *string {
	return &value
}
