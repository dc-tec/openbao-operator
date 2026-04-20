package rolling

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/backup"
)

func TestPatchStatusSSA_PreservesSiblingAdminOpsFieldsFromLatestObject(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = openbaov1alpha1.AddToScheme(scheme)

	stored := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster",
			Namespace: "ns1",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				FromVersion:      "2.4.3",
				TargetVersion:    "2.4.4",
				CurrentPartition: 1,
			},
			UpgradeRequests: &openbaov1alpha1.UpgradeRequestStatus{
				LastHandledRetry: "retry-1",
			},
			Backup: &openbaov1alpha1.BackupStatus{
				LastFailureReason: "persist-me",
			},
		},
	}
	cluster := stored.DeepCopy()
	cluster.Status.Backup = nil
	cluster.Status.Upgrade.CurrentPartition = 2

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(stored.DeepCopy()).
		Build()
	mgr := NewManagerWithClientFactory(c, scheme, backup.NewUpgradeStrategyRuntime(c, scheme), nil, portopenbao.ClientConfig{}, nil, "").
		WithReader(c).
		WithAdminOpsStatusMutator(testAdminOpsMutator(c))

	if err := mgr.patchStatusSSA(context.Background(), cluster); err != nil {
		t.Fatalf("patchStatusSSA() error = %v", err)
	}

	updated := &openbaov1alpha1.OpenBaoCluster{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(stored), updated); err != nil {
		t.Fatalf("Get() cluster error = %v", err)
	}
	if updated.Status.Backup == nil || updated.Status.Backup.LastFailureReason != "persist-me" {
		t.Fatalf("persisted backup = %#v, want sibling adminops field preserved", updated.Status.Backup)
	}
	if cluster.Status.Backup == nil || cluster.Status.Backup.LastFailureReason != "persist-me" {
		t.Fatalf("in-memory backup = %#v, want refreshed sibling adminops field", cluster.Status.Backup)
	}
	if updated.Status.Upgrade == nil || updated.Status.Upgrade.CurrentPartition != 2 {
		t.Fatalf("persisted upgrade = %#v, want updated rolling status", updated.Status.Upgrade)
	}
}

func TestWaitForFinalizationConverged_WaitsForStatefulSetConvergence(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster",
			Namespace: "ns1",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
		},
	}

	partition := int32(0)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: &partition,
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas:   3,
			UpdatedReplicas: 2,
			CurrentRevision: "rev-old",
			UpdateRevision:  "rev-new",
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts).Build()
	mgr := NewManagerWithClientFactory(c, scheme, backup.NewUpgradeStrategyRuntime(c, scheme), func(config portopenbao.ClientConfig) (portopenbao.ClusterActions, error) {
		return &openbaoapi.MockClusterActions{}, nil
	}, portopenbao.ClientConfig{}, nil, "")

	converged, err := mgr.waitForFinalizationConverged(context.Background(), testr.New(t), cluster)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if converged {
		t.Fatalf("expected convergence check to wait when StatefulSet has not converged")
	}
}

func TestWaitForFinalizationConverged_SucceedsWhenStatefulSetPodsAndHealthConverged(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster",
			Namespace: "ns1",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
		},
	}

	partition := int32(0)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: &partition,
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas:   3,
			UpdatedReplicas: 3,
			CurrentRevision: "rev-new",
			UpdateRevision:  "rev-new",
		},
	}

	readyCondition := []corev1.PodCondition{
		{
			Type:   corev1.PodReady,
			Status: corev1.ConditionTrue,
		},
	}

	pod0 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster-0",
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppInstance:      cluster.Name,
				constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
				constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
				appsv1.StatefulSetRevisionLabel: "rev-new",
			},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: readyCondition,
		},
	}
	pod1 := pod0.DeepCopy()
	pod1.Name = "upgrade-cluster-1"
	pod2 := pod0.DeepCopy()
	pod2.Name = "upgrade-cluster-2"

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + constants.SuffixTLSCA,
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("fake-ca"),
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts, pod0, pod1, pod2, caSecret).Build()
	mgr := NewManagerWithClientFactory(c, scheme, backup.NewUpgradeStrategyRuntime(c, scheme), func(config portopenbao.ClientConfig) (portopenbao.ClusterActions, error) {
		return &openbaoapi.MockClusterActions{
			IsHealthyFunc: func(ctx context.Context) (bool, error) {
				return true, nil
			},
		}, nil
	}, portopenbao.ClientConfig{}, nil, "")

	converged, err := mgr.waitForFinalizationConverged(context.Background(), testr.New(t), cluster)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !converged {
		t.Fatalf("expected convergence check to succeed when StatefulSet, pods, and health are converged")
	}
}

func TestWaitForFinalizationConverged_RepairsStalePartitionWhenStatusIsAlreadyComplete(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster",
			Namespace: "ns1",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				CurrentPartition: 0,
			},
		},
	}

	partition := int32(3)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: &partition,
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas:   3,
			UpdatedReplicas: 3,
			CurrentRevision: "rev-new",
			UpdateRevision:  "rev-new",
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts).Build()
	mgr := NewManagerWithClientFactory(c, scheme, backup.NewUpgradeStrategyRuntime(c, scheme), func(config portopenbao.ClientConfig) (portopenbao.ClusterActions, error) {
		return &openbaoapi.MockClusterActions{}, nil
	}, portopenbao.ClientConfig{}, nil, "")

	converged, err := mgr.waitForFinalizationConverged(context.Background(), testr.New(t), cluster)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if converged {
		t.Fatalf("expected convergence check to requeue after repairing stale partition")
	}

	updated := &appsv1.StatefulSet{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(sts), updated); err != nil {
		t.Fatalf("failed to reload StatefulSet: %v", err)
	}
	if updated.Spec.UpdateStrategy.RollingUpdate == nil || updated.Spec.UpdateStrategy.RollingUpdate.Partition == nil {
		t.Fatalf("expected rolling update partition to be set")
	}
	if *updated.Spec.UpdateStrategy.RollingUpdate.Partition != 0 {
		t.Fatalf("partition=%d, want 0", *updated.Spec.UpdateStrategy.RollingUpdate.Partition)
	}
}

func TestPatchFinalizedUpgradeStatus_ClearsUpgradeWithoutTouchingCurrentVersion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = openbaov1alpha1.AddToScheme(scheme)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster",
			Namespace: "ns1",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.4.4",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				FromVersion:      "2.4.3",
				TargetVersion:    "2.4.4",
				CurrentPartition: 0,
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		Build()
	mgr := NewManagerWithClientFactory(c, scheme, backup.NewUpgradeStrategyRuntime(c, scheme), nil, portopenbao.ClientConfig{}, nil, "").
		WithReader(c).
		WithAdminOpsStatusMutator(testAdminOpsMutator(c))

	if err := mgr.patchFinalizedUpgradeStatus(context.Background(), cluster); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cluster.Status.Upgrade != nil {
		t.Fatalf("expected upgrade status to be cleared after finalization")
	}
	if cluster.Status.CurrentVersion != "2.4.3" {
		t.Fatalf("expected CurrentVersion to remain unchanged, got %q", cluster.Status.CurrentVersion)
	}
}
