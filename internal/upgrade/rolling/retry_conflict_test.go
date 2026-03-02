package rolling

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

func TestPrepareFailedUpgradeRetry_HandlesStatusConflictAfterAnnotationPatch(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add openbao scheme: %v", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batchv1 scheme: %v", err)
	}

	now := metav1.Now()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
			Annotations: map[string]string{
				constants.AnnotationRetryRollingUpgrade: "retry-now",
			},
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.5.0",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				TargetVersion:    "2.5.0",
				FromVersion:      "2.4.0",
				CurrentPartition: 2,
				CompletedPods:    []int32{2},
				LastErrorReason:  upgrade.ReasonUpgradeFailed,
				LastErrorMessage: "step-down timed out",
				LastErrorAt:      &now,
				LastStepDownTime: &now,
			},
		},
	}

	jobName := upgrade.ExecutorJobName(cluster.Name, upgrade.ExecutorActionRollingStepDownLeader, "test-cluster-1", "", "")
	staleJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster, staleJob).
		Build()

	mgr := &Manager{client: k8sClient, scheme: scheme}

	resumed, err := mgr.prepareFailedUpgradeRetry(context.Background(), testLogger(), cluster)
	if err != nil {
		t.Fatalf("prepareFailedUpgradeRetry() unexpected error: %v", err)
	}
	if !resumed {
		t.Fatalf("prepareFailedUpgradeRetry() resumed=false, want true")
	}

	if cluster.Status.Upgrade == nil {
		t.Fatalf("Upgrade=nil, want non-nil")
	}
	if cluster.Status.Upgrade.LastErrorReason != "" {
		t.Fatalf("LastErrorReason=%q, want empty", cluster.Status.Upgrade.LastErrorReason)
	}
	if cluster.Status.Upgrade.LastErrorMessage != "" {
		t.Fatalf("LastErrorMessage=%q, want empty", cluster.Status.Upgrade.LastErrorMessage)
	}
	if cluster.Status.Upgrade.LastErrorAt != nil {
		t.Fatalf("LastErrorAt=%v, want nil", cluster.Status.Upgrade.LastErrorAt)
	}
	if cluster.Status.Upgrade.LastStepDownTime != nil {
		t.Fatalf("LastStepDownTime=%v, want nil", cluster.Status.Upgrade.LastStepDownTime)
	}

	if cluster.Annotations != nil {
		if _, exists := cluster.Annotations[constants.AnnotationRetryRollingUpgrade]; exists {
			t.Fatalf("retry annotation still present on in-memory cluster")
		}
	}

	storedCluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, storedCluster); err != nil {
		t.Fatalf("Get(cluster) error: %v", err)
	}
	if storedCluster.Annotations != nil {
		if _, exists := storedCluster.Annotations[constants.AnnotationRetryRollingUpgrade]; exists {
			t.Fatalf("retry annotation still present on stored cluster")
		}
	}
	if storedCluster.Status.Upgrade == nil {
		t.Fatalf("stored Upgrade=nil, want non-nil")
	}
	if storedCluster.Status.Upgrade.LastErrorReason != "" {
		t.Fatalf("stored LastErrorReason=%q, want empty", storedCluster.Status.Upgrade.LastErrorReason)
	}

	deletedJob := &batchv1.Job{}
	err = k8sClient.Get(context.Background(), types.NamespacedName{Name: jobName, Namespace: cluster.Namespace}, deletedJob)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected stale step-down job to be deleted, got err=%v", err)
	}
}
