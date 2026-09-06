//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/adminopsstatus"
	"github.com/dc-tec/openbao-operator/internal/port/adminops"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

var upgradeAcknowledgementCases = []struct {
	name             string
	mark             func(*openbaov1alpha1.OpenBaoClusterStatus, string)
	acknowledgements upgrade.RequestAcknowledgements
	pending          func(*openbaov1alpha1.OpenBaoCluster) bool
	phase            openbaov1alpha1.BlueGreenPhase
}{
	{"retry", upgrade.MarkRetryRequestHandled, upgrade.RequestAcknowledgements{Retry: "handled"}, upgrade.RetryRequestPending, ""},
	{"promote", upgrade.MarkPromoteRequestHandled, upgrade.RequestAcknowledgements{Promote: "handled"}, upgrade.PromoteRequestPending, openbaov1alpha1.PhasePromoting},
	{"rollback", upgrade.MarkRollbackRequestHandled, upgrade.RequestAcknowledgements{Rollback: "handled"}, upgrade.RollbackRequestPending, openbaov1alpha1.PhaseRollingBack},
}

func TestAdminOpsFinalPatchPreservesUpgradeRequestSiblings(t *testing.T) {
	baseWriter, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sScheme})
	require.NoError(t, err)
	for _, tt := range upgradeAcknowledgementCases {
		for _, raceApply := range []bool{false, true} {
			timing := "before-read"
			if raceApply {
				timing = "after-read"
			}
			t.Run(tt.name+"/"+timing, func(t *testing.T) {
				cluster := createMinimalCluster(t, newTestNamespace(t), "request-siblings")
				mutateStatus := adminopsstatus.NewMutator(k8sClient, k8sClient)
				require.NoError(t, mutateStatus(ctx, cluster, func(obj *openbaov1alpha1.OpenBaoCluster) error {
					if tt.phase != "" {
						obj.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{Phase: openbaov1alpha1.PhaseSyncing}
					}
					obj.Status.UpgradeRequests = &openbaov1alpha1.UpgradeRequestStatus{
						LastHandledRetry: "initial", LastHandledPromote: "initial", LastHandledRollback: "initial",
					}
					return nil
				}, adminops.RespectOwnership))
				cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{Requests: &openbaov1alpha1.UpgradeRequestConfig{
					Retry: "handled", Promote: "handled", Rollback: "handled",
				}}
				require.NoError(t, k8sClient.Update(ctx, cluster))
				original := cluster.DeepCopy()
				desired := cluster.DeepCopy()
				if tt.phase != "" {
					desired.Status.BlueGreen.Phase = tt.phase
				}
				latest := original.Status.DeepCopy()
				latest.UpgradeRequests = &openbaov1alpha1.UpgradeRequestStatus{
					LastHandledRetry: "latest", LastHandledPromote: "latest", LastHandledRollback: "latest",
				}
				tt.mark(latest, "initial")
				want := latest.DeepCopy()
				tt.mark(want, "handled")
				writeSiblings := func() {
					t.Helper()
					cluster.Spec.Upgrade.Requests = &openbaov1alpha1.UpgradeRequestConfig{
						Retry: "newer", Promote: "newer", Rollback: "newer",
					}
					require.NoError(t, k8sClient.Update(ctx, cluster))
					require.NoError(t, mutateStatus(ctx, cluster, func(obj *openbaov1alpha1.OpenBaoCluster) error {
						obj.Status.UpgradeRequests = latest.UpgradeRequests.DeepCopy()
						return nil
					}, adminops.RespectOwnership))
				}
				if !raceApply {
					writeSiblings()
				}
				applies, conflicts := 0, 0
				writer := interceptor.NewClient(baseWriter, interceptor.Funcs{
					SubResourceApply: func(ctx context.Context, c client.Client, subresource string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
						applies++
						if raceApply && applies == 1 {
							writeSiblings()
						}
						err := c.SubResource(subresource).Apply(ctx, obj, opts...)
						if apierrors.IsConflict(err) {
							conflicts++
						}
						return err
					},
				})
				require.NoError(t, openbaocluster.PatchAdminOpsOwnedFieldsWithReader(ctx, k8sClient, writer, logr.Discard(), original, desired, tt.acknowledgements, "request"))
				stored := &openbaov1alpha1.OpenBaoCluster{}
				require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), stored))
				require.Equal(t, want.UpgradeRequests, stored.Status.UpgradeRequests)
				require.Equal(t, stored.Status.UpgradeRequests, desired.Status.UpgradeRequests)
				require.True(t, tt.pending(stored), "a newer spec token must remain pending after the captured token is acknowledged")
				if tt.phase != "" {
					require.Equal(t, tt.phase, stored.Status.BlueGreen.Phase, "transition and acknowledgement must be saved together")
				}
				if raceApply {
					require.Equal(t, 1, conflicts)
					require.Equal(t, 2, applies)
				} else {
					require.Zero(t, conflicts)
					require.Equal(t, 1, applies)
				}
			})
		}
	}
}

func TestAdminOpsAcknowledgementAndTransitionWriteFailures(t *testing.T) {
	baseWriter, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sScheme})
	require.NoError(t, err)
	for _, failReadBack := range []bool{false, true} {
		name := "apply"
		if failReadBack {
			name = "read-back"
		}
		t.Run(name, func(t *testing.T) {
			cluster := createMinimalCluster(t, newTestNamespace(t), "request-failure")
			mutateStatus := adminopsstatus.NewMutator(k8sClient, k8sClient)
			require.NoError(t, mutateStatus(ctx, cluster, func(obj *openbaov1alpha1.OpenBaoCluster) error {
				obj.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{Phase: openbaov1alpha1.PhaseSyncing}
				return nil
			}, adminops.RespectOwnership))
			original := cluster.DeepCopy()
			desired := cluster.DeepCopy()
			desired.Status.BlueGreen.Phase = openbaov1alpha1.PhasePromoting
			acknowledgements := upgrade.RequestAcknowledgements{Promote: "handled"}
			writeErr := errors.New("injected status write failure")
			reads := 0
			c := interceptor.NewClient(baseWriter, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					reads++
					if failReadBack && reads == 2 {
						return writeErr
					}
					return c.Get(ctx, key, obj, opts...)
				},
				SubResourceApply: func(ctx context.Context, c client.Client, subresource string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
					if !failReadBack {
						return writeErr
					}
					return c.SubResource(subresource).Apply(ctx, obj, opts...)
				},
			})
			err := openbaocluster.PatchAdminOpsOwnedFieldsWithReader(ctx, c, c, logr.Discard(), original, desired, acknowledgements, "request")
			require.ErrorIs(t, err, writeErr)
			stored := &openbaov1alpha1.OpenBaoCluster{}
			require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), stored))
			if failReadBack {
				require.Equal(t, 2, reads)
				require.Equal(t, openbaov1alpha1.PhasePromoting, stored.Status.BlueGreen.Phase)
				require.Equal(t, "handled", stored.Status.UpgradeRequests.LastHandledPromote)
			} else {
				require.Equal(t, openbaov1alpha1.PhaseSyncing, stored.Status.BlueGreen.Phase)
				require.Nil(t, stored.Status.UpgradeRequests)
			}
			require.Nil(t, desired.Status.UpgradeRequests, "an error must not invent observed acknowledgement state")
			require.Equal(t, "handled", acknowledgements.Promote, "write intent remains available for retry")
		})
	}
}

func TestAdminOpsPendingUpgradeAcknowledgementSurvivesCheckpoint(t *testing.T) {
	for _, tt := range upgradeAcknowledgementCases {
		t.Run(tt.name, func(t *testing.T) {
			cluster := createMinimalCluster(t, newTestNamespace(t), "request-checkpoint")
			original := cluster.DeepCopy()
			want := original.Status.DeepCopy()
			tt.mark(want, "handled")
			mutateStatus := adminopsstatus.NewMutator(k8sClient, k8sClient)
			require.NoError(t, mutateStatus(ctx, cluster, func(obj *openbaov1alpha1.OpenBaoCluster) error {
				obj.Status.Backup = &openbaov1alpha1.BackupStatus{LastFailureReason: "checkpoint"}
				return nil
			}, adminops.RespectOwnership))
			require.Nil(t, cluster.Status.UpgradeRequests, "checkpoint read-back contains only observed acknowledgements")
			require.NoError(t, openbaocluster.PatchAdminOpsOwnedFieldsWithReader(ctx, k8sClient, k8sClient, logr.Discard(), original, cluster, tt.acknowledgements, "request"))
			stored := &openbaov1alpha1.OpenBaoCluster{}
			require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), stored))
			require.Equal(t, want.UpgradeRequests, stored.Status.UpgradeRequests)
			require.Equal(t, "checkpoint", stored.Status.Backup.LastFailureReason)
		})
	}
}
