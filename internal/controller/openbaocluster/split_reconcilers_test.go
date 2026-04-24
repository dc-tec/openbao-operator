package openbaocluster

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestOpenBaoClusterAdminOpsReconcilerReconcile_UsesAPIReaderWhenAvailable(t *testing.T) {
	t.Parallel()

	scheme := newOpenBaoClusterTestScheme(t)
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "adminops-api-reader",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Paused: true,
		},
	}

	cachedClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		Build()
	apiReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster.DeepCopy()).
		Build()

	reconciler := &openBaoClusterAdminOpsReconciler{
		parent: &OpenBaoClusterReconciler{
			Client: cachedClient,
			ControllerRuntime: ControllerRuntime{
				APIReader: apiReader,
			},
		},
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestReconcilers_PauseWhenTenantNamespaceProvisioningIsIncomplete(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		newReconciler func(parent *OpenBaoClusterReconciler) admissionRuntimeTestReconciler
	}{
		{
			name: "workload",
			newReconciler: func(parent *OpenBaoClusterReconciler) admissionRuntimeTestReconciler {
				return &openBaoClusterWorkloadReconciler{parent: parent}
			},
		},
		{
			name: "adminops",
			newReconciler: func(parent *OpenBaoClusterReconciler) admissionRuntimeTestReconciler {
				return &openBaoClusterAdminOpsReconciler{parent: parent}
			},
		},
		{
			name: "status",
			newReconciler: func(parent *OpenBaoClusterReconciler) admissionRuntimeTestReconciler {
				return &openBaoClusterStatusReconciler{parent: parent}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cluster, parent := newTenantOnboardingTestContext(t, false)
			reconciler := tc.newReconciler(parent)

			result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
			})
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if result.RequeueAfter != constants.RequeueShort {
				t.Fatalf("Reconcile() requeueAfter = %s, want %s", result.RequeueAfter, constants.RequeueShort)
			}
		})
	}
}

func TestOpenBaoClusterStatusReconciler_BypassesTenantOnboardingGateInSingleTenantMode(t *testing.T) {
	t.Parallel()

	cluster, parent := newTenantOnboardingTestContext(t, true)
	current := &openbaov1alpha1.OpenBaoCluster{}
	if err := parent.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, current); err != nil {
		t.Fatalf("Get() cluster: %v", err)
	}
	current.Spec.Paused = true
	if err := parent.Update(context.Background(), current); err != nil {
		t.Fatalf("Update() paused cluster: %v", err)
	}

	reconciler := &openBaoClusterStatusReconciler{parent: parent}
	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("Reconcile() requeueAfter = %s, want 0", result.RequeueAfter)
	}
}

func TestOpenBaoClusterStatusReconciler_BypassesTenantOnboardingGateWhenProvisionedTenantExists(t *testing.T) {
	t.Parallel()

	cluster, parent := newTenantOnboardingTestContext(t, false, &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-default",
			Namespace: "openbao-operator-system",
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{
			TargetNamespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoTenantStatus{
			Provisioned: true,
		},
	})
	current := &openbaov1alpha1.OpenBaoCluster{}
	if err := parent.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, current); err != nil {
		t.Fatalf("Get() cluster: %v", err)
	}
	current.Spec.Paused = true
	if err := parent.Update(context.Background(), current); err != nil {
		t.Fatalf("Update() paused cluster: %v", err)
	}

	reconciler := &openBaoClusterStatusReconciler{parent: parent}
	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("Reconcile() requeueAfter = %s, want 0", result.RequeueAfter)
	}
}

func TestOpenBaoClusterStatusReconciler_BypassesTenantOnboardingGateForClaimManagedClusterWhenReferencedTenantProvisioned(t *testing.T) {
	t.Parallel()

	cluster, parent := newTenantOnboardingTestContext(
		t,
		false,
		&openbaov1alpha1.OpenBaoClusterClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "payments-bao",
				Namespace: "openbao-operator-system",
			},
			Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
				TenantRef: openbaov1alpha1.LocalReference{Name: "payments"},
			},
		},
		&openbaov1alpha1.OpenBaoTenant{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "payments",
				Namespace: "openbao-operator-system",
			},
			Spec: openbaov1alpha1.OpenBaoTenantSpec{
				TargetNamespace: "default",
			},
			Status: openbaov1alpha1.OpenBaoTenantStatus{
				Provisioned: true,
			},
		},
	)
	current := &openbaov1alpha1.OpenBaoCluster{}
	if err := parent.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, current); err != nil {
		t.Fatalf("Get() cluster: %v", err)
	}
	current.Spec.Paused = true
	current.Labels = map[string]string{
		constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
		constants.LabelOpenBaoClaimNamespace: "openbao-operator-system",
		constants.LabelOpenBaoClaimName:      "payments-bao",
	}
	if err := parent.Update(context.Background(), current); err != nil {
		t.Fatalf("Update() claim-managed cluster: %v", err)
	}

	reconciler := &openBaoClusterStatusReconciler{parent: parent}
	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("Reconcile() requeueAfter = %s, want 0", result.RequeueAfter)
	}
}

func TestOpenBaoClusterStatusReconciler_FinalizerAddUsesMergePatch(t *testing.T) {
	t.Parallel()

	scheme := newOpenBaoClusterTestScheme(t)
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "finalizer-patch",
			Namespace: "default",
		},
	}

	var patches int
	var updates int
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				updates++
				return errors.New("unexpected update for finalizer")
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if obj.GetName() == cluster.Name && obj.GetNamespace() == cluster.Namespace {
					patches++
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	parent := &OpenBaoClusterReconciler{
		Client: fakeClient,
		ControllerRuntime: ControllerRuntime{
			APIReader:         fakeClient,
			Scheme:            scheme,
			SingleTenantMode:  true,
			OperatorNamespace: "openbao-operator-system",
		},
	}
	reconciler := &openBaoClusterStatusReconciler{parent: parent}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (ctrl.Result{}) {
		t.Fatalf("Reconcile() result = %v, want zero", result)
	}
	if updates != 0 {
		t.Fatalf("Update() calls = %d, want 0", updates)
	}
	if patches != 1 {
		t.Fatalf("Patch() calls = %d, want 1", patches)
	}

	updated := &openbaov1alpha1.OpenBaoCluster{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatalf("Get() updated cluster: %v", err)
	}
	if !controllerutil.ContainsFinalizer(updated, openbaov1alpha1.OpenBaoClusterFinalizer) {
		t.Fatalf("expected finalizer to be present")
	}
}

func TestOpenBaoClusterStatusReconciler_FinalizerRemoveUsesMergePatch(t *testing.T) {
	t.Parallel()

	scheme := newOpenBaoClusterTestScheme(t)
	now := metav1.Now()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "finalizer-remove-patch",
			Namespace:         "default",
			Finalizers:        []string{openbaov1alpha1.OpenBaoClusterFinalizer},
			DeletionTimestamp: &now,
		},
	}

	var patches int
	var updates int
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				updates++
				return errors.New("unexpected update for finalizer")
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if obj.GetName() == cluster.Name && obj.GetNamespace() == cluster.Namespace {
					patches++
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	parent := &OpenBaoClusterReconciler{
		Client: fakeClient,
		ControllerRuntime: ControllerRuntime{
			APIReader:         fakeClient,
			Scheme:            scheme,
			SingleTenantMode:  true,
			OperatorNamespace: "openbao-operator-system",
		},
	}
	reconciler := &openBaoClusterStatusReconciler{parent: parent}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (ctrl.Result{}) {
		t.Fatalf("Reconcile() result = %v, want zero", result)
	}
	if updates != 0 {
		t.Fatalf("Update() calls = %d, want 0", updates)
	}
	if patches != 1 {
		t.Fatalf("Patch() calls = %d, want 1", patches)
	}
}

func TestOpenBaoClusterStatusReconciler_FailsWhenMultipleTenantsTargetNamespace(t *testing.T) {
	t.Parallel()

	cluster, parent := newTenantOnboardingTestContext(
		t,
		false,
		&openbaov1alpha1.OpenBaoTenant{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tenant-a",
				Namespace: "openbao-operator-system",
			},
			Spec: openbaov1alpha1.OpenBaoTenantSpec{TargetNamespace: "default"},
			Status: openbaov1alpha1.OpenBaoTenantStatus{
				Provisioned: true,
			},
		},
		&openbaov1alpha1.OpenBaoTenant{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tenant-b",
				Namespace: "openbao-operator-system",
			},
			Spec: openbaov1alpha1.OpenBaoTenantSpec{TargetNamespace: "default"},
			Status: openbaov1alpha1.OpenBaoTenantStatus{
				Provisioned: true,
			},
		},
	)

	reconciler := &openBaoClusterStatusReconciler{parent: parent}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
	})
	if err == nil || err.Error() != "failed to verify tenant onboarding for namespace default: multiple OpenBaoTenants target namespace default" {
		t.Fatalf("Reconcile() error = %v, want multiple OpenBaoTenants error", err)
	}
}

func newTenantOnboardingTestContext(t *testing.T, singleTenant bool, objects ...client.Object) (*openbaov1alpha1.OpenBaoCluster, *OpenBaoClusterReconciler) {
	t.Helper()

	scheme := newOpenBaoClusterTestScheme(t)
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-onboarding",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Image:    "openbao/openbao:2.5.0",
			Replicas: 1,
			Profile:  openbaov1alpha1.ProfileHardened,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
			},
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
			},
		},
	}

	clientObjects := make([]client.Object, 0, 1+len(objects))
	clientObjects = append(clientObjects, cluster.DeepCopy())
	clientObjects = append(clientObjects, objects...)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(clientObjects...).
		Build()

	parent := &OpenBaoClusterReconciler{
		Client: fakeClient,
		ControllerRuntime: ControllerRuntime{
			APIReader:         fakeClient,
			Scheme:            scheme,
			OperatorNamespace: "openbao-operator-system",
			SingleTenantMode:  singleTenant,
		},
	}

	return cluster, parent
}
