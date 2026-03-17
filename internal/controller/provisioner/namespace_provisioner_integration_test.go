//go:build integration
// +build integration

package provisioner_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	provisionersvc "github.com/dc-tec/openbao-operator/internal/service/provisioner"
)

func TestNamespaceProvisioner_SetupWithManager_ProvisionsTenantNamespace(t *testing.T) {
	setAdmissionReady(t)

	ctx := context.Background()
	liveClient := startNamespaceProvisionerController(t)

	createNamespace(t, ctx, liveClient, operatorNamespace)
	createNamespace(t, ctx, liveClient, "tenant-provisioned")

	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-provisioned",
			Namespace: operatorNamespace,
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{
			TargetNamespace: "tenant-provisioned",
		},
	}
	require.NoError(t, liveClient.Create(ctx, tenant))

	tenantKey := types.NamespacedName{Namespace: operatorNamespace, Name: tenant.Name}
	waitForTenantProvisioned(t, ctx, liveClient, tenantKey)
	waitForRole(t, ctx, liveClient, types.NamespacedName{
		Namespace: "tenant-provisioned",
		Name:      provisionersvc.TenantRoleName,
	})
	waitForRoleBinding(t, ctx, liveClient, types.NamespacedName{
		Namespace: "tenant-provisioned",
		Name:      provisionersvc.TenantRoleBindingName,
	})
}

func TestNamespaceProvisioner_SetupWithManager_CleansUpTenantRBACOnDelete(t *testing.T) {
	setAdmissionReady(t)

	ctx := context.Background()
	liveClient := startNamespaceProvisionerController(t)

	createNamespace(t, ctx, liveClient, operatorNamespace)
	createNamespace(t, ctx, liveClient, "tenant-cleanup")

	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-cleanup",
			Namespace: operatorNamespace,
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{
			TargetNamespace: "tenant-cleanup",
		},
	}
	require.NoError(t, liveClient.Create(ctx, tenant))

	tenantKey := types.NamespacedName{Namespace: operatorNamespace, Name: tenant.Name}
	current := waitForTenantProvisioned(t, ctx, liveClient, tenantKey)
	require.NoError(t, liveClient.Delete(ctx, current))

	waitForNotFound(t, ctx, liveClient, types.NamespacedName{
		Namespace: "tenant-cleanup",
		Name:      provisionersvc.TenantRoleName,
	}, provisionersvc.GenerateTenantRole("tenant-cleanup"))
	waitForNotFound(t, ctx, liveClient, types.NamespacedName{
		Namespace: "tenant-cleanup",
		Name:      provisionersvc.TenantRoleBindingName,
	}, provisionersvc.GenerateTenantRoleBinding("tenant-cleanup", provisionersvc.OperatorServiceAccount{
		Name:      "openbao-operator-controller",
		Namespace: operatorNamespace,
	}))
	waitForNotFound(t, ctx, liveClient, tenantKey, &openbaov1alpha1.OpenBaoTenant{})
}
