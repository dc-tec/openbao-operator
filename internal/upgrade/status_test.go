package upgrade

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestSetUpgradeStarted(t *testing.T) {
	tests := []struct {
		name       string
		from       string
		to         string
		replicas   int32
		generation int64
	}{
		{
			name:       "basic upgrade start",
			from:       "2.4.0",
			to:         "2.5.0",
			replicas:   3,
			generation: 1,
		},
		{
			name:       "single replica upgrade",
			from:       "2.4.0",
			to:         "2.4.1",
			replicas:   1,
			generation: 5,
		},
		{
			name:       "large cluster upgrade",
			from:       "2.3.0",
			to:         "3.0.0",
			replicas:   5,
			generation: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &openbaov1alpha1.OpenBaoClusterStatus{}

			SetUpgradeStarted(status, tt.from, tt.to, tt.replicas, tt.generation)

			// Verify upgrade progress is set
			if status.Upgrade == nil {
				t.Fatal("expected Upgrade to be set")
			}
			if status.Upgrade.TargetVersion != tt.to {
				t.Errorf("TargetVersion = %q, want %q", status.Upgrade.TargetVersion, tt.to)
			}
			if status.Upgrade.FromVersion != tt.from {
				t.Errorf("FromVersion = %q, want %q", status.Upgrade.FromVersion, tt.from)
			}
			if status.Upgrade.CurrentPartition != tt.replicas {
				t.Errorf("CurrentPartition = %d, want %d", status.Upgrade.CurrentPartition, tt.replicas)
			}
			if status.Upgrade.StartedAt == nil {
				t.Error("expected StartedAt to be set")
			}
			if len(status.Upgrade.CompletedPods) != 0 {
				t.Errorf("CompletedPods should be empty, got %v", status.Upgrade.CompletedPods)
			}

			// Verify phase is set to Upgrading
			if status.Phase != openbaov1alpha1.ClusterPhaseUpgrading {
				t.Errorf("Phase = %q, want %q", status.Phase, openbaov1alpha1.ClusterPhaseUpgrading)
			}

			// Verify Upgrading condition is True
			upgradingCond := meta.FindStatusCondition(status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
			if upgradingCond == nil {
				t.Fatal("expected Upgrading condition to be set")
			}
			if upgradingCond.Status != metav1.ConditionTrue {
				t.Errorf("Upgrading condition status = %v, want True", upgradingCond.Status)
			}
			if upgradingCond.Reason != ReasonUpgradeStarted {
				t.Errorf("Upgrading condition reason = %q, want %q", upgradingCond.Reason, ReasonUpgradeStarted)
			}

			// Verify Degraded condition is cleared
			degradedCond := meta.FindStatusCondition(status.Conditions, string(openbaov1alpha1.ConditionDegraded))
			if degradedCond == nil {
				t.Fatal("expected Degraded condition to be set")
			}
			if degradedCond.Status != metav1.ConditionFalse {
				t.Errorf("Degraded condition status = %v, want False", degradedCond.Status)
			}
		})
	}
}

func TestSetUpgradeProgress(t *testing.T) {
	tests := []struct {
		name          string
		partition     int32
		completedPod  int32
		totalReplicas int32
		generation    int64
		initialPods   []int32
		wantPodCount  int
	}{
		{
			name:          "first pod completed",
			partition:     2,
			completedPod:  2,
			totalReplicas: 3,
			generation:    1,
			initialPods:   []int32{},
			wantPodCount:  1,
		},
		{
			name:          "second pod completed",
			partition:     1,
			completedPod:  1,
			totalReplicas: 3,
			generation:    1,
			initialPods:   []int32{2},
			wantPodCount:  2,
		},
		{
			name:          "last pod completed",
			partition:     0,
			completedPod:  0,
			totalReplicas: 3,
			generation:    1,
			initialPods:   []int32{2, 1},
			wantPodCount:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &openbaov1alpha1.OpenBaoClusterStatus{
				Upgrade: &openbaov1alpha1.UpgradeProgress{
					TargetVersion:    "2.5.0",
					FromVersion:      "2.4.0",
					CurrentPartition: tt.partition + 1,
					CompletedPods:    tt.initialPods,
				},
			}

			SetUpgradeProgress(status, tt.partition, tt.completedPod, tt.totalReplicas, tt.generation)

			if status.Upgrade.CurrentPartition != tt.partition {
				t.Errorf("CurrentPartition = %d, want %d", status.Upgrade.CurrentPartition, tt.partition)
			}
			if len(status.Upgrade.CompletedPods) != tt.wantPodCount {
				t.Errorf("CompletedPods count = %d, want %d", len(status.Upgrade.CompletedPods), tt.wantPodCount)
			}

			// Verify Upgrading condition is updated
			upgradingCond := meta.FindStatusCondition(status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
			if upgradingCond == nil {
				t.Fatal("expected Upgrading condition to be set")
			}
			if upgradingCond.Reason != ReasonUpgradeInProgress {
				t.Errorf("Upgrading condition reason = %q, want %q", upgradingCond.Reason, ReasonUpgradeInProgress)
			}
		})
	}
}

func TestSetUpgradeProgress_NilUpgrade(t *testing.T) {
	status := &openbaov1alpha1.OpenBaoClusterStatus{
		Upgrade: nil,
	}

	// Should not panic when Upgrade is nil
	SetUpgradeProgress(status, 1, 0, 3, 1)

	if status.Upgrade != nil {
		t.Error("expected Upgrade to remain nil")
	}
}

func TestSetStepDownPerformed(t *testing.T) {
	t.Run("sets step down time", func(t *testing.T) {
		status := &openbaov1alpha1.OpenBaoClusterStatus{
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				TargetVersion: "2.5.0",
				FromVersion:   "2.4.0",
			},
		}

		SetStepDownPerformed(status)

		if status.Upgrade.LastStepDownTime == nil {
			t.Error("expected LastStepDownTime to be set")
		}
	})

	t.Run("nil upgrade does not panic", func(t *testing.T) {
		status := &openbaov1alpha1.OpenBaoClusterStatus{
			Upgrade: nil,
		}

		// Should not panic
		SetStepDownPerformed(status)
	})
}

func TestSetUpgradeComplete(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		generation int64
		fromVer    string
	}{
		{
			name:       "basic completion",
			version:    "2.5.0",
			generation: 1,
			fromVer:    "2.4.0",
		},
		{
			name:       "major version upgrade",
			version:    "3.0.0",
			generation: 5,
			fromVer:    "2.9.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &openbaov1alpha1.OpenBaoClusterStatus{
				Phase: openbaov1alpha1.ClusterPhaseUpgrading,
				Upgrade: &openbaov1alpha1.UpgradeProgress{
					TargetVersion: tt.version,
					FromVersion:   tt.fromVer,
				},
			}

			SetUpgradeComplete(status, tt.version, tt.generation)

			// Verify upgrade is cleared
			if status.Upgrade != nil {
				t.Error("expected Upgrade to be nil")
			}

			// Verify current version is set
			if status.CurrentVersion != tt.version {
				t.Errorf("CurrentVersion = %q, want %q", status.CurrentVersion, tt.version)
			}

			// Verify phase is Running
			if status.Phase != openbaov1alpha1.ClusterPhaseRunning {
				t.Errorf("Phase = %q, want %q", status.Phase, openbaov1alpha1.ClusterPhaseRunning)
			}

			// Verify Upgrading condition is False
			upgradingCond := meta.FindStatusCondition(status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
			if upgradingCond == nil {
				t.Fatal("expected Upgrading condition to be set")
			}
			if upgradingCond.Status != metav1.ConditionFalse {
				t.Errorf("Upgrading condition status = %v, want False", upgradingCond.Status)
			}
			if upgradingCond.Reason != ReasonUpgradeComplete {
				t.Errorf("Upgrading condition reason = %q, want %q", upgradingCond.Reason, ReasonUpgradeComplete)
			}
		})
	}
}

func TestSetUpgradeFailed(t *testing.T) {
	tests := []struct {
		name       string
		reason     string
		message    string
		generation int64
	}{
		{
			name:       "step down timeout",
			reason:     ReasonStepDownTimeout,
			message:    "Leader step-down timed out",
			generation: 1,
		},
		{
			name:       "pod not ready",
			reason:     ReasonPodNotReady,
			message:    "Pod cluster-1 did not become ready",
			generation: 5,
		},
		{
			name:       "health check failed",
			reason:     ReasonHealthCheckFailed,
			message:    "Health check failed for pod cluster-0",
			generation: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &openbaov1alpha1.OpenBaoClusterStatus{
				Phase: openbaov1alpha1.ClusterPhaseUpgrading,
				Upgrade: &openbaov1alpha1.UpgradeProgress{
					TargetVersion:    "2.5.0",
					FromVersion:      "2.4.0",
					CurrentPartition: 2,
					CompletedPods:    []int32{2},
				},
			}

			SetUpgradeFailed(status, tt.reason, tt.message, tt.generation)

			// Verify upgrade is NOT cleared (preserved for inspection)
			if status.Upgrade == nil {
				t.Error("expected Upgrade to be preserved")
			}

			// Verify phase is Failed
			if status.Phase != openbaov1alpha1.ClusterPhaseFailed {
				t.Errorf("Phase = %q, want %q", status.Phase, openbaov1alpha1.ClusterPhaseFailed)
			}

			// Verify Upgrading condition is False
			upgradingCond := meta.FindStatusCondition(status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
			if upgradingCond == nil {
				t.Fatal("expected Upgrading condition to be set")
			}
			if upgradingCond.Status != metav1.ConditionFalse {
				t.Errorf("Upgrading condition status = %v, want False", upgradingCond.Status)
			}
			if upgradingCond.Reason != tt.reason {
				t.Errorf("Upgrading condition reason = %q, want %q", upgradingCond.Reason, tt.reason)
			}

			// Verify Degraded condition is True
			degradedCond := meta.FindStatusCondition(status.Conditions, string(openbaov1alpha1.ConditionDegraded))
			if degradedCond == nil {
				t.Fatal("expected Degraded condition to be set")
			}
			if degradedCond.Status != metav1.ConditionTrue {
				t.Errorf("Degraded condition status = %v, want True", degradedCond.Status)
			}
			if degradedCond.Reason != tt.reason {
				t.Errorf("Degraded condition reason = %q, want %q", degradedCond.Reason, tt.reason)
			}
		})
	}
}

func TestSetUpgradePaused(t *testing.T) {
	status := &openbaov1alpha1.OpenBaoClusterStatus{
		Phase: openbaov1alpha1.ClusterPhaseUpgrading,
		Upgrade: &openbaov1alpha1.UpgradeProgress{
			TargetVersion:    "2.5.0",
			FromVersion:      "2.4.0",
			CurrentPartition: 2,
		},
	}

	SetUpgradePaused(status, 1)

	// Verify Upgrading condition is False with Paused reason
	upgradingCond := meta.FindStatusCondition(status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
	if upgradingCond == nil {
		t.Fatal("expected Upgrading condition to be set")
	}
	if upgradingCond.Status != metav1.ConditionFalse {
		t.Errorf("Upgrading condition status = %v, want False", upgradingCond.Status)
	}
	if upgradingCond.Reason != ReasonUpgradePaused {
		t.Errorf("Upgrading condition reason = %q, want %q", upgradingCond.Reason, ReasonUpgradePaused)
	}

	// Verify upgrade progress is preserved
	if status.Upgrade == nil {
		t.Error("expected Upgrade to be preserved")
	}
}

func TestClearUpgrade(t *testing.T) {
	tests := []struct {
		name       string
		reason     string
		message    string
		generation int64
	}{
		{
			name:       "version mismatch",
			reason:     ReasonVersionMismatch,
			message:    "Target version changed mid-upgrade",
			generation: 1,
		},
		{
			name:       "manual clear",
			reason:     "ManualClear",
			message:    "Upgrade cleared by operator",
			generation: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &openbaov1alpha1.OpenBaoClusterStatus{
				Phase: openbaov1alpha1.ClusterPhaseUpgrading,
				Upgrade: &openbaov1alpha1.UpgradeProgress{
					TargetVersion:    "2.5.0",
					FromVersion:      "2.4.0",
					CurrentPartition: 2,
				},
			}

			ClearUpgrade(status, tt.reason, tt.message, tt.generation)

			// Verify upgrade is cleared
			if status.Upgrade != nil {
				t.Error("expected Upgrade to be nil")
			}

			// Verify phase is Running
			if status.Phase != openbaov1alpha1.ClusterPhaseRunning {
				t.Errorf("Phase = %q, want %q", status.Phase, openbaov1alpha1.ClusterPhaseRunning)
			}

			// Verify Upgrading condition is False
			upgradingCond := meta.FindStatusCondition(status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
			if upgradingCond == nil {
				t.Fatal("expected Upgrading condition to be set")
			}
			if upgradingCond.Status != metav1.ConditionFalse {
				t.Errorf("Upgrading condition status = %v, want False", upgradingCond.Status)
			}
			if upgradingCond.Reason != tt.reason {
				t.Errorf("Upgrading condition reason = %q, want %q", upgradingCond.Reason, tt.reason)
			}
		})
	}
}

func TestSetDowngradeBlocked(t *testing.T) {
	status := &openbaov1alpha1.OpenBaoClusterStatus{}

	SetDowngradeBlocked(status, "2.5.0", "2.4.0", 1)

	degradedCond := meta.FindStatusCondition(status.Conditions, string(openbaov1alpha1.ConditionDegraded))
	if degradedCond == nil {
		t.Fatal("expected Degraded condition to be set")
	}
	if degradedCond.Status != metav1.ConditionTrue {
		t.Errorf("Degraded condition status = %v, want True", degradedCond.Status)
	}
	if degradedCond.Reason != ReasonDowngradeBlocked {
		t.Errorf("Degraded condition reason = %q, want %q", degradedCond.Reason, ReasonDowngradeBlocked)
	}
}

func TestSetInvalidVersion(t *testing.T) {
	status := &openbaov1alpha1.OpenBaoClusterStatus{}

	testErr := errors.New("invalid format")
	SetInvalidVersion(status, "invalid", testErr, 1)

	degradedCond := meta.FindStatusCondition(status.Conditions, string(openbaov1alpha1.ConditionDegraded))
	if degradedCond == nil {
		t.Fatal("expected Degraded condition to be set")
	}
	if degradedCond.Status != metav1.ConditionTrue {
		t.Errorf("Degraded condition status = %v, want True", degradedCond.Status)
	}
	if degradedCond.Reason != ReasonInvalidVersion {
		t.Errorf("Degraded condition reason = %q, want %q", degradedCond.Reason, ReasonInvalidVersion)
	}
}

func TestSetClusterNotReady(t *testing.T) {
	status := &openbaov1alpha1.OpenBaoClusterStatus{}

	SetClusterNotReady(status, "only 1/3 replicas ready", 1)

	degradedCond := meta.FindStatusCondition(status.Conditions, string(openbaov1alpha1.ConditionDegraded))
	if degradedCond == nil {
		t.Fatal("expected Degraded condition to be set")
	}
	if degradedCond.Status != metav1.ConditionTrue {
		t.Errorf("Degraded condition status = %v, want True", degradedCond.Status)
	}
	if degradedCond.Reason != ReasonClusterNotReady {
		t.Errorf("Degraded condition reason = %q, want %q", degradedCond.Reason, ReasonClusterNotReady)
	}
}

func TestIsUpgradeInProgress(t *testing.T) {
	tests := []struct {
		name   string
		status *openbaov1alpha1.OpenBaoClusterStatus
		want   bool
	}{
		{
			name: "upgrade in progress",
			status: &openbaov1alpha1.OpenBaoClusterStatus{
				Upgrade: &openbaov1alpha1.UpgradeProgress{
					TargetVersion: "2.5.0",
				},
			},
			want: true,
		},
		{
			name: "no upgrade",
			status: &openbaov1alpha1.OpenBaoClusterStatus{
				Upgrade: nil,
			},
			want: false,
		},
		{
			name:   "empty status",
			status: &openbaov1alpha1.OpenBaoClusterStatus{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUpgradeInProgress(tt.status); got != tt.want {
				t.Errorf("IsUpgradeInProgress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetUpgradeTargetVersion(t *testing.T) {
	tests := []struct {
		name   string
		status *openbaov1alpha1.OpenBaoClusterStatus
		want   string
	}{
		{
			name: "upgrade in progress",
			status: &openbaov1alpha1.OpenBaoClusterStatus{
				Upgrade: &openbaov1alpha1.UpgradeProgress{
					TargetVersion: "2.5.0",
				},
			},
			want: "2.5.0",
		},
		{
			name: "no upgrade",
			status: &openbaov1alpha1.OpenBaoClusterStatus{
				Upgrade: nil,
			},
			want: "",
		},
		{
			name:   "empty status",
			status: &openbaov1alpha1.OpenBaoClusterStatus{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetUpgradeTargetVersion(tt.status); got != tt.want {
				t.Errorf("GetUpgradeTargetVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
