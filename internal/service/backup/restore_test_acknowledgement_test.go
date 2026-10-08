package backup

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/kubebao/openbao-operator/api/v1alpha1"
	"github.com/kubebao/openbao-operator/internal/platform/constants"
	"github.com/kubebao/openbao-operator/internal/port/adminops"
)

func TestRestoreTestReservationRelease(t *testing.T) {
	for _, tc := range []struct {
		name                                          string
		deleting, disabled, replacement, known, stale bool
	}{
		{name: "missing"}, {name: "replacement", replacement: true}, {name: "source deleting", deleting: true},
		{name: "disabled", disabled: true}, {name: "stale", stale: true}, {name: "known child", known: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cluster := newRestoreTestSource("source")
			if tc.disabled {
				cluster.Spec.Backup = nil
			}
			stamp := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
			run := &api.RestoreTestRun{Name: "uncertain", Namespace: "recovery", StartedAt: stamp}
			cluster.Status.Backup.RestoreTest = &api.RestoreTestStatus{Active: run, LastScheduledAt: &stamp, LastBackupCount: 7, LastSuccessTime: &stamp}
			ack := run.Name + "/Release"
			if tc.stale {
				ack = "previous/Release"
			}
			cluster.Annotations = map[string]string{constants.AnnotationRestoreTestAcknowledge: ack}
			initialStatus := cluster.Status.Backup.RestoreTest
			cluster.Status.Backup.RestoreTest = nil
			c := newTestClient(t, cluster)
			child := &api.OpenBaoRestore{ObjectMeta: metav1.ObjectMeta{Name: run.Name, Namespace: run.Namespace, UID: "child"}}
			if tc.known {
				child.Annotations = map[string]string{constants.AnnotationRestoreTestSource: string(cluster.UID)}
			}
			if tc.known || tc.replacement {
				require.NoError(t, c.Create(t.Context(), child))
			}
			m := newBackupManager(c)
			require.NoError(t, m.adminOpsMutator(t.Context(), cluster, func(current *api.OpenBaoCluster) error {
				current.Status.Backup.RestoreTest = initialStatus.DeepCopy()
				return nil
			}, adminops.ForceOwnership))
			var err error
			if tc.deleting {
				err = CancelRestoreTest(t.Context(), c, c, m.adminOpsMutator, cluster)
			} else {
				err = m.reconcileRestoreTest(t.Context(), logr.Discard(), cluster, time.Now())
			}
			require.NoError(t, err)
			require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(cluster), cluster))
			status := cluster.Status.Backup.RestoreTest
			if tc.stale || tc.known {
				require.NotNil(t, status.Active)
				require.Nil(t, status.Last)
			} else {
				require.Nil(t, status.Active)
				require.Equal(t, api.RestoreTestFailed, status.Last.Outcome)
				require.Equal(t, "AdministratorReleased", status.Conditions[0].Reason)
			}
			require.Equal(t, int64(7), status.LastBackupCount)
			require.Equal(t, &stamp, status.LastScheduledAt)
			require.Equal(t, &stamp, status.LastSuccessTime)
			if tc.known || tc.replacement {
				require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(child), child))
				require.Nil(t, child.DeletionTimestamp)
			}
		})
	}
}

func TestRestoreTestReleaseRejectsConcurrentChange(t *testing.T) {
	for _, changed := range []string{"source UID", "active run", "acknowledgement"} {
		t.Run(changed, func(t *testing.T) {
			cluster := newRestoreTestSource("source")
			cluster.Status.Backup.RestoreTest = &api.RestoreTestStatus{Active: &api.RestoreTestRun{Name: "run", Namespace: "recovery"}}
			cluster.Annotations = map[string]string{constants.AnnotationRestoreTestAcknowledge: "run/Release"}
			current := cluster.DeepCopy()
			switch changed {
			case "source UID":
				current.UID = "replacement"
			case "active run":
				current.Status.Backup.RestoreTest.Active.Name = "next"
			case "acknowledgement":
				delete(current.Annotations, constants.AnnotationRestoreTestAcknowledge)
			}
			mutate := func(_ context.Context, _ *api.OpenBaoCluster, apply func(*api.OpenBaoCluster) error, _ adminops.OwnershipPolicy) error {
				return apply(current)
			}
			released, err := releaseRestoreTestReservation(t.Context(), mutate, cluster)
			require.ErrorContains(t, err, "changed before release")
			require.False(t, released)
			require.NotNil(t, current.Status.Backup.RestoreTest.Active)
			require.Nil(t, current.Status.Backup.RestoreTest.Last)
		})
	}
}
