package provisioner

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/service/provisioner"
)

// TenantSecretsRBACReconciler keeps tenant Secret access scoped to explicit allowlists.
//
// It watches OpenBaoCluster resources and, for namespaces that have already been provisioned
// via OpenBaoTenant, maintains the per-namespace Secret reader/writer Roles and RoleBindings.
type TenantSecretsRBACReconciler struct {
	client.Client
	APIReader   client.Reader
	Scheme      *runtime.Scheme
	Recorder    events.EventRecorder
	Provisioner *provisioner.Manager
}

func (r *TenantSecretsRBACReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"tenant_namespace", req.Namespace,
	)

	if r.Provisioner == nil {
		return ctrl.Result{}, fmt.Errorf("provisioner manager is required")
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		cluster = nil
	}

	// SECURITY: If admission policies are not ready, do not create/update tenant Secret RBAC allowlists.
	if admission.UnsafeAdmissionDisabled() {
		// UNSAFE MODE: Caller explicitly disabled admission policies; proceed without fail-closed gating.
		admission.SetAdmissionDependenciesReady(true)
	} else if !admission.AdmissionDependenciesReady() {
		reader := r.APIReader
		if reader == nil {
			reader = r.Client
		}

		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		status, err := admission.CheckDependencies(
			checkCtx,
			reader,
			admission.DefaultDependencies(),
			admission.DefaultNamePrefixes(),
		)
		cancel()
		if err != nil {
			admission.SetAdmissionDependenciesReady(false)
			logger.Info("Admission policy dependencies not ready; delaying tenant Secret RBAC sync", "error", err)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}

		admission.SetAdmissionDependenciesReady(status.OverallReady)
		if !status.OverallReady {
			logger.Info("Admission policy dependencies not ready; delaying tenant Secret RBAC sync", "summary", status.SummaryMessage())
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
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

	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}

	before, err := loadSecretRBACSnapshot(ctx, reader, req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.Provisioner.EnsureTenantSecretRBAC(ctx, req.Namespace); err != nil {
		return ctrl.Result{}, err
	}

	after, err := loadSecretRBACSnapshot(ctx, reader, req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster != nil && !before.equal(after) {
		emitClusterNormalEvent(r.Recorder, cluster, ReasonTenantSecretRBACSynchronized, fmt.Sprintf("Synchronized tenant Secret RBAC allowlists for namespace %s", req.Namespace))
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
		Named(controllerNameTenantSecretsRBAC).
		Complete(r)
}
