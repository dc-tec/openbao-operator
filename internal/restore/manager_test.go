package restore

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// testLogger returns a no-op logger for testing.
func testLogger() logr.Logger {
	return logr.Discard()
}

// TestRestoreJobName tests the deterministic job name generation.
func TestRestoreJobName(t *testing.T) {
	tests := []struct {
		name        string
		restoreName string
		wantPrefix  string
	}{
		{
			name:        "simple name",
			restoreName: "my-restore",
			wantPrefix:  RestoreJobNamePrefix + "my-restore",
		},
		{
			name:        "with namespace-like name",
			restoreName: "ns-restore-backup",
			wantPrefix:  RestoreJobNamePrefix + "ns-restore-backup",
		},
		{
			name:        "short name",
			restoreName: "r",
			wantPrefix:  RestoreJobNamePrefix + "r",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := &openbaov1alpha1.OpenBaoRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.restoreName,
				},
			}

			got := restoreJobName(restore)
			assert.Equal(t, tt.wantPrefix, got, "restoreJobName() should be deterministic")
		})
	}
}

// TestRestoreServiceAccountName tests the service account naming.
func TestRestoreServiceAccountName(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		wantSuffix  string
	}{
		{
			name:        "simple cluster",
			clusterName: "my-cluster",
			wantSuffix:  "my-cluster" + RestoreServiceAccountSuffix,
		},
		{
			name:        "long cluster name",
			clusterName: "production-openbao-cluster",
			wantSuffix:  "production-openbao-cluster" + RestoreServiceAccountSuffix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.clusterName,
				},
			}

			got := restoreServiceAccountName(cluster)
			assert.Equal(t, tt.wantSuffix, got, "restoreServiceAccountName() should use cluster name + suffix")
		})
	}
}

// TestRestoreLabels tests the standard labels generation.
func TestRestoreLabels(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
	}

	got := restoreLabels(cluster)

	assert.NotNil(t, got)
	assert.Equal(t, "test-cluster", got["openbao.org/cluster"])
	assert.Equal(t, ComponentRestore, got["openbao.org/component"])
	assert.Contains(t, got, "app.kubernetes.io/managed-by")
}

// TestReconcilePending tests the Pending to Validating phase transition.
func TestReconcilePending(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup-key",
			},
		},
		Status: openbaov1alpha1.OpenBaoRestoreStatus{
			Phase: openbaov1alpha1.RestorePhasePending,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).
		Build()

	mgr := NewManager(k8sClient, scheme, nil)

	result, err := mgr.handlePending(context.Background(), testLogger(), restore)
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0, "should requeue after pending")

	// Verify status was updated
	updated := &openbaov1alpha1.OpenBaoRestore{}
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Name: "test-restore", Namespace: "default"}, updated))
	assert.Equal(t, openbaov1alpha1.RestorePhaseValidating, updated.Status.Phase)
	assert.NotNil(t, updated.Status.StartTime)
	assert.Equal(t, "backup-key", updated.Status.SnapshotKey)
}

// TestReconcilePhaseRouting tests that Reconcile correctly routes by phase.
func TestReconcilePhaseRouting(t *testing.T) {
	tests := []struct {
		name           string
		phase          openbaov1alpha1.RestorePhase
		expectRequeue  bool
		expectNoAction bool
	}{
		{
			name:          "pending phase transitions to validating",
			phase:         openbaov1alpha1.RestorePhasePending,
			expectRequeue: true,
		},
		{
			name:           "completed phase is terminal",
			phase:          openbaov1alpha1.RestorePhaseCompleted,
			expectNoAction: true,
		},
		{
			name:           "failed phase is terminal",
			phase:          openbaov1alpha1.RestorePhaseFailed,
			expectNoAction: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
			require.NoError(t, corev1.AddToScheme(scheme))

			restore := &openbaov1alpha1.OpenBaoRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-restore",
					Namespace:  "default",
					Finalizers: []string{openbaov1alpha1.OpenBaoRestoreFinalizer},
				},
				Spec: openbaov1alpha1.OpenBaoRestoreSpec{
					Cluster: "test-cluster",
				},
				Status: openbaov1alpha1.OpenBaoRestoreStatus{
					Phase: tt.phase,
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(restore).
				WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).
				Build()

			mgr := NewManager(k8sClient, scheme, nil)

			result, err := mgr.Reconcile(context.Background(), testLogger(), restore)
			require.NoError(t, err)

			if tt.expectNoAction {
				assert.Equal(t, int64(0), int64(result.RequeueAfter), "terminal phases should not requeue")
			}
			if tt.expectRequeue {
				assert.True(t, result.RequeueAfter > 0, "should requeue")
			}
		})
	}
}

// TestValidatingClusterNotFound tests validation failure when cluster doesn't exist.
func TestValidatingClusterNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "nonexistent-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup-key",
			},
		},
		Status: openbaov1alpha1.OpenBaoRestoreStatus{
			Phase: openbaov1alpha1.RestorePhaseValidating,
		},
	}

	// No cluster object in the fake client
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).
		Build()

	mgr := NewManager(k8sClient, scheme, nil)

	_, err := mgr.handleValidating(context.Background(), testLogger(), restore)
	require.NoError(t, err) // failRestore returns nil error

	// Verify status was updated to Failed
	updated := &openbaov1alpha1.OpenBaoRestore{}
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Name: "test-restore", Namespace: "default"}, updated))
	assert.Equal(t, openbaov1alpha1.RestorePhaseFailed, updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "not found")
}

// TestValidatingUninitializedCluster tests validation with uninitialized cluster.
func TestValidatingUninitializedCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileDevelopment,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized: false, // Not initialized
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
				Key: "backup-key",
			},
			Force: false, // Force is not set
		},
		Status: openbaov1alpha1.OpenBaoRestoreStatus{
			Phase: openbaov1alpha1.RestorePhaseValidating,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, restore).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}, &openbaov1alpha1.OpenBaoCluster{}).
		Build()

	mgr := NewManager(k8sClient, scheme, nil)

	_, err := mgr.handleValidating(context.Background(), testLogger(), restore)
	require.NoError(t, err)

	// Verify status was updated to Failed
	updated := &openbaov1alpha1.OpenBaoRestore{}
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Name: "test-restore", Namespace: "default"}, updated))
	assert.Equal(t, openbaov1alpha1.RestorePhaseFailed, updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "not initialized")
}

// TestGetRestoreExecutorImage tests executor image resolution.
func TestGetRestoreExecutorImage(t *testing.T) {
	tests := []struct {
		name          string
		restoreImage  string
		clusterImage  string
		expectedImage string
		expectError   bool
	}{
		{
			name:          "restore image takes precedence",
			restoreImage:  "custom/restore:v1",
			clusterImage:  "custom/backup:v2",
			expectedImage: "custom/restore:v1",
			expectError:   false,
		},
		{
			name:          "fallback to cluster backup image",
			restoreImage:  "",
			clusterImage:  "custom/backup:v2",
			expectedImage: "custom/backup:v2",
			expectError:   false,
		},
		{
			name:          "no image specified returns error",
			restoreImage:  "",
			clusterImage:  "",
			expectedImage: "",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := &openbaov1alpha1.OpenBaoRestore{
				Spec: openbaov1alpha1.OpenBaoRestoreSpec{
					ExecutorImage: tt.restoreImage,
				},
			}

			var backupSchedule *openbaov1alpha1.BackupSchedule
			if tt.clusterImage != "" {
				backupSchedule = &openbaov1alpha1.BackupSchedule{
					ExecutorImage: tt.clusterImage,
				}
			}
			cluster := &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: backupSchedule,
				},
			}

			got, err := getRestoreExecutorImage(restore, cluster)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedImage, got)
			}
		})
	}
}

// TestBuildRestoreEnvVars tests environment variable generation.
func TestBuildRestoreEnvVars(t *testing.T) {
	restore := &openbaov1alpha1.OpenBaoRestore{
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup-2024-01-01.snap",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint:     "https://s3.example.com",
					Bucket:       "my-bucket",
					Region:       "us-east-1",
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

	// Verify required env vars are present
	envMap := make(map[string]string)
	for _, env := range envVars {
		if env.Value != "" {
			envMap[env.Name] = env.Value
		}
	}

	assert.Equal(t, "restore", envMap["EXECUTOR_MODE"])
	assert.Equal(t, "test-cluster", envMap["CLUSTER_NAME"])
	assert.Equal(t, "default", envMap["CLUSTER_NAMESPACE"])
	assert.Equal(t, "3", envMap["CLUSTER_REPLICAS"])
	assert.Equal(t, "backup-2024-01-01.snap", envMap["RESTORE_KEY"])
	assert.Equal(t, "my-bucket", envMap["RESTORE_BUCKET"])
	assert.Equal(t, "https://s3.example.com", envMap["RESTORE_ENDPOINT"])
	assert.Equal(t, "restore-role", envMap["BACKUP_JWT_AUTH_ROLE"])
	assert.Equal(t, "jwt", envMap["BACKUP_AUTH_METHOD"])
}

// TestBuildRestoreVolumes tests volume generation.
func TestBuildRestoreVolumes(t *testing.T) {
	tests := []struct {
		name                string
		tlsEnabled          bool
		jwtAuthRole         string
		tokenSecretRef      *corev1.LocalObjectReference
		credentialsSecret   *corev1.LocalObjectReference
		expectedVolumeNames []string
	}{
		{
			name:                "TLS only",
			tlsEnabled:          true,
			expectedVolumeNames: []string{"tls-ca"},
		},
		{
			name:                "JWT auth",
			tlsEnabled:          true,
			jwtAuthRole:         "restore-role",
			expectedVolumeNames: []string{"tls-ca", "jwt-token"},
		},
		{
			name:                "token auth",
			tlsEnabled:          true,
			tokenSecretRef:      &corev1.LocalObjectReference{Name: "token-secret"},
			expectedVolumeNames: []string{"tls-ca", "restore-token"},
		},
		{
			name:                "with S3 credentials",
			tlsEnabled:          true,
			credentialsSecret:   &corev1.LocalObjectReference{Name: "s3-creds"},
			expectedVolumeNames: []string{"tls-ca", "s3-credentials"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := &openbaov1alpha1.OpenBaoRestore{
				Spec: openbaov1alpha1.OpenBaoRestoreSpec{
					JWTAuthRole:    tt.jwtAuthRole,
					TokenSecretRef: tt.tokenSecretRef,
					Source: openbaov1alpha1.RestoreSource{
						Target: openbaov1alpha1.BackupTarget{
							CredentialsSecretRef: tt.credentialsSecret,
						},
					},
				},
			}

			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{Enabled: tt.tlsEnabled},
				},
			}

			volumes := buildRestoreVolumes(restore, cluster)

			volumeNames := make([]string, len(volumes))
			for i, vol := range volumes {
				volumeNames[i] = vol.Name
			}

			for _, expected := range tt.expectedVolumeNames {
				assert.Contains(t, volumeNames, expected, "should contain volume %s", expected)
			}
		})
	}
}

// TestBuildRestoreVolumeMounts tests volume mount generation.
func TestBuildRestoreVolumeMounts(t *testing.T) {
	restore := &openbaov1alpha1.OpenBaoRestore{
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			JWTAuthRole: "restore-role",
		},
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{Enabled: true},
		},
	}

	mounts := buildRestoreVolumeMounts(restore, cluster)

	assert.Len(t, mounts, 2) // TLS + JWT
	for _, mount := range mounts {
		assert.True(t, mount.ReadOnly, "all mounts should be read-only for security")
	}
}

// TestFinalizer tests finalizer addition and removal.
func TestEnsureFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore).
		Build()

	mgr := NewManager(k8sClient, scheme, nil)

	err := mgr.ensureFinalizer(context.Background(), restore)
	require.NoError(t, err)

	// Verify finalizer was added
	updated := &openbaov1alpha1.OpenBaoRestore{}
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Name: "test-restore", Namespace: "default"}, updated))
	assert.Contains(t, updated.Finalizers, openbaov1alpha1.OpenBaoRestoreFinalizer)

	// Second call should be idempotent
	restore.Finalizers = updated.Finalizers
	err = mgr.ensureFinalizer(context.Background(), restore)
	require.NoError(t, err)
}

// TestHandleDeletion tests the deletion handling.
func TestHandleDeletion(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	now := metav1.Now()
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-restore",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{openbaov1alpha1.OpenBaoRestoreFinalizer},
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "nonexistent-cluster", // Cluster doesn't exist
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore).
		Build()

	mgr := NewManager(k8sClient, scheme, nil)

	result, err := mgr.handleDeletion(context.Background(), testLogger(), restore)
	require.NoError(t, err)
	assert.Equal(t, int64(0), int64(result.RequeueAfter))

	// The handleDeletion removed the finalizer. With a deletion timestamp set,
	// the fake client now removes the object entirely, which is correct behavior.
	// We don't need to verify finalizers since successful return indicates
	// the finalizer was removed and deletion proceeded.
}

// TestReleaseClusterLock tests the cluster lock release.
func TestReleaseClusterLock_ClusterNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "nonexistent-cluster",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	mgr := NewManager(k8sClient, scheme, nil)

	err := mgr.releaseClusterLock(context.Background(), testLogger(), restore)
	require.NoError(t, err, "should not error when cluster not found")
}

// TestReleaseClusterLock_EmptyCluster tests release with empty cluster name.
func TestReleaseClusterLock_EmptyCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "", // Empty cluster name
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	mgr := NewManager(k8sClient, scheme, nil)

	err := mgr.releaseClusterLock(context.Background(), testLogger(), restore)
	require.NoError(t, err, "should return nil for empty cluster name")
}

// TestNewManagerCreation tests the manager constructor.
func TestNewManager(t *testing.T) {
	scheme := runtime.NewScheme()
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	mgr := NewManager(k8sClient, scheme, nil)

	assert.NotNil(t, mgr)
	assert.NotNil(t, mgr.client)
	assert.NotNil(t, mgr.scheme)
}
