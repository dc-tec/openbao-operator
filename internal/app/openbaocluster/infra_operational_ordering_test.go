package openbaocluster

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
)

func TestApplyReadFirstRestartOrdering_StagesVoterRestartUntilReadPoolConverges(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
			Runtime: &openbaov1alpha1.RuntimeConfig{
				RestartAt: "2026-04-19T12:00:00Z",
			},
			ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{
				Replicas: 2,
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized: true,
		},
	}

	voterSpec := workloadsvc.StatefulSetSpec{Pool: constants.LabelValueOpenBaoWorkloadPoolVoter, Replicas: 3}
	readSpec := workloadsvc.StatefulSetSpec{Pool: constants.LabelValueOpenBaoWorkloadPoolReadReplica, Replicas: 2}

	currentVoter := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: restartTemplate("2026-04-18T00:00:00Z"),
		},
	}
	currentRead := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
		Spec: appsv1.StatefulSetSpec{
			Template: restartTemplate("2026-04-19T12:00:00Z"),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 2,
			ReadyReplicas:      1,
			UpdatedReplicas:    1,
			CurrentReplicas:    1,
			CurrentRevision:    "rev-a",
			UpdateRevision:     "rev-b",
		},
	}

	staged := (&infraReconciler{}).applyReadFirstRestartOrdering(cluster, &voterSpec, &readSpec, currentVoter, true, currentRead, true)
	if !staged {
		t.Fatal("expected voter restart to be staged behind read-pool convergence")
	}
	if voterSpec.RestartAt == nil || *voterSpec.RestartAt != "2026-04-18T00:00:00Z" {
		t.Fatalf("voter restartAt = %v, want preserved current value", voterSpec.RestartAt)
	}
	if readSpec.RestartAt == nil || *readSpec.RestartAt != "2026-04-19T12:00:00Z" {
		t.Fatalf("read restartAt = %v, want desired value", readSpec.RestartAt)
	}
}

func TestApplyReadFirstRestartOrdering_AllowsVoterRestartAfterReadPoolConverges(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
			Runtime: &openbaov1alpha1.RuntimeConfig{
				RestartAt: "2026-04-19T12:00:00Z",
			},
			ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{
				Replicas: 2,
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized: true,
		},
	}

	voterSpec := workloadsvc.StatefulSetSpec{Pool: constants.LabelValueOpenBaoWorkloadPoolVoter, Replicas: 3}
	readSpec := workloadsvc.StatefulSetSpec{Pool: constants.LabelValueOpenBaoWorkloadPoolReadReplica, Replicas: 2}

	currentVoter := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: restartTemplate("2026-04-18T00:00:00Z"),
		},
	}
	currentRead := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
		Spec: appsv1.StatefulSetSpec{
			Template: restartTemplate("2026-04-19T12:00:00Z"),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 2,
			ReadyReplicas:      2,
			UpdatedReplicas:    2,
			CurrentReplicas:    2,
			CurrentRevision:    "rev-b",
			UpdateRevision:     "rev-b",
		},
	}

	staged := (&infraReconciler{}).applyReadFirstRestartOrdering(cluster, &voterSpec, &readSpec, currentVoter, true, currentRead, true)
	if staged {
		t.Fatal("expected voter restart ordering to be complete once read pool converges")
	}
	if voterSpec.RestartAt == nil || *voterSpec.RestartAt != "2026-04-19T12:00:00Z" {
		t.Fatalf("voter restartAt = %v, want desired value", voterSpec.RestartAt)
	}
}

func TestPVCRestartPoolPriority_PrefersReadReplicaPool(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	readPod := resourceidentity.ReadReplicaStatefulSetName(cluster) + "-0"
	voterPod := cluster.Name + "-0"
	if got := pvcRestartPoolPriority(cluster, readPod); got != 0 {
		t.Fatalf("read pool priority = %d, want 0", got)
	}
	if got := pvcRestartPoolPriority(cluster, voterPod); got != 1 {
		t.Fatalf("voter pool priority = %d, want 1", got)
	}
}

func restartTemplate(restartAt string) corev1.PodTemplateSpec {
	annotations := map[string]string{}
	if restartAt != "" {
		annotations[constants.AnnotationRestartAt] = restartAt
	}
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: annotations,
		},
	}
}
