package restore

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	logger := logr.Discard()

	tests := []struct {
		name          string
		restore       *openbaov1alpha1.OpenBaoRestore
		cluster       *openbaov1alpha1.OpenBaoCluster
		existingOb    []client.Object
		expectedPhase openbaov1alpha1.RestorePhase
		wantErr       bool
	}{
		{
			name: "Pending to Validating",
			restore: &openbaov1alpha1.OpenBaoRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-restore",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoRestoreSpec{
					Cluster: "test-cluster",
					Source: openbaov1alpha1.RestoreSource{
						Key: "backup.enc",
						Target: openbaov1alpha1.BackupTarget{
							Endpoint: "http://minio",
							Bucket:   "backups",
						},
					},
				},
				Status: openbaov1alpha1.OpenBaoRestoreStatus{
					Phase: openbaov1alpha1.RestorePhasePending,
				},
			},
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
			},
			expectedPhase: openbaov1alpha1.RestorePhaseValidating,
		},
		{
			name: "Validating to Running",
			restore: &openbaov1alpha1.OpenBaoRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-restore",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoRestoreSpec{
					Cluster: "test-cluster",
					Source: openbaov1alpha1.RestoreSource{
						Key: "backup.enc",
						Target: openbaov1alpha1.BackupTarget{
							Endpoint: "http://minio",
							Bucket:   "backups",
						},
					},
				},
				Status: openbaov1alpha1.OpenBaoRestoreStatus{
					Phase: openbaov1alpha1.RestorePhaseValidating,
				},
			},
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Initialized: true,
				},
			},
			expectedPhase: openbaov1alpha1.RestorePhaseRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []client.Object{tt.restore}
			if tt.cluster != nil {
				objects = append(objects, tt.cluster)
			}
			objects = append(objects, tt.existingOb...)

			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).WithObjects(objects...).Build()
			m := NewManager(c, scheme)

			_, err := m.Reconcile(context.Background(), logger, tt.restore)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Check the updated object
			updated := &openbaov1alpha1.OpenBaoRestore{}
			err = c.Get(context.Background(), types.NamespacedName{Name: tt.restore.Name, Namespace: tt.restore.Namespace}, updated)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedPhase, updated.Status.Phase)
		})
	}
}

func TestHandleRunning(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	logger := logr.Discard()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 1,
			Backup: &openbaov1alpha1.BackupSchedule{
				ExecutorImage: "openbao:latest",
				Schedule:      "0 0 * * *",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://minio",
					Bucket:   "backups",
				},
			},
		},
	}

	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.enc",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://minio",
					Bucket:   "backups",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoRestoreStatus{
			Phase: openbaov1alpha1.RestorePhaseRunning,
		},
	}

	// 1. First run: Should create job
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).WithObjects(cluster, restore).Build()
	m := NewManager(c, scheme)

	result, err := m.Reconcile(context.Background(), logger, restore)
	assert.NoError(t, err)
	assert.Equal(t, 10*time.Second, result.RequeueAfter)

	// Verify job created
	jobKey := types.NamespacedName{Name: "restore-test-restore", Namespace: "default"}
	job := &batchv1.Job{}
	err = c.Get(context.Background(), jobKey, job)
	assert.NoError(t, err)

	// 2. Second run: Job succeeded
	succeededJob := job.DeepCopy()
	succeededJob.Status.Succeeded = 1
	c = fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).WithObjects(cluster, restore, succeededJob).Build()
	m = NewManager(c, scheme)

	_, err = m.Reconcile(context.Background(), logger, restore)
	assert.NoError(t, err)

	// Verify status completed
	updated := &openbaov1alpha1.OpenBaoRestore{}
	err = c.Get(context.Background(), types.NamespacedName{Name: restore.Name, Namespace: restore.Namespace}, updated)
	assert.NoError(t, err)
	assert.Equal(t, openbaov1alpha1.RestorePhaseCompleted, updated.Status.Phase)
}
