package backup

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestObserveBackupJobs_FiltersByBackupType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		backupType   string
		status       batchv1.JobStatus
		wantActive   bool
		wantTerminal bool
	}{
		{
			name:       "observes active scheduled job",
			backupType: constants.BackupTypeScheduled,
			wantActive: true,
		},
		{
			name:         "observes successful scheduled job",
			backupType:   constants.BackupTypeScheduled,
			status:       batchv1.JobStatus{Succeeded: 1},
			wantTerminal: true,
		},
		{
			name:         "observes failed scheduled job",
			backupType:   constants.BackupTypeScheduled,
			status:       batchv1.JobStatus{Failed: 1},
			wantTerminal: true,
		},
		{
			name:       "ignores active pre-upgrade job",
			backupType: constants.BackupTypePreUpgrade,
		},
		{
			name:       "ignores successful pre-upgrade job",
			backupType: constants.BackupTypePreUpgrade,
			status:     batchv1.JobStatus{Succeeded: 1},
		},
		{
			name:       "ignores failed pre-upgrade job",
			backupType: constants.BackupTypePreUpgrade,
			status:     batchv1.JobStatus{Failed: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cluster := newTestClusterWithBackup("job-observation", "default")
			job := newBackupJobForCluster(cluster, "observed-job", time.Now().UTC())
			job.Labels[constants.LabelOpenBaoBackupType] = tt.backupType
			job.Status = tt.status
			manager := newBackupManager(newTestClient(t, cluster, job))

			observation, err := manager.observeBackupJobs(context.Background(), cluster)
			require.NoError(t, err)
			assert.Equal(t, tt.wantActive, observation.hasActive)
			if tt.wantTerminal {
				require.NotNil(t, observation.mostRecentTerminal)
				assert.Equal(t, job.Name, observation.mostRecentTerminal.Name)
				return
			}
			assert.Nil(t, observation.mostRecentTerminal)
		})
	}
}

func TestCheckForCompletedJobs_RequiresBackupKeyOnlyForScheduledJobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		backupType string
		wantErr    string
	}{
		{
			name:       "rejects scheduled job without backup key",
			backupType: constants.BackupTypeScheduled,
			wantErr:    "without openbao.org/backup-key",
		},
		{
			name:       "ignores pre-upgrade job without backup key",
			backupType: constants.BackupTypePreUpgrade,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cluster := newTestClusterWithBackup("completed-job", "default")
			job := newBackupJobForCluster(cluster, "completed-job", time.Now().UTC())
			job.Labels[constants.LabelOpenBaoBackupType] = tt.backupType
			job.Status = batchv1.JobStatus{Succeeded: 1}
			manager := newBackupManager(newTestClient(t, cluster, job))

			result, err := manager.checkForCompletedJobs(context.Background(), logr.Discard(), cluster)
			assert.Equal(t, backupJobProcessResult{}, result)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
