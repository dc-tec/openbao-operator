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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openbao/operator/internal/provisioner"
)

// NamespaceProvisionerReconciler reconciles Namespace objects to provision
// RBAC resources for tenant namespaces labeled with openbao.org/tenant=true.
//
// The Provisioner is a lightweight controller responsible for onboarding new
// tenant namespaces by creating namespaced Role and RoleBinding resources
// that grant the operator permission to manage OpenBaoCluster resources in
// those namespaces.
type NamespaceProvisionerReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Provisioner *provisioner.Manager
}

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// Note: The following permissions are required because Kubernetes RBAC requires that when
// creating a Role, the creator must have all the permissions they are trying to grant.
// The Provisioner needs these cluster-wide permissions to grant them in namespace-scoped Roles.
// +kubebuilder:rbac:groups=openbao.org,resources=openbaoclusters,verbs=*
// +kubebuilder:rbac:groups=openbao.org,resources=openbaoclusters/status,verbs=*
// +kubebuilder:rbac:groups=openbao.org,resources=openbaoclusters/finalizers,verbs=*
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=*
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=*
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=*
// +kubebuilder:rbac:groups="",resources=services,verbs=*
// +kubebuilder:rbac:groups="",resources=secrets,verbs=*
// +kubebuilder:rbac:groups="",resources=pods,verbs=*
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=*
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=*
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=*
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=*
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=*
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=*
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tlsroutes,verbs=*
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=backendtlspolicies,verbs=*

// Reconcile is part of the main Kubernetes reconciliation loop which watches
// for Namespace resources and provisions RBAC when a namespace is labeled
// with openbao.org/tenant=true.
func (r *NamespaceProvisionerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	baseLogger := log.FromContext(ctx)
	logger := baseLogger.WithValues(
		"namespace", req.Name,
		"controller", "namespace-provisioner",
		"reconcile_id", time.Now().UnixNano(),
	)

	// Get Namespace
	namespace := &corev1.Namespace{}
	if err := r.Get(ctx, req.NamespacedName, namespace); err != nil {
		if apierrors.IsNotFound(err) {
			// Namespace deleted - cleanup Role/RoleBinding if they exist
			logger.Info("Namespace not found; cleaning up tenant RBAC", "namespace", req.Name)
			if err := r.Provisioner.CleanupTenantRBAC(ctx, req.Name); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to cleanup tenant RBAC for namespace %s: %w", req.Name, err)
			}
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to get Namespace %s: %w", req.Name, err)
	}

	// Check if namespace has tenant label
	tenantLabelValue, hasTenantLabel := namespace.Labels[provisioner.TenantLabelKey]
	if !hasTenantLabel || tenantLabelValue != provisioner.TenantLabelValue {
		// Not a tenant namespace - cleanup if RBAC exists
		logger.Info("Namespace is not a tenant; cleaning up tenant RBAC if exists", "namespace", req.Name)
		if err := r.Provisioner.CleanupTenantRBAC(ctx, req.Name); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to cleanup tenant RBAC for namespace %s: %w", req.Name, err)
		}
		return ctrl.Result{}, nil
	}

	// Ensure Role and RoleBinding exist
	logger.Info("Provisioning tenant RBAC", "namespace", req.Name)
	if err := r.Provisioner.EnsureTenantRBAC(ctx, req.Name); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure tenant RBAC for namespace %s: %w", req.Name, err)
	}

	logger.Info("Successfully provisioned tenant RBAC", "namespace", req.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceProvisionerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3,
			RateLimiter:             workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](1*time.Second, 60*time.Second),
		}).
		Named("namespace-provisioner").
		Complete(r)
}
