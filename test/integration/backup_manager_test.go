//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/backup"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/security"
)

func TestBackupManager_ManualTrigger_CreatesJobAndWiring(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "backup-manager")
	cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
		Schedule: "0 0 * * *",
		Target: openbaov1alpha1.BackupTarget{
			Endpoint:   "https://minio.example",
			Bucket:     "backups",
			RoleARN:    "arn:aws:iam::123456789012:role/openbao-backup",
			Region:     "us-east-1",
			PathPrefix: "openbao",
		},
		JWTAuthRole:   "backup",
		ExecutorImage: "openbao/backup-executor:dev",
	}
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}

	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
		status.Phase = openbaov1alpha1.ClusterPhaseRunning
		status.CurrentVersion = cluster.Spec.Version
	})

	var latest openbaov1alpha1.OpenBaoCluster
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), &latest); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	original := latest.DeepCopy()
	if latest.Annotations == nil {
		latest.Annotations = map[string]string{}
	}
	latest.Annotations[constants.AnnotationTriggerBackup] = "true"
	if err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original)); err != nil {
		t.Fatalf("set trigger annotation: %v", err)
	}

	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), &latest); err != nil {
		t.Fatalf("get cluster after trigger: %v", err)
	}

	mgr := backup.NewManager(k8sClient, k8sScheme, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), k8sClient, nil))
	result, err := mgr.Reconcile(ctx, logr.Discard(), &latest)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("expected requeue while backup job is in progress")
	}

	// ServiceAccount + RBAC.
	saName := cluster.Name + constants.SuffixBackupServiceAccount
	sa := &corev1.ServiceAccount{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: saName}, sa); err != nil {
		t.Fatalf("expected backup ServiceAccount to exist: %v", err)
	}

	role := &rbacv1.Role{}
	roleName := saName + "-role"
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: roleName}, role); err != nil {
		t.Fatalf("expected backup Role to exist: %v", err)
	}

	rb := &rbacv1.RoleBinding{}
	rbName := saName + "-rolebinding"
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: rbName}, rb); err != nil {
		t.Fatalf("expected backup RoleBinding to exist: %v", err)
	}

	// Backup Job exists.
	var jobs batchv1.JobList
	if err := k8sClient.List(ctx, &jobs,
		client.InNamespace(namespace),
		client.MatchingLabels(map[string]string{
			constants.LabelOpenBaoCluster:   cluster.Name,
			constants.LabelOpenBaoComponent: backup.ComponentBackup,
		}),
	); err != nil {
		t.Fatalf("list backup jobs: %v", err)
	}
	if len(jobs.Items) != 1 {
		t.Fatalf("expected 1 backup job, got %d", len(jobs.Items))
	}
	job := &jobs.Items[0]
	if !strings.HasPrefix(job.Name, "backup-"+cluster.Name+"-") {
		t.Fatalf("unexpected job name %q", job.Name)
	}
	if job.Annotations["openbao.org/backup-key"] == "" {
		t.Fatalf("expected job to have openbao.org/backup-key annotation")
	}

	// Manual trigger annotation is cleared (best-effort).
	var after openbaov1alpha1.OpenBaoCluster
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), &after); err != nil {
		t.Fatalf("get cluster after reconcile: %v", err)
	}
	if after.Annotations != nil {
		if _, ok := after.Annotations[constants.AnnotationTriggerBackup]; ok {
			t.Fatalf("expected trigger annotation to be cleared")
		}
	}
}
