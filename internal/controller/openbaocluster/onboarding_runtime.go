package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func (r *OpenBaoClusterReconciler) pauseForTenantOnboarding(ctx context.Context, logger logr.Logger, controllerName, namespace string) (ctrl.Result, error, bool) {
	if r.SingleTenantMode {
		return ctrl.Result{}, nil, false
	}

	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}

	roleBinding := &rbacv1.RoleBinding{}
	err := reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: constants.TenantRoleBindingName}, roleBinding)
	if err == nil {
		return ctrl.Result{}, nil, false
	}
	if apierrors.IsNotFound(err) {
		logger.V(1).Info("Tenant onboarding not finished; pausing reconciliation", "controller", controllerName, "roleBinding", constants.TenantRoleBindingName)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil, true
	}

	return ctrl.Result{}, fmt.Errorf("failed to verify tenant onboarding for namespace %s: %w", namespace, err), true
}
