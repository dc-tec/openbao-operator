package backup

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/security"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

var testScheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)
	return scheme
}()

const testBackupJobName = "backup-test-cluster-20250101-120000"

func newTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	builder := fake.NewClientBuilder().WithScheme(testScheme)
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

//nolint:unparam // Keeping parameters makes tests easier to expand later.
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
	scheduled := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	jobName := backupJobName(cluster, scheduled)

	if !strings.HasPrefix(jobName, "backup-test-cluster-") {
		t.Errorf("backupJobName() = %v, want prefix 'backup-test-cluster-'", jobName)
	}

	// Should contain timestamp
	if len(jobName) <= len("backup-test-cluster-") {
		t.Error("backupJobName() should include timestamp")
	}
}

func TestGetBackupExecutorImage(t *testing.T) {
	// Set OPERATOR_VERSION env var for tests (otherwise default is ":latest")
	t.Setenv(constants.EnvOperatorVersion, "v1.0.0")

	tests := []struct {
		name        string
		cluster     *openbaov1alpha1.OpenBaoCluster
		want        string
		wantDefault bool // true if we expect the default image
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
			want:        "openbao/backup-executor:v0.1.0",
			wantDefault: false,
		},
		{
			name: "with empty executor image returns default",
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
			want:        constants.DefaultBackupImageRepository + ":v1.0.0",
			wantDefault: true,
		},
		{
			name: "with whitespace executor image returns default",
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
			want:        constants.DefaultBackupImageRepository + ":v1.0.0",
			wantDefault: true,
		},
		{
			name: "without backup config returns default",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			want:        constants.DefaultBackupImageRepository + ":v1.0.0",
			wantDefault: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetBackupExecutorImage(tt.cluster)
			if err != nil {
				t.Fatalf("GetBackupExecutorImage() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("GetBackupExecutorImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildBackupJob(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	jobName := testBackupJobName

	job, err := BuildJob(cluster, JobOptions{
		JobName:   jobName,
		JobType:   JobTypeScheduled,
		BackupKey: "test-key-12345",
		Platform:  "",
	})
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
		constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:      cluster.Name,
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:   cluster.Name,
		constants.LabelOpenBaoComponent: "backup",
	}
	for k, v := range expectedLabels {
		if job.Labels[k] != v {
			t.Errorf("buildBackupJob() label[%s] = %v, want %v", k, job.Labels[k], v)
		}
	}

	// Verify annotations
	if job.Annotations["openbao.org/backup-key"] != "test-key-12345" {
		t.Errorf("buildBackupJob() annotation[openbao.org/backup-key] = %v, want test-key-12345", job.Annotations["openbao.org/backup-key"])
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
		constants.EnvClusterNamespace:   cluster.Namespace,
		constants.EnvClusterName:        cluster.Name,
		constants.EnvClusterReplicas:    "3",
		constants.EnvBackupEndpoint:     cluster.Spec.Backup.Target.Endpoint,
		constants.EnvBackupBucket:       cluster.Spec.Backup.Target.Bucket,
		constants.EnvBackupPathPrefix:   cluster.Spec.Backup.Target.PathPrefix,
		constants.EnvBackupRegion:       "us-east-1",
		constants.EnvBackupUsePathStyle: "false",
		constants.EnvBackupKey:          "test-key-12345",
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

	if securityContext.RunAsUser == nil || *securityContext.RunAsUser != constants.UserBackup {
		t.Errorf("buildBackupJob() RunAsUser = %v, want %v", securityContext.RunAsUser, constants.UserBackup)
	}

	if securityContext.RunAsGroup == nil || *securityContext.RunAsGroup != constants.GroupBackup {
		t.Errorf("buildBackupJob() RunAsGroup = %v, want %v", securityContext.RunAsGroup, constants.GroupBackup)
	}
}

func TestBuildBackupJob_UsesVerifiedExecutorDigest(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	jobName := testBackupJobName
	verifiedDigest := "my-registry/backup:sha256:verified"

	job, err := BuildJob(cluster, JobOptions{
		JobName:                jobName,
		JobType:                JobTypeScheduled,
		BackupKey:              "test-key-12345",
		VerifiedExecutorDigest: verifiedDigest,
		Platform:               "",
	})
	if err != nil {
		t.Fatalf("buildBackupJob() error = %v", err)
	}

	if len(job.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("buildBackupJob() containers count = %v, want 1", len(job.Spec.Template.Spec.Containers))
	}

	container := job.Spec.Template.Spec.Containers[0]
	if container.Image != verifiedDigest {
		t.Errorf("buildBackupJob() container.Image = %v, want %v", container.Image, verifiedDigest)
	}
}

func TestBuildBackupJob_WithCredentialsSecret(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	cluster.Spec.Backup.Target.CredentialsSecretRef = &corev1.LocalObjectReference{
		Name: backupCredentialsVolumeName,
	}

	jobName := testBackupJobName
	job, err := BuildJob(cluster, JobOptions{
		JobName:   jobName,
		JobType:   JobTypeScheduled,
		BackupKey: "test-key",
		Platform:  "",
	})
	if err != nil {
		t.Fatalf("buildBackupJob() error = %v", err)
	}

	container := job.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	if envMap[constants.EnvBackupCredentialsSecretName] != backupCredentialsVolumeName {
		t.Errorf(
			"buildBackupJob() BACKUP_CREDENTIALS_SECRET_NAME = %v, want %s",
			envMap[constants.EnvBackupCredentialsSecretName],
			backupCredentialsVolumeName,
		)
	}

	// Verify volume mount
	hasCredentialsMount := false
	for _, mount := range container.VolumeMounts {
		if mount.Name == backupCredentialsVolumeName {
			hasCredentialsMount = true
			if mount.MountPath != constants.PathBackupCredentials {
				t.Errorf("buildBackupJob() credentials mount path = %v, want %s", mount.MountPath, constants.PathBackupCredentials)
			}
		}
	}
	if !hasCredentialsMount {
		t.Error("buildBackupJob() should have backup-credentials volume mount")
	}

	// Verify volume
	hasCredentialsVolume := false
	for _, volume := range job.Spec.Template.Spec.Volumes {
		if volume.Name == backupCredentialsVolumeName {
			hasCredentialsVolume = true
			if volume.Secret.SecretName != backupCredentialsVolumeName {
				t.Errorf(
					"buildBackupJob() credentials volume secret name = %v, want %s",
					volume.Secret.SecretName,
					backupCredentialsVolumeName,
				)
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

	jobName := testBackupJobName
	job, err := BuildJob(cluster, JobOptions{
		JobName:   jobName,
		JobType:   JobTypeScheduled,
		BackupKey: "test",
		Platform:  "",
	})
	if err != nil {
		t.Fatalf("buildBackupJob() error = %v", err)
	}

	container := job.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	if envMap[constants.EnvBackupJWTAuthRole] != "backup-role" {
		t.Errorf("buildBackupJob() BACKUP_JWT_AUTH_ROLE = %v, want backup-role", envMap[constants.EnvBackupJWTAuthRole])
	}

	if envMap[constants.EnvBackupAuthMethod] != constants.BackupAuthMethodJWT {
		t.Errorf("buildBackupJob() BACKUP_AUTH_METHOD = %v, want %s", envMap[constants.EnvBackupAuthMethod], constants.BackupAuthMethodJWT)
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

func TestBuildBackupJob_WithRoleARN(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	cluster.Spec.Backup.Target.RoleARN = "arn:aws:iam::123456789012:role/backup-role"

	jobName := testBackupJobName
	job, err := BuildJob(cluster, JobOptions{
		JobName:   jobName,
		JobType:   JobTypeScheduled,
		BackupKey: "test",
		Platform:  "",
	})
	if err != nil {
		t.Fatalf("buildBackupJob() error = %v", err)
	}

	container := job.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	if envMap[constants.EnvAWSRoleARN] != cluster.Spec.Backup.Target.RoleARN {
		t.Errorf("buildBackupJob() AWS_ROLE_ARN = %v, want %v", envMap[constants.EnvAWSRoleARN], cluster.Spec.Backup.Target.RoleARN)
	}

	if envMap[constants.EnvAWSWebIdentityTokenFile] != awsWebIdentityTokenFile {
		t.Errorf("buildBackupJob() AWS_WEB_IDENTITY_TOKEN_FILE = %v, want %s", envMap[constants.EnvAWSWebIdentityTokenFile], awsWebIdentityTokenFile)
	}

	hasTokenMount := false
	for _, mount := range container.VolumeMounts {
		if mount.Name == "aws-iam-token" {
			hasTokenMount = true
			if mount.MountPath != "/var/run/secrets/aws" {
				t.Errorf("buildBackupJob() aws token mount path = %v, want /var/run/secrets/aws", mount.MountPath)
			}
		}
	}
	if !hasTokenMount {
		t.Error("buildBackupJob() expected aws-iam-token volume mount when RoleARN is set")
	}

	hasTokenVolume := false
	for _, vol := range job.Spec.Template.Spec.Volumes {
		if vol.Name != "aws-iam-token" {
			continue
		}
		hasTokenVolume = true
		if vol.Projected == nil || len(vol.Projected.Sources) != 1 {
			t.Fatal("buildBackupJob() expected aws-iam-token projected volume with one source")
		}

		sat := vol.Projected.Sources[0].ServiceAccountToken
		if sat == nil {
			t.Fatal("buildBackupJob() expected aws-iam-token projected volume to have ServiceAccountToken source")
		}
		if sat.Path != "token" {
			t.Errorf("buildBackupJob() expected aws token path to be 'token', got %q", sat.Path)
		}
		if sat.Audience != "sts.amazonaws.com" {
			t.Errorf("buildBackupJob() expected aws token audience to be 'sts.amazonaws.com', got %q", sat.Audience)
		}
	}
	if !hasTokenVolume {
		t.Error("buildBackupJob() expected aws-iam-token projected volume when RoleARN is set")
	}
}

func TestBuildBackupJob_WithTokenSecret(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	cluster.Spec.Backup.TokenSecretRef = &corev1.LocalObjectReference{
		Name: "backup-token",
	}

	jobName := testBackupJobName
	job, err := BuildJob(cluster, JobOptions{
		JobName:   jobName,
		JobType:   JobTypeScheduled,
		BackupKey: "test",
		Platform:  "",
	})
	if err != nil {
		t.Fatalf("buildBackupJob() error = %v", err)
	}

	container := job.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	if envMap[constants.EnvBackupTokenSecretName] != "backup-token" {
		t.Errorf("buildBackupJob() BACKUP_TOKEN_SECRET_NAME = %v, want backup-token", envMap[constants.EnvBackupTokenSecretName])
	}

	if envMap[constants.EnvBackupAuthMethod] != constants.BackupAuthMethodToken {
		t.Errorf("buildBackupJob() BACKUP_AUTH_METHOD = %v, want %s", envMap[constants.EnvBackupAuthMethod], constants.BackupAuthMethodToken)
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

func TestBuildBackupJob_UsesDefaultExecutorImage(t *testing.T) {
	// Set OPERATOR_VERSION env var for tests
	t.Setenv(constants.EnvOperatorVersion, "v1.0.0")

	cluster := newTestClusterWithBackup("test-cluster", "default")
	cluster.Spec.Backup.ExecutorImage = "" // Empty - should use default

	jobName := testBackupJobName
	job, err := BuildJob(cluster, JobOptions{
		JobName:   jobName,
		JobType:   JobTypeScheduled,
		BackupKey: "test",
		Platform:  "",
	})

	if err != nil {
		t.Fatalf("buildBackupJob() unexpected error = %v", err)
	}

	if job == nil {
		t.Fatal("buildBackupJob() returned nil job")
	}

	container := job.Spec.Template.Spec.Containers[0]
	expectedImage := constants.DefaultBackupImageRepository + ":v1.0.0"
	if container.Image != expectedImage {
		t.Errorf("buildBackupJob() container.Image = %v, want %v", container.Image, expectedImage)
	}
}

func TestEnsureBackupJob_CreatesJob(t *testing.T) {
	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	cluster := newTestClusterWithBackup("test-cluster", "default")
	scheduled := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	jobName := backupJobName(cluster, scheduled)

	created, err := manager.ensureBackupJob(ctx, logger, cluster, jobName, scheduled)
	if err != nil {
		t.Fatalf("ensureBackupJob() error = %v", err)
	}

	if !created {
		t.Error("ensureBackupJob() should return true when creating job")
	}

	// Verify Job was created
	job := &batchv1.Job{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      jobName,
	}, job)

	if err != nil {
		t.Fatalf("expected Job to exist: %v", err)
	}
}

func TestEnsureBackupJob_JobAlreadyRunning(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	scheduled := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	jobName := backupJobName(cluster, scheduled)

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
	k8sClient := newTestClient(t, runningJob)
	manager := NewManager(k8sClient, testScheme, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	created, err := manager.ensureBackupJob(ctx, logger, cluster, jobName, scheduled)
	if err != nil {
		t.Fatalf("ensureBackupJob() error = %v", err)
	}

	if !created {
		t.Error("ensureBackupJob() should return true when job is running")
	}
}

func TestEnsureBackupJob_JobCompleted(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	scheduled := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	jobName := backupJobName(cluster, scheduled)

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
	k8sClient := newTestClient(t, completedJob)
	manager := NewManager(k8sClient, testScheme, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	created, err := manager.ensureBackupJob(ctx, logger, cluster, jobName, scheduled)
	if err != nil {
		t.Fatalf("ensureBackupJob() error = %v", err)
	}

	if created {
		t.Error("ensureBackupJob() should return false when job is completed")
	}
}

func TestEnsureBackupJob_JobFailed(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	scheduled := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	jobName := backupJobName(cluster, scheduled)

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
	k8sClient := newTestClient(t, failedJob)
	manager := NewManager(k8sClient, testScheme, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	created, err := manager.ensureBackupJob(ctx, logger, cluster, jobName, scheduled)
	if err != nil {
		t.Fatalf("ensureBackupJob() error = %v", err)
	}

	if created {
		t.Error("ensureBackupJob() should return false when job failed")
	}
}

func TestProcessBackupJobResult_JobSucceeded(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	scheduled := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	jobName := backupJobName(cluster, scheduled)

	succeededJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
			Annotations: map[string]string{
				"openbao.org/backup-key": "test-key-abc",
			},
		},
		Status: batchv1.JobStatus{
			Succeeded: 1,
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t, succeededJob)
	manager := NewManager(k8sClient, testScheme, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	statusUpdated, err := manager.processBackupJobResult(ctx, logger, cluster, jobName)
	if err != nil {
		t.Fatalf("processBackupJobResult() error = %v", err)
	}

	// Verify status was updated
	if !statusUpdated {
		t.Error("processBackupJobResult() should return true when job succeeded")
	}

	if cluster.Status.Backup.LastBackupTime == nil {
		t.Error("processBackupJobResult() should set LastBackupTime")
	}

	if cluster.Status.Backup.LastBackupName != "test-key-abc" {
		t.Errorf("processBackupJobResult() LastBackupName = %v, want test-key-abc", cluster.Status.Backup.LastBackupName)
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
	scheduled := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	jobName := backupJobName(cluster, scheduled)

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
	k8sClient := newTestClient(t, failedJob)
	manager := NewManager(k8sClient, testScheme, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	statusUpdated, err := manager.processBackupJobResult(ctx, logger, cluster, jobName)
	if err != nil {
		t.Fatalf("processBackupJobResult() error = %v", err)
	}

	// Verify status was updated
	if !statusUpdated {
		t.Error("processBackupJobResult() should return true when job failed")
	}

	if cluster.Status.Backup.ConsecutiveFailures != 1 {
		t.Errorf("processBackupJobResult() ConsecutiveFailures = %v, want 1", cluster.Status.Backup.ConsecutiveFailures)
	}

	if cluster.Status.Backup.LastFailureReason == "" {
		t.Error("processBackupJobResult() should set LastFailureReason")
	}
}

func TestProcessBackupJobResult_JobFailedIdempotent(t *testing.T) {
	// This test verifies that processBackupJobResult is idempotent for failed jobs.
	// Calling it multiple times for the same failed job should only increment
	// ConsecutiveFailures once, not on every call.
	cluster := newTestClusterWithBackup("test-cluster", "default")
	cluster.Status.Backup.ConsecutiveFailures = 0
	scheduled := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	jobName := backupJobName(cluster, scheduled)

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
	k8sClient := newTestClient(t, failedJob)
	manager := NewManager(k8sClient, testScheme, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	// First call should update status
	statusUpdated, err := manager.processBackupJobResult(ctx, logger, cluster, jobName)
	if err != nil {
		t.Fatalf("processBackupJobResult() first call error = %v", err)
	}
	if !statusUpdated {
		t.Error("processBackupJobResult() first call should return true")
	}
	if cluster.Status.Backup.ConsecutiveFailures != 1 {
		t.Errorf("processBackupJobResult() first call ConsecutiveFailures = %v, want 1", cluster.Status.Backup.ConsecutiveFailures)
	}

	// Second call should NOT update status (already processed)
	statusUpdated, err = manager.processBackupJobResult(ctx, logger, cluster, jobName)
	if err != nil {
		t.Fatalf("processBackupJobResult() second call error = %v", err)
	}
	if statusUpdated {
		t.Error("processBackupJobResult() second call should return false (already processed)")
	}
	// ConsecutiveFailures should still be 1, not 2
	if cluster.Status.Backup.ConsecutiveFailures != 1 {
		t.Errorf("processBackupJobResult() second call ConsecutiveFailures = %v, want 1 (idempotent)", cluster.Status.Backup.ConsecutiveFailures)
	}
}

func TestProcessBackupJobResult_JobNotFound(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	scheduled := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	jobName := backupJobName(cluster, scheduled)

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	// Should not error when job doesn't exist
	statusUpdated, err := manager.processBackupJobResult(ctx, logger, cluster, jobName)
	if err != nil {
		t.Fatalf("processBackupJobResult() with missing job should not error, got: %v", err)
	}

	if statusUpdated {
		t.Error("processBackupJobResult() should return false when job doesn't exist")
	}
}

func TestProcessBackupJobResult_JobRunning(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	scheduled := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	jobName := backupJobName(cluster, scheduled)

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
	k8sClient := newTestClient(t, runningJob)
	manager := NewManager(k8sClient, testScheme, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")
	manager.operatorImageVerifier = security.NewImageVerifier(log.Log.WithName("backup-image-verifier"), k8sClient, nil)

	statusUpdated, err := manager.processBackupJobResult(ctx, logger, cluster, jobName)
	if err != nil {
		t.Fatalf("processBackupJobResult() error = %v", err)
	}

	// Status should indicate backup is in progress
	// (We can't easily test the condition without exposing setBackingUpCondition)
	// Status was updated (condition set) but job is still running, so no requeue needed
	if statusUpdated {
		t.Error("processBackupJobResult() should return false when job is still running")
	}
}

func TestBuildBackupJob_SecurityContext(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	jobName := testBackupJobName

	tests := []struct {
		name     string
		platform string
		wantUser *int64
		wantGrp  *int64
		wantFS   *int64
	}{
		{
			name:     "kubernetes platform pins IDs",
			platform: constants.PlatformKubernetes,
			wantUser: ptr.To(constants.UserBackup),
			wantGrp:  ptr.To(constants.GroupBackup),
			wantFS:   ptr.To(constants.GroupBackup),
		},
		{
			name:     "openshift platform omits IDs",
			platform: constants.PlatformOpenShift,
			wantUser: nil,
			wantGrp:  nil,
			wantFS:   nil,
		},
		{
			name:     "empty platform defaults to pinning IDs",
			platform: "",
			wantUser: ptr.To(int64(constants.UserBackup)),
			wantGrp:  ptr.To(int64(constants.GroupBackup)),
			wantFS:   ptr.To(int64(constants.GroupBackup)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job, err := BuildJob(cluster, JobOptions{
				JobName:   jobName,
				JobType:   JobTypeScheduled,
				BackupKey: "test-key",
				Platform:  tt.platform,
			})
			if err != nil {
				t.Fatalf("BuildJob() error = %v", err)
			}

			sc := job.Spec.Template.Spec.SecurityContext
			if sc == nil {
				t.Fatal("SecurityContext is nil")
			}

			if !ptrInt64Equal(sc.RunAsUser, tt.wantUser) {
				t.Errorf("RunAsUser = %v, want %v", ptrInt64Value(sc.RunAsUser), ptrInt64Value(tt.wantUser))
			}
			if !ptrInt64Equal(sc.RunAsGroup, tt.wantGrp) {
				t.Errorf("RunAsGroup = %v, want %v", ptrInt64Value(sc.RunAsGroup), ptrInt64Value(tt.wantGrp))
			}
			if !ptrInt64Equal(sc.FSGroup, tt.wantFS) {
				t.Errorf("FSGroup = %v, want %v", ptrInt64Value(sc.FSGroup), ptrInt64Value(tt.wantFS))
			}
		})
	}
}

func ptrInt64Equal(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptrInt64Value(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}
