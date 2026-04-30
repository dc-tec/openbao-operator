package core_test

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

func TestSetUpgradeStarted(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		replicas int32
	}{
		{
			name:     "basic upgrade start",
			from:     "2.4.0",
			to:       "2.5.0",
			replicas: 3,
		},
		{
			name:     "single replica upgrade",
			from:     "2.4.0",
			to:       "2.4.1",
			replicas: 1,
		},
		{
			name:     "large cluster upgrade",
			from:     "2.3.0",
			to:       "3.0.0",
			replicas: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &openbaov1alpha1.OpenBaoClusterStatus{}

			core.SetUpgradeStarted(status, tt.from, tt.to, tt.replicas)

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
			if status.Upgrade.Failure != nil {
				t.Errorf("Failure = %#v, want nil until the first error is recorded", status.Upgrade.Failure)
			}
		})
	}
}

func TestSetUpgradeProgress(t *testing.T) {
	tests := []struct {
		name         string
		partition    int32
		completedPod int32
		initialPods  []int32
		wantPodCount int
	}{
		{
			name:         "first pod completed",
			partition:    2,
			completedPod: 2,
			initialPods:  []int32{},
			wantPodCount: 1,
		},
		{
			name:         "second pod completed",
			partition:    1,
			completedPod: 1,
			initialPods:  []int32{2},
			wantPodCount: 2,
		},
		{
			name:         "last pod completed",
			partition:    0,
			completedPod: 0,
			initialPods:  []int32{2, 1},
			wantPodCount: 3,
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

			core.SetUpgradeProgress(status, tt.partition, tt.completedPod)

			if status.Upgrade.CurrentPartition != tt.partition {
				t.Errorf("CurrentPartition = %d, want %d", status.Upgrade.CurrentPartition, tt.partition)
			}
			if len(status.Upgrade.CompletedPods) != tt.wantPodCount {
				t.Errorf("CompletedPods count = %d, want %d", len(status.Upgrade.CompletedPods), tt.wantPodCount)
			}
		})
	}
}

func TestSetUpgradeProgressNilUpgrade(t *testing.T) {
	status := &openbaov1alpha1.OpenBaoClusterStatus{}

	core.SetUpgradeProgress(status, 1, 0)

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

		core.SetStepDownPerformed(status)

		if status.Upgrade.LastStepDownTime == nil {
			t.Error("expected LastStepDownTime to be set")
		}
	})

	t.Run("nil upgrade does not panic", func(t *testing.T) {
		status := &openbaov1alpha1.OpenBaoClusterStatus{}
		core.SetStepDownPerformed(status)
	})
}

func TestSetUpgradeComplete(t *testing.T) {
	tests := []struct {
		name    string
		version string
		fromVer string
	}{
		{
			name:    "basic completion",
			version: "2.5.0",
			fromVer: "2.4.0",
		},
		{
			name:    "major version upgrade",
			version: "3.0.0",
			fromVer: "2.9.0",
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

			core.SetUpgradeComplete(status, tt.version)

			if status.Upgrade != nil {
				t.Error("expected Upgrade to be nil")
			}
			if status.CurrentVersion != tt.version {
				t.Errorf("CurrentVersion = %q, want %q", status.CurrentVersion, tt.version)
			}
		})
	}
}

func TestSetUpgradeFailed(t *testing.T) {
	tests := []struct {
		name    string
		reason  string
		message string
	}{
		{
			name:    "step down timeout",
			reason:  upgrade.ReasonStepDownTimeout,
			message: "Leader step-down timed out",
		},
		{
			name:    "pod not ready",
			reason:  upgrade.ReasonPodNotReady,
			message: "Pod cluster-1 did not become ready",
		},
		{
			name:    "health check failed",
			reason:  upgrade.ReasonHealthCheckFailed,
			message: "Health check failed for pod cluster-0",
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

			core.SetUpgradeFailed(status, tt.reason, tt.message)

			if status.Upgrade == nil {
				t.Error("expected Upgrade to be preserved")
			}
			if status.Upgrade.LastErrorReason != tt.reason {
				t.Errorf("LastErrorReason = %q, want %q", status.Upgrade.LastErrorReason, tt.reason)
			}
			if status.Upgrade.LastErrorMessage != tt.message {
				t.Errorf("LastErrorMessage = %q, want %q", status.Upgrade.LastErrorMessage, tt.message)
			}
			if status.Upgrade.LastErrorAt == nil {
				t.Error("expected LastErrorAt to be set")
			}
		})
	}
}

func TestClearUpgrade(t *testing.T) {
	status := &openbaov1alpha1.OpenBaoClusterStatus{
		Phase: openbaov1alpha1.ClusterPhaseUpgrading,
		Upgrade: &openbaov1alpha1.UpgradeProgress{
			TargetVersion:    "2.5.0",
			FromVersion:      "2.4.0",
			CurrentPartition: 2,
		},
	}

	core.ClearUpgrade(status)

	if status.Upgrade != nil {
		t.Error("expected Upgrade to be nil")
	}
}
