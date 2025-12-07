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

package controller

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openbao/operator/internal/provisioner"
)

var testScheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
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

func TestNamespaceProvisionerReconcile_TenantNamespace(t *testing.T) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant-ns",
			Labels: map[string]string{
				provisioner.TenantLabelKey: provisioner.TenantLabelValue,
			},
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t, namespace)
	provisionerManager := provisioner.NewManager(client, logger)

	reconciler := &NamespaceProvisionerReconciler{
		Client:      client,
		Scheme:      testScheme,
		Provisioner: provisionerManager,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "tenant-ns",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.Requeue {
		t.Error("Reconcile() should not requeue for tenant namespace")
	}

	// Verify Role was created
	role := &rbacv1.Role{}
	err = client.Get(ctx, types.NamespacedName{
		Namespace: "tenant-ns",
		Name:      provisioner.TenantRoleName,
	}, role)
	if err != nil {
		t.Fatalf("expected Role to exist: %v", err)
	}

	// Verify RoleBinding was created
	roleBinding := &rbacv1.RoleBinding{}
	err = client.Get(ctx, types.NamespacedName{
		Namespace: "tenant-ns",
		Name:      provisioner.TenantRoleBindingName,
	}, roleBinding)
	if err != nil {
		t.Fatalf("expected RoleBinding to exist: %v", err)
	}
}

func TestNamespaceProvisionerReconcile_NonTenantNamespace(t *testing.T) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "non-tenant-ns",
			Labels: map[string]string{},
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t, namespace)
	provisionerManager := provisioner.NewManager(client, logger)

	reconciler := &NamespaceProvisionerReconciler{
		Client:      client,
		Scheme:      testScheme,
		Provisioner: provisionerManager,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "non-tenant-ns",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.Requeue {
		t.Error("Reconcile() should not requeue for non-tenant namespace")
	}

	// Verify Role was NOT created
	role := &rbacv1.Role{}
	err = client.Get(ctx, types.NamespacedName{
		Namespace: "non-tenant-ns",
		Name:      provisioner.TenantRoleName,
	}, role)
	if err == nil {
		t.Error("expected Role to not exist for non-tenant namespace")
	}
}

func TestNamespaceProvisionerReconcile_NamespaceDeleted(t *testing.T) {
	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t)
	provisionerManager := provisioner.NewManager(client, logger)

	reconciler := &NamespaceProvisionerReconciler{
		Client:      client,
		Scheme:      testScheme,
		Provisioner: provisionerManager,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "deleted-ns",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.Requeue {
		t.Error("Reconcile() should not requeue for deleted namespace")
	}

	// Should not error when namespace doesn't exist
}

func TestNamespaceProvisionerReconcile_TenantLabelRemoved(t *testing.T) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "tenant-ns",
			Labels: map[string]string{}, // No tenant label
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

	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t, namespace, existingRole, existingRoleBinding)
	provisionerManager := provisioner.NewManager(client, logger)

	reconciler := &NamespaceProvisionerReconciler{
		Client:      client,
		Scheme:      testScheme,
		Provisioner: provisionerManager,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "tenant-ns",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.Requeue {
		t.Error("Reconcile() should not requeue")
	}

	// Verify Role was deleted
	role := &rbacv1.Role{}
	err = client.Get(ctx, types.NamespacedName{
		Namespace: "tenant-ns",
		Name:      provisioner.TenantRoleName,
	}, role)
	if err == nil {
		t.Error("expected Role to be deleted when tenant label is removed")
	}

	// Verify RoleBinding was deleted
	roleBinding := &rbacv1.RoleBinding{}
	err = client.Get(ctx, types.NamespacedName{
		Namespace: "tenant-ns",
		Name:      provisioner.TenantRoleBindingName,
	}, roleBinding)
	if err == nil {
		t.Error("expected RoleBinding to be deleted when tenant label is removed")
	}
}

func TestNamespaceProvisionerReconcile_WrongTenantLabelValue(t *testing.T) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant-ns",
			Labels: map[string]string{
				provisioner.TenantLabelKey: "false", // Wrong value
			},
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	client := newTestClient(t, namespace)
	provisionerManager := provisioner.NewManager(client, logger)

	reconciler := &NamespaceProvisionerReconciler{
		Client:      client,
		Scheme:      testScheme,
		Provisioner: provisionerManager,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "tenant-ns",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.Requeue {
		t.Error("Reconcile() should not requeue")
	}

	// Verify Role was NOT created
	role := &rbacv1.Role{}
	err = client.Get(ctx, types.NamespacedName{
		Namespace: "tenant-ns",
		Name:      provisioner.TenantRoleName,
	}, role)
	if err == nil {
		t.Error("expected Role to not exist when tenant label value is wrong")
	}
}
