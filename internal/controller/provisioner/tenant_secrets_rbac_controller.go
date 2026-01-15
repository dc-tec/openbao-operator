package provisioner

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/provisioner"
)

// TenantSecretsRBACReconciler keeps tenant Secret access scoped to explicit allowlists.
//
// It watches OpenBaoCluster resources and, for namespaces that have already been provisioned
// via OpenBaoTenant, maintains the per-namespace Secret reader/writer Roles and RoleBindings.
type TenantSecretsRBACReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Provisioner *provisioner.Manager
}

func (r *TenantSecretsRBACReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"controller", "tenant-secrets-rbac",
		"namespace", req.Namespace,
	)

	if r.Provisioner == nil {
		return ctrl.Result{}, fmt.Errorf("provisioner manager is required")
	}

	// check if the tenant namespace is provisioned
	provisioned, err := r.Provisioner.IsTenantNamespaceProvisioned(ctx, req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !provisioned {
		// If not yet provisioned, we should verify that this is actually a target namespace
		// for some OpenBaoTenant. However, this controller watches OpenBaoClusters.
		// If an OpenBaoCluster exists, we generally expect the namespace to be provisioned soon.
		// We requeue with a delay to wait for the Provisioner to complete the base RBAC setup.
		logger.V(1).Info("Tenant namespace not yet provisioned; requeueing to sync secrets RBAC")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if err := r.Provisioner.EnsureTenantSecretRBAC(ctx, req.Namespace); err != nil {
		return ctrl.Result{}, err
	}

	logger.V(1).Info("Synced tenant Secret RBAC allowlists")
	return ctrl.Result{}, nil
}

func (r *TenantSecretsRBACReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoCluster{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3,
			RateLimiter:             workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](1*time.Second, 60*time.Second),
		}).
		Named(constants.ControllerNameNamespaceProvisioner + "-tenant-secrets").
		Complete(r)
}
