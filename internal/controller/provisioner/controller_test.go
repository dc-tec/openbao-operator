/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provisioner

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/provisioner"
)

var testScheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)
	return scheme
}()

func newTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	builder := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoTenant{})
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

func TestNamespaceProvisionerReconcile_TenantProvisioning(t *testing.T) {
	// Create target namespace
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant-ns",
		},
	}

	// Create OpenBaoTenant CR
	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tenant",
			Namespace: "openbao-operator-system",
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{
			TargetNamespace: "tenant-ns",
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t, namespace, tenant)
	provisionerManager, err := provisioner.NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("failed to create provisioner manager: %v", err)
	}

	reconciler := &NamespaceProvisionerReconciler{
		Client:            k8sClient,
		Scheme:            testScheme,
		Provisioner:       provisionerManager,
		OperatorNamespace: "openbao-operator-system",
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-tenant",
			Namespace: "openbao-operator-system",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// First reconcile should add finalizer and requeue
	if result.RequeueAfter == 0 {
		t.Error("Reconcile() should requeue after adding finalizer")
	}

	// Second reconcile should provision RBAC
	result, err = reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter > 0 {
		t.Error("Reconcile() should not requeue after successful provisioning")
	}

	// Verify Role was created
	role := &rbacv1.Role{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "tenant-ns",
		Name:      provisioner.TenantRoleName,
	}, role)
	if err != nil {
		t.Fatalf("expected Role to exist: %v", err)
	}

	// Verify RoleBinding was created
	roleBinding := &rbacv1.RoleBinding{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "tenant-ns",
		Name:      provisioner.TenantRoleBindingName,
	}, roleBinding)
	if err != nil {
		t.Fatalf("expected RoleBinding to exist: %v", err)
	}

	// Verify status was updated
	updatedTenant := &openbaov1alpha1.OpenBaoTenant{}
	err = k8sClient.Get(ctx, req.NamespacedName, updatedTenant)
	if err != nil {
		t.Fatalf("failed to get updated tenant: %v", err)
	}
	if !updatedTenant.Status.Provisioned {
		t.Error("expected status.Provisioned to be true")
	}
}

func TestNamespaceProvisionerReconcile_TargetNamespaceNotFound(t *testing.T) {
	// Create OpenBaoTenant CR with non-existent target namespace
	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tenant",
			Namespace: "openbao-operator-system",
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{
			TargetNamespace: "non-existent-ns",
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t, tenant)
	provisionerManager, err := provisioner.NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("failed to create provisioner manager: %v", err)
	}

	reconciler := &NamespaceProvisionerReconciler{
		Client:            k8sClient,
		Scheme:            testScheme,
		Provisioner:       provisionerManager,
		OperatorNamespace: "openbao-operator-system",
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-tenant",
			Namespace: "openbao-operator-system",
		},
	}

	// First reconcile should add finalizer
	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Error("Reconcile() should requeue after adding finalizer")
	}

	// Second reconcile should detect missing namespace and update status
	result, err = reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter == 0 {
		t.Error("Reconcile() should requeue after detecting missing namespace")
	}

	// Verify status was updated with error
	updatedTenant := &openbaov1alpha1.OpenBaoTenant{}
	err = k8sClient.Get(ctx, req.NamespacedName, updatedTenant)
	if err != nil {
		t.Fatalf("failed to get updated tenant: %v", err)
	}
	if updatedTenant.Status.Provisioned {
		t.Error("expected status.Provisioned to be false when namespace is missing")
	}
	if updatedTenant.Status.LastError == "" {
		t.Error("expected status.LastError to be set when namespace is missing")
	}
}

func TestNamespaceProvisionerReconcile_OpenBaoTenantDeleted(t *testing.T) {
	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t)
	provisionerManager, err := provisioner.NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("failed to create provisioner manager: %v", err)
	}

	reconciler := &NamespaceProvisionerReconciler{
		Client:            k8sClient,
		Scheme:            testScheme,
		Provisioner:       provisionerManager,
		OperatorNamespace: "openbao-operator-system",
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "deleted-tenant",
			Namespace: "openbao-operator-system",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter > 0 {
		t.Error("Reconcile() should not requeue for deleted OpenBaoTenant")
	}

	// Should not error when OpenBaoTenant doesn't exist
}

func TestNamespaceProvisionerReconcile_DeletionWithFinalizer(t *testing.T) {
	// Create target namespace
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant-ns",
		},
	}

	// Create existing RBAC resources
	existingRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provisioner.TenantRoleName,
			Namespace: "tenant-ns",
		},
	}

	existingRoleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provisioner.TenantRoleBindingName,
			Namespace: "tenant-ns",
		},
	}

	// Create OpenBaoTenant with deletion timestamp and finalizer
	now := metav1.Now()
	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-tenant",
			Namespace:         "openbao-operator-system",
			DeletionTimestamp: &now,
			Finalizers:        []string{openbaov1alpha1.OpenBaoTenantFinalizer},
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{
			TargetNamespace: "tenant-ns",
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t, namespace, tenant, existingRole, existingRoleBinding)
	provisionerManager, err := provisioner.NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("failed to create provisioner manager: %v", err)
	}

	reconciler := &NamespaceProvisionerReconciler{
		Client:            k8sClient,
		Scheme:            testScheme,
		Provisioner:       provisionerManager,
		OperatorNamespace: "openbao-operator-system",
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-tenant",
			Namespace: "openbao-operator-system",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter > 0 {
		t.Error("Reconcile() should not requeue after cleanup")
	}

	// Verify Role was deleted
	role := &rbacv1.Role{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "tenant-ns",
		Name:      provisioner.TenantRoleName,
	}, role)
	if err == nil {
		t.Error("expected Role to be deleted when OpenBaoTenant is deleted")
	}

	// Verify RoleBinding was deleted
	roleBinding := &rbacv1.RoleBinding{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "tenant-ns",
		Name:      provisioner.TenantRoleBindingName,
	}, roleBinding)
	if err == nil {
		t.Error("expected RoleBinding to be deleted when OpenBaoTenant is deleted")
	}

	// Verify finalizer was removed
	updatedTenant := &openbaov1alpha1.OpenBaoTenant{}
	err = k8sClient.Get(ctx, req.NamespacedName, updatedTenant)
	if err == nil {
		for _, f := range updatedTenant.Finalizers {
			if f == openbaov1alpha1.OpenBaoTenantFinalizer {
				t.Error("expected finalizer to be removed")
				break
			}
		}
	}
}

func TestNamespaceProvisionerReconcile_SecurityViolation(t *testing.T) {
	// Create OpenBaoTenant in user namespace targeting a different namespace
	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "malicious-tenant",
			Namespace: "user-ns",
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{
			TargetNamespace: "victim-ns",
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t, tenant)
	provisionerManager, err := provisioner.NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("failed to create provisioner manager: %v", err)
	}

	reconciler := &NamespaceProvisionerReconciler{
		Client:            k8sClient,
		Scheme:            testScheme,
		Provisioner:       provisionerManager,
		OperatorNamespace: "openbao-operator-system", // Trusted NS is different from user-ns
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "malicious-tenant",
			Namespace: "user-ns",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Should not requeue on security violation
	if result.RequeueAfter != 0 {
		t.Error("Reconcile() should not requeue on security violation")
	}

	// Verify status
	updatedTenant := &openbaov1alpha1.OpenBaoTenant{}
	if err := k8sClient.Get(ctx, req.NamespacedName, updatedTenant); err != nil {
		t.Fatalf("failed to get updated tenant: %v", err)
	}

	if updatedTenant.Status.Provisioned {
		t.Error("expected Status.Provisioned to be false")
	}

	condition := meta.FindStatusCondition(updatedTenant.Status.Conditions, "Provisioned")
	if condition == nil {
		t.Error("expected Provisioned condition to be set")
	} else {
		if condition.Status != metav1.ConditionFalse {
			t.Errorf("expected Condition Status to be False, got %s", condition.Status)
		}
		if condition.Reason != "SecurityViolation" {
			t.Errorf("expected Condition Reason to be SecurityViolation, got %s", condition.Reason)
		}
	}
}

func TestNamespaceProvisionerReconcile_SelfService_Success(t *testing.T) {
	// Create OpenBaoTenant in user namespace targeting SAME namespace
	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-tenant",
			Namespace: "user-ns",
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{
			TargetNamespace: "user-ns",
		},
	}

	// Create the namespace (target must exist)
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "user-ns",
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t, tenant, namespace)
	provisionerManager, err := provisioner.NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("failed to create provisioner manager: %v", err)
	}

	reconciler := &NamespaceProvisionerReconciler{
		Client:            k8sClient,
		Scheme:            testScheme,
		Provisioner:       provisionerManager,
		OperatorNamespace: "openbao-operator-system",
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "my-tenant",
			Namespace: "user-ns",
		},
	}

	// First reconcile: add finalizer
	_, err = reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() 1 error = %v", err)
	}

	// Second reconcile: provision
	_, err = reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() 2 error = %v", err)
	}

	// Verify status
	updatedTenant := &openbaov1alpha1.OpenBaoTenant{}
	if err := k8sClient.Get(ctx, req.NamespacedName, updatedTenant); err != nil {
		t.Fatalf("failed to get updated tenant: %v", err)
	}

	if !updatedTenant.Status.Provisioned {
		t.Error("expected Status.Provisioned to be true for valid self-service")
	}
}
