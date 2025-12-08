package backup

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

var testScheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)
	return scheme
}()

func newTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	builder := fake.NewClientBuilder().WithScheme(testScheme)
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

func newTestClusterWithBackup(name, namespace string) *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			Backup: &openbaov1alpha1.BackupSchedule{
				ExecutorImage: "openbao/backup-executor:v0.1.0",
				Schedule:      "0 3 * * *",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint:     "https://s3.amazonaws.com",
					Bucket:       "test-bucket",
					PathPrefix:   "backups",
					UsePathStyle: false,
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Backup: &openbaov1alpha1.BackupStatus{},
		},
	}
}

func TestBackupJobName(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	jobName := backupJobName(cluster)

	if !strings.HasPrefix(jobName, "backup-test-cluster-") {
		t.Errorf("backupJobName() = %v, want prefix 'backup-test-cluster-'", jobName)
	}

	// Should contain timestamp
	if len(jobName) <= len("backup-test-cluster-") {
		t.Error("backupJobName() should include timestamp")
	}
}

func TestGetBackupExecutorImage(t *testing.T) {
	tests := []struct {
		name      string
		cluster   *openbaov1alpha1.OpenBaoCluster
		want      string
		wantEmpty bool
	}{
		{
			name: "with executor image",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{
						ExecutorImage: "openbao/backup-executor:v0.1.0",
						Schedule:      "0 3 * * *",
						Target: openbaov1alpha1.BackupTarget{
							Endpoint: "https://s3.amazonaws.com",
							Bucket:   "test-bucket",
						},
					},
				},
			},
			want:      "openbao/backup-executor:v0.1.0",
			wantEmpty: false,
		},
		{
			name: "with empty executor image",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{
						ExecutorImage: "",
						Schedule:      "0 3 * * *",
						Target: openbaov1alpha1.BackupTarget{
							Endpoint: "https://s3.amazonaws.com",
							Bucket:   "test-bucket",
						},
					},
				},
			},
			want:      "",
			wantEmpty: true,
		},
		{
			name: "with whitespace executor image",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{
						ExecutorImage: "   ",
						Schedule:      "0 3 * * *",
						Target: openbaov1alpha1.BackupTarget{
							Endpoint: "https://s3.amazonaws.com",
							Bucket:   "test-bucket",
						},
					},
				},
			},
			want:      "",
			wantEmpty: true,
		},
		{
			name: "without backup config",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			want:      "",
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getBackupExecutorImage(tt.cluster)
			if (got == "") != tt.wantEmpty {
				t.Errorf("getBackupExecutorImage() empty = %v, wantEmpty %v", got == "", tt.wantEmpty)
			}
			if !tt.wantEmpty && got != tt.want {
				t.Errorf("getBackupExecutorImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildBackupJob(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	jobName := "backup-test-cluster-20250101-120000"

	job, err := buildBackupJob(cluster, jobName)
	if err != nil {
		t.Fatalf("buildBackupJob() error = %v", err)
	}

	if job == nil {
		t.Fatal("buildBackupJob() returned nil")
	}

	if job.Name != jobName {
		t.Errorf("buildBackupJob() job.Name = %v, want %v", job.Name, jobName)
	}

	if job.Namespace != cluster.Namespace {
		t.Errorf("buildBackupJob() job.Namespace = %v, want %v", job.Namespace, cluster.Namespace)
	}

	// Verify labels
	expectedLabels := map[string]string{
		"app.kubernetes.io/name":       "openbao",
		"app.kubernetes.io/instance":   cluster.Name,
		"app.kubernetes.io/managed-by": "openbao-operator",
		"openbao.org/cluster":          cluster.Name,
		"openbao.org/component":        "backup",
	}
	for k, v := range expectedLabels {
		if job.Labels[k] != v {
			t.Errorf("buildBackupJob() label[%s] = %v, want %v", k, job.Labels[k], v)
		}
	}

	// Verify Job spec
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 0 {
		t.Error("buildBackupJob() BackoffLimit should be 0")
	}

	if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished != backupJobTTLSeconds {
		t.Errorf("buildBackupJob() TTLSecondsAfterFinished = %v, want %v", job.Spec.TTLSecondsAfterFinished, backupJobTTLSeconds)
	}

	// Verify container
	if len(job.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("buildBackupJob() containers count = %v, want 1", len(job.Spec.Template.Spec.Containers))
	}

	container := job.Spec.Template.Spec.Containers[0]
	if container.Name != "backup" {
		t.Errorf("buildBackupJob() container.Name = %v, want backup", container.Name)
	}

	if container.Image != cluster.Spec.Backup.ExecutorImage {
		t.Errorf("buildBackupJob() container.Image = %v, want %v", container.Image, cluster.Spec.Backup.ExecutorImage)
	}

	// Verify environment variables
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	expectedEnv := map[string]string{
		"CLUSTER_NAMESPACE":     cluster.Namespace,
		"CLUSTER_NAME":          cluster.Name,
		"CLUSTER_REPLICAS":      "3",
		"BACKUP_ENDPOINT":       cluster.Spec.Backup.Target.Endpoint,
		"BACKUP_BUCKET":         cluster.Spec.Backup.Target.Bucket,
		"BACKUP_PATH_PREFIX":    cluster.Spec.Backup.Target.PathPrefix,
		"BACKUP_USE_PATH_STYLE": "false",
	}

	for k, v := range expectedEnv {
		if envMap[k] != v {
			t.Errorf("buildBackupJob() env[%s] = %v, want %v", k, envMap[k], v)
		}
	}

	// Verify security context
	securityContext := job.Spec.Template.Spec.SecurityContext
	if securityContext == nil {
		t.Fatal("buildBackupJob() SecurityContext should be set")
	}

	if securityContext.RunAsNonRoot == nil || !*securityContext.RunAsNonRoot {
		t.Error("buildBackupJob() RunAsNonRoot should be true")
	}

	if securityContext.RunAsUser == nil || *securityContext.RunAsUser != backupUserID {
		t.Errorf("buildBackupJob() RunAsUser = %v, want %v", securityContext.RunAsUser, backupUserID)
	}

	if securityContext.RunAsGroup == nil || *securityContext.RunAsGroup != backupGroupID {
		t.Errorf("buildBackupJob() RunAsGroup = %v, want %v", securityContext.RunAsGroup, backupGroupID)
	}
}

func TestBuildBackupJob_WithCredentialsSecret(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	cluster.Spec.Backup.Target.CredentialsSecretRef = &corev1.SecretReference{
		Name:      "backup-credentials",
		Namespace: "default",
	}

	jobName := "backup-test-cluster-20250101-120000"
	job, err := buildBackupJob(cluster, jobName)
	if err != nil {
		t.Fatalf("buildBackupJob() error = %v", err)
	}

	container := job.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	if envMap["BACKUP_CREDENTIALS_SECRET_NAME"] != "backup-credentials" {
		t.Errorf("buildBackupJob() BACKUP_CREDENTIALS_SECRET_NAME = %v, want backup-credentials", envMap["BACKUP_CREDENTIALS_SECRET_NAME"])
	}

	// Verify volume mount
	hasCredentialsMount := false
	for _, mount := range container.VolumeMounts {
		if mount.Name == "backup-credentials" {
			hasCredentialsMount = true
			if mount.MountPath != "/etc/bao/backup/credentials" {
				t.Errorf("buildBackupJob() credentials mount path = %v, want /etc/bao/backup/credentials", mount.MountPath)
			}
		}
	}
	if !hasCredentialsMount {
		t.Error("buildBackupJob() should have backup-credentials volume mount")
	}

	// Verify volume
	hasCredentialsVolume := false
	for _, volume := range job.Spec.Template.Spec.Volumes {
		if volume.Name == "backup-credentials" {
			hasCredentialsVolume = true
			if volume.Secret.SecretName != "backup-credentials" {
				t.Errorf("buildBackupJob() credentials volume secret name = %v, want backup-credentials", volume.Secret.SecretName)
			}
			if volume.Secret.DefaultMode == nil || *volume.Secret.DefaultMode != 0400 {
				t.Errorf("buildBackupJob() credentials volume mode = %v, want 0400", volume.Secret.DefaultMode)
			}
		}
	}
	if !hasCredentialsVolume {
		t.Error("buildBackupJob() should have backup-credentials volume")
	}
}

func TestBuildBackupJob_WithJWTAuth(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	cluster.Spec.Backup.JWTAuthRole = "backup-role"

	jobName := "backup-test-cluster-20250101-120000"
	job, err := buildBackupJob(cluster, jobName)
	if err != nil {
		t.Fatalf("buildBackupJob() error = %v", err)
	}

	container := job.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	if envMap["BACKUP_JWT_AUTH_ROLE"] != "backup-role" {
		t.Errorf("buildBackupJob() BACKUP_JWT_AUTH_ROLE = %v, want backup-role", envMap["BACKUP_JWT_AUTH_ROLE"])
	}

	if envMap["BACKUP_AUTH_METHOD"] != "jwt" {
		t.Errorf("buildBackupJob() BACKUP_AUTH_METHOD = %v, want jwt", envMap["BACKUP_AUTH_METHOD"])
	}

	// Verify projected volume is mounted for JWT token
	volumeMounts := make(map[string]bool)
	for _, mount := range container.VolumeMounts {
		volumeMounts[mount.Name] = true
	}
	if !volumeMounts["openbao-token"] {
		t.Error("buildBackupJob() expected openbao-token volume mount for JWT auth")
	}

	// Verify projected volume exists
	volumes := make(map[string]bool)
	for _, vol := range job.Spec.Template.Spec.Volumes {
		volumes[vol.Name] = true
		if vol.Name == "openbao-token" {
			if vol.Projected == nil {
				t.Error("buildBackupJob() expected openbao-token volume to be projected")
			} else if len(vol.Projected.Sources) == 0 {
				t.Error("buildBackupJob() expected openbao-token projected volume to have sources")
			} else {
				sat := vol.Projected.Sources[0].ServiceAccountToken
				if sat == nil {
					t.Error("buildBackupJob() expected openbao-token projected volume to have ServiceAccountToken source")
				} else {
					if sat.Path != "openbao-token" {
						t.Errorf("buildBackupJob() expected ServiceAccountToken path to be 'openbao-token', got %q", sat.Path)
					}
					if sat.Audience != "openbao-internal" {
						t.Errorf("buildBackupJob() expected ServiceAccountToken audience to be 'openbao-internal', got %q", sat.Audience)
					}
				}
			}
		}
	}
	if !volumes["openbao-token"] {
		t.Error("buildBackupJob() expected openbao-token projected volume for JWT auth")
	}
}

func TestBuildBackupJob_WithTokenSecret(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	cluster.Spec.Backup.TokenSecretRef = &corev1.SecretReference{
		Name:      "backup-token",
		Namespace: "default",
	}

	jobName := "backup-test-cluster-20250101-120000"
	job, err := buildBackupJob(cluster, jobName)
	if err != nil {
		t.Fatalf("buildBackupJob() error = %v", err)
	}

	container := job.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	if envMap["BACKUP_TOKEN_SECRET_NAME"] != "backup-token" {
		t.Errorf("buildBackupJob() BACKUP_TOKEN_SECRET_NAME = %v, want backup-token", envMap["BACKUP_TOKEN_SECRET_NAME"])
	}

	if envMap["BACKUP_AUTH_METHOD"] != "token" {
		t.Errorf("buildBackupJob() BACKUP_AUTH_METHOD = %v, want token", envMap["BACKUP_AUTH_METHOD"])
	}

	// Verify volume mount
	hasTokenMount := false
	for _, mount := range container.VolumeMounts {
		if mount.Name == "backup-token" {
			hasTokenMount = true
		}
	}
	if !hasTokenMount {
		t.Error("buildBackupJob() should have backup-token volume mount")
	}
}

func TestBuildBackupJob_MissingExecutorImage(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	cluster.Spec.Backup.ExecutorImage = ""

	jobName := "backup-test-cluster-20250101-120000"
	job, err := buildBackupJob(cluster, jobName)

	if err == nil {
		t.Error("buildBackupJob() with missing executor image should return error")
	}

	if job != nil {
		t.Error("buildBackupJob() with missing executor image should return nil job")
	}

	if !strings.Contains(err.Error(), "backup executor image is required") {
		t.Errorf("buildBackupJob() error = %v, want error about missing executor image", err)
	}
}

func TestEnsureBackupJob_CreatesJob(t *testing.T) {
	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t)
	manager := NewManager(client, testScheme)

	cluster := newTestClusterWithBackup("test-cluster", "default")

	created, err := manager.ensureBackupJob(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("ensureBackupJob() error = %v", err)
	}

	if !created {
		t.Error("ensureBackupJob() should return true when creating job")
	}

	// Verify Job was created
	jobName := backupJobName(cluster)
	job := &batchv1.Job{}
	err = client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      jobName,
	}, job)

	if err != nil {
		t.Fatalf("expected Job to exist: %v", err)
	}
}

func TestEnsureBackupJob_JobAlreadyRunning(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	jobName := backupJobName(cluster)

	runningJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Status: batchv1.JobStatus{
			Active: 1,
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t, runningJob)
	manager := NewManager(client, testScheme)

	created, err := manager.ensureBackupJob(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("ensureBackupJob() error = %v", err)
	}

	if !created {
		t.Error("ensureBackupJob() should return true when job is running")
	}
}

func TestEnsureBackupJob_JobCompleted(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	jobName := backupJobName(cluster)

	completedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Status: batchv1.JobStatus{
			Succeeded: 1,
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t, completedJob)
	manager := NewManager(client, testScheme)

	created, err := manager.ensureBackupJob(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("ensureBackupJob() error = %v", err)
	}

	if created {
		t.Error("ensureBackupJob() should return false when job is completed")
	}
}

func TestEnsureBackupJob_JobFailed(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	jobName := backupJobName(cluster)

	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Status: batchv1.JobStatus{
			Failed: 1,
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t, failedJob)
	manager := NewManager(client, testScheme)

	created, err := manager.ensureBackupJob(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("ensureBackupJob() error = %v", err)
	}

	if created {
		t.Error("ensureBackupJob() should return false when job failed")
	}
}

func TestProcessBackupJobResult_JobSucceeded(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	jobName := backupJobName(cluster)

	succeededJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Status: batchv1.JobStatus{
			Succeeded: 1,
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t, succeededJob)
	manager := NewManager(client, testScheme)

	err := manager.processBackupJobResult(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("processBackupJobResult() error = %v", err)
	}

	// Verify status was updated
	if cluster.Status.Backup.LastBackupTime == nil {
		t.Error("processBackupJobResult() should set LastBackupTime")
	}

	if cluster.Status.Backup.ConsecutiveFailures != 0 {
		t.Errorf("processBackupJobResult() ConsecutiveFailures = %v, want 0", cluster.Status.Backup.ConsecutiveFailures)
	}

	if cluster.Status.Backup.LastFailureReason != "" {
		t.Errorf("processBackupJobResult() LastFailureReason = %v, want empty", cluster.Status.Backup.LastFailureReason)
	}
}

func TestProcessBackupJobResult_JobFailed(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	cluster.Status.Backup.ConsecutiveFailures = 0
	jobName := backupJobName(cluster)

	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Status: batchv1.JobStatus{
			Failed: 1,
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t, failedJob)
	manager := NewManager(client, testScheme)

	err := manager.processBackupJobResult(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("processBackupJobResult() error = %v", err)
	}

	// Verify status was updated
	if cluster.Status.Backup.ConsecutiveFailures != 1 {
		t.Errorf("processBackupJobResult() ConsecutiveFailures = %v, want 1", cluster.Status.Backup.ConsecutiveFailures)
	}

	if cluster.Status.Backup.LastFailureReason == "" {
		t.Error("processBackupJobResult() should set LastFailureReason")
	}
}

func TestProcessBackupJobResult_JobNotFound(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")

	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t)
	manager := NewManager(client, testScheme)

	// Should not error when job doesn't exist
	err := manager.processBackupJobResult(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("processBackupJobResult() with missing job should not error, got: %v", err)
	}
}

func TestProcessBackupJobResult_JobRunning(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	jobName := backupJobName(cluster)

	runningJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Status: batchv1.JobStatus{
			Active: 1,
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t, runningJob)
	manager := NewManager(client, testScheme)

	err := manager.processBackupJobResult(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("processBackupJobResult() error = %v", err)
	}

	// Status should indicate backup is in progress
	// (We can't easily test the condition without exposing setBackingUpCondition)
}
