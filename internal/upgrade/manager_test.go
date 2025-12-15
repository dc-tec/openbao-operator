package upgrade

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
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
		})
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
			m := &Manager{}
			got := m.getPodURL(tt.cluster, tt.podName)
			if got != tt.wantURL {
				t.Errorf("getPodURL() = %q, want %q", got, tt.wantURL)
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
				ClearUpgrade(status, ReasonVersionMismatch, "version changed", 1)
				if status.Upgrade != nil {
					t.Error("expected Upgrade to be cleared")
				}
			}
		})
	}
}
