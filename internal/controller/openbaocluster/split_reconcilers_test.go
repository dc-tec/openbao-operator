package openbaocluster

import (
	"context"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

func TestOpenBaoClusterStatusReconciler_BypassesTenantOnboardingGateWhenRoleBindingExists(t *testing.T) {
	t.Parallel()

	cluster, parent := newTenantOnboardingTestContext(t, false, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.TenantRoleBindingName,
			Namespace: "default",
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
			APIReader:        fakeClient,
			Scheme:           scheme,
			SingleTenantMode: singleTenant,
		},
	}

	return cluster, parent
}
