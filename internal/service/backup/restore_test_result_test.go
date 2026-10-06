package backup

import (
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/adminops"
)

func TestRestoreTestSummaryPreservesIdentityWithoutProviderText(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		status       api.OpenBaoRestoreStatus
	}{
		{"preparation", "PreparationFailed", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseFailed}},
		{"executor", "ExecutorFailed", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseFailed, Execution: &api.RestoreExecutionStatus{}}},
		{"unconfirmed", "ApplicationUnconfirmed", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseFailed, SubmissionClaim: &api.RestoreSubmissionClaim{}}},
		{"abandoned", "AdministratorAbandoned", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseFailed, AdministratorDisposition: api.RestoreAdministratorAbandon}},
		{"bootstrap timeout", "TargetBootstrapTimedOut", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseFailed, Conditions: []metav1.Condition{{Type: constants.RestoreConditionType, Reason: "TargetBootstrapTimedOut"}}}},
		{"passed", "SnapshotApplied", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseCompleted, Target: &api.RestoreTargetStatus{AppliedAt: &metav1.Time{}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const sensitive = "provider-error-with-secret-token"
			run := &api.RestoreTestRun{Name: "run", Namespace: "destination", Key: "original/snapshot"}
			child := &api.OpenBaoRestore{Status: tc.status}
			child.Spec.Source.ExpectedDigest = "sha256:" + strings.Repeat("a", 64)
			child.Status.Message = sensitive
			for i := range child.Status.Conditions {
				child.Status.Conditions[i].Message = sensitive
			}
			result := summarizeRestoreTest(run, child, time.Now())
			require.Equal(t, run.Namespace, result.Namespace)
			require.Equal(t, run.Key, result.Key)
			require.Equal(t, child.Spec.Source.ExpectedDigest, result.Digest)
			require.Equal(t, tc.reason, result.Reason)
			require.NotContains(t, result.Message, sensitive)
			require.LessOrEqual(t, len(result.Message), 512)
		})
	}
}

func TestFailedRestoreTestKeepsSummaryAfterChildDeletion(t *testing.T) {
	cluster := newRestoreTestSource("source")
	run := &api.RestoreTestRun{Name: "failed-child", Namespace: "recovery", Key: "old/snapshot", UID: "child-uid"}
	child := &api.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{Name: run.Name, Namespace: run.Namespace, UID: run.UID,
			Annotations: map[string]string{constants.AnnotationRestoreTestSource: string(cluster.UID)}},
		Spec: api.OpenBaoRestoreSpec{Source: api.RestoreSource{ExpectedDigest: "sha256:" + strings.Repeat("b", 64)}},
		Status: api.OpenBaoRestoreStatus{Phase: api.RestorePhaseFailed, Message: "sensitive provider response",
			Execution: &api.RestoreExecutionStatus{TerminalResult: api.RestoreExecutionResultFailed},
			Target:    &api.RestoreTargetStatus{Cleanup: api.RestoreTargetCleanupComplete}},
	}
	c := newTestClient(t, cluster, child)
	m := newBackupManager(c)
	require.NoError(t, m.adminOpsMutator(t.Context(), cluster, func(current *api.OpenBaoCluster) error {
		current.Status.Backup.RestoreTest = &api.RestoreTestStatus{Active: run}
		return nil
	}, adminops.ForceOwnership))
	for range 3 {
		require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(cluster), cluster))
		require.NoError(t, m.reconcileRestoreTest(t.Context(), logr.Discard(), cluster, time.Now()))
	}
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(cluster), cluster))
	require.Nil(t, cluster.Status.Backup.RestoreTest.Active)
	require.True(t, apierrors.IsNotFound(c.Get(t.Context(), client.ObjectKeyFromObject(child), child)))
	result := cluster.Status.Backup.RestoreTest.Last
	require.Equal(t, api.RestoreTestFailed, result.Outcome)
	require.Equal(t, run.Namespace, result.Namespace)
	require.Equal(t, run.Key, result.Key, "later backups must not replace the tested snapshot identity")
	require.Equal(t, "sha256:"+strings.Repeat("b", 64), result.Digest)
	require.Equal(t, "ExecutorFailed", result.Reason)
	require.NotContains(t, result.Message, "sensitive")
}
