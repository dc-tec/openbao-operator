//go:build integration
// +build integration

package integration

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/adminopsstatus"
	"github.com/dc-tec/openbao-operator/internal/port/adminops"
)

func TestAdminOpsStatusOwnershipPolicy(t *testing.T) {
	for _, tt := range []struct {
		name      string
		ownership adminops.OwnershipPolicy
		conflict  bool
	}{
		{name: "respect", ownership: adminops.RespectOwnership, conflict: true},
		{name: "fallback", ownership: adminops.ForceOwnershipOnConflict},
		{name: "force", ownership: adminops.ForceOwnership},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cluster := createMinimalCluster(t, newTestNamespace(t), "adminops-ownership-"+tt.name)
			cluster.Status.Backup = &openbaov1alpha1.BackupStatus{LastFailureReason: "other-writer"}
			cluster.Status.CurrentVersion = testOpenBaoVersion244
			require.NoError(t, k8sClient.Status().Update(ctx, cluster, client.FieldOwner("other-status-writer")))
			before := cluster.DeepCopy()

			mutateStatus := adminopsstatus.NewMutator(k8sClient, k8sClient)
			err := mutateStatus(ctx, cluster, func(obj *openbaov1alpha1.OpenBaoCluster) error {
				obj.Status.Backup.LastFailureReason = "adminops-writer"
				return nil
			}, tt.ownership)

			stored := &openbaov1alpha1.OpenBaoCluster{}
			require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), stored))
			if tt.conflict {
				require.True(t, apierrors.HasStatusCause(err, metav1.CauseTypeFieldManagerConflict), "expected ownership conflict, got %v", err)
				require.Equal(t, before, cluster, "failed writes leave the caller unchanged")
				require.Equal(t, before.Status, stored.Status)
				require.Equal(t, before.ResourceVersion, stored.ResourceVersion)
			} else {
				require.NoError(t, err)
				require.Equal(t, "adminops-writer", stored.Status.Backup.LastFailureReason)
				require.NotEqual(t, before.ResourceVersion, stored.ResourceVersion)
				require.Equal(t, stored.Status, cluster.Status)
				require.Equal(t, stored.ResourceVersion, cluster.ResourceVersion)
			}
			require.Equal(t, before.Status.CurrentVersion, stored.Status.CurrentVersion)
		})
	}
}

func TestAdminOpsStatusReadBackAfterUpgradeClears(t *testing.T) {
	tests := []struct {
		name  string
		clear func(*openbaov1alpha1.UpgradeProgress) *openbaov1alpha1.UpgradeProgress
	}{
		{name: "upgrade", clear: func(*openbaov1alpha1.UpgradeProgress) *openbaov1alpha1.UpgradeProgress {
			return nil
		}},
		{name: "failure", clear: func(progress *openbaov1alpha1.UpgradeProgress) *openbaov1alpha1.UpgradeProgress {
			progress.Failure = nil
			return progress
		}},
		{name: "failure-time", clear: func(progress *openbaov1alpha1.UpgradeProgress) *openbaov1alpha1.UpgradeProgress {
			progress.Failure.At = nil
			return progress
		}},
		{name: "step-down-time", clear: func(progress *openbaov1alpha1.UpgradeProgress) *openbaov1alpha1.UpgradeProgress {
			progress.LastStepDownTime = nil
			return progress
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := createMinimalCluster(t, newTestNamespace(t), "adminops-clear-"+tt.name)
			updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
				status.CurrentVersion = testOpenBaoVersion244
				status.Initialized = true
				status.OperationLock = &openbaov1alpha1.OperationLockStatus{
					Operation: openbaov1alpha1.ClusterOperationUpgrade,
					Holder:    "test/upgrade",
				}
			})

			// Seed through the shared field manager so it owns the fields to clear.
			now := metav1.NewTime(time.Now().UTC().Truncate(time.Second))
			if err := adminopsstatus.MutateWithReader(ctx, k8sClient, k8sClient, cluster,
				func(obj *openbaov1alpha1.OpenBaoCluster) error {
					obj.Status.Upgrade = &openbaov1alpha1.UpgradeProgress{
						FromVersion:      testOpenBaoVersion244,
						TargetVersion:    "2.5.0",
						CurrentPartition: 2,
						StartedAt:        &now,
						CompletedPods:    []int32{2},
						Failure: &openbaov1alpha1.ControllerErrorStatus{
							Reason: "UpgradeFailed", Message: "step-down timed out", At: &now,
						},
						LastStepDownTime: &now,
					}
					obj.Status.Backup = &openbaov1alpha1.BackupStatus{LastFailureReason: "preserve-backup"}
					return nil
				}, adminops.RespectOwnership); err != nil {
				t.Fatalf("seed adminops status: %v", err)
			}

			beforeVersion := cluster.ResourceVersion
			wantStatus := cluster.Status.DeepCopy()
			wantStatus.Upgrade = tt.clear(wantStatus.Upgrade)
			if err := adminopsstatus.MutateWithReader(ctx, k8sClient, k8sClient, cluster,
				func(obj *openbaov1alpha1.OpenBaoCluster) error {
					obj.Status.Upgrade = tt.clear(obj.Status.Upgrade)
					return nil
				}, adminops.RespectOwnership); err != nil {
				t.Fatalf("clear upgrade status: %v", err)
			}

			stored := &openbaov1alpha1.OpenBaoCluster{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), stored); err != nil {
				t.Fatalf("read persisted status: %v", err)
			}
			if !reflect.DeepEqual(stored.Status, *wantStatus) {
				t.Fatalf("persisted status = %+v, want %+v", stored.Status, *wantStatus)
			}
			if !reflect.DeepEqual(cluster.Status, stored.Status) {
				t.Fatalf("caller status = %+v, want persisted status %+v", cluster.Status, stored.Status)
			}
			if cluster.ResourceVersion != stored.ResourceVersion || cluster.ResourceVersion == beforeVersion {
				t.Fatalf("caller resourceVersion = %q, stored = %q, before clear = %q",
					cluster.ResourceVersion, stored.ResourceVersion, beforeVersion)
			}
		})
	}
}

func TestAdminOpsStatusReadBackPreservesFieldsOwnedByAnotherWriter(t *testing.T) {
	cluster := createMinimalCluster(t, newTestNamespace(t), "adminops-foreign-upgrade")
	cluster.Status.Upgrade = &openbaov1alpha1.UpgradeProgress{
		FromVersion: testOpenBaoVersion244, TargetVersion: "2.5.0", CurrentPartition: 2,
	}
	if err := k8sClient.Status().Update(ctx, cluster, client.FieldOwner("other-status-writer")); err != nil {
		t.Fatalf("seed upgrade status with another owner: %v", err)
	}
	wantUpgrade := cluster.Status.Upgrade.DeepCopy()

	if err := adminopsstatus.MutateWithReader(ctx, k8sClient, k8sClient, cluster,
		func(obj *openbaov1alpha1.OpenBaoCluster) error {
			// Omission does not remove fields owned by another writer.
			obj.Status.Upgrade = nil
			obj.Status.Backup = &openbaov1alpha1.BackupStatus{LastFailureReason: "persist-backup"}
			return nil
		}, adminops.RespectOwnership); err != nil {
		t.Fatalf("apply adminops status: %v", err)
	}

	stored := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), stored); err != nil {
		t.Fatalf("read persisted status: %v", err)
	}
	if !reflect.DeepEqual(stored.Status.Upgrade, wantUpgrade) {
		t.Fatalf("persisted upgrade = %+v, want other writer's status %+v", stored.Status.Upgrade, wantUpgrade)
	}
	if stored.Status.Backup == nil || stored.Status.Backup.LastFailureReason != "persist-backup" {
		t.Fatalf("persisted backup = %+v, want successful apply", stored.Status.Backup)
	}
	if !reflect.DeepEqual(cluster.Status, stored.Status) || cluster.ResourceVersion != stored.ResourceVersion {
		t.Fatalf("caller status = %+v (version %s), want persisted status %+v (version %s)",
			cluster.Status, cluster.ResourceVersion, stored.Status, stored.ResourceVersion)
	}
}
