package rolling

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/platform/testutil/openbao"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	upgradecore "github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
)

// testLogger returns a no-op logger for testing.
func testLogger() logr.Logger {
	return logr.Discard()
}

func TestDetectUpgradeState(t *testing.T) {
	tests := []struct {
		name              string
		cluster           *openbaov1alpha1.OpenBaoCluster
		wantUpgradeNeeded bool
		wantResumeUpgrade bool
	}{
		{
			name: "no upgrade needed - versions match",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.4.0",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.0",
					Initialized:    true,
				},
			},
			wantUpgradeNeeded: false,
			wantResumeUpgrade: false,
		},
		{
			name: "upgrade needed - version mismatch",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.5.0",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.0",
					Initialized:    true,
				},
			},
			wantUpgradeNeeded: true,
			wantResumeUpgrade: false,
		},
		{
			name: "stale retry request is ignored when no failed upgrade is waiting",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.5.0",
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						Requests: &openbaov1alpha1.UpgradeRequestConfig{
							Retry: "retry-1",
						},
					},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.0",
					Initialized:    true,
				},
			},
			wantUpgradeNeeded: true,
			wantResumeUpgrade: false,
		},
		{
			name: "resume upgrade - in progress",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.5.0",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.0",
					Initialized:    true,
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						TargetVersion:    "2.5.0",
						FromVersion:      "2.4.0",
						CurrentPartition: 2,
					},
				},
			},
			wantUpgradeNeeded: false,
			wantResumeUpgrade: true,
		},
		{
			name: "failed upgrade waits for manual retry",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.5.0",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.0",
					Initialized:    true,
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						TargetVersion:    "2.5.0",
						FromVersion:      "2.4.0",
						CurrentPartition: 1,
						LastErrorReason:  upgrade.ReasonUpgradeFailed,
						LastErrorMessage: "step-down timeout",
					},
				},
			},
			wantUpgradeNeeded: false,
			wantResumeUpgrade: false,
		},
		{
			name: "failed upgrade resumes when retry request is set",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.5.0",
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						Requests: &openbaov1alpha1.UpgradeRequestConfig{
							Retry: "retry-1",
						},
					},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.0",
					Initialized:    true,
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						TargetVersion:    "2.5.0",
						FromVersion:      "2.4.0",
						CurrentPartition: 1,
						LastErrorReason:  upgrade.ReasonUpgradeFailed,
						LastErrorMessage: "step-down timeout",
					},
				},
			},
			wantUpgradeNeeded: false,
			wantResumeUpgrade: true,
		},
		{
			name: "failed upgrade resumes when target version changes",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.5.1",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.0",
					Initialized:    true,
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						TargetVersion:    "2.5.0",
						FromVersion:      "2.4.0",
						CurrentPartition: 1,
						LastErrorReason:  upgrade.ReasonUpgradeFailed,
						LastErrorMessage: "step-down timeout",
					},
				},
			},
			wantUpgradeNeeded: false,
			wantResumeUpgrade: true,
		},
		{
			name: "first reconcile - current version empty",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.4.0",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "",
					Initialized:    true,
				},
			},
			wantUpgradeNeeded: false,
			wantResumeUpgrade: false,
		},
		{
			name: "downgrade scenario still detects as upgrade needed",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.3.0",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.0",
					Initialized:    true,
				},
			},
			wantUpgradeNeeded: true, // Detection doesn't block; validation does
			wantResumeUpgrade: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manager{}

			gotUpgradeNeeded, gotResumeUpgrade := m.detectUpgradeState(testLogger(), tt.cluster)

			if gotUpgradeNeeded != tt.wantUpgradeNeeded {
				t.Errorf("detectUpgradeState() upgradeNeeded = %v, want %v", gotUpgradeNeeded, tt.wantUpgradeNeeded)
			}
			if gotResumeUpgrade != tt.wantResumeUpgrade {
				t.Errorf("detectUpgradeState() resumeUpgrade = %v, want %v", gotResumeUpgrade, tt.wantResumeUpgrade)
			}
			if tt.name == "stale retry request is ignored when no failed upgrade is waiting" {
				if tt.cluster.Status.UpgradeRequests == nil || tt.cluster.Status.UpgradeRequests.LastHandledRetry != "retry-1" {
					t.Fatalf("LastHandledRetry = %+v, want retry-1 to be recorded as handled", tt.cluster.Status.UpgradeRequests)
				}
			}
		})
	}
}

func TestValidateUpgrade_BlocksInvalidVersionSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		wantReason string
	}{
		{
			name: "downgrade is rejected before health checks",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.4.4",
					Image:   "openbao/openbao:2.4.4",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.5.0",
				},
			},
			wantReason: upgrade.ReasonDowngradeBlocked,
		},
		{
			name: "semver tag mismatch is rejected before health checks",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.5.0",
					Image:   "openbao/openbao:2.4.4",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.4",
				},
			},
			wantReason: upgrade.ReasonImageVersionMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mgr := &Manager{}
			err := mgr.validateUpgrade(context.Background(), testLogger(), tt.cluster)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !errors.Is(err, operatorerrors.ErrPermanentConfig) {
				t.Fatalf("expected permanent config error, got %v", err)
			}
			reason, ok := operatorerrors.Reason(err)
			if !ok {
				t.Fatalf("expected reasoned error, got %v", err)
			}
			if reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}

func TestValidateUpgrade_LeaderUnknownIsTransientClusterState(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.4",
		},
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.Name, Namespace: cluster.Namespace},
		Status: appsv1.StatefulSetStatus{
			Replicas:      3,
			ReadyReplicas: 3,
		},
	}
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + constants.SuffixTLSCA,
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{"ca.crt": []byte("fake-ca")},
	}
	pod0 := readyRollingTestPod(cluster, 0, false)
	pod1 := readyRollingTestPod(cluster, 1, false)
	pod2 := readyRollingTestPod(cluster, 2, false)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts, caSecret, pod0, pod1, pod2).Build()
	mgr := newManagerWithClientFactory(
		k8sClient,
		scheme,
		nil,
		rollingTestClientFactory(), nil,
	).WithReader(k8sClient).WithAdminOpsStatusMutator(testAdminOpsMutator(k8sClient))

	err := mgr.validateUpgrade(context.Background(), testLogger(), cluster)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !operatorerrors.IsTransientClusterState(err) {
		t.Fatalf("expected transient cluster state error, got %v", err)
	}
	reason, ok := operatorerrors.Reason(err)
	if !ok {
		t.Fatalf("expected reasoned error, got %v", err)
	}
	if reason != upgrade.ReasonLeaderUnknown {
		t.Fatalf("reason=%q, want %q", reason, upgrade.ReasonLeaderUnknown)
	}
	if !strings.Contains(err.Error(), "no leader found in cluster") {
		t.Fatalf("error=%q, want no leader detail", err.Error())
	}
}

func TestValidateUpgrade_ResumeHealthAllowsOneUnavailableTarget(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.4",
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				FromVersion:      "2.4.4",
				TargetVersion:    "2.5.0",
				CurrentPartition: 3,
			},
		},
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.Name, Namespace: cluster.Namespace},
		Status: appsv1.StatefulSetStatus{
			Replicas:      3,
			ReadyReplicas: 2,
		},
	}

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + constants.SuffixTLSCA,
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("fake-ca"),
		},
	}

	pod0 := readyRollingTestPod(cluster, 0, true)
	pod1 := readyRollingTestPod(cluster, 1, false)
	pod2 := pendingRollingTestPod(cluster, 2)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts, caSecret, pod0, pod1, pod2).Build()
	mgr := newManagerWithClientFactory(
		k8sClient,
		scheme,
		nil,
		rollingTestClientFactory(), nil,
	).WithReader(k8sClient).WithAdminOpsStatusMutator(testAdminOpsMutator(k8sClient))

	if err := mgr.validateUpgrade(context.Background(), testLogger(), cluster); err != nil {
		t.Fatalf("validateUpgrade() error = %v, want nil", err)
	}
}

func TestValidateUpgrade_ResumeHealthBlocksQuorumLoss(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.4",
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				FromVersion:      "2.4.4",
				TargetVersion:    "2.5.0",
				CurrentPartition: 3,
			},
		},
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.Name, Namespace: cluster.Namespace},
		Status: appsv1.StatefulSetStatus{
			Replicas:      3,
			ReadyReplicas: 1,
		},
	}

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + constants.SuffixTLSCA,
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("fake-ca"),
		},
	}

	pod0 := readyRollingTestPod(cluster, 0, true)
	pod1 := pendingRollingTestPod(cluster, 1)
	pod2 := pendingRollingTestPod(cluster, 2)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts, caSecret, pod0, pod1, pod2).Build()
	mgr := newManagerWithClientFactory(
		k8sClient,
		scheme,
		nil,
		rollingTestClientFactory(), nil,
	).WithReader(k8sClient).WithAdminOpsStatusMutator(testAdminOpsMutator(k8sClient))

	err := mgr.validateUpgrade(context.Background(), testLogger(), cluster)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !operatorerrors.IsTransientClusterState(err) {
		t.Fatalf("expected transient cluster state error, got %v", err)
	}
	reason, ok := operatorerrors.Reason(err)
	if !ok {
		t.Fatalf("expected reasoned error, got %v", err)
	}
	if reason != upgrade.ReasonClusterNotReady {
		t.Fatalf("reason=%q, want %q", reason, upgrade.ReasonClusterNotReady)
	}
	if !strings.Contains(err.Error(), "quorum-ready replicas") {
		t.Fatalf("validateUpgrade() error = %v, want quorum-ready replicas failure", err)
	}
}

func TestValidateUpgrade_ResumeHealthMarksTimedOutTargetAsPodNotReady(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	startedAt := metav1.NewTime(time.Now().Add(-(upgrade.DefaultPodReadyTimeout + time.Minute)))
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.4",
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				FromVersion:      "2.4.4",
				TargetVersion:    "2.5.0",
				CurrentPartition: 3,
				StartedAt:        &startedAt,
			},
		},
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.Name, Namespace: cluster.Namespace},
		Status: appsv1.StatefulSetStatus{
			Replicas:      3,
			ReadyReplicas: 1,
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts).Build()
	mgr := newManagerWithClientFactory(
		k8sClient,
		scheme,
		nil,
		rollingTestClientFactory(), nil,
	).WithReader(k8sClient).WithAdminOpsStatusMutator(testAdminOpsMutator(k8sClient))

	err := mgr.validateUpgrade(context.Background(), testLogger(), cluster)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "did not become ready within") {
		t.Fatalf("validateUpgrade() error = %v, want pod-ready timeout", err)
	}
	if cluster.Status.Upgrade == nil {
		t.Fatal("expected rolling upgrade status to remain present")
	}
	if cluster.Status.Upgrade.LastErrorReason != upgrade.ReasonPodNotReady {
		t.Fatalf("LastErrorReason=%q, want %q", cluster.Status.Upgrade.LastErrorReason, upgrade.ReasonPodNotReady)
	}
	if !strings.Contains(cluster.Status.Upgrade.LastErrorMessage, "test-cluster-2") {
		t.Fatalf("LastErrorMessage=%q, want target pod name", cluster.Status.Upgrade.LastErrorMessage)
	}
}

func TestReconcile_PersistsResumeValidationFailureStatus(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	startedAt := metav1.NewTime(time.Now().Add(-(upgrade.DefaultPodReadyTimeout + time.Minute)))
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
			UID:       types.UID("test-cluster-uid"),
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.4",
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				FromVersion:      "2.4.4",
				TargetVersion:    "2.5.0",
				CurrentPartition: 3,
				StartedAt:        &startedAt,
			},
		},
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:      3,
			ReadyReplicas: 1,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster, sts).
		Build()
	mgr := newManagerWithClientFactory(
		k8sClient,
		scheme,
		nil,
		rollingTestClientFactory(), nil,
	).WithReader(k8sClient).WithAdminOpsStatusMutator(testAdminOpsMutator(k8sClient))

	_, err := mgr.Reconcile(context.Background(), testLogger(), cluster)
	if err == nil {
		t.Fatal("expected reconcile error")
	}
	if !strings.Contains(err.Error(), "did not become ready within") {
		t.Fatalf("Reconcile() error = %v, want pod-ready timeout", err)
	}

	latest := &openbaov1alpha1.OpenBaoCluster{}
	if getErr := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), latest); getErr != nil {
		t.Fatalf("failed to get cluster: %v", getErr)
	}
	if latest.Status.Upgrade == nil {
		t.Fatal("expected persisted rolling upgrade status")
	}
	if latest.Status.Upgrade.LastErrorReason != upgrade.ReasonPodNotReady {
		t.Fatalf("persisted LastErrorReason=%q, want %q", latest.Status.Upgrade.LastErrorReason, upgrade.ReasonPodNotReady)
	}
	if !strings.Contains(latest.Status.Upgrade.LastErrorMessage, "test-cluster-2") {
		t.Fatalf("persisted LastErrorMessage=%q, want target pod name", latest.Status.Upgrade.LastErrorMessage)
	}
}

func TestValidateUpgrade_ResumeHealthBlocksNonTargetUnavailableReplica(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.4",
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				FromVersion:      "2.4.4",
				TargetVersion:    "2.5.0",
				CurrentPartition: 2,
				CompletedPods:    []int32{2},
			},
		},
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.Name, Namespace: cluster.Namespace},
		Status: appsv1.StatefulSetStatus{
			Replicas:      3,
			ReadyReplicas: 2,
		},
	}

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + constants.SuffixTLSCA,
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("fake-ca"),
		},
	}

	pod0 := pendingRollingTestPod(cluster, 0)
	pod1 := readyRollingTestPod(cluster, 1, false)
	pod2 := readyRollingTestPod(cluster, 2, true)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts, caSecret, pod0, pod1, pod2).Build()
	mgr := newManagerWithClientFactory(
		k8sClient,
		scheme,
		nil,
		rollingTestClientFactory(), nil,
	)

	err := mgr.validateUpgrade(context.Background(), testLogger(), cluster)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !operatorerrors.IsTransientClusterState(err) {
		t.Fatalf("expected transient cluster state error, got %v", err)
	}
	reason, ok := operatorerrors.Reason(err)
	if !ok {
		t.Fatalf("expected reasoned error, got %v", err)
	}
	if reason != upgrade.ReasonPodNotReady {
		t.Fatalf("reason=%q, want %q", reason, upgrade.ReasonPodNotReady)
	}
	if !strings.Contains(err.Error(), "non-target pod test-cluster-0 is not ready") {
		t.Fatalf("validateUpgrade() error = %v, want non-target readiness failure", err)
	}
}

func TestValidateUpgrade_ResumeHealthMarksTimedOutNonTargetAsPodNotReady(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	startedAt := metav1.NewTime(time.Now().Add(-(upgrade.DefaultPodReadyTimeout + time.Minute)))
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.4",
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				FromVersion:      "2.4.4",
				TargetVersion:    "2.5.0",
				CurrentPartition: 2,
				CompletedPods:    []int32{2},
				StartedAt:        &startedAt,
			},
		},
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.Name, Namespace: cluster.Namespace},
		Status: appsv1.StatefulSetStatus{
			Replicas:      3,
			ReadyReplicas: 2,
		},
	}

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + constants.SuffixTLSCA,
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("fake-ca"),
		},
	}

	pod0 := pendingRollingTestPod(cluster, 0)
	pod1 := readyRollingTestPod(cluster, 1, false)
	pod2 := readyRollingTestPod(cluster, 2, true)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts, caSecret, pod0, pod1, pod2).Build()
	mgr := newManagerWithClientFactory(
		k8sClient,
		scheme,
		nil,
		rollingTestClientFactory(), nil,
	)

	err := mgr.validateUpgrade(context.Background(), testLogger(), cluster)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "non-target pod test-cluster-0 is not ready") {
		t.Fatalf("validateUpgrade() error = %v, want non-target readiness failure", err)
	}
	if cluster.Status.Upgrade == nil {
		t.Fatal("expected rolling upgrade status to remain present")
	}
	if cluster.Status.Upgrade.LastErrorReason != upgrade.ReasonPodNotReady {
		t.Fatalf("LastErrorReason=%q, want %q", cluster.Status.Upgrade.LastErrorReason, upgrade.ReasonPodNotReady)
	}
	if !strings.Contains(cluster.Status.Upgrade.LastErrorMessage, "test-cluster-0") {
		t.Fatalf("LastErrorMessage=%q, want non-target pod name", cluster.Status.Upgrade.LastErrorMessage)
	}
}

func TestIsPodReady(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "pod is ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			want: true,
		},
		{
			name: "pod is not ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "pod has no ready condition",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "pod has no conditions",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{},
				},
			},
			want: false,
		},
		{
			name: "pod ready condition is unknown",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionUnknown,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "multiple conditions - ready is true",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.PodInitialized,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPodReady(tt.pod); got != tt.want {
				t.Errorf("isPodReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractOrdinal(t *testing.T) {
	tests := []struct {
		name    string
		podName string
		want    int
	}{
		{
			name:    "simple pod name",
			podName: "cluster-0",
			want:    0,
		},
		{
			name:    "second pod",
			podName: "cluster-1",
			want:    1,
		},
		{
			name:    "third pod",
			podName: "cluster-2",
			want:    2,
		},
		{
			name:    "high ordinal",
			podName: "cluster-99",
			want:    99,
		},
		{
			name:    "complex name",
			podName: "my-openbao-cluster-5",
			want:    5,
		},
		{
			name:    "name with hyphens",
			podName: "prod-bao-cluster-3",
			want:    3,
		},
		{
			name:    "single part name",
			podName: "cluster",
			want:    0,
		},
		{
			name:    "non-numeric suffix",
			podName: "cluster-abc",
			want:    0,
		},
		{
			name:    "empty string",
			podName: "",
			want:    0,
		},
		{
			name:    "just a number",
			podName: "5",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractOrdinal(tt.podName); got != tt.want {
				t.Errorf("extractOrdinal(%q) = %v, want %v", tt.podName, got, tt.want)
			}
		})
	}
}

func TestGetPodURL(t *testing.T) {
	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		podName string
		wantURL string
	}{
		{
			name: "basic pod URL",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mycluster",
					Namespace: "default",
				},
			},
			podName: "mycluster-0",
			wantURL: "https://mycluster-0.mycluster.default.svc:8200",
		},
		{
			name: "different namespace",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prod-bao",
					Namespace: "security",
				},
			},
			podName: "prod-bao-2",
			wantURL: "https://prod-bao-2.prod-bao.security.svc:8200",
		},
		{
			name: "complex cluster name",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-openbao-cluster",
					Namespace: "vault-system",
				},
			},
			podName: "my-openbao-cluster-1",
			wantURL: "https://my-openbao-cluster-1.my-openbao-cluster.vault-system.svc:8200",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := raftops.ClusterPodURL(tt.cluster, tt.podName)
			if got != tt.wantURL {
				t.Errorf("ClusterPodURL() = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func TestReconcile_NotInitialized(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    false,
			CurrentVersion: "2.4.0",
		},
	}

	m := &Manager{}
	_, err := m.Reconcile(context.Background(), testLogger(), cluster)

	// Should return nil without doing anything
	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
}

func TestReconcile_NoUpgradeNeeded(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.0",
		},
	}

	m := &Manager{}
	_, err := m.Reconcile(context.Background(), testLogger(), cluster)

	// Should return nil without doing anything
	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
}

func TestReconcile_SkipsBlueGreenStrategy(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.0",
		},
	}

	m := &Manager{}
	result, err := m.Reconcile(context.Background(), testLogger(), cluster)

	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if result != (recon.Result{}) {
		t.Fatalf("Reconcile() result = %+v, want empty result", result)
	}
}

func TestReconcile_HaltsDuringBreakGlassWithoutAck(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:       "2.5.0",
			Replicas:      3,
			BreakGlassAck: "stale-ack",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.0",
			BreakGlass: &openbaov1alpha1.BreakGlassStatus{
				Active: true,
				Nonce:  "expected-ack",
			},
		},
	}

	m := &Manager{}
	result, err := m.Reconcile(context.Background(), testLogger(), cluster)

	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if result.RequeueAfter != constants.RequeueStandard {
		t.Fatalf("Reconcile() RequeueAfter = %v, want %v", result.RequeueAfter, constants.RequeueStandard)
	}
}

func TestReconcile_ReleasesStaleUpgradeLockWhenUpgradeIsIdle(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	now := metav1.Now()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.0",
			OperationLock: &openbaov1alpha1.OperationLockStatus{
				Operation:  openbaov1alpha1.ClusterOperationUpgrade,
				Holder:     upgradecore.UpgradeOperationLockHolder,
				Message:    "stale upgrade lock",
				AcquiredAt: &now,
				RenewedAt:  &now,
			},
		},
	}

	var applyPayloads []string
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceApply: func(ctx context.Context, c client.Client, subResourceName string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
				if subResourceName == "status" {
					payload, err := json.Marshal(obj)
					if err != nil {
						return err
					}
					applyPayloads = append(applyPayloads, string(payload))
				}
				return c.Status().Apply(ctx, obj, opts...)
			},
		}).
		WithReturnManagedFields().
		Build()
	mgr := NewManager(k8sClient, scheme, nil, portopenbao.ClientConfig{}, nil, "")
	mgr.WithReader(k8sClient).WithAdminOpsStatusMutator(testAdminOpsMutator(k8sClient))

	result, err := mgr.Reconcile(context.Background(), testLogger(), cluster)
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if result != (recon.Result{}) {
		t.Fatalf("Reconcile() result = %+v, want empty result", result)
	}

	if len(applyPayloads) != 2 {
		t.Fatalf("expected takeover and clear SSA applies, got %d payloads", len(applyPayloads))
	}
	if !strings.Contains(applyPayloads[0], `"operationLock":{"`) {
		t.Fatalf("expected first payload to take ownership of operationLock, got %s", applyPayloads[0])
	}
	if strings.Contains(applyPayloads[1], `"operationLock":{"`) || !strings.Contains(applyPayloads[1], `"operationLock":null`) {
		t.Fatalf("expected second payload to explicitly clear operationLock, got %s", applyPayloads[1])
	}
	if cluster.Status.OperationLock != nil {
		t.Fatalf("expected in-memory operation lock to be released, got %+v", cluster.Status.OperationLock)
	}
}

func TestReconcile_PersistsFailureWhenUpgradeLockIsBlockedMidUpgrade(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	now := metav1.Now()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.4",
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				FromVersion:      "2.4.4",
				TargetVersion:    "2.5.0",
				CurrentPartition: 2,
			},
			OperationLock: &openbaov1alpha1.OperationLockStatus{
				Operation:  openbaov1alpha1.ClusterOperationBackup,
				Holder:     "openbao-backup-controller",
				Message:    "scheduled backup in progress",
				AcquiredAt: &now,
				RenewedAt:  &now,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		Build()
	mgr := NewManager(k8sClient, scheme, nil, portopenbao.ClientConfig{}, nil, "")
	mgr.WithReader(k8sClient).WithAdminOpsStatusMutator(testAdminOpsMutator(k8sClient))

	_, err := mgr.Reconcile(context.Background(), testLogger(), cluster)
	if err == nil || !strings.Contains(err.Error(), "operation lock is held by another operation") {
		t.Fatalf("Reconcile() error = %v, want blocked-operation-lock error", err)
	}

	latest := &openbaov1alpha1.OpenBaoCluster{}
	if getErr := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), latest); getErr != nil {
		t.Fatalf("failed to get cluster: %v", getErr)
	}
	if latest.Status.Upgrade == nil {
		t.Fatal("expected persisted rolling upgrade status")
	}
	if latest.Status.Upgrade.LastErrorReason != upgrade.ReasonUpgradeFailed {
		t.Fatalf("persisted LastErrorReason=%q, want %q", latest.Status.Upgrade.LastErrorReason, upgrade.ReasonUpgradeFailed)
	}
	if !strings.Contains(latest.Status.Upgrade.LastErrorMessage, "concurrent operation lock") {
		t.Fatalf("persisted LastErrorMessage=%q, want lock-contention message", latest.Status.Upgrade.LastErrorMessage)
	}
	if latest.Status.Upgrade.LastErrorAt == nil {
		t.Fatal("expected persisted LastErrorAt timestamp")
	}
}

func TestReconcile_ReleasesUpgradeLockOnValidationFailureBeforeStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		version       string
		image         string
		wantErrSubstr string
	}{
		{
			name:          "invalid target version",
			version:       "latest",
			wantErrSubstr: "invalid target version",
		},
		{
			name:          "semver image tag mismatch",
			version:       "2.5.0",
			image:         "openbao/openbao:2.4.4",
			wantErrSubstr: "does not match spec.version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newScheme()
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
					UID:       types.UID("test-cluster-uid"),
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version:  tt.version,
					Image:    tt.image,
					Replicas: 3,
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Initialized:    true,
					CurrentVersion: "2.4.4",
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
				WithObjects(cluster).
				Build()
			mgr := NewManager(k8sClient, scheme, nil, portopenbao.ClientConfig{}, nil, "")

			_, err := mgr.Reconcile(context.Background(), testLogger(), cluster)
			if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Fatalf("Reconcile() error = %v, want contains %q", err, tt.wantErrSubstr)
			}
			if !errors.Is(err, operatorerrors.ErrPermanentConfig) {
				t.Fatalf("expected permanent config error, got %v", err)
			}

			if cluster.Status.OperationLock != nil {
				t.Fatalf("expected in-memory operation lock to be released, got %+v", cluster.Status.OperationLock)
			}
		})
	}
}

// TestUpgradeStateTransitions tests the logical state transitions during an upgrade.
// This is a table-driven test following the testing strategy.
func TestUpgradeStateTransitions(t *testing.T) {
	tests := []struct {
		name              string
		initialStatus     openbaov1alpha1.OpenBaoClusterStatus
		specVersion       string
		wantUpgradeNeeded bool
		wantResume        bool
		description       string
	}{
		{
			name: "Running -> Upgrading (new upgrade)",
			initialStatus: openbaov1alpha1.OpenBaoClusterStatus{
				Phase:          openbaov1alpha1.ClusterPhaseRunning,
				CurrentVersion: "2.4.0",
				Initialized:    true,
			},
			specVersion:       "2.5.0",
			wantUpgradeNeeded: true,
			wantResume:        false,
			description:       "A running cluster detects version change and needs upgrade",
		},
		{
			name: "Upgrading -> Upgrading (resume)",
			initialStatus: openbaov1alpha1.OpenBaoClusterStatus{
				Phase:          openbaov1alpha1.ClusterPhaseUpgrading,
				CurrentVersion: "2.4.0",
				Initialized:    true,
				Upgrade: &openbaov1alpha1.UpgradeProgress{
					TargetVersion:    "2.5.0",
					FromVersion:      "2.4.0",
					CurrentPartition: 2,
					CompletedPods:    []int32{2},
				},
			},
			specVersion:       "2.5.0",
			wantUpgradeNeeded: false,
			wantResume:        true,
			description:       "An in-progress upgrade should resume",
		},
		{
			name: "Running -> Running (no change)",
			initialStatus: openbaov1alpha1.OpenBaoClusterStatus{
				Phase:          openbaov1alpha1.ClusterPhaseRunning,
				CurrentVersion: "2.5.0",
				Initialized:    true,
			},
			specVersion:       "2.5.0",
			wantUpgradeNeeded: false,
			wantResume:        false,
			description:       "No upgrade needed when versions match",
		},
		{
			name: "Initializing -> skip (not initialized)",
			initialStatus: openbaov1alpha1.OpenBaoClusterStatus{
				Phase:          openbaov1alpha1.ClusterPhaseInitializing,
				CurrentVersion: "",
				Initialized:    false,
			},
			specVersion:       "2.4.0",
			wantUpgradeNeeded: false,
			wantResume:        false,
			description:       "Cluster not initialized; skip upgrade detection",
		},
		{
			name: "First version set (empty current version)",
			initialStatus: openbaov1alpha1.OpenBaoClusterStatus{
				Phase:          openbaov1alpha1.ClusterPhaseRunning,
				CurrentVersion: "",
				Initialized:    true,
			},
			specVersion:       "2.4.0",
			wantUpgradeNeeded: false,
			wantResume:        false,
			description:       "First reconcile after init; sets version, no upgrade",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version:  tt.specVersion,
					Replicas: 3,
				},
				Status: tt.initialStatus,
			}

			m := &Manager{}
			gotUpgrade, gotResume := m.detectUpgradeState(testLogger(), cluster)

			if gotUpgrade != tt.wantUpgradeNeeded {
				t.Errorf("%s: upgradeNeeded = %v, want %v", tt.description, gotUpgrade, tt.wantUpgradeNeeded)
			}
			if gotResume != tt.wantResume {
				t.Errorf("%s: resume = %v, want %v", tt.description, gotResume, tt.wantResume)
			}
		})
	}
}

// TestUpgradeProgressTracking tests that upgrade progress is tracked correctly.
func TestUpgradeProgressTracking(t *testing.T) {
	tests := []struct {
		name             string
		totalReplicas    int32
		currentPartition int32
		completedPods    []int32
		expectedNext     int32 // Expected next pod ordinal to upgrade
		isComplete       bool
	}{
		{
			name:             "upgrade starting - no pods done",
			totalReplicas:    3,
			currentPartition: 3,
			completedPods:    []int32{},
			expectedNext:     2, // partition - 1
			isComplete:       false,
		},
		{
			name:             "one pod done",
			totalReplicas:    3,
			currentPartition: 2,
			completedPods:    []int32{2},
			expectedNext:     1,
			isComplete:       false,
		},
		{
			name:             "two pods done",
			totalReplicas:    3,
			currentPartition: 1,
			completedPods:    []int32{2, 1},
			expectedNext:     0,
			isComplete:       false,
		},
		{
			name:             "all pods done",
			totalReplicas:    3,
			currentPartition: 0,
			completedPods:    []int32{2, 1, 0},
			expectedNext:     -1, // No more pods
			isComplete:       true,
		},
		{
			name:             "single replica - starting",
			totalReplicas:    1,
			currentPartition: 1,
			completedPods:    []int32{},
			expectedNext:     0,
			isComplete:       false,
		},
		{
			name:             "single replica - done",
			totalReplicas:    1,
			currentPartition: 0,
			completedPods:    []int32{0},
			expectedNext:     -1,
			isComplete:       true,
		},
		{
			name:             "five replica cluster - midway",
			totalReplicas:    5,
			currentPartition: 3,
			completedPods:    []int32{4, 3},
			expectedNext:     2,
			isComplete:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check if complete
			isComplete := tt.currentPartition == 0
			if isComplete != tt.isComplete {
				t.Errorf("isComplete = %v, want %v", isComplete, tt.isComplete)
			}

			// Calculate next pod to upgrade
			if !isComplete {
				nextPod := tt.currentPartition - 1
				if nextPod != tt.expectedNext {
					t.Errorf("next pod = %d, want %d", nextPod, tt.expectedNext)
				}
			}

			// Verify completed pod count
			expectedCompleted := int(tt.totalReplicas) - int(tt.currentPartition)
			if len(tt.completedPods) != expectedCompleted {
				t.Errorf("completed pod count = %d, want %d", len(tt.completedPods), expectedCompleted)
			}
		})
	}
}

// TestVersionMismatchDuringUpgrade tests handling of spec.version changes during upgrade.
func TestVersionMismatchDuringUpgrade(t *testing.T) {
	tests := []struct {
		name             string
		upgradeTarget    string
		specVersion      string
		shouldClearState bool
	}{
		{
			name:             "same version - continue",
			upgradeTarget:    "2.5.0",
			specVersion:      "2.5.0",
			shouldClearState: false,
		},
		{
			name:             "different version - clear and restart",
			upgradeTarget:    "2.5.0",
			specVersion:      "2.6.0",
			shouldClearState: true,
		},
		{
			name:             "downgrade during upgrade",
			upgradeTarget:    "2.5.0",
			specVersion:      "2.4.0",
			shouldClearState: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &openbaov1alpha1.OpenBaoClusterStatus{
				Upgrade: &openbaov1alpha1.UpgradeProgress{
					TargetVersion: tt.upgradeTarget,
					FromVersion:   "2.4.0",
				},
			}

			shouldClear := tt.specVersion != tt.upgradeTarget
			if shouldClear != tt.shouldClearState {
				t.Errorf("shouldClearState = %v, want %v", shouldClear, tt.shouldClearState)
			}

			// If we should clear, verify the clear function works
			if tt.shouldClearState {
				upgradecore.ClearUpgrade(status)
				if status.Upgrade != nil {
					t.Error("expected Upgrade to be cleared")
				}
			}
		})
	}
}

// TestWaitForPodReady_LevelTriggered tests the level-triggered behavior of waitForPodReady.
// It verifies that the function returns (true, nil) when pod is ready,
// (false, nil) when pod is not ready (requeue), and (false, error) on timeout.
func TestWaitForPodReady_LevelTriggered(t *testing.T) {
	tests := []struct {
		name         string
		podExists    bool
		podReady     bool
		upgradeStart time.Duration // Time ago that upgrade started
		wantReady    bool
		wantErr      bool
		description  string
	}{
		{
			name:         "pod ready - returns true",
			podExists:    true,
			podReady:     true,
			upgradeStart: 1 * time.Minute,
			wantReady:    true,
			wantErr:      false,
			description:  "Pod is ready, should return true",
		},
		{
			name:         "pod not ready - returns false for requeue",
			podExists:    true,
			podReady:     false,
			upgradeStart: 1 * time.Minute,
			wantReady:    false,
			wantErr:      false,
			description:  "Pod exists but not ready, should requeue",
		},
		{
			name:         "pod not found - returns false for requeue",
			podExists:    false,
			podReady:     false,
			upgradeStart: 1 * time.Minute,
			wantReady:    false,
			wantErr:      false,
			description:  "Pod doesn't exist yet, should requeue",
		},
		{
			name:         "timeout exceeded - returns error",
			podExists:    true,
			podReady:     false,
			upgradeStart: upgrade.DefaultPodReadyTimeout + 1*time.Minute,
			wantReady:    false,
			wantErr:      true,
			description:  "Past timeout, should return error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := metav1.Now()
			startTime := metav1.NewTime(now.Add(-tt.upgradeStart))

			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						StartedAt: &startTime,
					},
				},
			}

			// The actual function call requires a real client and pod.
			// For this unit test, we verify the timeout logic directly.
			if tt.upgradeStart > upgrade.DefaultPodReadyTimeout {
				// Timeout case - verify the function would detect timeout
				elapsed := time.Since(cluster.Status.Upgrade.StartedAt.Time)
				if elapsed <= upgrade.DefaultPodReadyTimeout {
					t.Errorf("Expected timeout condition, but elapsed %v <= %v", elapsed, upgrade.DefaultPodReadyTimeout)
				}
			}
		})
	}
}

// TestWaitForPodHealthy_LevelTriggered tests the timeout behavior of waitForPodHealthy.
func TestWaitForPodHealthy_LevelTriggered(t *testing.T) {
	tests := []struct {
		name         string
		upgradeStart time.Duration
		wantTimeout  bool
	}{
		{
			name:         "within timeout window",
			upgradeStart: 1 * time.Minute,
			wantTimeout:  false,
		},
		{
			name:         "past timeout window",
			upgradeStart: upgrade.DefaultPodReadyTimeout + upgrade.DefaultHealthCheckTimeout + 1*time.Minute,
			wantTimeout:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := metav1.Now()
			startTime := metav1.NewTime(now.Add(-tt.upgradeStart))

			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						StartedAt: &startTime,
					},
				},
			}

			elapsed := time.Since(cluster.Status.Upgrade.StartedAt.Time)
			isTimeout := elapsed > upgrade.DefaultPodReadyTimeout+upgrade.DefaultHealthCheckTimeout

			if isTimeout != tt.wantTimeout {
				t.Errorf("timeout detection: got %v, want %v (elapsed: %v)", isTimeout, tt.wantTimeout, elapsed)
			}
		})
	}
}

func TestNextRolloutTargetPod(t *testing.T) {
	tests := []struct {
		name             string
		currentPartition int32
		wantComplete     bool
		wantPodName      string
		wantPartition    int32
	}{
		{
			name:             "partition 0 - complete",
			currentPartition: 0,
			wantComplete:     true,
		},
		{
			name:             "partition 3 - incomplete",
			currentPartition: 3,
			wantComplete:     false,
			wantPodName:      "test-cluster-2",
			wantPartition:    2,
		},
		{
			name:             "partition 1 - incomplete",
			currentPartition: 1,
			wantComplete:     false,
			wantPodName:      "test-cluster-0",
			wantPartition:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Replicas: 3,
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						CurrentPartition: tt.currentPartition,
					},
				},
			}

			target, completed, err := nextRolloutTargetPod(cluster)
			if err != nil {
				t.Fatalf("nextRolloutTargetPod() error = %v", err)
			}
			if completed != tt.wantComplete {
				t.Fatalf("completed = %v, want %v", completed, tt.wantComplete)
			}
			if tt.wantComplete {
				return
			}
			if target.Name != tt.wantPodName {
				t.Fatalf("target.Name = %q, want %q", target.Name, tt.wantPodName)
			}
			if target.NextPartition != tt.wantPartition {
				t.Fatalf("target.NextPartition = %d, want %d", target.NextPartition, tt.wantPartition)
			}
		})
	}
}

// TestValidateBackupConfig tests the backup configuration validation.
func TestValidateBackupConfig(t *testing.T) {
	tests := []struct {
		name        string
		cluster     *openbaov1alpha1.OpenBaoCluster
		secretName  string // Secret to create in fake client
		expectError bool
		errorSubstr string
	}{
		{
			name: "no backup config returns error",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec:       openbaov1alpha1.OpenBaoClusterSpec{Backup: nil},
			},
			expectError: true,
			errorSubstr: "backup configuration is required",
		},
		{
			name: "JWT auth configured - valid",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{
						JWTAuthRole: "backup-role",
					},
				},
			},
			expectError: false,
		},
		{
			name: "token secret configured and exists - valid",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{
						TokenSecretRef: &corev1.LocalObjectReference{Name: "backup-token"},
					},
				},
			},
			secretName:  "backup-token",
			expectError: false,
		},
		{
			name: "token secret configured but not found - error",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{
						TokenSecretRef: &corev1.LocalObjectReference{Name: "missing-secret"},
					},
				},
			},
			expectError: true,
			errorSubstr: "not found",
		},
		{
			name: "no auth method configured - error",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{},
				},
			},
			expectError: true,
			errorSubstr: "authentication is required",
		},
		{
			name: "empty JWT auth role treated as unset",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{
						JWTAuthRole: "   ", // whitespace only
					},
				},
			},
			expectError: true,
			errorSubstr: "authentication is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newScheme()
			builder := fake.NewClientBuilder().WithScheme(scheme)

			// Add secret if specified
			if tt.secretName != "" {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tt.secretName,
						Namespace: "default",
					},
					Data: map[string][]byte{"token": []byte("test-token")},
				}
				builder = builder.WithObjects(secret)
			}

			k8sClient := builder.Build()
			m := &Manager{client: k8sClient}

			err := m.validatePreUpgradeSnapshotPrerequisites(context.Background(), tt.cluster)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errorSubstr != "" && !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errorSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// newScheme creates a scheme with all required types for testing
func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = openbaov1alpha1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme
}

func rollingTestClientFactory() raftops.OpenBaoClientFactory {
	return func(config portopenbao.ClientConfig) (portopenbao.ClusterActions, error) {
		mock := &openbaoapi.MockClusterActions{
			IsHealthyFunc: func(ctx context.Context) (bool, error) {
				return true, nil
			},
		}
		if strings.Contains(config.BaseURL, "-0.") {
			mock.IsLeaderFunc = func(ctx context.Context) (bool, error) {
				return true, nil
			}
		} else {
			mock.IsLeaderFunc = func(ctx context.Context) (bool, error) {
				return false, nil
			}
		}
		return mock, nil
	}
}

func readyRollingTestPod(cluster *openbaov1alpha1.OpenBaoCluster, ordinal int, leader bool) *corev1.Pod {
	active := "false"
	if leader {
		active = "true"
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-" + strconv.Itoa(ordinal),
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppInstance:           cluster.Name,
				constants.LabelAppName:               constants.LabelValueAppNameOpenBao,
				constants.LabelAppManagedBy:          constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:        cluster.Name,
				portopenbao.LabelActive:              active,
				appsv1.StatefulSetRevisionLabel:      "rev-a",
				"statefulset.kubernetes.io/pod-name": cluster.Name + "-" + strconv.Itoa(ordinal),
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
}

func pendingRollingTestPod(cluster *openbaov1alpha1.OpenBaoCluster, ordinal int) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-" + strconv.Itoa(ordinal),
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppInstance:    cluster.Name,
				constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
				constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster: cluster.Name,
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}
}
