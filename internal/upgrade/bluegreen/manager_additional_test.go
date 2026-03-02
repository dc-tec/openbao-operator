package bluegreen

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/opslifecycle"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
)

func TestManager_ShouldReconcileBlueGreen(t *testing.T) {
	t.Parallel()

	mgr := &Manager{}
	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		want    bool
	}{
		{
			name:    "nil upgrade config",
			cluster: &openbaov1alpha1.OpenBaoCluster{},
			want:    false,
		},
		{
			name: "rolling strategy",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Upgrade: &openbaov1alpha1.UpgradeConfig{Strategy: openbaov1alpha1.UpdateStrategyRollingUpdate},
				},
			},
			want: false,
		},
		{
			name: "bluegreen strategy",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Upgrade: &openbaov1alpha1.UpgradeConfig{Strategy: openbaov1alpha1.UpdateStrategyBlueGreen},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mgr.shouldReconcileBlueGreen(logr.Discard(), tt.cluster); got != tt.want {
				t.Fatalf("shouldReconcileBlueGreen()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestManager_MaybeAcquireUpgradeLock_Contention(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{Phase: openbaov1alpha1.PhaseDeployingGreen},
			OperationLock: &openbaov1alpha1.OperationLockStatus{
				Operation: openbaov1alpha1.ClusterOperationBackup,
				Holder:    "backup-controller",
				Message:   "backup running",
			},
		},
	}
	mgr := &Manager{}

	t.Run("not active upgrade requeues on contention", func(t *testing.T) {
		handled, result, err := mgr.maybeAcquireUpgradeLock(context.Background(), logr.Discard(), cluster.DeepCopy(), false, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !handled {
			t.Fatalf("expected handled=true")
		}
		if result.RequeueAfter != opslifecycle.RequeueDelay(opslifecycle.RetryClassLockContention) {
			t.Fatalf("requeueAfter=%v, want %v", result.RequeueAfter, opslifecycle.RequeueDelay(opslifecycle.RetryClassLockContention))
		}
	})

	t.Run("active upgrade returns explicit error on contention", func(t *testing.T) {
		handled, result, err := mgr.maybeAcquireUpgradeLock(context.Background(), logr.Discard(), cluster.DeepCopy(), true, true)
		if !handled {
			t.Fatalf("expected handled=true")
		}
		if result != (recon.Result{}) {
			t.Fatalf("result=%v, want zero", result)
		}
		if err == nil || !strings.Contains(err.Error(), "operation lock is held") {
			t.Fatalf("expected contention error, got %v", err)
		}
	})
}

func TestManager_HandleNoUpgradeNeeded(t *testing.T) {
	t.Parallel()

	mgr := &Manager{}

	t.Run("current version not established", func(t *testing.T) {
		cluster := &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{Version: "1.2.3"},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				CurrentVersion: "",
			},
		}
		handled, result, err := mgr.handleNoUpgradeNeeded(context.Background(), logr.Discard(), cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !handled {
			t.Fatalf("expected handled=true")
		}
		if result.RequeueAfter != 1*time.Minute {
			t.Fatalf("requeueAfter=%v, want 1m", result.RequeueAfter)
		}
	})

	t.Run("already on target version", func(t *testing.T) {
		cluster := &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{Version: "1.2.3"},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				CurrentVersion: "1.2.3",
			},
		}
		handled, result, err := mgr.handleNoUpgradeNeeded(context.Background(), logr.Discard(), cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !handled {
			t.Fatalf("expected handled=true")
		}
		if result != (recon.Result{}) {
			t.Fatalf("result=%v, want zero", result)
		}
	})

	t.Run("upgrade still needed", func(t *testing.T) {
		cluster := &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{Version: "1.2.4"},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				CurrentVersion: "1.2.3",
			},
		}
		handled, result, err := mgr.handleNoUpgradeNeeded(context.Background(), logr.Discard(), cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if handled {
			t.Fatalf("expected handled=false")
		}
		if result != (recon.Result{}) {
			t.Fatalf("result=%v, want zero", result)
		}
	})
}
