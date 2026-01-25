package openbaocluster

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	openbao "github.com/dc-tec/openbao-operator/internal/openbao"
)

func TestStorageReconciler_ExpandsPVCs(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Storage: openbaov1alpha1.StorageConfig{
				Size: "20Gi",
			},
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-0",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelOpenBaoCluster: "test",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	r := &storageReconciler{
		client:   c,
		recorder: record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	require.NoError(t, err)

	got := &corev1.PersistentVolumeClaim{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(pvc), got))
	require.Equal(t, resource.MustParse("20Gi"), got.Spec.Resources.Requests[corev1.ResourceStorage])
}

func TestStorageReconciler_RejectsShrink(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Storage: openbaov1alpha1.StorageConfig{
				Size: "5Gi",
			},
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-0",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelOpenBaoCluster: "test",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	r := &storageReconciler{
		client:   c,
		recorder: record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	require.Error(t, err)
	require.Contains(t, err.Error(), ReasonStorageShrinkNotSupported)
}

func TestStorageResizeRestartReconciler_RestartsFollowerPod(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Maintenance: &openbaov1alpha1.MaintenanceConfig{Enabled: true},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized: true,
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-0",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelOpenBaoCluster: "test",
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Conditions: []corev1.PersistentVolumeClaimCondition{
				{Type: corev1.PersistentVolumeClaimFileSystemResizePending, Status: corev1.ConditionTrue},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-0",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, pvc, pod).Build()

	r := &storageResizeRestartReconciler{
		client:    c,
		apiReader: c,
		recorder:  record.NewFakeRecorder(10),
		clientForPodFunc: func(_ *openbaov1alpha1.OpenBaoCluster, _ string) (openbao.ClusterActions, error) {
			return &openbao.MockClusterActions{
				IsLeaderFunc: func(ctx context.Context) (bool, error) { return false, nil },
			}, nil
		},
	}

	res, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	require.NoError(t, err)
	require.NotZero(t, res.RequeueAfter)

	got := &corev1.Pod{}
	err = c.Get(context.Background(), client.ObjectKeyFromObject(pod), got)
	require.Error(t, err)
}

func TestStorageResizeRestartReconciler_StepsDownLeaderFirst(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas:    3,
			Maintenance: &openbaov1alpha1.MaintenanceConfig{Enabled: true},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized: true,
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-0",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelOpenBaoCluster: "test",
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Conditions: []corev1.PersistentVolumeClaimCondition{
				{Type: corev1.PersistentVolumeClaimFileSystemResizePending, Status: corev1.ConditionTrue},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-0",
			Namespace: "default",
			Labels: map[string]string{
				openbao.LabelActive: "true",
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, pvc, pod).Build()

	stepDownCalled := 0
	r := &storageResizeRestartReconciler{
		client:    c,
		apiReader: c,
		recorder:  record.NewFakeRecorder(10),
		clientForPodFunc: func(_ *openbaov1alpha1.OpenBaoCluster, _ string) (openbao.ClusterActions, error) {
			return &openbao.MockClusterActions{
				IsLeaderFunc: func(ctx context.Context) (bool, error) { return true, nil },
				StepDownLeaderFunc: func(ctx context.Context) error {
					stepDownCalled++
					return nil
				},
			}, nil
		},
	}

	res, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	require.NoError(t, err)
	require.NotZero(t, res.RequeueAfter)
	require.Equal(t, 1, stepDownCalled)

	got := &corev1.Pod{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(pod), got))
}

func TestStorageResizeRestartReconciler_RequiresMaintenance(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized: true,
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-0",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelOpenBaoCluster: "test",
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Conditions: []corev1.PersistentVolumeClaimCondition{
				{Type: corev1.PersistentVolumeClaimFileSystemResizePending, Status: corev1.ConditionTrue},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, pvc).Build()

	r := &storageResizeRestartReconciler{
		client:    c,
		apiReader: c,
		recorder:  record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	require.Error(t, err)
	require.Contains(t, err.Error(), ReasonStorageRestartRequired)
}
