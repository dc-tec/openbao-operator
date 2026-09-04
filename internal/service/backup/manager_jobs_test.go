package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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

func TestObserveBackupJobs_ReturnsListError(t *testing.T) {
	t.Parallel()

	cluster := newTestClusterWithBackup("job-observation-error", "default")
	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errors.New("list failed")
			},
		}).
		Build()

	_, err := newBackupManager(k8sClient).observeBackupJobs(context.Background(), cluster)
	require.EqualError(t, err, "failed to list backup jobs: list failed")
}
