package rolling

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/backup"
	"github.com/dc-tec/openbao-operator/internal/constants"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/security"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

func TestHandlePreUpgradeSnapshot_NotEnabled(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.4.4",
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: false,
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Initialized:    true,
		},
	}

	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, scheme, backup.NewUpgradeStrategyRuntime(k8sClient, scheme), openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	complete, err := manager.handlePreUpgradeSnapshot(context.Background(), testLogger(), cluster)
	assert.NoError(t, err, "should return nil when preUpgradeSnapshot is disabled")
	assert.True(t, complete, "should return complete=true when disabled")
}

func TestHandlePreUpgradeSnapshot_NoBackupConfig(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.4.4",
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: true,
			},
			// No Backup config
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Initialized:    true,
		},
	}

	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, scheme, backup.NewUpgradeStrategyRuntime(k8sClient, scheme), openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	complete, err := manager.handlePreUpgradeSnapshot(context.Background(), testLogger(), cluster)
	assert.Error(t, err, "should return error when backup config is missing")
	assert.False(t, complete, "should return complete=false on error")
	assert.Contains(t, err.Error(), "backup configuration is required")
}

func TestHandlePreUpgradeSnapshot_CreatesJob(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile:  openbaov1alpha1.ProfileDevelopment,
			Version:  "2.4.4",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: true,
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				Image:       "test-image:latest",
				JWTAuthRole: "backup",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://test-endpoint",
					Bucket:   "test-bucket",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Initialized:    true,
		},
	}

	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	// Create secret for backup token (if needed)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-tls-ca",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"ca.crt": []byte("test-ca-cert"),
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster, secret).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, scheme, backup.NewUpgradeStrategyRuntime(k8sClient, scheme), openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	complete, err := manager.handlePreUpgradeSnapshot(context.Background(), testLogger(), cluster)
	assert.NoError(t, err, "should create backup job successfully")
	assert.False(t, complete, "should return complete=false when job is created")

	// Verify job was created
	jobList := &batchv1.JobList{}
	err = k8sClient.List(context.Background(), jobList, client.InNamespace("test-ns"))
	require.NoError(t, err)
	assert.Len(t, jobList.Items, 1, "should have created one backup job")
	assert.Contains(t, jobList.Items[0].Name, "pre-upgrade-backup", "job name should contain pre-upgrade-backup")
}

func TestHandlePreUpgradeSnapshot_CreateAlreadyExists(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile:  openbaov1alpha1.ProfileDevelopment,
			Version:  "2.4.4",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: true,
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				Image:       "test-image:latest",
				JWTAuthRole: "backup",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://test-endpoint",
					Bucket:   "test-bucket",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Initialized:    true,
		},
	}

	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*batchv1.Job); ok {
					return apierrors.NewAlreadyExists(schema.GroupResource{Group: "batch", Resource: "jobs"}, obj.GetName())
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, scheme, backup.NewUpgradeStrategyRuntime(k8sClient, scheme), openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	complete, err := manager.handlePreUpgradeSnapshot(context.Background(), testLogger(), cluster)
	assert.NoError(t, err, "AlreadyExists on create should be treated as idempotent")
	assert.False(t, complete, "snapshot should remain in-progress when create races")
}

func TestHandlePreUpgradeSnapshot_CreatesJobWithOIDCDefaultRole(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile:  openbaov1alpha1.ProfileDevelopment,
			Version:  "2.4.4",
			Replicas: 3,
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled: true,
				},
			},
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: true,
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				Image: "test-image:latest",
				// JWTAuthRole intentionally omitted; should default when OIDC is enabled
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://test-endpoint",
					Bucket:   "test-bucket",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Initialized:    true,
		},
	}

	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-tls-ca",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"ca.crt": []byte("test-ca-cert"),
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster, secret).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, scheme, backup.NewUpgradeStrategyRuntime(k8sClient, scheme), openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	complete, err := manager.handlePreUpgradeSnapshot(context.Background(), testLogger(), cluster)
	assert.NoError(t, err, "should create backup job successfully when OIDC is enabled")
	assert.False(t, complete, "should return complete=false when job is created")

	jobList := &batchv1.JobList{}
	err = k8sClient.List(context.Background(), jobList, client.InNamespace("test-ns"))
	require.NoError(t, err)
	assert.Len(t, jobList.Items, 1, "should have created one backup job")
	assert.Contains(t, jobList.Items[0].Name, "pre-upgrade-backup", "job name should contain pre-upgrade-backup")
}

func TestHandlePreUpgradeSnapshot_HardenedRequiresEgressRules(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileHardened,
			Version: "2.4.4",
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: true,
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				Image:       "test-image:latest",
				JWTAuthRole: "backup",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "https://example.com",
					Bucket:   "test-bucket",
					RoleARN:  "arn:aws:iam::123456789012:role/test",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Initialized:    true,
		},
	}

	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, scheme, backup.NewUpgradeStrategyRuntime(k8sClient, scheme), openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	complete, err := manager.handlePreUpgradeSnapshot(context.Background(), testLogger(), cluster)
	assert.Error(t, err)
	assert.False(t, complete)
	assert.Contains(t, err.Error(), "spec.network.egressRules")
}

func TestHandlePreUpgradeSnapshot_WaitsForRunningJob(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: true,
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				Image:       "test-image:latest",
				JWTAuthRole: "backup",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://test-endpoint",
					Bucket:   "test-bucket",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Initialized:    true,
		},
	}

	// Create a running backup job
	runningJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pre-upgrade-backup-test-cluster-gen0",
			Namespace: "test-ns",
			Labels: map[string]string{
				constants.LabelAppInstance:  "test-cluster",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
				"openbao.org/cluster":       "test-cluster",
				"openbao.org/component":     "backup",
				"openbao.org/backup-type":   "pre-upgrade",
			},
		},
		Status: batchv1.JobStatus{
			Active:    1,
			Succeeded: 0,
			Failed:    0,
		},
	}

	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster, runningJob).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, scheme, backup.NewUpgradeStrategyRuntime(k8sClient, scheme), openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	complete, err := manager.handlePreUpgradeSnapshot(context.Background(), testLogger(), cluster)
	assert.NoError(t, err, "should return nil when job is running (requeue)")
	assert.False(t, complete, "should return complete=false when job is running")
}

func TestHandlePreUpgradeSnapshot_JobCompleted(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: true,
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				Image:       "test-image:latest",
				JWTAuthRole: "backup",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://test-endpoint",
					Bucket:   "test-bucket",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Initialized:    true,
		},
	}

	// Create a completed backup job
	completedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pre-upgrade-backup-test-cluster-gen0",
			Namespace: "test-ns",
			Labels: map[string]string{
				constants.LabelAppInstance:  "test-cluster",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
				"openbao.org/cluster":       "test-cluster",
				"openbao.org/component":     "backup",
				"openbao.org/backup-type":   "pre-upgrade",
			},
		},
		Status: batchv1.JobStatus{
			Active:    0,
			Succeeded: 1,
			Failed:    0,
		},
	}

	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster, completedJob).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, scheme, backup.NewUpgradeStrategyRuntime(k8sClient, scheme), openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	complete, err := manager.handlePreUpgradeSnapshot(context.Background(), testLogger(), cluster)
	assert.NoError(t, err, "should return nil when job is completed")
	assert.True(t, complete, "should return complete=true when job is completed")
}

func TestHandlePreUpgradeSnapshot_JobFailed(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: true,
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				Image:       "test-image:latest",
				JWTAuthRole: "backup",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://test-endpoint",
					Bucket:   "test-bucket",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Initialized:    true,
		},
	}

	// Create failed backup jobs for the current attempt family:
	// one exact expected name + suffixed variants.
	// This simulates leftover retries from the same upgrade attempt.
	var failedJobs []client.Object
	for i := 0; i < upgrade.DefaultMaxPreUpgradeBackupRetries; i++ {
		name := fmt.Sprintf("pre-upgrade-backup-test-cluster-gen0-attempt-%d", i)
		if i == 0 {
			// Exact name is required for strict lookup in findExistingPreUpgradeBackupJob.
			name = "pre-upgrade-backup-test-cluster-gen0"
		}
		failedJobs = append(failedJobs, &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "test-ns",
				Labels: map[string]string{
					constants.LabelAppInstance:       "test-cluster",
					constants.LabelAppManagedBy:      constants.LabelValueAppManagedByOpenBaoOperator,
					constants.LabelOpenBaoCluster:    "test-cluster",
					constants.LabelOpenBaoComponent:  "backup",
					constants.LabelOpenBaoBackupType: constants.BackupTypePreUpgrade,
				},
			},
			Status: batchv1.JobStatus{
				Active:    0,
				Succeeded: 0,
				Failed:    1,
			},
		})
	}

	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	objs := []client.Object{cluster}
	objs = append(objs, failedJobs...)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(objs...).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, scheme, backup.NewUpgradeStrategyRuntime(k8sClient, scheme), openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	// With max retries exceeded, should return error
	complete, err := manager.handlePreUpgradeSnapshot(context.Background(), testLogger(), cluster)
	assert.Error(t, err, "should return error after max retries exceeded")
	assert.False(t, complete, "should return complete=false when job failed")
	assert.Contains(t, err.Error(), "max retries exceeded", "error should mention max retries exceeded")
}

func TestHandlePreUpgradeSnapshot_JobFailedRetriesOnFirstFailure(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: true,
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				Image:       "test-image:latest",
				JWTAuthRole: "backup",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://test-endpoint",
					Bucket:   "test-bucket",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Initialized:    true,
		},
	}

	// Create single failed backup job - should trigger retry, not error
	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pre-upgrade-backup-test-cluster-gen0",
			Namespace: "test-ns",
			Labels: map[string]string{
				constants.LabelAppInstance:       "test-cluster",
				constants.LabelAppManagedBy:      constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:    "test-cluster",
				constants.LabelOpenBaoComponent:  "backup",
				constants.LabelOpenBaoBackupType: constants.BackupTypePreUpgrade,
			},
		},
		Status: batchv1.JobStatus{
			Active:    0,
			Succeeded: 0,
			Failed:    1,
		},
	}

	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster, failedJob).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, scheme, backup.NewUpgradeStrategyRuntime(k8sClient, scheme), openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	// With single failure, should delete and return nil to trigger retry
	complete, err := manager.handlePreUpgradeSnapshot(context.Background(), testLogger(), cluster)
	assert.NoError(t, err, "should not return error on first failure (retry)")
	assert.False(t, complete, "should return complete=false for requeue")

	// Verify job was deleted
	jobList := &batchv1.JobList{}
	err = k8sClient.List(context.Background(), jobList, client.InNamespace("test-ns"))
	require.NoError(t, err)
	assert.Len(t, jobList.Items, 0, "failed job should have been deleted for retry")
}

func TestHandlePreUpgradeSnapshot_IgnoresFailedJobsFromPreviousGenerations(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-cluster",
			Namespace:  "test-ns",
			Generation: 7,
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: true,
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				Image:       "test-image:latest",
				JWTAuthRole: "backup",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://test-endpoint",
					Bucket:   "test-bucket",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Initialized:    true,
		},
	}

	currentFailedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pre-upgrade-backup-test-cluster-gen7",
			Namespace: "test-ns",
			Labels: map[string]string{
				constants.LabelAppInstance:       "test-cluster",
				constants.LabelAppManagedBy:      constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:    "test-cluster",
				constants.LabelOpenBaoComponent:  "backup",
				constants.LabelOpenBaoBackupType: constants.BackupTypePreUpgrade,
			},
		},
		Status: batchv1.JobStatus{
			Failed: 1,
		},
	}

	staleFailedJob1 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pre-upgrade-backup-test-cluster-gen6",
			Namespace: "test-ns",
			Labels: map[string]string{
				constants.LabelAppInstance:       "test-cluster",
				constants.LabelAppManagedBy:      constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:    "test-cluster",
				constants.LabelOpenBaoComponent:  "backup",
				constants.LabelOpenBaoBackupType: constants.BackupTypePreUpgrade,
			},
		},
		Status: batchv1.JobStatus{
			Failed: 1,
		},
	}

	staleFailedJob2 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pre-upgrade-backup-test-cluster-gen5-attempt-1",
			Namespace: "test-ns",
			Labels: map[string]string{
				constants.LabelAppInstance:       "test-cluster",
				constants.LabelAppManagedBy:      constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:    "test-cluster",
				constants.LabelOpenBaoComponent:  "backup",
				constants.LabelOpenBaoBackupType: constants.BackupTypePreUpgrade,
			},
		},
		Status: batchv1.JobStatus{
			Failed: 1,
		},
	}

	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster, currentFailedJob, staleFailedJob1, staleFailedJob2).
		WithReturnManagedFields().
		Build()

	manager := NewManager(k8sClient, scheme, backup.NewUpgradeStrategyRuntime(k8sClient, scheme), openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	complete, err := manager.handlePreUpgradeSnapshot(context.Background(), testLogger(), cluster)
	assert.NoError(t, err, "historical failed jobs from older generations must not consume current retry budget")
	assert.False(t, complete, "failed current-attempt job should trigger retry flow")

	jobList := &batchv1.JobList{}
	err = k8sClient.List(context.Background(), jobList, client.InNamespace("test-ns"))
	require.NoError(t, err)

	remainingNames := make([]string, 0, len(jobList.Items))
	for i := range jobList.Items {
		remainingNames = append(remainingNames, jobList.Items[i].Name)
	}
	assert.NotContains(t, remainingNames, "pre-upgrade-backup-test-cluster-gen7")
	assert.Contains(t, remainingNames, "pre-upgrade-backup-test-cluster-gen6")
	assert.Contains(t, remainingNames, "pre-upgrade-backup-test-cluster-gen5-attempt-1")
}

func TestPreUpgradeSnapshotBlocksUpgradeInitialization(t *testing.T) {
	// This test verifies that upgrade initialization is blocked when backup job is running
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: true,
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				Image:       "test-image:latest",
				JWTAuthRole: "backup",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://test-endpoint",
					Bucket:   "test-bucket",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			Initialized:    true,
			Upgrade:        nil, // No upgrade in progress
		},
	}

	// Create a running backup job
	runningJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			// Name must match the expected format: pre-upgrade-backup-<cluster>-gen<generation>
			// Cluster generation defaults to 0 in tests unless specified
			Name:      "pre-upgrade-backup-test-cluster-gen0",
			Namespace: "test-ns",
			Labels: map[string]string{
				constants.LabelAppInstance:  "test-cluster",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
				"openbao.org/cluster":       "test-cluster",
				"openbao.org/component":     "backup",
				"openbao.org/backup-type":   "pre-upgrade",
			},
		},
		Status: batchv1.JobStatus{
			Active:    1,
			Succeeded: 0,
			Failed:    0,
		},
	}

	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	// Create StatefulSet
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: 3,
		},
	}

	// Create CA Secret (needed for getClusterCACert)
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-tls-ca",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"ca.crt": []byte("test-ca-cert"),
		},
	}

	// Create Pods (needed for verifyClusterHealth -> getClusterPods)
	var pods []client.Object
	for i := 0; i < 3; i++ {
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-cluster-%d", i),
				Namespace: "test-ns",
				Labels: map[string]string{
					constants.LabelAppInstance:  "test-cluster",
					constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
					constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		})
	}

	// Objects to add to the fake client
	objs := []client.Object{cluster, runningJob, sts, caSecret}
	objs = append(objs, pods...)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(objs...).
		WithReturnManagedFields().
		Build()

	// Mock Client Factory that uses MockClusterActions to avoid HTTP servers
	// verifyClusterHealth expects: healthyCount >= quorum, leaderCount == 1.
	// We need 1 leader, 2 standbys.
	mockFactory := func(config openbaoapi.ClientConfig) (openbaoapi.ClusterActions, error) {
		// Determine if this should be the leader based on the pod name in BaseURL
		isLeader := strings.Contains(config.BaseURL, "-0.")

		return &openbaoapi.MockClusterActions{
			IsHealthyFunc: func(ctx context.Context) (bool, error) {
				return true, nil
			},
			IsLeaderFunc: func(ctx context.Context) (bool, error) {
				return isLeader, nil
			},
		}, nil
	}

	manager := NewManagerWithClientFactory(k8sClient, scheme, backup.NewUpgradeStrategyRuntime(k8sClient, scheme), mockFactory, openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	// Call Reconcile - it should handle pre-upgrade snapshot and requeue
	_, err := manager.Reconcile(context.Background(), testLogger(), cluster)
	assert.NoError(t, err, "should not error when backup is running")

	// Verify upgrade was NOT initialized (Status.Upgrade should still be nil)
	assert.Nil(t, cluster.Status.Upgrade, "upgrade should not be initialized while backup is running")
}
