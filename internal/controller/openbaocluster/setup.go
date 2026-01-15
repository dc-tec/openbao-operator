package openbaocluster

import (
	"time"

	"golang.org/x/time/rate"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	controllerutil "github.com/dc-tec/openbao-operator/internal/controller"
	"github.com/dc-tec/openbao-operator/internal/security"
)

// SetupWithManager sets up the OpenBaoCluster controllers with the Manager.
// It registers three controller-runtime controllers that watch OpenBaoCluster:
// workload (certs/infra/init), adminops (upgrade/backup), and status (finalizers/conditions).
//
// In single-tenant mode, the controller uses Owns() watches for event-driven reconciliation.
// In multi-tenant mode, the controller uses polling-based reconciliation to avoid requiring
// cluster-wide list/watch permissions.
func (r *OpenBaoClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize ImageVerifier with embedded trusted root (nil config)
	// This ensures we reuse the same verifier (and its internal caches) across reconciliations.
	r.ImageVerifier = security.NewImageVerifier(
		mgr.GetLogger().WithName("image-verifier"),
		r.Client,
		nil, // trustedRootConfig
	)

	// Initialize OperatorImageVerifier with embedded trusted root (nil config)
	// This ensures we reuse the same verifier (and its internal caches) across reconciliations.
	r.OperatorImageVerifier = security.NewImageVerifier(
		mgr.GetLogger().WithName("operator-image-verifier"),
		r.Client,
		nil, // trustedRootConfig
	)

	if r.SingleTenantMode {
		return r.setupSingleTenantMode(mgr)
	}
	return r.setupMultiTenantMode(mgr)
}

// setupSingleTenantMode configures the controller for single-tenant mode.
// This mode uses Owns() watches for event-driven reconciliation, providing
// immediate reaction to child resource changes (StatefulSet, Service, etc.).
// This requires list/watch permissions in the watched namespace.
func (r *OpenBaoClusterReconciler) setupSingleTenantMode(mgr ctrl.Manager) error {
	sharedOptions := controller.Options{
		RateLimiter: workqueue.NewTypedMaxOfRateLimiter(
			workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](1*time.Second, 60*time.Second),
			&workqueue.TypedBucketRateLimiter[ctrl.Request]{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		),
	}

	// Workload controller with Owns() watches for event-driven reconciliation.
	// Also reacts to BlueGreen status changes to clean up temporary services after upgrades.
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.ServiceAccount{}).
		WithEventFilter(controllerutil.OpenBaoClusterPredicateWithOptions(controllerutil.OpenBaoClusterPredicateOptions{
			ReconcileOnBlueGreenStatus: true,
		})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 2,
			RateLimiter:             sharedOptions.RateLimiter,
		}).
		Named(constants.ControllerNameOpenBaoClusterWorkload).
		Complete(&openBaoClusterWorkloadReconciler{parent: r}); err != nil {
		return err
	}

	// AdminOps controller for upgrades/backups
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoCluster{}).
		WithEventFilter(controllerutil.OpenBaoClusterPredicateWithOptions(controllerutil.OpenBaoClusterPredicateOptions{
			ReconcileOnUpgradeStatus: true,
			ReconcileOnBackupStatus:  true,
		})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RateLimiter:             sharedOptions.RateLimiter,
		}).
		Named(constants.ControllerNameOpenBaoClusterAdminOps).
		Complete(&openBaoClusterAdminOpsReconciler{parent: r}); err != nil {
		return err
	}

	// Status controller for finalizer and condition aggregation
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoCluster{}).
		WithEventFilter(controllerutil.OpenBaoClusterPredicateWithOptions(controllerutil.OpenBaoClusterPredicateOptions{
			ReconcileOnUpgradeStatus:   true,
			ReconcileOnBackupStatus:    true,
			ReconcileOnBlueGreenStatus: true,
			ReconcileOnBreakGlass:      true,
			ReconcileOnWorkloadError:   true,
			ReconcileOnAdminOpsError:   true,
		})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RateLimiter:             sharedOptions.RateLimiter,
		}).
		Named(constants.ControllerNameOpenBaoClusterStatus).
		Complete(&openBaoClusterStatusReconciler{parent: r}); err != nil {
		return err
	}

	return nil
}

// setupMultiTenantMode configures the controller for multi-tenant mode.
// SECURITY: To preserve the zero-trust model, the controller does not
// register ownership watches (Owns) for child resources like StatefulSets,
// Services, ConfigMaps, Jobs, Secrets, ServiceAccounts, or Ingresses. Ownership
// watches require cluster-wide list/watch permissions for those resource
// types, which conflicts with the design where the controller only has
// namespace-scoped permissions via tenant Roles. Instead, the controller
// reconciles child resources when the OpenBaoCluster itself changes or via
// explicit requeues in the reconciliation logic.
func (r *OpenBaoClusterReconciler) setupMultiTenantMode(mgr ctrl.Manager) error {
	sharedOptions := controller.Options{
		RateLimiter: workqueue.NewTypedMaxOfRateLimiter(
			workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](1*time.Second, 60*time.Second),
			&workqueue.TypedBucketRateLimiter[ctrl.Request]{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		),
	}

	// Workload controller: reconciles certs/infra/init. Also reacts to BlueGreen status changes
	// to clean up temporary services after upgrades.
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoCluster{}).
		WithEventFilter(controllerutil.OpenBaoClusterPredicateWithOptions(controllerutil.OpenBaoClusterPredicateOptions{
			ReconcileOnBlueGreenStatus: true,
		})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 2,
			RateLimiter:             sharedOptions.RateLimiter,
		}).
		Named(constants.ControllerNameOpenBaoClusterWorkload).
		Complete(&openBaoClusterWorkloadReconciler{parent: r}); err != nil {
		return err
	}

	// AdminOps controller: runs upgrades and backups.
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoCluster{}).
		WithEventFilter(controllerutil.OpenBaoClusterPredicateWithOptions(controllerutil.OpenBaoClusterPredicateOptions{
			ReconcileOnUpgradeStatus: true,
			ReconcileOnBackupStatus:  true,
		})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RateLimiter:             sharedOptions.RateLimiter,
		}).
		Named(constants.ControllerNameOpenBaoClusterAdminOps).
		Complete(&openBaoClusterAdminOpsReconciler{parent: r}); err != nil {
		return err
	}

	// Status controller: owns finalizer and condition/status aggregation.
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoCluster{}).
		WithEventFilter(controllerutil.OpenBaoClusterPredicateWithOptions(controllerutil.OpenBaoClusterPredicateOptions{
			ReconcileOnUpgradeStatus:   true,
			ReconcileOnBackupStatus:    true,
			ReconcileOnBlueGreenStatus: true,
			ReconcileOnBreakGlass:      true,
			ReconcileOnWorkloadError:   true,
			ReconcileOnAdminOpsError:   true,
		})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RateLimiter:             sharedOptions.RateLimiter,
		}).
		Named(constants.ControllerNameOpenBaoClusterStatus).
		Complete(&openBaoClusterStatusReconciler{parent: r}); err != nil {
		return err
	}

	return nil
}
