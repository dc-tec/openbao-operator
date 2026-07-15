package workload

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReconcileIdleStatefulSetUpdateStrategySwitchesToOnDeleteAtomically(t *testing.T) {
	cluster := workloadOwnershipCluster()
	partition := int32(0)
	statefulSet := workloadOwnershipStatefulSet(cluster.Name, cluster.Namespace, workloadOwnershipRef(cluster), 3)
	statefulSet.UID = types.UID("stable-statefulset-uid")
	statefulSet.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
			Partition: &partition,
		},
	}
	mgr := workloadOwnershipManager(cluster, statefulSet)

	if err := mgr.reconcileIdleStatefulSetUpdateStrategy(context.Background(), logr.Discard(), cluster, statefulSet, false); err != nil {
		t.Fatalf("reconcileIdleStatefulSetUpdateStrategy() error = %v", err)
	}

	stored := &appsv1.StatefulSet{}
	if err := mgr.client.Get(context.Background(), client.ObjectKeyFromObject(statefulSet), stored); err != nil {
		t.Fatalf("get transitioned StatefulSet: %v", err)
	}
	if stored.UID != statefulSet.UID {
		t.Fatalf("StatefulSet UID = %q, want stable UID %q", stored.UID, statefulSet.UID)
	}
	if stored.Spec.UpdateStrategy.Type != appsv1.OnDeleteStatefulSetStrategyType {
		t.Fatalf("update strategy type = %q, want %q", stored.Spec.UpdateStrategy.Type, appsv1.OnDeleteStatefulSetStrategyType)
	}
	if stored.Spec.UpdateStrategy.RollingUpdate != nil {
		t.Fatalf("rollingUpdate = %#v, want nil for OnDelete", stored.Spec.UpdateStrategy.RollingUpdate)
	}
}

func TestReconcileIdleStatefulSetUpdateStrategySwitchesToRollingUpdate(t *testing.T) {
	cluster := workloadOwnershipCluster()
	statefulSet := workloadOwnershipStatefulSet(cluster.Name, cluster.Namespace, workloadOwnershipRef(cluster), 3)
	statefulSet.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.OnDeleteStatefulSetStrategyType,
	}
	statefulSet.Status.CurrentRevision = "current-blue-revision"
	statefulSet.Status.UpdateRevision = "post-promotion-template-revision"
	mgr := workloadOwnershipManager(cluster, statefulSet)

	if err := mgr.reconcileIdleStatefulSetUpdateStrategy(context.Background(), logr.Discard(), cluster, statefulSet, true); err != nil {
		t.Fatalf("reconcileIdleStatefulSetUpdateStrategy() error = %v", err)
	}

	stored := &appsv1.StatefulSet{}
	if err := mgr.client.Get(context.Background(), client.ObjectKeyFromObject(statefulSet), stored); err != nil {
		t.Fatalf("get transitioned StatefulSet: %v", err)
	}
	if stored.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		t.Fatalf("update strategy type = %q, want %q", stored.Spec.UpdateStrategy.Type, appsv1.RollingUpdateStatefulSetStrategyType)
	}
	if stored.Spec.UpdateStrategy.RollingUpdate == nil || stored.Spec.UpdateStrategy.RollingUpdate.Partition == nil {
		t.Fatalf("rollingUpdate partition = %#v, want %d", stored.Spec.UpdateStrategy.RollingUpdate, cluster.Spec.Replicas)
	}
	if partition := *stored.Spec.UpdateStrategy.RollingUpdate.Partition; partition != cluster.Spec.Replicas {
		t.Fatalf("rollingUpdate partition = %d, want %d", partition, cluster.Spec.Replicas)
	}
}

func TestReconcileIdleStatefulSetUpdateStrategyRejectsUnownedStatefulSet(t *testing.T) {
	cluster := workloadOwnershipCluster()
	statefulSet := workloadOwnershipStatefulSet(cluster.Name, cluster.Namespace, nil, 3)
	statefulSet.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
	mgr := workloadOwnershipManager(cluster, statefulSet)

	err := mgr.reconcileIdleStatefulSetUpdateStrategy(context.Background(), logr.Discard(), cluster, statefulSet, false)
	if err == nil || !strings.Contains(err.Error(), "requires OpenBaoCluster owner proof") {
		t.Fatalf("reconcileIdleStatefulSetUpdateStrategy() error = %v, want owner proof error", err)
	}
}
