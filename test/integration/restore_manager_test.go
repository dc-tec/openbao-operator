//go:build integration
// +build integration

package integration

import (
	"testing"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
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
					Endpoint: "http://minio",
					Bucket:   "backups",
				},
			},
			ExecutorImage: "openbao/backup-executor:dev",
			JWTAuthRole:   "restore-role", // Required for auth validation
		},
	}
	if err := k8sClient.Create(ctx, restoreObj); err != nil {
		t.Fatalf("create OpenBaoRestore: %v", err)
	}

	mgr := restore.NewManager(k8sClient, k8sScheme, nil, security.NewImageVerifier(logr.Discard(), k8sClient, nil))

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
