package provisioner

import (
	"context"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appprovisioner "github.com/dc-tec/openbao-operator/internal/app/provisioner"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/service/provisioner"
)

func expectEventContains(t *testing.T, recorder *events.FakeRecorder, parts ...string) {
	t.Helper()

	select {
	case event := <-recorder.Events:
		for _, part := range parts {
			if !strings.Contains(event, part) {
				t.Fatalf("event %q does not contain %q", event, part)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("expected event, got none")
	}
}

func newProvisionerManager(t *testing.T, k8sClient client.Client) appprovisioner.Provisioner {
	t.Helper()

	manager, err := appprovisioner.NewProvisioner(appprovisioner.ProvisionerDependencies{
		Client: k8sClient,
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("failed to create provisioner manager: %v", err)
	}
	return manager
}

func TestTenantSecretsRBACReconcile_NilProvisioner(t *testing.T) {
	setAdmissionReady(t)

	reconciler := &TenantSecretsRBACReconciler{}
	_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "tenant-a",
			Name:      "cluster-a",
		},
	})
	if err == nil {
		t.Fatal("expected reconcile to fail when provisioner manager is nil")
	}
}

func TestTenantSecretsRBACReconcile_AdmissionDependenciesNotReady(t *testing.T) {
	admission.SetAdmissionDependenciesReady(false)
	t.Cleanup(func() {
		admission.SetAdmissionDependenciesReady(false)
	})
	t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "")

	ctx := context.Background()
	k8sClient := newTestClient(t)
	reconciler := &TenantSecretsRBACReconciler{
		Client:      k8sClient,
		APIReader:   k8sClient,
		Scheme:      testScheme,
		Provisioner: newProvisionerManager(t, k8sClient),
	}

	result, err := reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "tenant-a",
			Name:      "cluster-a",
		},
	})
	if err != nil {
		t.Fatalf("reconcile error = %v", err)
	}
	if result.RequeueAfter != 10*time.Second {
		t.Fatalf("requeueAfter = %v, want %v", result.RequeueAfter, 10*time.Second)
	}
}

func TestTenantSecretsRBACReconcile_UnprovisionedNamespace(t *testing.T) {
	setAdmissionReady(t)

	ctx := context.Background()
	k8sClient := newTestClient(t)
	reconciler := &TenantSecretsRBACReconciler{
		Client:      k8sClient,
		APIReader:   k8sClient,
		Scheme:      testScheme,
		Provisioner: newProvisionerManager(t, k8sClient),
	}

	result, err := reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "tenant-a",
			Name:      "cluster-a",
		},
	})
	if err != nil {
		t.Fatalf("reconcile error = %v", err)
	}
	if result.RequeueAfter != 5*time.Second {
		t.Fatalf("requeueAfter = %v, want %v", result.RequeueAfter, 5*time.Second)
	}
}

func TestTenantSecretsRBACReconcile_ProvisionedNamespaceSyncsAllowlists(t *testing.T) {
	setAdmissionReady(t)

	ctx := context.Background()
	provisionedBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provisioner.TenantRoleBindingName,
			Namespace: "tenant-a",
		},
	}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-a",
			Namespace: "tenant-a",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				Schedule: "0 3 * * *",
				Target: openbaov1alpha1.BackupTarget{
					Bucket:               "backups",
					CredentialsSecretRef: &corev1.LocalObjectReference{Name: "backup-creds"},
				},
				TokenSecretRef: &corev1.LocalObjectReference{Name: "backup-token"},
			},
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				JWTAuthRole: "upgrade-role",
			},
			ImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled: true,
				ImagePullSecrets: []corev1.LocalObjectReference{
					{Name: "main-registry-creds"},
				},
			},
			OperatorImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled: true,
				ImagePullSecrets: []corev1.LocalObjectReference{
					{Name: "helper-registry-creds"},
				},
			},
			Unseal: &openbaov1alpha1.UnsealConfig{
				CredentialsSecretRef: &corev1.LocalObjectReference{Name: "unseal-creds"},
			},
		},
	}
	k8sClient := newTestClient(t, provisionedBinding, cluster)
	recorder := events.NewFakeRecorder(10)
	reconciler := &TenantSecretsRBACReconciler{
		Client:      k8sClient,
		APIReader:   k8sClient,
		Scheme:      testScheme,
		Recorder:    recorder,
		Provisioner: newProvisionerManager(t, k8sClient),
	}

	result, err := reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "tenant-a",
			Name:      "cluster-a",
		},
	})
	if err != nil {
		t.Fatalf("reconcile error = %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("requeueAfter = %v, want zero", result.RequeueAfter)
	}

	writerRole := &rbacv1.Role{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "tenant-a",
		Name:      provisioner.TenantSecretsWriterRoleName,
	}, writerRole); err != nil {
		t.Fatalf("expected writer role: %v", err)
	}
	wantWriterSecrets := []string{"cluster-a-root-token", "cluster-a-tls-ca", "cluster-a-tls-server", "cluster-a-unseal-key"}
	sort.Strings(wantWriterSecrets)
	if gotWriterSecrets := extractSecretResourceNames(writerRole.Rules); !slices.Equal(gotWriterSecrets, wantWriterSecrets) {
		t.Fatalf("writer secrets = %v, want %v", gotWriterSecrets, wantWriterSecrets)
	}

	readerRole := &rbacv1.Role{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "tenant-a",
		Name:      provisioner.TenantSecretsReaderRoleName,
	}, readerRole); err != nil {
		t.Fatalf("expected reader role: %v", err)
	}
	wantReaderSecrets := []string{"backup-creds", "backup-token", "helper-registry-creds", "main-registry-creds", "unseal-creds"}
	sort.Strings(wantReaderSecrets)
	if gotReaderSecrets := extractSecretResourceNames(readerRole.Rules); !slices.Equal(gotReaderSecrets, wantReaderSecrets) {
		t.Fatalf("reader secrets = %v, want %v", gotReaderSecrets, wantReaderSecrets)
	}

	writerBinding := &rbacv1.RoleBinding{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "tenant-a",
		Name:      provisioner.TenantSecretsWriterRoleBindingName,
	}, writerBinding); err != nil {
		t.Fatalf("expected writer rolebinding: %v", err)
	}
	readerBinding := &rbacv1.RoleBinding{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "tenant-a",
		Name:      provisioner.TenantSecretsReaderRoleBindingName,
	}, readerBinding); err != nil {
		t.Fatalf("expected reader rolebinding: %v", err)
	}

	expectEventContains(t, recorder, "Normal", ReasonTenantSecretRBACSynchronized)
}

func TestTenantSecretsRBACReconcile_RemovesStaleSecretRBAC(t *testing.T) {
	setAdmissionReady(t)

	ctx := context.Background()
	provisionedBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provisioner.TenantRoleBindingName,
			Namespace: "tenant-a",
		},
	}
	readerRole := provisioner.GenerateTenantSecretsReaderRole("tenant-a", []string{"stale-reader"})
	writerRole := provisioner.GenerateTenantSecretsWriterRole("tenant-a", []string{"stale-writer"})
	readerBinding := provisioner.GenerateTenantSecretsReaderRoleBinding("tenant-a", provisioner.OperatorServiceAccount{
		Name:      "openbao-operator-controller",
		Namespace: "openbao-operator-system",
	})
	writerBinding := provisioner.GenerateTenantSecretsWriterRoleBinding("tenant-a", provisioner.OperatorServiceAccount{
		Name:      "openbao-operator-controller",
		Namespace: "openbao-operator-system",
	})
	k8sClient := newTestClient(t, provisionedBinding, readerRole, writerRole, readerBinding, writerBinding)
	reconciler := &TenantSecretsRBACReconciler{
		Client:      k8sClient,
		APIReader:   k8sClient,
		Scheme:      testScheme,
		Provisioner: newProvisionerManager(t, k8sClient),
	}

	result, err := reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "tenant-a",
			Name:      "cluster-a",
		},
	})
	if err != nil {
		t.Fatalf("reconcile error = %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("requeueAfter = %v, want zero", result.RequeueAfter)
	}

	for _, key := range []types.NamespacedName{
		{Namespace: "tenant-a", Name: provisioner.TenantSecretsReaderRoleName},
		{Namespace: "tenant-a", Name: provisioner.TenantSecretsWriterRoleName},
	} {
		role := &rbacv1.Role{}
		if err := k8sClient.Get(ctx, key, role); err == nil {
			t.Fatalf("expected role %s/%s to be deleted", key.Namespace, key.Name)
		}
	}
	for _, key := range []types.NamespacedName{
		{Namespace: "tenant-a", Name: provisioner.TenantSecretsReaderRoleBindingName},
		{Namespace: "tenant-a", Name: provisioner.TenantSecretsWriterRoleBindingName},
	} {
		roleBinding := &rbacv1.RoleBinding{}
		if err := k8sClient.Get(ctx, key, roleBinding); err == nil {
			t.Fatalf("expected rolebinding %s/%s to be deleted", key.Namespace, key.Name)
		}
	}
}

func TestTenantSecretsRBACReconcile_UnsafeAdmissionDisabledBypassesDependencyCheck(t *testing.T) {
	admission.SetAdmissionDependenciesReady(false)
	t.Cleanup(func() {
		admission.SetAdmissionDependenciesReady(false)
	})
	t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "true")

	ctx := context.Background()
	provisionedBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provisioner.TenantRoleBindingName,
			Namespace: "tenant-a",
		},
	}
	k8sClient := newTestClient(t, provisionedBinding)
	reconciler := &TenantSecretsRBACReconciler{
		Client:      k8sClient,
		APIReader:   k8sClient,
		Scheme:      testScheme,
		Provisioner: newProvisionerManager(t, k8sClient),
	}

	result, err := reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "tenant-a",
			Name:      "cluster-a",
		},
	})
	if err != nil {
		t.Fatalf("reconcile error = %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("requeueAfter = %v, want zero", result.RequeueAfter)
	}
}

func extractSecretResourceNames(rules []rbacv1.PolicyRule) []string {
	var out []string
	for i := range rules {
		rule := rules[i]
		if !slices.Contains(rule.Resources, "secrets") {
			continue
		}
		if len(rule.ResourceNames) == 0 {
			continue
		}
		out = append(out, rule.ResourceNames...)
	}
	sort.Strings(out)
	return out
}
