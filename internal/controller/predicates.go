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
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// OpenBaoClusterPredicateOptions controls which OpenBaoCluster changes should trigger reconciliation.
type OpenBaoClusterPredicateOptions struct {
	// ReconcileOnSentinelTrigger enables reconciliation when status.sentinel.triggerID changes.
	ReconcileOnSentinelTrigger bool
	// ReconcileOnSentinelHandled enables reconciliation when status.sentinel.lastHandledTriggerID changes.
	ReconcileOnSentinelHandled bool
	// ReconcileOnUpgradeStatus enables reconciliation when status.upgrade changes.
	ReconcileOnUpgradeStatus bool
	// ReconcileOnBackupStatus enables reconciliation when status.backup changes.
	ReconcileOnBackupStatus bool
	// ReconcileOnBlueGreenStatus enables reconciliation when status.blueGreen changes.
	ReconcileOnBlueGreenStatus bool
	// ReconcileOnBreakGlass enables reconciliation when status.breakGlass changes.
	ReconcileOnBreakGlass bool
	// ReconcileOnWorkloadError enables reconciliation when status.workload.lastError changes.
	ReconcileOnWorkloadError bool
	// ReconcileOnAdminOpsError enables reconciliation when status.adminOps.lastError changes.
	ReconcileOnAdminOpsError bool
}

// OpenBaoClusterPredicate filters OpenBaoCluster events to only reconcile on
// meaningful changes. This reduces noise and CPU usage by preventing the
// operator from waking up for irrelevant changes like status-only updates.
//
// The predicate allows reconciliation when:
//   - The resource is created
//   - The resource is deleted
//   - The Spec changes (detected via Generation change)
//   - DeletionTimestamp changes (triggers deletion handling)
//   - Finalizers change (triggers finalizer handling)
//   - Metadata labels or annotations change (may affect behavior)
//
// Status-only updates (like ReadyReplicas, Phase, Conditions) are filtered out
// since they don't require reconciliation - the controller updates status
// based on observed state, not in response to status changes.
func OpenBaoClusterPredicate() predicate.Predicate {
	return OpenBaoClusterPredicateWithOptions(OpenBaoClusterPredicateOptions{
		ReconcileOnSentinelTrigger: true,
	})
}

// OpenBaoClusterPredicateWithOptions is like OpenBaoClusterPredicate but allows opting into
// reconciliation on specific status-based signals (such as Sentinel triggers).
func OpenBaoClusterPredicateWithOptions(opts OpenBaoClusterPredicateOptions) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Always reconcile on create
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Always reconcile on delete
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCluster, ok := e.ObjectOld.(*openbaov1alpha1.OpenBaoCluster)
			if !ok {
				return true // If type assertion fails, allow reconciliation to be safe
			}
			newCluster, ok := e.ObjectNew.(*openbaov1alpha1.OpenBaoCluster)
			if !ok {
				return true // If type assertion fails, allow reconciliation to be safe
			}

			if opts.ReconcileOnSentinelTrigger {
				// Reconcile if Sentinel emitted a new drift trigger via status.
				oldTriggerID := ""
				if oldCluster.Status.Sentinel != nil {
					oldTriggerID = oldCluster.Status.Sentinel.TriggerID
				}
				newTriggerID := ""
				if newCluster.Status.Sentinel != nil {
					newTriggerID = newCluster.Status.Sentinel.TriggerID
				}
				if newTriggerID != "" && newTriggerID != oldTriggerID {
					return true
				}
			}

			if opts.ReconcileOnSentinelHandled {
				oldHandled := ""
				if oldCluster.Status.Sentinel != nil {
					oldHandled = oldCluster.Status.Sentinel.LastHandledTriggerID
				}
				newHandled := ""
				if newCluster.Status.Sentinel != nil {
					newHandled = newCluster.Status.Sentinel.LastHandledTriggerID
				}
				if newHandled != "" && newHandled != oldHandled {
					return true
				}
			}

			if opts.ReconcileOnUpgradeStatus {
				if !equality.Semantic.DeepEqual(oldCluster.Status.Upgrade, newCluster.Status.Upgrade) {
					return true
				}
			}

			if opts.ReconcileOnBackupStatus {
				if !equality.Semantic.DeepEqual(oldCluster.Status.Backup, newCluster.Status.Backup) {
					return true
				}
			}

			if opts.ReconcileOnBlueGreenStatus {
				if !equality.Semantic.DeepEqual(oldCluster.Status.BlueGreen, newCluster.Status.BlueGreen) {
					return true
				}
			}

			if opts.ReconcileOnBreakGlass {
				if !equality.Semantic.DeepEqual(oldCluster.Status.BreakGlass, newCluster.Status.BreakGlass) {
					return true
				}
			}

			if opts.ReconcileOnWorkloadError {
				var oldErr, newErr *openbaov1alpha1.ControllerErrorStatus
				if oldCluster.Status.Workload != nil {
					oldErr = oldCluster.Status.Workload.LastError
				}
				if newCluster.Status.Workload != nil {
					newErr = newCluster.Status.Workload.LastError
				}
				if !equality.Semantic.DeepEqual(oldErr, newErr) {
					return true
				}
			}

			if opts.ReconcileOnAdminOpsError {
				var oldErr, newErr *openbaov1alpha1.ControllerErrorStatus
				if oldCluster.Status.AdminOps != nil {
					oldErr = oldCluster.Status.AdminOps.LastError
				}
				if newCluster.Status.AdminOps != nil {
					newErr = newCluster.Status.AdminOps.LastError
				}
				if !equality.Semantic.DeepEqual(oldErr, newErr) {
					return true
				}
			}

			// Reconcile if Generation changed (indicates Spec change)
			if oldCluster.Generation != newCluster.Generation {
				return true
			}

			// Reconcile if DeletionTimestamp changed
			if !oldCluster.DeletionTimestamp.Equal(newCluster.DeletionTimestamp) {
				return true
			}

			// Reconcile if finalizers changed
			if !equality.Semantic.DeepEqual(oldCluster.Finalizers, newCluster.Finalizers) {
				return true
			}

			// Reconcile if labels changed (may affect resource selection or behavior)
			if !equality.Semantic.DeepEqual(oldCluster.Labels, newCluster.Labels) {
				return true
			}

			// Reconcile if annotations changed (may affect behavior)
			if !equality.Semantic.DeepEqual(oldCluster.Annotations, newCluster.Annotations) {
				return true
			}

			// Filter out status-only updates
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			// Always reconcile on generic events (rare, but be safe)
			return true
		},
	}
}

// StatefulSetReadyReplicasPredicate filters StatefulSet update events to only
// trigger reconciliation when ReadyReplicas changes. This is useful when
// watching StatefulSets to detect when pods become ready.
//
// Note: Currently, the OpenBaoCluster controller does not watch StatefulSets
// directly due to security constraints (namespace-scoped permissions). This
// predicate is provided for future use or for controllers that do watch
// StatefulSets.
func StatefulSetReadyReplicasPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Always reconcile on create
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Always reconcile on delete
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSts, ok := e.ObjectOld.(*appsv1.StatefulSet)
			if !ok {
				return true // If type assertion fails, allow reconciliation to be safe
			}
			newSts, ok := e.ObjectNew.(*appsv1.StatefulSet)
			if !ok {
				return true // If type assertion fails, allow reconciliation to be safe
			}

			// Only reconcile if ReadyReplicas changed
			return oldSts.Status.ReadyReplicas != newSts.Status.ReadyReplicas
		},
		GenericFunc: func(e event.GenericEvent) bool {
			// Always reconcile on generic events (rare, but be safe)
			return true
		},
	}
}

// OpenBaoTenantPredicate filters OpenBaoTenant events to only reconcile on
// meaningful changes. Similar to OpenBaoClusterPredicate, this filters out
// status-only updates.
func OpenBaoTenantPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Always reconcile on create
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Always reconcile on delete
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldTenant, ok := e.ObjectOld.(*openbaov1alpha1.OpenBaoTenant)
			if !ok {
				return true // If type assertion fails, allow reconciliation to be safe
			}
			newTenant, ok := e.ObjectNew.(*openbaov1alpha1.OpenBaoTenant)
			if !ok {
				return true // If type assertion fails, allow reconciliation to be safe
			}

			// Reconcile if Generation changed (indicates Spec change)
			if oldTenant.Generation != newTenant.Generation {
				return true
			}

			// Reconcile if DeletionTimestamp changed
			if !oldTenant.DeletionTimestamp.Equal(newTenant.DeletionTimestamp) {
				return true
			}

			// Reconcile if finalizers changed
			if !equality.Semantic.DeepEqual(oldTenant.Finalizers, newTenant.Finalizers) {
				return true
			}

			// Reconcile if labels changed
			if !equality.Semantic.DeepEqual(oldTenant.Labels, newTenant.Labels) {
				return true
			}

			// Reconcile if annotations changed
			if !equality.Semantic.DeepEqual(oldTenant.Annotations, newTenant.Annotations) {
				return true
			}

			// Filter out status-only updates
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			// Always reconcile on generic events (rare, but be safe)
			return true
		},
	}
}

// ResourceGenerationChangedPredicate is a generic predicate that filters
// update events to only trigger reconciliation when the Generation changes.
// Generation changes indicate that the Spec has been modified.
//
// This is useful for any resource type that follows the standard Kubernetes
// pattern where Generation increments on Spec changes.
func ResourceGenerationChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj, ok := e.ObjectOld.(metav1.Object)
			if !ok {
				return true
			}
			newObj, ok := e.ObjectNew.(metav1.Object)
			if !ok {
				return true
			}

			// Only reconcile if Generation changed
			return oldObj.GetGeneration() != newObj.GetGeneration()
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return true
		},
	}
}
