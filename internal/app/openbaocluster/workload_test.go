package openbaocluster

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

type autopilotRuntimeStub struct {
	reconcileFunc func(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error
}

func (s autopilotRuntimeStub) ReconcileAutopilotConfig(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if s.reconcileFunc != nil {
		return s.reconcileFunc(ctx, logger, cluster)
	}
	return nil
}

type readerStub struct {
	getFunc func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
}

func (s readerStub) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if s.getFunc != nil {
		return s.getFunc(ctx, key, obj, opts...)
	}
	return nil
}

func (s readerStub) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return nil
}

func TestAutopilotConfigReconciler_Reconcile_UsesStatefulSetReplicasDuringScaleDown(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))
	assert.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "openbaocluster-demo",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 1,
		},
	}
	currentReplicas := int32(2)
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &currentReplicas,
		},
	}

	k8sReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(statefulSet).
		Build()

	var gotReplicas int32
	reconciler := &autopilotConfigReconciler{
		autopilotRuntime: autopilotRuntimeStub{
			reconcileFunc: func(_ context.Context, _ logr.Logger, gotCluster *openbaov1alpha1.OpenBaoCluster) error {
				gotReplicas = gotCluster.Spec.Replicas
				return nil
			},
		},
		statefulSetReader: k8sReader,
		requeueShort:      5 * time.Second,
	}

	result, err := reconciler.Reconcile(context.Background(), logr.Discard(), cluster)
	assert.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
	assert.Equal(t, int32(2), gotReplicas)
	assert.Equal(t, int32(1), cluster.Spec.Replicas)
}

func TestAutopilotConfigReconciler_Reconcile_UsesRevisionedStatefulSetDuringScaleDown(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))
	assert.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "openbaocluster-demo",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 1,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				BlueRevision: "blue-rev",
			},
		},
	}
	currentReplicas := int32(2)
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-blue-rev",
			Namespace: cluster.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &currentReplicas,
		},
	}

	k8sReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(statefulSet).
		Build()

	var gotReplicas int32
	reconciler := &autopilotConfigReconciler{
		autopilotRuntime: autopilotRuntimeStub{
			reconcileFunc: func(_ context.Context, _ logr.Logger, gotCluster *openbaov1alpha1.OpenBaoCluster) error {
				gotReplicas = gotCluster.Spec.Replicas
				return nil
			},
		},
		statefulSetReader: k8sReader,
		requeueShort:      5 * time.Second,
	}

	result, err := reconciler.Reconcile(context.Background(), logr.Discard(), cluster)
	assert.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
	assert.Equal(t, int32(2), gotReplicas)
	assert.Equal(t, int32(1), cluster.Spec.Replicas)
}

func TestAutopilotConfigReconciler_Reconcile_RequeuesWhenStatefulSetReadFails(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "openbaocluster-demo",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 1,
		},
	}

	runtimeCalled := false
	reconciler := &autopilotConfigReconciler{
		autopilotRuntime: autopilotRuntimeStub{
			reconcileFunc: func(_ context.Context, _ logr.Logger, _ *openbaov1alpha1.OpenBaoCluster) error {
				runtimeCalled = true
				return nil
			},
		},
		statefulSetReader: readerStub{
			getFunc: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("boom")
			},
		},
		requeueShort: 5 * time.Second,
	}

	result, err := reconciler.Reconcile(context.Background(), logr.Discard(), cluster)
	assert.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter)
	assert.False(t, runtimeCalled)
}
