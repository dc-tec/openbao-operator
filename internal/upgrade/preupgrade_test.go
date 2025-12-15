package upgrade

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/constants"
	openbaoapi "github.com/openbao/operator/internal/openbao"
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
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	manager := NewManager(k8sClient, scheme)

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
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	manager := NewManager(k8sClient, scheme)

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
			Version:  "2.4.4",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				PreUpgradeSnapshot: true,
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				ExecutorImage: "test-image:latest",
				JWTAuthRole:   "backup",
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

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, secret).Build()
	manager := NewManager(k8sClient, scheme)

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
				ExecutorImage: "test-image:latest",
				JWTAuthRole:   "backup",
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
			Name:      "pre-upgrade-backup-test-cluster-20251207-120000",
			Namespace: "test-ns",
			Labels: map[string]string{
				constants.LabelAppInstance:  "test-cluster",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
				"openbao.org/cluster":          "test-cluster",
				"openbao.org/component":        "backup",
				"openbao.org/backup-type":      "pre-upgrade",
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
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, runningJob).Build()
	manager := NewManager(k8sClient, scheme)

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
				ExecutorImage: "test-image:latest",
				JWTAuthRole:   "backup",
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
			Name:      "pre-upgrade-backup-test-cluster-20251207-120000",
			Namespace: "test-ns",
			Labels: map[string]string{
				constants.LabelAppInstance:  "test-cluster",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
				"openbao.org/cluster":          "test-cluster",
				"openbao.org/component":        "backup",
				"openbao.org/backup-type":      "pre-upgrade",
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
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, completedJob).Build()
	manager := NewManager(k8sClient, scheme)

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
				ExecutorImage: "test-image:latest",
				JWTAuthRole:   "backup",
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

	// Create a failed backup job
	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pre-upgrade-backup-test-cluster-20251207-120000",
			Namespace: "test-ns",
			Labels: map[string]string{
				constants.LabelAppInstance:  "test-cluster",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
				"openbao.org/cluster":          "test-cluster",
				"openbao.org/component":        "backup",
				"openbao.org/backup-type":      "pre-upgrade",
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
	_ = openbaov1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, failedJob).Build()
	manager := NewManager(k8sClient, scheme)

	complete, err := manager.handlePreUpgradeSnapshot(context.Background(), testLogger(), cluster)
	assert.Error(t, err, "should return error when job failed")
	assert.False(t, complete, "should return complete=false when job failed")
	assert.Contains(t, err.Error(), "failed", "error should mention job failure")
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
				ExecutorImage: "test-image:latest",
				JWTAuthRole:   "backup",
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
			Name:      "pre-upgrade-backup-test-cluster-20251207-120000",
			Namespace: "test-ns",
			Labels: map[string]string{
				constants.LabelAppInstance:  "test-cluster",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
				"openbao.org/cluster":          "test-cluster",
				"openbao.org/component":        "backup",
				"openbao.org/backup-type":      "pre-upgrade",
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

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

	// Create a mock OpenBao server to handle health checks
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock healthy response for all pods
		if r.URL.Path == constants.APIPathSysHealth {
			w.WriteHeader(http.StatusOK)
			// Return leader for one pod, standby for others to satisfy quorum and leader checks
			// Since we don't know easily which client is calling, we'll return initialized=true, sealed=false
			// and standby=false (Leader) for simplicity. This satisfies "single leader" check if called sequentially,
			// but verifyClusterHealth might complain about multiple leaders if called for all 3.
			// However, verifyClusterHealth iterates.
			// Let's make it smarter or just simple for now:
			// If verifyClusterHealth sees 3 leaders, it errors with "multiple leaders detected".
			// We need to differentiate.
			// We can differentiate based on the Host header or assume the test environment doesn't check host.
			// A simpler way: The client factory can configure the response.
			_, _ = w.Write([]byte(`{"initialized": true, "sealed": false, "standby": false}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Mock Client Factory that returns a client pointing to our test server
	// AND handles the "single leader" constraint by toggling response based on config.BaseURL if possible,
	// or we just make sure verifyClusterHealth sees what it expects.
	// verifyClusterHealth expects: healthyCount >= quorum, leaderCount == 1.
	// We need 1 leader, 2 standbys.
	mockFactory := func(config openbaoapi.ClientConfig) (*openbaoapi.Client, error) {
		// Determine if this should be the leader based on the pod name in BaseURL
		isLeader := strings.Contains(config.BaseURL, "-0.")

		// Create a specific server for this client to control response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == constants.APIPathSysHealth {
				w.WriteHeader(http.StatusOK)
				if isLeader {
					_, _ = w.Write([]byte(`{"initialized": true, "sealed": false, "standby": false}`))
				} else {
					_, _ = w.Write([]byte(`{"initialized": true, "sealed": false, "standby": true}`))
				}
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		// Note: leaking server here for simplicity in test, will be cleaned up on process exit

		// Override URL to point to this pod's mock server
		config.BaseURL = server.URL
		config.CACert = nil // Disable TLS for mock server
		return openbaoapi.NewClient(config)
	}

	manager := NewManagerWithClientFactory(k8sClient, scheme, mockFactory)

	// Call Reconcile - it should handle pre-upgrade snapshot and requeue
	err := manager.Reconcile(context.Background(), testLogger(), cluster)
	assert.NoError(t, err, "should not error when backup is running")

	// Verify upgrade was NOT initialized (Status.Upgrade should still be nil)
	assert.Nil(t, cluster.Status.Upgrade, "upgrade should not be initialized while backup is running")
}
