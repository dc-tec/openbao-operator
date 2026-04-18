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
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	openbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
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
				labelOpenBaoCluster: "test",
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
	r := NewStorageReconciler(
		StorageDependencies{
			Resources: StorageResourceRuntime{Client: c},
			Events:    StorageEventRuntime{Recorder: events.NewFakeRecorder(10)},
		},
	)

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
				labelOpenBaoCluster: "test",
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
	r := NewStorageReconciler(
		StorageDependencies{
			Resources: StorageResourceRuntime{Client: c},
			Events:    StorageEventRuntime{Recorder: events.NewFakeRecorder(10)},
		},
	)

	_, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	require.Error(t, err)
	require.Contains(t, err.Error(), constants.ReasonStorageShrinkNotSupported)
}

func TestStorageReconciler_RejectsStorageClassChangeWhenPVCClassIsUnset(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	className := "gp3"
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Storage: openbaov1alpha1.StorageConfig{
				Size:             "10Gi",
				StorageClassName: &className,
			},
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-0",
			Namespace: "default",
			Labels: map[string]string{
				labelOpenBaoCluster: "test",
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
	r := NewStorageReconciler(
		StorageDependencies{
			Resources: StorageResourceRuntime{Client: c},
			Events:    StorageEventRuntime{Recorder: events.NewFakeRecorder(10)},
		},
	)

	_, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	require.Error(t, err)
	require.Contains(t, err.Error(), constants.ReasonStorageClassChangeNotSupported)
}

func TestStorageReconciler_IgnoresACMECachePVCForStorageClassValidation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	className := "gp3"
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Storage: openbaov1alpha1.StorageConfig{
				Size:             "10Gi",
				StorageClassName: &className,
			},
		},
	}

	dataPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-0",
			Namespace: "default",
			Labels: map[string]string{
				labelOpenBaoCluster: "test",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &className,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}

	acmeClass := "efs-acme"
	acmeCachePVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-acme-cache",
			Namespace: "default",
			Labels: map[string]string{
				labelOpenBaoCluster: "test",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &acmeClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dataPVC, acmeCachePVC).Build()
	r := NewStorageReconciler(
		StorageDependencies{
			Resources: StorageResourceRuntime{Client: c},
			Events:    StorageEventRuntime{Recorder: events.NewFakeRecorder(10)},
		},
	)

	_, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	require.NoError(t, err)
}

func TestStorageReconciler_ExpandsReadReplicaPVCsUsingReadReplicaStorageSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	readClass := "read-gp3"
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
			ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{
				Replicas: 2,
				Storage: &openbaov1alpha1.ReadReplicaStorageConfig{
					Size:             quantityPtr(resource.MustParse("20Gi")),
					StorageClassName: &readClass,
				},
			},
		},
	}

	voterPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-0",
			Namespace: "default",
			Labels: map[string]string{
				labelOpenBaoCluster: "test",
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
	readPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-" + resourceidentity.ReadReplicaStatefulSetName(cluster) + "-0",
			Namespace: "default",
			Labels: map[string]string{
				labelOpenBaoCluster: "test",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &readClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(voterPVC, readPVC).Build()
	r := NewStorageReconciler(
		StorageDependencies{
			Resources: StorageResourceRuntime{Client: c},
			Events:    StorageEventRuntime{Recorder: events.NewFakeRecorder(10)},
		},
	)

	_, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	require.NoError(t, err)

	gotRead := &corev1.PersistentVolumeClaim{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(readPVC), gotRead))
	require.Equal(t, resource.MustParse("20Gi"), gotRead.Spec.Resources.Requests[corev1.ResourceStorage])

	gotVoter := &corev1.PersistentVolumeClaim{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(voterPVC), gotVoter))
	require.Equal(t, resource.MustParse("10Gi"), gotVoter.Spec.Resources.Requests[corev1.ResourceStorage])
}

func TestStorageReconciler_RejectsReadReplicaStorageSmallerThanVoterStorage(t *testing.T) {
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
			ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{
				Replicas: 1,
				Storage: &openbaov1alpha1.ReadReplicaStorageConfig{
					Size: quantityPtr(resource.MustParse("10Gi")),
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := NewStorageReconciler(
		StorageDependencies{
			Resources: StorageResourceRuntime{Client: c},
			Events:    StorageEventRuntime{Recorder: events.NewFakeRecorder(10)},
		},
	)

	_, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	require.Error(t, err)
	require.Contains(t, err.Error(), "spec.readReplicas.storage.size cannot be smaller than spec.storage.size")
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
				labelOpenBaoCluster: "test",
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

	r := NewStorageResizeRestartReconciler(
		StorageResizeRestartDependencies{
			Resources: StorageResourceRuntime{Client: c, APIReader: c},
			Events:    StorageEventRuntime{Recorder: events.NewFakeRecorder(10)},
			Pods: StoragePodRuntime{
				ClientForPodFunc: func(_ context.Context, _ *openbaov1alpha1.OpenBaoCluster, _ string) (StoragePodClient, error) {
					return &openbao.MockClusterActions{
						IsLeaderFunc: func(ctx context.Context) (bool, error) { return false, nil },
					}, nil
				},
			},
		},
	)

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
				labelOpenBaoCluster: "test",
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
				portopenbao.LabelActive: "true",
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
	r := NewStorageResizeRestartReconciler(
		StorageResizeRestartDependencies{
			Resources: StorageResourceRuntime{Client: c, APIReader: c},
			Events:    StorageEventRuntime{Recorder: events.NewFakeRecorder(10)},
			Pods: StoragePodRuntime{
				ClientForPodFunc: func(_ context.Context, _ *openbaov1alpha1.OpenBaoCluster, _ string) (StoragePodClient, error) {
					return &openbao.MockClusterActions{
						IsLeaderFunc: func(ctx context.Context) (bool, error) { return true, nil },
						StepDownLeaderFunc: func(ctx context.Context) error {
							stepDownCalled++
							return nil
						},
					}, nil
				},
			},
		},
	)

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
				labelOpenBaoCluster: "test",
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Conditions: []corev1.PersistentVolumeClaimCondition{
				{Type: corev1.PersistentVolumeClaimFileSystemResizePending, Status: corev1.ConditionTrue},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, pvc).Build()

	r := NewStorageResizeRestartReconciler(
		StorageResizeRestartDependencies{
			Resources: StorageResourceRuntime{Client: c, APIReader: c},
			Events:    StorageEventRuntime{Recorder: events.NewFakeRecorder(10)},
		},
	)

	_, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	require.Error(t, err)
	require.Contains(t, err.Error(), constants.ReasonStorageRestartRequired)
}

func TestStorageResizeRestartReconciler_PrefersReadReplicaPods(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Maintenance: &openbaov1alpha1.MaintenanceConfig{Enabled: true},
			ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{
				Replicas: 1,
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized: true,
		},
	}

	voterPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-0",
			Namespace: "default",
			Labels: map[string]string{
				labelOpenBaoCluster: "test",
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Conditions: []corev1.PersistentVolumeClaimCondition{
				{Type: corev1.PersistentVolumeClaimFileSystemResizePending, Status: corev1.ConditionTrue},
			},
		},
	}
	readPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-" + resourceidentity.ReadReplicaStatefulSetName(cluster) + "-0",
			Namespace: "default",
			Labels: map[string]string{
				labelOpenBaoCluster: "test",
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Conditions: []corev1.PersistentVolumeClaimCondition{
				{Type: corev1.PersistentVolumeClaimFileSystemResizePending, Status: corev1.ConditionTrue},
			},
		},
	}
	voterPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-0", Namespace: "default"},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
	readPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: resourceidentity.ReadReplicaStatefulSetName(cluster) + "-0", Namespace: "default"},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, voterPVC, readPVC, voterPod, readPod).Build()
	var selectedPod string
	r := NewStorageResizeRestartReconciler(
		StorageResizeRestartDependencies{
			Resources: StorageResourceRuntime{Client: c, APIReader: c},
			Events:    StorageEventRuntime{Recorder: events.NewFakeRecorder(10)},
			Pods: StoragePodRuntime{
				ClientForPodFunc: func(_ context.Context, _ *openbaov1alpha1.OpenBaoCluster, podName string) (StoragePodClient, error) {
					selectedPod = podName
					return &openbao.MockClusterActions{
						IsLeaderFunc: func(ctx context.Context) (bool, error) { return false, nil },
					}, nil
				},
			},
		},
	)

	res, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	require.NoError(t, err)
	require.NotZero(t, res.RequeueAfter)
	require.Equal(t, resourceidentity.ReadReplicaStatefulSetName(cluster)+"-0", selectedPod)
}

func quantityPtr(q resource.Quantity) *resource.Quantity {
	return &q
}
