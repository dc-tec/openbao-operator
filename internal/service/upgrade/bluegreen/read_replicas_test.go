package bluegreen

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

func TestEnsureSteadyReadReplicasScaledDown_WaitsForDrain(t *testing.T) {
	scheme := newBlueGreenTestScheme(t)
	cluster := newBlueGreenCluster()
	cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{Replicas: 2}

	replicas := int32(2)
	readSTS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       resourceidentity.ReadReplicaStatefulSetName(cluster),
			Namespace:  cluster.Namespace,
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			Replicas:           2,
			ReadyReplicas:      2,
			CurrentReplicas:    2,
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readSTS).Build()
	manager := &Manager{client: client, reader: client, scheme: scheme}

	result, waiting, err := manager.ensureSteadyReadReplicasScaledDown(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("ensureSteadyReadReplicasScaledDown() error = %v", err)
	}
	if !waiting {
		t.Fatal("ensureSteadyReadReplicasScaledDown() waiting = false, want true")
	}
	if result.RequeueAfter != constants.RequeueShort {
		t.Fatalf("requeueAfter = %s, want %s", result.RequeueAfter, constants.RequeueShort)
	}
}

func TestEnsureSteadyReadReplicasScaledDown_AllowsProgressWhenScaledDown(t *testing.T) {
	scheme := newBlueGreenTestScheme(t)
	cluster := newBlueGreenCluster()
	cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{Replicas: 2}

	replicas := int32(0)
	readSTS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       resourceidentity.ReadReplicaStatefulSetName(cluster),
			Namespace:  cluster.Namespace,
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readSTS).Build()
	manager := &Manager{client: client, reader: client, scheme: scheme}

	result, waiting, err := manager.ensureSteadyReadReplicasScaledDown(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("ensureSteadyReadReplicasScaledDown() error = %v", err)
	}
	if waiting {
		t.Fatal("ensureSteadyReadReplicasScaledDown() waiting = true, want false")
	}
	if result != (recon.Result{}) {
		t.Fatalf("result = %+v, want empty", result)
	}
}
