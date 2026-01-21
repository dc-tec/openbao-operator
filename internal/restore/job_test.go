package restore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestBuildRestoreJob_PodSecurityContext_Platform(t *testing.T) {
	restoreObj := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Key: "snapshots/test.snap",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint:     "https://s3.example.com",
					Bucket:       "bao",
					Region:       "us-east-1",
					UsePathStyle: true,
				},
			},
			ExecutorImage: "example.com/restore-executor:v1",
		},
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
		},
	}

	t.Run("openshift omits pinned IDs", func(t *testing.T) {
		mgr := &Manager{Platform: constants.PlatformOpenShift}
		job, err := mgr.buildRestoreJob(restoreObj, cluster, "")
		require.NoError(t, err)

		sc := job.Spec.Template.Spec.SecurityContext
		require.NotNil(t, sc)
		require.Equal(t, ptr.To(true), sc.RunAsNonRoot)
		require.NotNil(t, sc.SeccompProfile)
		require.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, sc.SeccompProfile.Type)

		require.Nil(t, sc.RunAsUser)
		require.Nil(t, sc.RunAsGroup)
		require.Nil(t, sc.FSGroup)
	})

	t.Run("kubernetes pins IDs", func(t *testing.T) {
		mgr := &Manager{Platform: constants.PlatformKubernetes}
		job, err := mgr.buildRestoreJob(restoreObj, cluster, "")
		require.NoError(t, err)

		sc := job.Spec.Template.Spec.SecurityContext
		require.NotNil(t, sc)
		require.Equal(t, ptr.To(true), sc.RunAsNonRoot)

		require.Equal(t, ptr.To(constants.UserBackup), sc.RunAsUser)
		require.Equal(t, ptr.To(constants.GroupBackup), sc.RunAsGroup)
		require.Equal(t, ptr.To(constants.GroupBackup), sc.FSGroup)
	})
}

func TestBuildRestoreEnvVars_S3(t *testing.T) {
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.snap",
				Target: openbaov1alpha1.BackupTarget{
					Provider:     constants.StorageProviderS3,
					Endpoint:     "https://s3.amazonaws.com",
					Bucket:       "test-bucket",
					Region:       "us-west-2",
					UsePathStyle: true,
				},
			},
			JWTAuthRole: "restore-role",
		},
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
		},
	}

	envVars := buildRestoreEnvVars(restore, cluster)

	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Name] = env.Value
	}

	// Verify common env vars
	assert.Equal(t, "restore", envMap["EXECUTOR_MODE"])
	assert.Equal(t, "test-cluster", envMap[constants.EnvClusterName])
	assert.Equal(t, "default", envMap[constants.EnvClusterNamespace])
	assert.Equal(t, "3", envMap[constants.EnvClusterReplicas])
	assert.Equal(t, constants.StorageProviderS3, envMap[constants.EnvBackupProvider])
	assert.Equal(t, "https://s3.amazonaws.com", envMap[constants.EnvBackupEndpoint])
	assert.Equal(t, "test-bucket", envMap[constants.EnvBackupBucket])
	assert.Equal(t, "backup.snap", envMap[constants.EnvRestoreKey])
	assert.Equal(t, "test-bucket", envMap[constants.EnvRestoreBucket])
	assert.Equal(t, "https://s3.amazonaws.com", envMap[constants.EnvRestoreEndpoint])

	// Verify S3-specific env vars
	assert.Equal(t, "us-west-2", envMap[constants.EnvBackupRegion])
	assert.Equal(t, "true", envMap[constants.EnvBackupUsePathStyle])
	assert.Equal(t, "us-west-2", envMap[constants.EnvRestoreRegion])
	assert.Equal(t, "true", envMap[constants.EnvRestoreUsePathStyle])

	// Verify JWT auth
	assert.Equal(t, "restore-role", envMap[constants.EnvBackupJWTAuthRole])
	assert.Equal(t, constants.BackupAuthMethodJWT, envMap[constants.EnvBackupAuthMethod])

	// Verify GCS/Azure vars are NOT set
	assert.Empty(t, envMap[constants.EnvBackupGCSProject])
	assert.Empty(t, envMap[constants.EnvBackupAzureStorageAccount])
	assert.Empty(t, envMap[constants.EnvBackupAzureContainer])
}

func TestBuildRestoreEnvVars_S3Default(t *testing.T) {
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.snap",
				Target: openbaov1alpha1.BackupTarget{
					// Provider not set, should default to S3
					Endpoint: "https://s3.amazonaws.com",
					Bucket:   "test-bucket",
					Region:   "us-east-1",
				},
			},
		},
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 1,
		},
	}

	envVars := buildRestoreEnvVars(restore, cluster)

	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Name] = env.Value
	}

	// Should default to S3
	assert.Equal(t, constants.StorageProviderS3, envMap[constants.EnvBackupProvider])
	assert.Equal(t, "us-east-1", envMap[constants.EnvBackupRegion])
	assert.Equal(t, "false", envMap[constants.EnvBackupUsePathStyle]) // default
}

func TestBuildRestoreEnvVars_GCS(t *testing.T) {
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.snap",
				Target: openbaov1alpha1.BackupTarget{
					Provider: "gcs",
					Endpoint: "https://storage.googleapis.com",
					Bucket:   "test-bucket",
					GCS: &openbaov1alpha1.GCSTargetConfig{
						Project: "my-gcp-project",
					},
				},
			},
			JWTAuthRole: "restore-role",
		},
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 2,
		},
	}

	envVars := buildRestoreEnvVars(restore, cluster)

	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Name] = env.Value
	}

	// Verify common env vars
	assert.Equal(t, constants.StorageProviderGCS, envMap[constants.EnvBackupProvider])
	assert.Equal(t, "https://storage.googleapis.com", envMap[constants.EnvBackupEndpoint])
	assert.Equal(t, "test-bucket", envMap[constants.EnvBackupBucket])

	// Verify GCS-specific env var
	assert.Equal(t, "my-gcp-project", envMap[constants.EnvBackupGCSProject])

	// Verify S3/Azure vars are NOT set
	assert.Empty(t, envMap[constants.EnvBackupRegion])
	assert.Empty(t, envMap[constants.EnvBackupUsePathStyle])
	assert.Empty(t, envMap[constants.EnvRestoreRegion])
	assert.Empty(t, envMap[constants.EnvRestoreUsePathStyle])
	assert.Empty(t, envMap[constants.EnvBackupAzureStorageAccount])
	assert.Empty(t, envMap[constants.EnvBackupAzureContainer])
}

func TestBuildRestoreEnvVars_GCS_NoProject(t *testing.T) {
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.snap",
				Target: openbaov1alpha1.BackupTarget{
					Provider: "gcs",
					Bucket:   "test-bucket",
					GCS:      &openbaov1alpha1.GCSTargetConfig{
						// Project not set (optional)
					},
				},
			},
		},
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 1,
		},
	}

	envVars := buildRestoreEnvVars(restore, cluster)

	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Name] = env.Value
	}

	// GCS project should not be set when not specified
	assert.Empty(t, envMap[constants.EnvBackupGCSProject])
	assert.Equal(t, constants.StorageProviderGCS, envMap[constants.EnvBackupProvider])
}

func TestBuildRestoreEnvVars_Azure(t *testing.T) {
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.snap",
				Target: openbaov1alpha1.BackupTarget{
					Provider: "azure",
					Endpoint: "https://myaccount.blob.core.windows.net",
					Bucket:   "test-container",
					Azure: &openbaov1alpha1.AzureTargetConfig{
						StorageAccount: "myaccount",
						Container:      "test-container",
					},
				},
			},
			JWTAuthRole: "restore-role",
		},
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
		},
	}

	envVars := buildRestoreEnvVars(restore, cluster)

	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Name] = env.Value
	}

	// Verify common env vars
	assert.Equal(t, constants.StorageProviderAzure, envMap[constants.EnvBackupProvider])
	assert.Equal(t, "https://myaccount.blob.core.windows.net", envMap[constants.EnvBackupEndpoint])
	assert.Equal(t, "test-container", envMap[constants.EnvBackupBucket])

	// Verify Azure-specific env vars
	assert.Equal(t, "myaccount", envMap[constants.EnvBackupAzureStorageAccount])
	assert.Equal(t, "test-container", envMap[constants.EnvBackupAzureContainer])

	// Verify S3/GCS vars are NOT set
	assert.Empty(t, envMap[constants.EnvBackupRegion])
	assert.Empty(t, envMap[constants.EnvBackupUsePathStyle])
	assert.Empty(t, envMap[constants.EnvRestoreRegion])
	assert.Empty(t, envMap[constants.EnvRestoreUsePathStyle])
	assert.Empty(t, envMap[constants.EnvBackupGCSProject])
}

func TestBuildRestoreEnvVars_Azure_NoContainer(t *testing.T) {
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.snap",
				Target: openbaov1alpha1.BackupTarget{
					Provider: "azure",
					Bucket:   "test-container",
					Azure: &openbaov1alpha1.AzureTargetConfig{
						StorageAccount: "myaccount",
						// Container not set (optional, uses bucket)
					},
				},
			},
		},
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 1,
		},
	}

	envVars := buildRestoreEnvVars(restore, cluster)

	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Name] = env.Value
	}

	// Azure container should not be set when not specified
	assert.Equal(t, "myaccount", envMap[constants.EnvBackupAzureStorageAccount])
	assert.Empty(t, envMap[constants.EnvBackupAzureContainer])
	assert.Equal(t, constants.StorageProviderAzure, envMap[constants.EnvBackupProvider])
}

func TestBuildRestoreEnvVars_JWTAuth(t *testing.T) {
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.snap",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "https://s3.amazonaws.com",
					Bucket:   "test-bucket",
				},
			},
			JWTAuthRole: "restore-role",
		},
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 1,
		},
	}

	envVars := buildRestoreEnvVars(restore, cluster)

	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Name] = env.Value
	}

	assert.Equal(t, "restore-role", envMap[constants.EnvBackupJWTAuthRole])
	assert.Equal(t, constants.BackupAuthMethodJWT, envMap[constants.EnvBackupAuthMethod])
}

func TestBuildRestoreEnvVars_TokenAuth(t *testing.T) {
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.snap",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "https://s3.amazonaws.com",
					Bucket:   "test-bucket",
				},
			},
			TokenSecretRef: &corev1.LocalObjectReference{
				Name: "restore-token",
			},
		},
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 1,
		},
	}

	envVars := buildRestoreEnvVars(restore, cluster)

	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Name] = env.Value
	}

	assert.Empty(t, envMap[constants.EnvBackupJWTAuthRole])
	assert.Equal(t, constants.BackupAuthMethodToken, envMap[constants.EnvBackupAuthMethod])
}
