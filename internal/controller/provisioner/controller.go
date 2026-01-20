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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	controllerpredicates "github.com/dc-tec/openbao-operator/internal/controller"
	"github.com/dc-tec/openbao-operator/internal/provisioner"
)

// NamespaceProvisionerReconciler reconciles OpenBaoTenant objects to provision
// RBAC resources for tenant namespaces.
//
// The Provisioner is a lightweight controller responsible for onboarding new
// tenant namespaces by creating namespaced Role and RoleBinding resources
// that grant the operator permission to manage OpenBaoCluster resources in
// those namespaces.
//
// SECURITY: This controller uses a governance model where OpenBaoTenant CRDs
// explicitly declare which namespaces should be provisioned. This eliminates
// the need for list/watch permissions on namespaces, improving the security
// posture by preventing the Provisioner from surveying the cluster topology.
type NamespaceProvisionerReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Provisioner       *provisioner.Manager
	OperatorNamespace string
}

// SECURITY: RBAC is manually maintained in config/rbac/provisioner_minimal_role.yaml.
// We do NOT use kubebuilder annotations because:
// 1. The Provisioner uses impersonation (impersonate serviceaccounts verb), which kubebuilder cannot generate
// 2. The RBAC is security-critical and must be explicitly controlled
// 3. The Provisioner only needs: namespace get/update/patch, RBAC read access, and impersonation permission
// 4. All create/update/delete operations on Roles/RoleBindings are performed via impersonation
//    of the delegate ServiceAccount, which enforces least privilege at the API server level.

// patchStatusSSA updates the tenant status using Server-Side Apply.
func (r *NamespaceProvisionerReconciler) patchStatusSSA(ctx context.Context, tenant *openbaov1alpha1.OpenBaoTenant) error {
	applyTenant := &openbaov1alpha1.OpenBaoTenant{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoTenant",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      tenant.Name,
			Namespace: tenant.Namespace,
		},
		Status: tenant.Status,
	}

	return r.Status().Patch(ctx, applyTenant, client.Apply,
		client.FieldOwner("openbao-provisioner-controller"),
		client.ForceOwnership,
	)
}

// Reconcile is part of the main Kubernetes reconciliation loop which watches
// for OpenBaoTenant resources and provisions RBAC for the target namespace
// specified in the CRD.
func (r *NamespaceProvisionerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	baseLogger := log.FromContext(ctx)
	logger := baseLogger.WithValues(
		"tenant", req.NamespacedName,
		"controller", constants.ControllerNameNamespaceProvisioner,
		"reconcile_id", time.Now().UnixNano(),
	)

	// Fetch the OpenBaoTenant CR
	tenant := &openbaov1alpha1.OpenBaoTenant{}
	if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
		if apierrors.IsNotFound(err) {
			// OpenBaoTenant deleted - nothing to do
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get OpenBaoTenant %s: %w", req.NamespacedName, err)
	}

	// SECURITY: Self-Service Validation
	// To prevent cross-tenant attacks ("Confused Deputy"), we enforce that
	// OpenBaoTenant resources created in user namespaces can ONLY target
	// their own namespace. Only the Operator namespace is trusted to
	// delegate to arbitrary namespaces.
	//
	// Note: We check r.OperatorNamespace != "" to allow local testing/development
	// where the env var might not be set, though properly it should always be set.
	isTrustedNamespace := tenant.Namespace == r.OperatorNamespace
	isSelfTargeting := tenant.Namespace == tenant.Spec.TargetNamespace

	if !isTrustedNamespace && !isSelfTargeting {
		err := fmt.Errorf("security violation: OpenBaoTenant in namespace %q cannot target namespace %q",
			tenant.Namespace, tenant.Spec.TargetNamespace)

		logger.Error(err, "Blocking provisioning attempt")

		// Update status to reflect failure
		tenant.Status.Provisioned = false
		tenant.Status.LastError = err.Error()

		meta.SetStatusCondition(&tenant.Status.Conditions, metav1.Condition{
			Type:    constants.ConditionTypeProvisioned,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonSecurityViolation,
			Message: err.Error(),
		})

		if patchErr := r.patchStatusSSA(ctx, tenant); patchErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch status for security violation: %w", patchErr)
		}

		// Do not requeue. User must fix the CR.
		return ctrl.Result{}, nil
	}

	targetNS := tenant.Spec.TargetNamespace
	logger = logger.WithValues("target_namespace", targetNS)

	// Handle deletion
	if !tenant.DeletionTimestamp.IsZero() {
		if containsFinalizer(tenant.Finalizers, openbaov1alpha1.OpenBaoTenantFinalizer) {
			logger.Info("OpenBaoTenant is being deleted", "target_namespace", targetNS)

			// 1. Check if any active OpenBaoCluster resources still exist in the target namespace.
			// If they do, we must NOT remove the RBAC yet, because the OpenBaoCluster finalizers
			// depend on these permissions (e.g. to delete PVCs or orphan secrets).
			// If we remove RBAC too early, the OpenBaoCluster finalizers will fail with Forbidden errors
			// and the cluster deletion will get stuck.
			clusterList := &openbaov1alpha1.OpenBaoClusterList{}
			if err := r.List(ctx, clusterList, client.InNamespace(targetNS)); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to list keys in namespace %s: %w", targetNS, err)
			}

			if len(clusterList.Items) > 0 {
				logger.Info("Waiting for OpenBaoClusters to be deleted before cleaning up RBAC",
					"target_namespace", targetNS,
					"cluster_count", len(clusterList.Items))
				// Requeue to check again later.
				// We use a slightly longer requeue time to avoid spamming the API server while waiting.
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}

			// 2. All clusters are gone, now it is safe to clean up RBAC.
			logger.Info("No OpenBaoClusters found; cleaning up tenant RBAC", "target_namespace", targetNS)
			if err := r.Provisioner.CleanupTenantRBAC(ctx, targetNS); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to cleanup tenant RBAC for namespace %s: %w", targetNS, err)
			}

			// Remove finalizer
			tenant.Finalizers = removeFinalizer(tenant.Finalizers, openbaov1alpha1.OpenBaoTenantFinalizer)
			if err := r.Update(ctx, tenant); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove finalizer from OpenBaoTenant %s: %w", req.NamespacedName, err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !containsFinalizer(tenant.Finalizers, openbaov1alpha1.OpenBaoTenantFinalizer) {
		tenant.Finalizers = append(tenant.Finalizers, openbaov1alpha1.OpenBaoTenantFinalizer)
		if err := r.Update(ctx, tenant); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer to OpenBaoTenant %s: %w", req.NamespacedName, err)
		}
		// Requeue to observe the resource with the finalizer attached
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	// Verify target namespace exists
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: targetNS}, ns); err != nil {
		if apierrors.IsNotFound(err) {
			// Namespace not found - update status and requeue
			tenant.Status.Provisioned = false
			tenant.Status.LastError = fmt.Sprintf("target namespace %s not found", targetNS)
			if err := r.patchStatusSSA(ctx, tenant); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update OpenBaoTenant status: %w", err)
			}
			logger.Info("Target namespace not found; will retry", "target_namespace", targetNS)
			return ctrl.Result{RequeueAfter: constants.RequeueStandard}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get namespace %s: %w", targetNS, err)
	}

	// Provision RBAC
	logger.Info("Provisioning tenant RBAC", "target_namespace", targetNS)
	if err := r.Provisioner.EnsureTenantRBAC(ctx, tenant); err != nil {
		tenant.Status.Provisioned = false
		tenant.Status.LastError = err.Error()
		if statusErr := r.patchStatusSSA(ctx, tenant); statusErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update OpenBaoTenant status: %w (original error: %w)", statusErr, err)
		}
		return ctrl.Result{}, fmt.Errorf("failed to ensure tenant RBAC for namespace %s: %w", targetNS, err)
	}

	// Update status to success
	tenant.Status.Provisioned = true
	tenant.Status.LastError = ""
	if err := r.patchStatusSSA(ctx, tenant); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update OpenBaoTenant status: %w", err)
	}

	logger.Info("Successfully provisioned tenant RBAC", "target_namespace", targetNS)
	return ctrl.Result{}, nil
}

// containsFinalizer checks if a finalizer is present in the slice.
func containsFinalizer(finalizers []string, value string) bool {
	for _, f := range finalizers {
		if f == value {
			return true
		}
	}
	return false
}

// removeFinalizer removes a finalizer from the slice.
func removeFinalizer(finalizers []string, value string) []string {
	result := make([]string, 0, len(finalizers))
	for _, f := range finalizers {
		if f != value {
			result = append(result, f)
		}
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceProvisionerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoTenant{}).
		WithEventFilter(controllerpredicates.OpenBaoTenantPredicate()).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3,
			RateLimiter:             workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](1*time.Second, 60*time.Second),
		}).
		Named(constants.ControllerNameNamespaceProvisioner).
		Complete(r)
}
