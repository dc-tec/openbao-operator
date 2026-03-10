package rolling

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func TestRollingRetryToken_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		want    bool
	}{
		{
			name:    "nil cluster",
			cluster: nil,
			want:    false,
		},
		{
			name: "missing annotations",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{},
			},
			want: false,
		},
		{
			name: "empty annotation value",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.AnnotationRetryRollingUpgrade: "",
					},
				},
			},
			want: false,
		},
		{
			name: "whitespace annotation value",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.AnnotationRetryRollingUpgrade: "   ",
					},
				},
			},
			want: false,
		},
		{
			name: "non-empty annotation value",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.AnnotationRetryRollingUpgrade: "retry-1",
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := rollingRetryToken(tt.cluster); got != tt.want {
				t.Fatalf("rollingRetryToken()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestClearUpgradeFailureForRetry_TableDriven(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Now())
	startedAt := metav1.NewTime(time.Now().Add(-10 * time.Minute))

	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		assert  func(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster)
	}{
		{
			name:    "nil cluster is no-op",
			cluster: nil,
			assert: func(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster) {
				t.Helper()
				if cluster != nil {
					t.Fatalf("cluster=%v, want nil", cluster)
				}
			},
		},
		{
			name: "nil upgrade status is no-op",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Status: openbaov1alpha1.OpenBaoClusterStatus{Upgrade: nil},
			},
			assert: func(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster) {
				t.Helper()
				if cluster.Status.Upgrade != nil {
					t.Fatalf("Upgrade=%v, want nil", cluster.Status.Upgrade)
				}
			},
		},
		{
			name: "clears failure fields but keeps structural progress",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						TargetVersion:    "2.5.0",
						FromVersion:      "2.4.0",
						CurrentPartition: 2,
						CompletedPods:    []int32{2},
						StartedAt:        &startedAt,
						LastErrorReason:  upgrade.ReasonStepDownTimeout,
						LastErrorMessage: "leader step down timed out",
						LastErrorAt:      &now,
						LastStepDownTime: &now,
					},
				},
			},
			assert: func(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster) {
				t.Helper()
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
				if cluster.Status.Upgrade.StartedAt == nil {
					t.Fatalf("StartedAt=nil, want refreshed timestamp")
				}
				if !cluster.Status.Upgrade.StartedAt.After(startedAt.Time) {
					t.Fatalf("StartedAt=%v, want time after %v", cluster.Status.Upgrade.StartedAt, startedAt)
				}
				if cluster.Status.Upgrade.TargetVersion != "2.5.0" {
					t.Fatalf("TargetVersion=%q, want %q", cluster.Status.Upgrade.TargetVersion, "2.5.0")
				}
				if cluster.Status.Upgrade.CurrentPartition != 2 {
					t.Fatalf("CurrentPartition=%d, want 2", cluster.Status.Upgrade.CurrentPartition)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clearUpgradeFailureForRetry(tt.cluster)
			tt.assert(t, tt.cluster)
		})
	}
}

func TestPrepareFailedUpgradeRetry_GuardConditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
	}{
		{
			name:    "nil cluster",
			cluster: nil,
		},
		{
			name: "nil upgrade status",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{Version: "2.5.0"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Upgrade: nil,
				},
			},
		},
		{
			name: "empty failure reason",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{Version: "2.5.0"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						TargetVersion:   "2.5.0",
						LastErrorReason: "",
					},
				},
			},
		},
		{
			name: "target version mismatch",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{Version: "2.5.1"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						TargetVersion:   "2.5.0",
						LastErrorReason: upgrade.ReasonUpgradeFailed,
					},
				},
			},
		},
		{
			name: "missing retry token",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{Version: "2.5.0"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						TargetVersion:   "2.5.0",
						LastErrorReason: upgrade.ReasonUpgradeFailed,
					},
				},
			},
		},
	}

	mgr := &Manager{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			before := (*openbaov1alpha1.OpenBaoCluster)(nil)
			if tt.cluster != nil {
				before = tt.cluster.DeepCopy()
			}

			resumed, err := mgr.prepareFailedUpgradeRetry(context.Background(), logr.Discard(), tt.cluster)
			if err != nil {
				t.Fatalf("prepareFailedUpgradeRetry() unexpected error: %v", err)
			}
			if resumed {
				t.Fatalf("prepareFailedUpgradeRetry() resumed=true, want false")
			}

			if tt.cluster != nil && before != nil {
				if tt.cluster.Status.Upgrade == nil && before.Status.Upgrade != nil {
					t.Fatalf("Upgrade unexpectedly changed from non-nil to nil")
				}
				if tt.cluster.Status.Upgrade != nil && before.Status.Upgrade != nil {
					if tt.cluster.Status.Upgrade.LastErrorReason != before.Status.Upgrade.LastErrorReason {
						t.Fatalf("LastErrorReason mutated to %q, want unchanged %q", tt.cluster.Status.Upgrade.LastErrorReason, before.Status.Upgrade.LastErrorReason)
					}
				}
			}
		})
	}
}

func TestPrepareFailedUpgradeRetry_SuccessClearsFailureAndRemovesRetrySignal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		withManagedFields bool
	}{
		{name: "with managed fields", withManagedFields: true},
		{name: "without managed fields", withManagedFields: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
				t.Fatalf("add openbao scheme: %v", err)
			}
			if err := appsv1.AddToScheme(scheme); err != nil {
				t.Fatalf("add appsv1 scheme: %v", err)
			}
			if err := batchv1.AddToScheme(scheme); err != nil {
				t.Fatalf("add batchv1 scheme: %v", err)
			}
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatalf("add corev1 scheme: %v", err)
			}

			now := metav1.Now()
			startedAt := metav1.NewTime(time.Now().Add(-10 * time.Minute))
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
						StartedAt:        &startedAt,
						LastErrorReason:  upgrade.ReasonUpgradeFailed,
						LastErrorMessage: "step-down timed out",
						LastErrorAt:      &now,
						LastStepDownTime: &now,
					},
				},
			}

			targetOrdinal := cluster.Status.Upgrade.CurrentPartition - 1
			targetPod := "test-cluster-1"
			if targetOrdinal != 1 {
				t.Fatalf("targetOrdinal=%d, want 1", targetOrdinal)
			}

			jobName := upgrade.ExecutorJobName(cluster.Name, upgrade.ExecutorActionRollingStepDownLeader, targetPod, "", "")
			staleJob := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: cluster.Namespace,
				},
			}
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
				Status: appsv1.StatefulSetStatus{
					UpdateRevision: "rev-good",
				},
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  constants.ContainerBao,
									Image: "openbao/openbao:2.5.0",
								},
							},
						},
					},
				},
			}
			staleTargetPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetPod,
					Namespace: cluster.Namespace,
					Labels: map[string]string{
						appsv1.StatefulSetRevisionLabel: "rev-bad",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  constants.ContainerBao,
							Image: "openbao/openbao:retry-image-does-not-exist",
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			}

			builder := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
				WithObjects(cluster, staleJob, sts, staleTargetPod)
			if tt.withManagedFields {
				builder = builder.WithReturnManagedFields()
			}
			k8sClient := builder.Build()

			mgr := &Manager{client: k8sClient, scheme: scheme}

			resumed, err := mgr.prepareFailedUpgradeRetry(context.Background(), logr.Discard(), cluster)
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
			if cluster.Status.Upgrade.StartedAt == nil {
				t.Fatalf("StartedAt=nil, want refreshed timestamp")
			}
			if !cluster.Status.Upgrade.StartedAt.After(startedAt.Time) {
				t.Fatalf("StartedAt=%v, want time after %v", cluster.Status.Upgrade.StartedAt, startedAt)
			}
			if cluster.Status.Upgrade.TargetVersion != "2.5.0" {
				t.Fatalf("TargetVersion=%q, want %q", cluster.Status.Upgrade.TargetVersion, "2.5.0")
			}
			if cluster.Status.Upgrade.CurrentPartition != 2 {
				t.Fatalf("CurrentPartition=%d, want 2", cluster.Status.Upgrade.CurrentPartition)
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
			if storedCluster.Status.Upgrade.StartedAt == nil {
				t.Fatalf("stored StartedAt=nil, want refreshed timestamp")
			}
			if !storedCluster.Status.Upgrade.StartedAt.After(startedAt.Time) {
				t.Fatalf("stored StartedAt=%v, want time after %v", storedCluster.Status.Upgrade.StartedAt, startedAt)
			}

			deletedJob := &batchv1.Job{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Name: jobName, Namespace: cluster.Namespace}, deletedJob)
			if !apierrors.IsNotFound(err) {
				t.Fatalf("expected stale step-down job to be deleted, got err=%v", err)
			}

			deletedPod := &corev1.Pod{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Name: targetPod, Namespace: cluster.Namespace}, deletedPod)
			if !apierrors.IsNotFound(err) {
				t.Fatalf("expected stale target pod to be deleted, got err=%v", err)
			}
		})
	}
}
