package provisioner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	provisionermanager "github.com/dc-tec/openbao-operator/internal/service/provisioner"
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

func newTenantScheme(t *testing.T) *k8sruntime.Scheme {
	t.Helper()
	s := k8sruntime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add openbao scheme: %v", err)
	}
	return s
}

func newTenantRuntime(t *testing.T, objs ...client.Object) TenantRuntime {
	t.Helper()
	builder := fake.NewClientBuilder().
		WithScheme(newTenantScheme(t)).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoTenant{})
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	c := builder.Build()
	mgr, err := provisionermanager.NewManager(context.Background(), c, logr.Discard())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return TenantRuntime{
		Client:            c,
		APIReader:         c,
		Provisioner:       mgr,
		OperatorNamespace: "openbao-operator-system",
	}
}

func setAdmissionReady(t *testing.T, ready bool) {
	t.Helper()
	admission.SetAdmissionDependenciesReady(ready)
	t.Cleanup(func() {
		admission.SetAdmissionDependenciesReady(false)
	})
}

func TestReconcileOpenBaoTenant_Validation(t *testing.T) {
	req := types.NamespacedName{Name: "tenant", Namespace: "ns"}
	if _, err := ReconcileOpenBaoTenant(context.Background(), req, logr.Discard(), TenantRuntime{}); err == nil || !strings.Contains(err.Error(), "client is required") {
		t.Fatalf("expected client required error, got %v", err)
	}
	if _, err := ReconcileOpenBaoTenant(context.Background(), req, logr.Discard(), TenantRuntime{Client: fake.NewClientBuilder().Build()}); err == nil || !strings.Contains(err.Error(), "provisioner manager is required") {
		t.Fatalf("expected provisioner required error, got %v", err)
	}
}

func TestReconcileOpenBaoTenant_SecurityViolation(t *testing.T) {
	setAdmissionReady(t, true)

	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant", Namespace: "team-a"},
		Spec:       openbaov1alpha1.OpenBaoTenantSpec{TargetNamespace: "team-b"},
	}
	runtime := newTenantRuntime(t, tenant)
	recorder := events.NewFakeRecorder(10)
	runtime.Recorder = recorder

	result, err := ReconcileOpenBaoTenant(context.Background(), types.NamespacedName{Name: "tenant", Namespace: "team-a"}, logr.Discard(), runtime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != (recon.Result{}) {
		t.Fatalf("result=%v, want zero", result)
	}

	updated := &openbaov1alpha1.OpenBaoTenant{}
	if getErr := runtime.Client.Get(context.Background(), types.NamespacedName{Name: "tenant", Namespace: "team-a"}, updated); getErr != nil {
		t.Fatalf("get updated tenant: %v", getErr)
	}
	if updated.Status.Provisioned {
		t.Fatalf("expected Provisioned=false")
	}
	if !strings.Contains(updated.Status.LastError, "security violation") {
		t.Fatalf("expected security violation message, got %q", updated.Status.LastError)
	}

	expectEventContains(t, recorder, "Warning", ReasonTenantProvisioningBlocked)
}

func TestReconcileOpenBaoTenant_FinalizerAndProvisioning(t *testing.T) {
	setAdmissionReady(t, true)

	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant", Namespace: "openbao-operator-system"},
		Spec:       openbaov1alpha1.OpenBaoTenantSpec{TargetNamespace: "tenant-ns"},
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tenant-ns"}}
	runtime := newTenantRuntime(t, tenant, ns)
	recorder := events.NewFakeRecorder(10)
	runtime.Recorder = recorder
	req := types.NamespacedName{Name: "tenant", Namespace: "openbao-operator-system"}

	result, err := ReconcileOpenBaoTenant(context.Background(), req, logr.Discard(), runtime)
	if err != nil {
		t.Fatalf("reconcile 1: %v", err)
	}
	if result.RequeueAfter != 5*time.Second {
		t.Fatalf("requeueAfter=%v, want 5s", result.RequeueAfter)
	}

	result, err = ReconcileOpenBaoTenant(context.Background(), req, logr.Discard(), runtime)
	if err != nil {
		t.Fatalf("reconcile 2: %v", err)
	}
	if result != (recon.Result{}) {
		t.Fatalf("result=%v, want zero", result)
	}

	updated := &openbaov1alpha1.OpenBaoTenant{}
	if getErr := runtime.Client.Get(context.Background(), req, updated); getErr != nil {
		t.Fatalf("get updated tenant: %v", getErr)
	}
	if !containsFinalizer(updated.Finalizers, openbaov1alpha1.OpenBaoTenantFinalizer) {
		t.Fatalf("expected finalizer to be present")
	}
	if !updated.Status.Provisioned || updated.Status.LastError != "" {
		t.Fatalf("expected successful provision status, got provisioned=%v lastError=%q", updated.Status.Provisioned, updated.Status.LastError)
	}

	expectEventContains(t, recorder, "Normal", ReasonTenantProvisioned)

	role := &rbacv1.Role{}
	if getErr := runtime.Client.Get(context.Background(), types.NamespacedName{Name: provisionermanager.TenantRoleName, Namespace: "tenant-ns"}, role); getErr != nil {
		t.Fatalf("expected tenant role: %v", getErr)
	}
	binding := &rbacv1.RoleBinding{}
	if getErr := runtime.Client.Get(context.Background(), types.NamespacedName{Name: provisionermanager.TenantRoleBindingName, Namespace: "tenant-ns"}, binding); getErr != nil {
		t.Fatalf("expected tenant rolebinding: %v", getErr)
	}
}

func TestReconcileOpenBaoTenant_TargetNamespaceMissing(t *testing.T) {
	setAdmissionReady(t, true)

	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "tenant",
			Namespace:  "openbao-operator-system",
			Finalizers: []string{openbaov1alpha1.OpenBaoTenantFinalizer},
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{TargetNamespace: "missing-ns"},
	}
	runtime := newTenantRuntime(t, tenant)
	req := types.NamespacedName{Name: "tenant", Namespace: "openbao-operator-system"}

	result, err := ReconcileOpenBaoTenant(context.Background(), req, logr.Discard(), runtime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != time.Minute {
		t.Fatalf("requeueAfter=%v, want 1m", result.RequeueAfter)
	}

	updated := &openbaov1alpha1.OpenBaoTenant{}
	if getErr := runtime.Client.Get(context.Background(), req, updated); getErr != nil {
		t.Fatalf("get updated tenant: %v", getErr)
	}
	if updated.Status.Provisioned {
		t.Fatalf("expected Provisioned=false")
	}
	if !strings.Contains(updated.Status.LastError, "target namespace missing-ns not found") {
		t.Fatalf("unexpected LastError: %q", updated.Status.LastError)
	}
}

func TestReconcileOpenBaoTenant_AdmissionNotReadyRequeues(t *testing.T) {
	setAdmissionReady(t, false)

	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "tenant",
			Namespace:  "openbao-operator-system",
			Finalizers: []string{openbaov1alpha1.OpenBaoTenantFinalizer},
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{TargetNamespace: "tenant-ns"},
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tenant-ns"}}
	runtime := newTenantRuntime(t, tenant, ns)
	req := types.NamespacedName{Name: "tenant", Namespace: "openbao-operator-system"}

	result, err := ReconcileOpenBaoTenant(context.Background(), req, logr.Discard(), runtime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != admissionDependencyRequeueAfter {
		t.Fatalf("requeueAfter=%v, want %v", result.RequeueAfter, admissionDependencyRequeueAfter)
	}

	updated := &openbaov1alpha1.OpenBaoTenant{}
	if getErr := runtime.Client.Get(context.Background(), req, updated); getErr != nil {
		t.Fatalf("get updated tenant: %v", getErr)
	}
	if updated.Status.Provisioned {
		t.Fatalf("expected Provisioned=false")
	}
	if updated.Status.LastError == "" {
		t.Fatalf("expected LastError to be set when admission dependencies are not ready")
	}
}

func TestReconcileOpenBaoTenant_DeletionPaths(t *testing.T) {
	setAdmissionReady(t, true)

	t.Run("requeues while clusters remain", func(t *testing.T) {
		now := metav1.Now()
		tenant := &openbaov1alpha1.OpenBaoTenant{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "tenant",
				Namespace:         "openbao-operator-system",
				Finalizers:        []string{openbaov1alpha1.OpenBaoTenantFinalizer},
				DeletionTimestamp: &now,
			},
			Spec: openbaov1alpha1.OpenBaoTenantSpec{TargetNamespace: "tenant-ns"},
		}
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tenant-ns"}}
		cluster := &openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "tenant-ns"}}
		runtime := newTenantRuntime(t, tenant, ns, cluster)

		result, err := ReconcileOpenBaoTenant(context.Background(), types.NamespacedName{Name: "tenant", Namespace: "openbao-operator-system"}, logr.Discard(), runtime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.RequeueAfter != 5*time.Second {
			t.Fatalf("requeueAfter=%v, want 5s", result.RequeueAfter)
		}
	})

	t.Run("removes finalizer after cleanup", func(t *testing.T) {
		now := metav1.Now()
		tenant := &openbaov1alpha1.OpenBaoTenant{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "tenant",
				Namespace:         "openbao-operator-system",
				Finalizers:        []string{openbaov1alpha1.OpenBaoTenantFinalizer},
				DeletionTimestamp: &now,
			},
			Spec: openbaov1alpha1.OpenBaoTenantSpec{TargetNamespace: "tenant-ns"},
		}
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tenant-ns"}}
		runtime := newTenantRuntime(t, tenant, ns)
		recorder := events.NewFakeRecorder(10)
		runtime.Recorder = recorder
		req := types.NamespacedName{Name: "tenant", Namespace: "openbao-operator-system"}

		result, err := ReconcileOpenBaoTenant(context.Background(), req, logr.Discard(), runtime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != (recon.Result{}) {
			t.Fatalf("result=%v, want zero", result)
		}

		updated := &openbaov1alpha1.OpenBaoTenant{}
		if getErr := runtime.Client.Get(context.Background(), req, updated); getErr != nil {
			if !apierrors.IsNotFound(getErr) {
				t.Fatalf("get updated tenant: %v", getErr)
			}
		} else if containsFinalizer(updated.Finalizers, openbaov1alpha1.OpenBaoTenantFinalizer) {
			t.Fatalf("expected finalizer to be removed")
		}

		expectEventContains(t, recorder, "Normal", ReasonTenantRBACCleaned)
	})
}

func TestFinalizerHelpers(t *testing.T) {
	if containsFinalizer([]string{"a", "b"}, "c") {
		t.Fatalf("containsFinalizer should be false for missing value")
	}
	if !containsFinalizer([]string{"a", "b"}, "a") {
		t.Fatalf("containsFinalizer should be true for present value")
	}
	remaining := removeFinalizer([]string{"a", "b", "a"}, "a")
	if len(remaining) != 1 || remaining[0] != "b" {
		t.Fatalf("removeFinalizer result=%v, want [b]", remaining)
	}
}
