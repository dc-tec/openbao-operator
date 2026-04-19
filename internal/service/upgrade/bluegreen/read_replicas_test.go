package bluegreen

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
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

func TestHandlePhaseRestoringReadReplicas_WaitsForConvergence(t *testing.T) {
	scheme := newBlueGreenTestScheme(t)
	cluster := newBlueGreenCluster()
	cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{Replicas: 2}
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseRestoringReadReplicas
	cluster.Status.ReadReplicas = &openbaov1alpha1.ReadReplicaStatus{
		DesiredReplicas:    2,
		ReadyReplicas:      1,
		RegisteredReplicas: 1,
	}

	manager := &Manager{client: fake.NewClientBuilder().WithScheme(scheme).Build(), scheme: scheme}

	outcome, err := manager.handlePhaseRestoringReadReplicas(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("handlePhaseRestoringReadReplicas() error = %v", err)
	}
	if outcome.kind != phaseOutcomeRequeueAfter || outcome.after != constants.RequeueShort {
		t.Fatalf("handlePhaseRestoringReadReplicas() outcome = %+v, want short requeue", outcome)
	}
	if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseRestoringReadReplicas {
		t.Fatalf("phase = %s, want %s", cluster.Status.BlueGreen.Phase, openbaov1alpha1.PhaseRestoringReadReplicas)
	}
}

func TestHandlePhaseRestoringReadReplicas_FinalizesWhenHealthy(t *testing.T) {
	scheme := newBlueGreenTestScheme(t)
	cluster := newBlueGreenCluster()
	cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{Replicas: 2}
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseRestoringReadReplicas
	cluster.Status.ReadReplicas = &openbaov1alpha1.ReadReplicaStatus{
		DesiredReplicas:    2,
		ReadyReplicas:      2,
		RegisteredReplicas: 2,
	}
	cluster.Status.OperationLock = &openbaov1alpha1.OperationLockStatus{
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Holder:    core.UpgradeOperationLockHolder,
	}
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:   string(openbaov1alpha1.ConditionReadReplicasReady),
		Status: metav1.ConditionTrue,
	})
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:   string(openbaov1alpha1.ConditionReadServingAvailable),
		Status: metav1.ConditionTrue,
	})
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:   string(openbaov1alpha1.ConditionRaftMembershipReady),
		Status: metav1.ConditionTrue,
	})

	manager := &Manager{
		client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
			WithObjects(cluster).
			Build(),
		scheme: scheme,
	}

	outcome, err := manager.handlePhaseRestoringReadReplicas(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("handlePhaseRestoringReadReplicas() error = %v", err)
	}
	if outcome.kind != phaseOutcomeRequeueAfter || outcome.after != constants.RequeueShort {
		t.Fatalf("handlePhaseRestoringReadReplicas() outcome = %+v, want short requeue", outcome)
	}
	if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
		t.Fatalf("phase = %s, want %s", cluster.Status.BlueGreen.Phase, openbaov1alpha1.PhaseIdle)
	}
	if cluster.Status.OperationLock != nil {
		t.Fatal("expected operation lock to be released")
	}
}
