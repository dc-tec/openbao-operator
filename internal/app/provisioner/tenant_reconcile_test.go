package provisioner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	provisionermanager "github.com/dc-tec/openbao-operator/internal/service/provisioner"
)

type failingTenantProvisioner struct {
	Provisioner
	err error
}

func (p failingTenantProvisioner) EnsureTenantRBAC(context.Context, *openbaov1alpha1.OpenBaoTenant) error {
	return p.err
}

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
	return newTenantRuntimeWithClient(t, builder.Build())
}

func newTenantRuntimeWithClient(t *testing.T, c client.Client) TenantRuntime {
	t.Helper()
	mgr, err := provisionermanager.NewManager(c, logr.Discard())
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

func requireProvisionedCondition(
	t *testing.T,
	tenant *openbaov1alpha1.OpenBaoTenant,
	wantStatus metav1.ConditionStatus,
	wantReason string,
	wantMessage string,
) {
	t.Helper()

	condition := meta.FindStatusCondition(tenant.Status.Conditions, constants.TenantProvisionedConditionType)
	if condition == nil {
		t.Fatal("expected Provisioned condition")
	}
	if condition.Status != wantStatus {
		t.Fatalf("Provisioned condition status=%q, want %q", condition.Status, wantStatus)
	}
	if condition.ObservedGeneration != tenant.Generation {
		t.Fatalf(
			"Provisioned condition observedGeneration=%d, want %d",
			condition.ObservedGeneration,
			tenant.Generation,
		)
	}
	if condition.Reason != wantReason {
		t.Fatalf("Provisioned condition reason=%q, want %q", condition.Reason, wantReason)
	}
	if condition.Message != wantMessage {
		t.Fatalf("Provisioned condition message=%q, want %q", condition.Message, wantMessage)
	}
	if condition.LastTransitionTime.IsZero() {
		t.Fatal("expected Provisioned condition lastTransitionTime")
	}
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
		ObjectMeta: metav1.ObjectMeta{Name: "tenant", Namespace: "team-a", Generation: 7},
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
	requireProvisionedCondition(
		t,
		updated,
		metav1.ConditionFalse,
		constants.ReasonSecurityViolation,
		updated.Status.LastError,
	)

	expectEventContains(t, recorder, "Warning", ReasonTenantProvisioningBlocked)
}

func TestReconcileOpenBaoTenant_FinalizerAndProvisioning(t *testing.T) {
	setAdmissionReady(t, true)

	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant", Namespace: "openbao-operator-system", Generation: 3},
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
	if !controllerutil.ContainsFinalizer(updated, openbaov1alpha1.OpenBaoTenantFinalizer) {
		t.Fatalf("expected finalizer to be present")
	}
	if !updated.Status.Provisioned || updated.Status.LastError != "" {
		t.Fatalf("expected successful provision status, got provisioned=%v lastError=%q", updated.Status.Provisioned, updated.Status.LastError)
	}
	requireProvisionedCondition(
		t,
		updated,
		metav1.ConditionTrue,
		ReasonTenantProvisioned,
		"Tenant RBAC provisioned for namespace tenant-ns",
	)

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
			Generation: 5,
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
	requireProvisionedCondition(
		t,
		updated,
		metav1.ConditionFalse,
		ReasonTenantProvisioningBlocked,
		"target namespace missing-ns not found",
	)
}

func TestReconcileOpenBaoTenant_AdmissionNotReadyRequeues(t *testing.T) {
	setAdmissionReady(t, false)

	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "tenant",
			Namespace:  "openbao-operator-system",
			Finalizers: []string{openbaov1alpha1.OpenBaoTenantFinalizer},
			Generation: 9,
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
	requireProvisionedCondition(
		t,
		updated,
		metav1.ConditionFalse,
		ReasonTenantProvisioningBlocked,
		updated.Status.LastError,
	)
}

func TestReconcileOpenBaoTenant_ProvisioningFailure(t *testing.T) {
	setAdmissionReady(t, true)

	provisioningErr := errors.New("tenant RBAC apply failed")
	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "tenant",
			Namespace:  "openbao-operator-system",
			Finalizers: []string{openbaov1alpha1.OpenBaoTenantFinalizer},
			Generation: 11,
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{TargetNamespace: "tenant-ns"},
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tenant-ns"}}
	runtime := newTenantRuntime(t, tenant, ns)
	runtime.Provisioner = failingTenantProvisioner{Provisioner: runtime.Provisioner, err: provisioningErr}
	req := types.NamespacedName{Name: "tenant", Namespace: "openbao-operator-system"}

	result, err := ReconcileOpenBaoTenant(context.Background(), req, logr.Discard(), runtime)
	if err == nil || !strings.Contains(err.Error(), "failed to ensure tenant RBAC") {
		t.Fatalf("expected tenant RBAC provisioning error, got %v", err)
	}
	if result != (recon.Result{}) {
		t.Fatalf("result=%v, want zero", result)
	}

	updated := &openbaov1alpha1.OpenBaoTenant{}
	if getErr := runtime.Client.Get(context.Background(), req, updated); getErr != nil {
		t.Fatalf("get updated tenant: %v", getErr)
	}
	if updated.Status.Provisioned {
		t.Fatal("expected Provisioned=false")
	}
	if updated.Status.LastError != provisioningErr.Error() {
		t.Fatalf("LastError=%q, want %q", updated.Status.LastError, provisioningErr.Error())
	}
	requireProvisionedCondition(
		t,
		updated,
		metav1.ConditionFalse,
		ReasonTenantProvisioningFailed,
		provisioningErr.Error(),
	)
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
		} else if controllerutil.ContainsFinalizer(updated, openbaov1alpha1.OpenBaoTenantFinalizer) {
			t.Fatalf("expected finalizer to be removed")
		}

		expectEventContains(t, recorder, "Normal", ReasonTenantRBACCleaned)
	})
}

func TestReconcileOpenBaoTenant_FinalizerAddUsesMergePatch(t *testing.T) {
	setAdmissionReady(t, true)

	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant", Namespace: "openbao-operator-system"},
		Spec:       openbaov1alpha1.OpenBaoTenantSpec{TargetNamespace: "tenant-ns"},
	}

	var patches int
	var updates int
	c := fake.NewClientBuilder().
		WithScheme(newTenantScheme(t)).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoTenant{}).
		WithObjects(tenant.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				updates++
				return errors.New("unexpected update for finalizer")
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if obj.GetName() == tenant.Name && obj.GetNamespace() == tenant.Namespace {
					patches++
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()
	runtime := newTenantRuntimeWithClient(t, c)
	req := types.NamespacedName{Name: "tenant", Namespace: "openbao-operator-system"}

	result, err := ReconcileOpenBaoTenant(context.Background(), req, logr.Discard(), runtime)
	if err != nil {
		t.Fatalf("ReconcileOpenBaoTenant() error = %v", err)
	}
	if result.RequeueAfter != 5*time.Second {
		t.Fatalf("requeueAfter=%v, want 5s", result.RequeueAfter)
	}
	if updates != 0 {
		t.Fatalf("Update() calls = %d, want 0", updates)
	}
	if patches != 1 {
		t.Fatalf("Patch() calls = %d, want 1", patches)
	}

	updated := &openbaov1alpha1.OpenBaoTenant{}
	if getErr := runtime.Client.Get(context.Background(), req, updated); getErr != nil {
		t.Fatalf("get updated tenant: %v", getErr)
	}
	if !controllerutil.ContainsFinalizer(updated, openbaov1alpha1.OpenBaoTenantFinalizer) {
		t.Fatalf("expected finalizer to be present")
	}
}

func TestReconcileOpenBaoTenant_FinalizerRemoveUsesMergePatch(t *testing.T) {
	setAdmissionReady(t, true)

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

	var patches int
	var updates int
	c := fake.NewClientBuilder().
		WithScheme(newTenantScheme(t)).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoTenant{}).
		WithObjects(tenant.DeepCopy(), ns.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				updates++
				return errors.New("unexpected update for finalizer")
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if obj.GetName() == tenant.Name && obj.GetNamespace() == tenant.Namespace {
					patches++
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()
	runtime := newTenantRuntimeWithClient(t, c)
	req := types.NamespacedName{Name: "tenant", Namespace: "openbao-operator-system"}

	result, err := ReconcileOpenBaoTenant(context.Background(), req, logr.Discard(), runtime)
	if err != nil {
		t.Fatalf("ReconcileOpenBaoTenant() error = %v", err)
	}
	if result != (recon.Result{}) {
		t.Fatalf("result=%v, want zero", result)
	}
	if updates != 0 {
		t.Fatalf("Update() calls = %d, want 0", updates)
	}
	if patches != 1 {
		t.Fatalf("Patch() calls = %d, want 1", patches)
	}
}
