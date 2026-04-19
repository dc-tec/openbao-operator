//go:build integration
// +build integration

package openbaocluster

import (
	"context"

	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

type testCompositeReconciler struct {
	parent *OpenBaoClusterReconciler
}

func (r *testCompositeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	statusReconciler := &openBaoClusterStatusReconciler{parent: r.parent}
	workloadReconciler := &openBaoClusterWorkloadReconciler{parent: r.parent}
	adminOpsReconciler := &openBaoClusterAdminOpsReconciler{parent: r.parent}

	if result, err := statusReconciler.Reconcile(ctx, req); err != nil {
		return result, err
	}
	if result, err := workloadReconciler.Reconcile(ctx, req); err != nil {
		return result, err
	}
	if result, err := adminOpsReconciler.Reconcile(ctx, req); err != nil {
		return result, err
	}
	return statusReconciler.Reconcile(ctx, req)
}

func ensureTenantNamespaceProvisioned(ctx context.Context, namespace string) {
	key := types.NamespacedName{Namespace: namespace, Name: constants.TenantRoleBindingName}
	existing := &rbacv1.RoleBinding{}
	err := k8sClient.Get(ctx, key, existing)
	if err == nil {
		return
	}
	Expect(apierrors.IsNotFound(err)).To(BeTrue())

	Expect(k8sClient.Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.TenantRoleBindingName,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     constants.TenantRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "openbao-operator-controller",
				Namespace: "openbao-operator-system",
			},
		},
	})).To(Succeed())
}
