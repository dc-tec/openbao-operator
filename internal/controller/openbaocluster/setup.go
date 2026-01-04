package openbaocluster

import (
	"context"
	"time"

	"golang.org/x/time/rate"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	controllerutil "github.com/dc-tec/openbao-operator/internal/controller"
)

// SetupWithManager sets up the OpenBaoCluster controllers with the Manager.
// It registers three controller-runtime controllers that watch OpenBaoCluster:
// workload (certs/infra/init), adminops (upgrade/backup), and status (finalizers/conditions).
//
// In single-tenant mode, the controller uses Owns() watches for event-driven reconciliation.
// In multi-tenant mode, the controller uses polling-based reconciliation to avoid requiring
// cluster-wide list/watch permissions.
func (r *OpenBaoClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
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

	// Workload controller with Owns() watches for event-driven reconciliation
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.ServiceAccount{}).
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
	sentinelAwareHandler := handler.TypedFuncs[client.Object, ctrl.Request]{
		CreateFunc: func(ctx context.Context, evt event.TypedCreateEvent[client.Object], q workqueue.TypedRateLimitingInterface[ctrl.Request]) {
			if evt.Object == nil {
				return
			}
			q.Add(ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: evt.Object.GetNamespace(),
					Name:      evt.Object.GetName(),
				},
			})
		},
		DeleteFunc: func(ctx context.Context, evt event.TypedDeleteEvent[client.Object], q workqueue.TypedRateLimitingInterface[ctrl.Request]) {
			if evt.Object == nil {
				return
			}
			q.Add(ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: evt.Object.GetNamespace(),
					Name:      evt.Object.GetName(),
				},
			})
		},
		UpdateFunc: func(ctx context.Context, evt event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[ctrl.Request]) {
			if evt.ObjectOld == nil || evt.ObjectNew == nil {
				return
			}
			oldCluster, okOld := evt.ObjectOld.(*openbaov1alpha1.OpenBaoCluster)
			newCluster, okNew := evt.ObjectNew.(*openbaov1alpha1.OpenBaoCluster)
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: evt.ObjectNew.GetNamespace(),
					Name:      evt.ObjectNew.GetName(),
				},
			}

			// Rate limit Sentinel fast-path triggers to avoid a single cluster
			// generating a continuous high-rate reconcile loop.
			if okOld && okNew {
				oldTriggerID := ""
				if oldCluster.Status.Sentinel != nil {
					oldTriggerID = oldCluster.Status.Sentinel.TriggerID
				}
				newTriggerID := ""
				if newCluster.Status.Sentinel != nil {
					newTriggerID = newCluster.Status.Sentinel.TriggerID
				}
				if newTriggerID != "" && newTriggerID != oldTriggerID {
					q.AddRateLimited(req)
					return
				}
			}

			q.Add(req)
		},
		GenericFunc: func(ctx context.Context, evt event.TypedGenericEvent[client.Object], q workqueue.TypedRateLimitingInterface[ctrl.Request]) {
			if evt.Object == nil {
				return
			}
			q.Add(ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: evt.Object.GetNamespace(),
					Name:      evt.Object.GetName(),
				},
			})
		},
	}

	sharedOptions := controller.Options{
		RateLimiter: workqueue.NewTypedMaxOfRateLimiter(
			workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](1*time.Second, 60*time.Second),
			&workqueue.TypedBucketRateLimiter[ctrl.Request]{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		),
	}

	// Workload controller: reacts to Sentinel triggerID changes and performs drift correction.
	if err := ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(controllerutil.OpenBaoClusterPredicateWithOptions(controllerutil.OpenBaoClusterPredicateOptions{
			ReconcileOnSentinelTrigger: true,
		})).
		Watches(&openbaov1alpha1.OpenBaoCluster{}, sentinelAwareHandler).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 2,
			RateLimiter:             sharedOptions.RateLimiter,
		}).
		Named(constants.ControllerNameOpenBaoClusterWorkload).
		Complete(&openBaoClusterWorkloadReconciler{parent: r}); err != nil {
		return err
	}

	// AdminOps controller: runs upgrades/backups and reacts to fast-path completion (lastHandledTriggerID).
	if err := ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(controllerutil.OpenBaoClusterPredicateWithOptions(controllerutil.OpenBaoClusterPredicateOptions{
			ReconcileOnSentinelHandled: true,
		})).
		Watches(&openbaov1alpha1.OpenBaoCluster{}, sentinelAwareHandler).
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
		WithEventFilter(controllerutil.OpenBaoClusterPredicateWithOptions(controllerutil.OpenBaoClusterPredicateOptions{
			ReconcileOnUpgradeStatus:   true,
			ReconcileOnBackupStatus:    true,
			ReconcileOnBlueGreenStatus: true,
			ReconcileOnBreakGlass:      true,
			ReconcileOnWorkloadError:   true,
			ReconcileOnAdminOpsError:   true,
		})).
		Watches(&openbaov1alpha1.OpenBaoCluster{}, sentinelAwareHandler).
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
