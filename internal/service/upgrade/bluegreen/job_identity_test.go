package bluegreen

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

func TestExecutorJobsAreScopedToUpgradeOperation(t *testing.T) {
	t.Setenv(constants.EnvOperatorVersion, "1.0.0")
	for _, action := range []ExecutorAction{
		ActionJoinGreenNonVoters, ActionWaitGreenSynced, ActionRepairConsensus, ActionRemoveGreenPeers,
	} {
		for _, legacyID := range []string{"", "legacy-operation", "new-operation"} {
			t.Run(string(action)+"/"+legacyID, func(t *testing.T) {
				ctx := context.Background()
				scheme := runtime.NewScheme()
				require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
				require.NoError(t, batchv1.AddToScheme(scheme))
				require.NoError(t, corev1.AddToScheme(scheme))
				cluster := newPhaseMachineCluster()
				cluster.UID = "test-uid"
				cluster.Spec.Upgrade.JWTAuthRole = "upgrade"
				cluster.Status.BlueGreen.OperationID = legacyID
				c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&batchv1.Job{}).Build()
				manager := &Manager{client: c, reader: c, scheme: scheme}
				if legacyID == "new-operation" {
					cluster.Status.BlueGreen.OperationID = ""
					manager.recordBlueGreenUpgradeStart(logr.Discard(), cluster)
				}
				previousID := cluster.Status.BlueGreen.OperationID
				run := func() bool {
					t.Helper()
					if action == ActionRepairConsensus || action == ActionRemoveGreenPeers {
						_, waiting, err := manager.ensureRollbackExecutorJob(ctx, logr.Discard(), cluster, action,
							"waiting", "failed", func(logr.Logger, *openbaov1alpha1.OpenBaoCluster, string) { t.Fatal("unexpected Job failure") })
						require.NoError(t, err)
						return !waiting
					}
					step, err := manager.runExecutorJobStep(ctx, logr.Discard(), cluster, action, "failed")
					require.NoError(t, err)
					return step.Completed
				}
				require.False(t, run())
				jobs := &batchv1.JobList{}
				require.NoError(t, c.List(ctx, jobs))
				require.Len(t, jobs.Items, 1)
				oldJob := jobs.Items[0].DeepCopy()
				// A restart must observe the same pending Job, including legacy operations.
				cluster = cluster.DeepCopy()
				require.False(t, run())
				require.NoError(t, c.List(ctx, jobs))
				require.Len(t, jobs.Items, 1)
				oldJob.Status.Succeeded = 1
				require.NoError(t, c.Status().Update(ctx, oldJob))
				require.True(t, run())

				// Rollback leaves completed executor Jobs until their TTL expires.
				blueRevision, greenRevision := cluster.Status.BlueGreen.BlueRevision, cluster.Status.BlueGreen.GreenRevision
				core.FinalizeBlueGreenTerminalState(cluster, false)
				manager.recordBlueGreenUpgradeStart(logr.Discard(), cluster)
				require.NotEmpty(t, cluster.Status.BlueGreen.OperationID)
				require.NotEqual(t, previousID, cluster.Status.BlueGreen.OperationID)
				cluster.Status.BlueGreen.BlueRevision = blueRevision
				cluster.Status.BlueGreen.GreenRevision = greenRevision
				require.False(t, run(), "a new upgrade must not accept the previous operation's completed Job")
				require.NoError(t, c.List(ctx, jobs))
				require.Len(t, jobs.Items, 2)
				require.False(t, run(), "repeated reconciliation must keep waiting for the new Job")
				require.NoError(t, c.List(ctx, jobs))
				require.Len(t, jobs.Items, 2)
				for i := range jobs.Items {
					job := &jobs.Items[i]
					if job.Name == oldJob.Name {
						continue
					}
					job.Status.Succeeded = 1
					require.NoError(t, c.Status().Update(ctx, job))
				}
				require.True(t, run())
				require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(oldJob), &batchv1.Job{}))
			})
		}
	}
}

func TestOperationScopedRunID(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, operationID, runID, want string }{
		{"legacy initial", "", "", ""},
		{"legacy retry", "legacy-uuid", "retry-2", "retry-2"},
		{"legacy rollback", "legacy-uuid", "rollback", "rollback"},
		{"new initial", "bg-v2-uuid", "", "bg-v2-uuid/"},
		{"new retry", "bg-v2-uuid", "retry-2", "bg-v2-uuid/retry-2"},
		{"new rollback retry", "bg-v2-uuid", "rollback-retry-2", "bg-v2-uuid/rollback-retry-2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, operationScopedRunID(tt.operationID, tt.runID))
		})
	}
}
