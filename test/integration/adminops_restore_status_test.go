//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/adminopsstatus"
	"github.com/dc-tec/openbao-operator/internal/port/adminops"
)

func TestAdminOpsFinalPatchPreservesConcurrentRestore(t *testing.T) {
	baseWriter, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sScheme})
	require.NoError(t, err)
	completedAt := metav1.NewTime(time.Now().UTC().Truncate(time.Second))
	for _, tt := range []struct {
		name   string
		before *openbaov1alpha1.ClusterRestoreStatus
		after  *openbaov1alpha1.ClusterRestoreStatus
	}{
		{
			name:  "new-request",
			after: &openbaov1alpha1.ClusterRestoreStatus{Name: "restore", UID: "new"},
		},
		{
			name:   "restart-completed",
			before: &openbaov1alpha1.ClusterRestoreStatus{Name: "restore", UID: "current"},
			after:  &openbaov1alpha1.ClusterRestoreStatus{Name: "restore", UID: "current", RestartCompletedAt: &completedAt},
		},
		{
			name:   "replacement-request",
			before: &openbaov1alpha1.ClusterRestoreStatus{Name: "old-restore", UID: "old", RestartCompletedAt: &completedAt},
			after:  &openbaov1alpha1.ClusterRestoreStatus{Name: "new-restore", UID: "new"},
		},
	} {
		for _, raceApply := range []bool{false, true} {
			timing := "before-read"
			if raceApply {
				timing = "after-read"
			}
			t.Run(tt.name+"/"+timing, func(t *testing.T) {
				cluster := createMinimalCluster(t, newTestNamespace(t), "adminops-restore")
				mutateStatus := adminopsstatus.NewMutator(k8sClient, k8sClient)
				require.NoError(t, mutateStatus(ctx, cluster, func(obj *openbaov1alpha1.OpenBaoCluster) error {
					obj.Status.Restore = tt.before.DeepCopy()
					obj.Status.Backup = &openbaov1alpha1.BackupStatus{LastFailureReason: "preserve-backup"}
					return nil
				}, adminops.RespectOwnership))
				original := cluster.DeepCopy()
				desired := cluster.DeepCopy()
				desired.Status.AdminOps = &openbaov1alpha1.AdminOpsControllerStatus{
					LastError: &openbaov1alpha1.ControllerErrorStatus{Reason: "TestError", Message: "unrelated adminops error"},
				}
				wantAdminOps := desired.Status.AdminOps.DeepCopy()
				var wantRestore *openbaov1alpha1.ClusterRestoreStatus
				writeRestore := func() {
					t.Helper()
					require.NoError(t, mutateStatus(ctx, cluster, func(obj *openbaov1alpha1.OpenBaoCluster) error {
						obj.Status.Restore = tt.after.DeepCopy()
						return nil
					}, adminops.RespectOwnership))
					wantRestore = cluster.Status.Restore.DeepCopy()
				}
				if !raceApply {
					writeRestore()
				}

				applies, conflicts := 0, 0
				writer := interceptor.NewClient(baseWriter, interceptor.Funcs{
					SubResourceApply: func(ctx context.Context, c client.Client, subresource string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
						applies++
						if raceApply && applies == 1 {
							writeRestore()
						}
						err := c.SubResource(subresource).Apply(ctx, obj, opts...)
						if apierrors.IsConflict(err) {
							conflicts++
						}
						return err
					},
				})
				require.NoError(t, openbaocluster.PatchAdminOpsOwnedFieldsWithReader(ctx, k8sClient, writer, logr.Discard(), original, desired, "adminops-error"))

				stored := &openbaov1alpha1.OpenBaoCluster{}
				require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), stored))
				require.Equal(t, wantRestore, stored.Status.Restore, "final patch must preserve the restore manager's latest state")
				require.Equal(t, wantAdminOps, stored.Status.AdminOps)
				require.Equal(t, original.Status.Backup, stored.Status.Backup)
				require.Equal(t, stored.Status, desired.Status)
				require.Equal(t, stored.ResourceVersion, desired.ResourceVersion)
				if raceApply {
					require.Equal(t, 1, conflicts, "the API server must reject the stale resource version")
					require.Equal(t, 2, applies)
				} else {
					require.Zero(t, conflicts)
					require.Equal(t, 1, applies)
				}
			})
		}
	}
}
