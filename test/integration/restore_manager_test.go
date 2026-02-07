//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/restore"
	"github.com/dc-tec/openbao-operator/internal/security"
)

func TestRestoreManager_TransitionsAndCreatesJob(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "restore-target")
	cluster.Spec.TLS.Enabled = false
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
	})

	restoreObj := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-1",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: cluster.Name,
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.enc",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://minio.svc",
					Bucket:   "backups",
				},
			},
			Image:       "openbao-backup:dev",
			JWTAuthRole: "restore-role", // Required for auth validation
		},
	}
	if err := k8sClient.Create(ctx, restoreObj); err != nil {
		t.Fatalf("create OpenBaoRestore: %v", err)
	}

	mgr := restore.NewManager(k8sClient, k8sScheme, nil, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	// Pending -> Validating
	latest := &openbaov1alpha1.OpenBaoRestore{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	res, err := mgr.Reconcile(ctx, logr.Discard(), latest)
	if err != nil {
		t.Fatalf("reconcile pending: %v", err)
	}
	if res == (ctrl.Result{}) {
		t.Fatalf("expected requeue from pending->validating")
	}

	// Validating -> Running
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	res, err = mgr.Reconcile(ctx, logr.Discard(), latest)
	if err != nil {
		t.Fatalf("reconcile validating: %v", err)
	}
	if res == (ctrl.Result{}) {
		t.Fatalf("expected requeue from validating->running")
	}

	// Running -> create Job and RBAC
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	res, err = mgr.Reconcile(ctx, logr.Discard(), latest)
	if err != nil {
		t.Fatalf("reconcile running (create job): %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Fatalf("expected requeue after creating restore job")
	}

	job := &batchv1.Job{}
	jobName := restore.RestoreJobNamePrefix + latest.Name
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: jobName}, job); err != nil {
		t.Fatalf("expected restore job to exist: %v", err)
	}

	// Mark job succeeded.
	job.Status.Succeeded = 1
	if err := k8sClient.Status().Update(ctx, job); err != nil {
		t.Fatalf("update job status: %v", err)
	}

	// Running -> Completed
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	_, err = mgr.Reconcile(ctx, logr.Discard(), latest)
	if err != nil {
		t.Fatalf("reconcile after job success: %v", err)
	}

	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	if latest.Status.Phase != openbaov1alpha1.RestorePhaseCompleted {
		t.Fatalf("phase=%s want=%s", latest.Status.Phase, openbaov1alpha1.RestorePhaseCompleted)
	}

	// Restore service account should exist (created during validation).
	sa := &corev1.ServiceAccount{}
	saName := cluster.Name + restore.RestoreServiceAccountSuffix
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: saName}, sa); err != nil {
		if apierrors.IsNotFound(err) {
			t.Fatalf("expected restore ServiceAccount %q to exist", saName)
		}
		t.Fatalf("get restore ServiceAccount: %v", err)
	}
}

func TestRestoreManager_GCSProvider(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "restore-target-gcs")
	cluster.Spec.TLS.Enabled = false
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
	})

	restoreObj := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-gcs",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: cluster.Name,
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.enc",
				Target: openbaov1alpha1.BackupTarget{
					Provider: "gcs",
					Endpoint: "https://storage.googleapis.com",
					Bucket:   "backups",
					GCS: &openbaov1alpha1.GCSTargetConfig{
						Project: "my-gcp-project",
					},
				},
			},
			Image:       "openbao-backup:dev",
			JWTAuthRole: "restore-role",
		},
	}
	if err := k8sClient.Create(ctx, restoreObj); err != nil {
		t.Fatalf("create OpenBaoRestore: %v", err)
	}

	mgr := restore.NewManager(k8sClient, k8sScheme, nil, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	// Pending -> Validating
	latest := &openbaov1alpha1.OpenBaoRestore{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	res, err := mgr.Reconcile(ctx, logr.Discard(), latest)
	if err != nil {
		t.Fatalf("reconcile pending: %v", err)
	}
	if res == (ctrl.Result{}) {
		t.Fatalf("expected requeue from pending->validating")
	}

	// Validating -> Running
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	res, err = mgr.Reconcile(ctx, logr.Discard(), latest)
	if err != nil {
		t.Fatalf("reconcile validating: %v", err)
	}
	if res == (ctrl.Result{}) {
		t.Fatalf("expected requeue from validating->running")
	}

	// Running -> create Job
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	res, err = mgr.Reconcile(ctx, logr.Discard(), latest)
	if err != nil {
		t.Fatalf("reconcile running (create job): %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Fatalf("expected requeue after creating restore job")
	}

	// Verify job exists and has correct env vars
	job := &batchv1.Job{}
	jobName := restore.RestoreJobNamePrefix + latest.Name
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: jobName}, job); err != nil {
		t.Fatalf("expected restore job to exist: %v", err)
	}

	// Verify GCS-specific environment variables are set
	container := job.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	if envMap[constants.EnvBackupProvider] != "gcs" {
		t.Errorf("BACKUP_PROVIDER = %v, want gcs", envMap[constants.EnvBackupProvider])
	}
	if envMap[constants.EnvBackupGCSProject] != "my-gcp-project" {
		t.Errorf("BACKUP_GCS_PROJECT = %v, want my-gcp-project", envMap[constants.EnvBackupGCSProject])
	}
	if envMap[constants.EnvBackupEndpoint] != "https://storage.googleapis.com" {
		t.Errorf("BACKUP_ENDPOINT = %v, want https://storage.googleapis.com", envMap[constants.EnvBackupEndpoint])
	}
	if envMap[constants.EnvBackupBucket] != "backups" {
		t.Errorf("BACKUP_BUCKET = %v, want backups", envMap[constants.EnvBackupBucket])
	}

	// Verify S3-specific vars are NOT set
	if envMap[constants.EnvBackupRegion] != "" {
		t.Errorf("BACKUP_REGION should not be set for GCS, got %v", envMap[constants.EnvBackupRegion])
	}
	if envMap[constants.EnvBackupUsePathStyle] != "" {
		t.Errorf("BACKUP_USE_PATH_STYLE should not be set for GCS, got %v", envMap[constants.EnvBackupUsePathStyle])
	}

	// Verify Azure-specific vars are NOT set
	if envMap[constants.EnvBackupAzureStorageAccount] != "" {
		t.Errorf("BACKUP_AZURE_STORAGE_ACCOUNT should not be set for GCS, got %v", envMap[constants.EnvBackupAzureStorageAccount])
	}
}

func TestRestoreManager_AzureProvider(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "restore-target-azure")
	cluster.Spec.TLS.Enabled = false
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
	})

	restoreObj := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-azure",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: cluster.Name,
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.enc",
				Target: openbaov1alpha1.BackupTarget{
					Provider: "azure",
					Endpoint: "https://myaccount.blob.core.windows.net",
					Bucket:   "backups",
					Azure: &openbaov1alpha1.AzureTargetConfig{
						StorageAccount: "myaccount",
						Container:      "backups",
					},
				},
			},
			Image:       "openbao-backup:dev",
			JWTAuthRole: "restore-role",
		},
	}
	if err := k8sClient.Create(ctx, restoreObj); err != nil {
		t.Fatalf("create OpenBaoRestore: %v", err)
	}

	mgr := restore.NewManager(k8sClient, k8sScheme, nil, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	// Pending -> Validating
	latest := &openbaov1alpha1.OpenBaoRestore{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	res, err := mgr.Reconcile(ctx, logr.Discard(), latest)
	if err != nil {
		t.Fatalf("reconcile pending: %v", err)
	}
	if res == (ctrl.Result{}) {
		t.Fatalf("expected requeue from pending->validating")
	}

	// Validating -> Running
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	res, err = mgr.Reconcile(ctx, logr.Discard(), latest)
	if err != nil {
		t.Fatalf("reconcile validating: %v", err)
	}
	if res == (ctrl.Result{}) {
		t.Fatalf("expected requeue from validating->running")
	}

	// Running -> create Job
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	res, err = mgr.Reconcile(ctx, logr.Discard(), latest)
	if err != nil {
		t.Fatalf("reconcile running (create job): %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Fatalf("expected requeue after creating restore job")
	}

	// Verify job exists and has correct env vars
	job := &batchv1.Job{}
	jobName := restore.RestoreJobNamePrefix + latest.Name
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: jobName}, job); err != nil {
		t.Fatalf("expected restore job to exist: %v", err)
	}

	// Verify Azure-specific environment variables are set
	container := job.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	if envMap[constants.EnvBackupProvider] != "azure" {
		t.Errorf("BACKUP_PROVIDER = %v, want azure", envMap[constants.EnvBackupProvider])
	}
	if envMap[constants.EnvBackupAzureStorageAccount] != "myaccount" {
		t.Errorf("BACKUP_AZURE_STORAGE_ACCOUNT = %v, want myaccount", envMap[constants.EnvBackupAzureStorageAccount])
	}
	if envMap[constants.EnvBackupAzureContainer] != "backups" {
		t.Errorf("BACKUP_AZURE_CONTAINER = %v, want backups", envMap[constants.EnvBackupAzureContainer])
	}
	if envMap[constants.EnvBackupEndpoint] != "https://myaccount.blob.core.windows.net" {
		t.Errorf("BACKUP_ENDPOINT = %v, want https://myaccount.blob.core.windows.net", envMap[constants.EnvBackupEndpoint])
	}
	if envMap[constants.EnvBackupBucket] != "backups" {
		t.Errorf("BACKUP_BUCKET = %v, want backups", envMap[constants.EnvBackupBucket])
	}

	// Verify S3-specific vars are NOT set
	if envMap[constants.EnvBackupRegion] != "" {
		t.Errorf("BACKUP_REGION should not be set for Azure, got %v", envMap[constants.EnvBackupRegion])
	}
	if envMap[constants.EnvBackupUsePathStyle] != "" {
		t.Errorf("BACKUP_USE_PATH_STYLE should not be set for Azure, got %v", envMap[constants.EnvBackupUsePathStyle])
	}

	// Verify GCS-specific vars are NOT set
	if envMap[constants.EnvBackupGCSProject] != "" {
		t.Errorf("BACKUP_GCS_PROJECT should not be set for Azure, got %v", envMap[constants.EnvBackupGCSProject])
	}
}

func TestRestoreManager_ValidatingLockContention_RequeuesWithWaitingMessage(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "restore-lock-contention")
	cluster.Spec.TLS.Enabled = false
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
	})

	restoreObj := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-lock-contention",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: cluster.Name,
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.enc",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://minio.svc",
					Bucket:   "backups",
				},
			},
			Image:       "openbao-backup:dev",
			JWTAuthRole: "restore-role",
		},
	}
	if err := k8sClient.Create(ctx, restoreObj); err != nil {
		t.Fatalf("create OpenBaoRestore: %v", err)
	}

	mgr := restore.NewManager(k8sClient, k8sScheme, nil, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")
	latest := &openbaov1alpha1.OpenBaoRestore{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	if _, err := mgr.Reconcile(ctx, logr.Discard(), latest); err != nil {
		t.Fatalf("reconcile pending: %v", err)
	}

	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.OperationLock = &openbaov1alpha1.OperationLockStatus{
			Operation: openbaov1alpha1.ClusterOperationBackup,
			Holder:    constants.ControllerNameOpenBaoCluster + "/backup",
			Message:   "backup operation",
		}
	})

	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore for validating: %v", err)
	}
	if latest.Status.Phase != openbaov1alpha1.RestorePhaseValidating {
		t.Fatalf("expected restore to be in Validating after first reconcile, got %s", latest.Status.Phase)
	}

	result, err := mgr.Reconcile(ctx, logr.Discard(), latest)
	if err != nil {
		t.Fatalf("reconcile validating with lock contention: %v", err)
	}
	if result.RequeueAfter != constants.RequeueShort {
		t.Fatalf("expected requeue=%v when lock is held, got %v", constants.RequeueShort, result.RequeueAfter)
	}

	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get updated restore: %v", err)
	}
	if latest.Status.Phase != openbaov1alpha1.RestorePhaseValidating {
		t.Fatalf("expected restore to remain in Validating phase, got %s", latest.Status.Phase)
	}
	if !strings.Contains(latest.Status.Message, "Waiting for cluster operation lock") {
		t.Fatalf("expected waiting-for-lock message, got %q", latest.Status.Message)
	}
}

func TestRestoreManager_RunningLockTaken_FailsDeterministically(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "restore-running-lock-taken")
	cluster.Spec.TLS.Enabled = false
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
	})

	restoreObj := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-running-lock-taken",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: cluster.Name,
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.enc",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://minio.svc",
					Bucket:   "backups",
				},
			},
			Image:       "openbao-backup:dev",
			JWTAuthRole: "restore-role",
		},
	}
	if err := k8sClient.Create(ctx, restoreObj); err != nil {
		t.Fatalf("create OpenBaoRestore: %v", err)
	}

	mgr := restore.NewManager(k8sClient, k8sScheme, nil, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")
	latest := &openbaov1alpha1.OpenBaoRestore{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	if _, err := mgr.Reconcile(ctx, logr.Discard(), latest); err != nil {
		t.Fatalf("reconcile pending: %v", err)
	}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore for validating: %v", err)
	}
	if _, err := mgr.Reconcile(ctx, logr.Discard(), latest); err != nil {
		t.Fatalf("reconcile validating: %v", err)
	}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore for running: %v", err)
	}
	if latest.Status.Phase != openbaov1alpha1.RestorePhaseRunning {
		t.Fatalf("expected restore phase Running before lock-steal simulation, got %s", latest.Status.Phase)
	}

	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.OperationLock = &openbaov1alpha1.OperationLockStatus{
			Operation: openbaov1alpha1.ClusterOperationUpgrade,
			Holder:    constants.ControllerNameOpenBaoCluster + "/upgrade",
			Message:   "upgrade operation",
		}
	})

	if _, err := mgr.Reconcile(ctx, logr.Discard(), latest); err != nil {
		t.Fatalf("reconcile running with stolen lock: %v", err)
	}

	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get updated restore: %v", err)
	}
	if latest.Status.Phase != openbaov1alpha1.RestorePhaseFailed {
		t.Fatalf("expected restore to fail when lock is stolen, got phase %s", latest.Status.Phase)
	}
	expected := "cluster operation lock was taken by another operation while restore was running"
	if latest.Status.Message != expected {
		t.Fatalf("expected failure message %q, got %q", expected, latest.Status.Message)
	}
}

func TestRestoreManager_FailedJob_RemainsTerminalAcrossReconcileRetries(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "restore-terminal-failure")
	cluster.Spec.TLS.Enabled = false
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
	})

	restoreObj := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-terminal-failure",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: cluster.Name,
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup.enc",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "http://minio.svc",
					Bucket:   "backups",
				},
			},
			Image:       "openbao-backup:dev",
			JWTAuthRole: "restore-role",
		},
	}
	if err := k8sClient.Create(ctx, restoreObj); err != nil {
		t.Fatalf("create OpenBaoRestore: %v", err)
	}

	mgr := restore.NewManager(k8sClient, k8sScheme, nil, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")
	latest := &openbaov1alpha1.OpenBaoRestore{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	if _, err := mgr.Reconcile(ctx, logr.Discard(), latest); err != nil {
		t.Fatalf("reconcile pending: %v", err)
	}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore for validating: %v", err)
	}
	if _, err := mgr.Reconcile(ctx, logr.Discard(), latest); err != nil {
		t.Fatalf("reconcile validating: %v", err)
	}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get restore for running: %v", err)
	}
	if latest.Status.Phase != openbaov1alpha1.RestorePhaseRunning {
		t.Fatalf("expected restore phase Running before failed job processing, got %s", latest.Status.Phase)
	}

	createRestoreJobWithStatus(t, namespace, restore.RestoreJobNamePrefix+restoreObj.Name, 0, 1)

	if _, err := mgr.Reconcile(ctx, logr.Discard(), latest); err != nil {
		t.Fatalf("reconcile running with failed job: %v", err)
	}

	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get failed restore: %v", err)
	}
	if latest.Status.Phase != openbaov1alpha1.RestorePhaseFailed {
		t.Fatalf("expected restore phase Failed after failed job, got %s", latest.Status.Phase)
	}
	if latest.Status.Message != "Restore job failed" {
		t.Fatalf("expected default failed-job message, got %q", latest.Status.Message)
	}
	if latest.Status.CompletionTime == nil {
		t.Fatalf("expected completionTime to be set on terminal failure")
	}
	terminalMessage := latest.Status.Message

	if _, err := mgr.Reconcile(ctx, logr.Discard(), latest); err != nil {
		t.Fatalf("reconcile terminal restore: %v", err)
	}

	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreObj.Name}, latest); err != nil {
		t.Fatalf("get terminal restore after second reconcile: %v", err)
	}
	if latest.Status.Phase != openbaov1alpha1.RestorePhaseFailed {
		t.Fatalf("expected restore to remain Failed after terminal reconcile, got %s", latest.Status.Phase)
	}
	if latest.Status.Message != terminalMessage {
		t.Fatalf("expected terminal failure message to remain stable, got %q", latest.Status.Message)
	}
}

func createRestoreJobWithStatus(t *testing.T, namespace, name string, succeeded, failed int32) {
	t.Helper()

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "test",
							Image:   "busybox:1.36",
							Command: []string{"sh", "-c", "true"},
						},
					},
				},
			},
		},
	}
	if err := k8sClient.Create(ctx, job); err != nil {
		t.Fatalf("create restore job %q: %v", name, err)
	}

	latest := &batchv1.Job{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, latest); err != nil {
		t.Fatalf("get restore job %q: %v", name, err)
	}

	latest.Status.Succeeded = succeeded
	latest.Status.Failed = failed
	now := metav1.Now()
	latest.Status.StartTime = &now
	if err := k8sClient.Status().Update(ctx, latest); err != nil {
		t.Fatalf("update restore job status %q: %v", name, err)
	}
}
