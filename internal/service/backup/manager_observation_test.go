package backup

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestObserveBackup_DoesNotMutateCluster(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	cluster := newTestClusterWithBackup("observation", "default")
	cluster.Spec.Backup.JWTAuthRole = testBackupJWTAuthRole
	cluster.Status.Initialized = true
	cluster.Status.CurrentVersion = cluster.Spec.Version
	next := metav1.NewTime(now.Add(time.Hour))
	cluster.Status.Backup.NextScheduledBackup = &next

	manager := newBackupManager(newTestClient(t, cluster))
	original := cluster.DeepCopy()
	observation, err := manager.observeBackup(context.Background(), logr.Discard(), cluster, now)
	if err != nil {
		t.Fatalf("observeBackup() error = %v", err)
	}
	if !reflect.DeepEqual(cluster, original) {
		t.Fatal("observeBackup() mutated the cluster")
	}
	if observation.due {
		t.Fatal("due = true, want false")
	}
	if !observation.scheduledTime.Equal(next.Time) {
		t.Fatalf("scheduledTime = %v, want %v", observation.scheduledTime, next.Time)
	}
	if got := decideBackup(observation).kind; got != backupDecisionIdle {
		t.Fatalf("decision kind = %d, want %d", got, backupDecisionIdle)
	}
}

func TestObserveBackup_ActiveJobDefersManualTriggerEffect(t *testing.T) {
	t.Parallel()

	const triggerToken = "manual-1"

	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	cluster := newTestClusterWithBackup("active-observation", "default")
	cluster.Annotations = map[string]string{constants.AnnotationTriggerBackup: triggerToken}
	job := newBackupJobForCluster(cluster, "active-backup", now.Add(-time.Minute))
	job.Status.Active = 1

	manager := newBackupManager(newTestClient(t, cluster, job))
	original := cluster.DeepCopy()
	observation, err := manager.observeBackup(context.Background(), logr.Discard(), cluster, now)
	if err != nil {
		t.Fatalf("observeBackup() error = %v", err)
	}
	if !reflect.DeepEqual(cluster, original) {
		t.Fatal("observeBackup() applied the manual trigger effect")
	}
	if observation.manualTriggerToken != triggerToken {
		t.Fatalf("manualTriggerToken = %q, want %s", observation.manualTriggerToken, triggerToken)
	}
	if got := decideBackup(observation).kind; got != backupDecisionObserve {
		t.Fatalf("decision kind = %d, want %d", got, backupDecisionObserve)
	}
}
