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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appprovisioner "github.com/dc-tec/openbao-operator/internal/app/provisioner"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	observability "github.com/dc-tec/openbao-operator/internal/observability"
	operatorpredicates "github.com/dc-tec/openbao-operator/internal/predicates"
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
	APIReader         client.Reader
	Scheme            *runtime.Scheme
	Provisioner       *provisioner.Manager
	OperatorNamespace string
}

// Reconcile is part of the main Kubernetes reconciliation loop which watches
// for OpenBaoTenant resources and provisions RBAC for the target namespace
// specified in the CRD.
func (r *NamespaceProvisionerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	start := time.Now()
	reconcileMetrics := observability.NewReconcileMetrics(req.Namespace, req.Name, constants.ControllerNameNamespaceProvisioner)
	recordedError := false
	recordError := func(e error) {
		if e == nil {
			return
		}
		reason := "Error"
		if r, ok := operatorerrors.Reason(e); ok {
			reason = r
		}
		reconcileMetrics.IncrementError(reason)
		recordedError = true
	}
	defer func() {
		reconcileMetrics.ObserveDuration(time.Since(start).Seconds())
		if err != nil && !recordedError {
			recordError(err)
		}
	}()

	baseLogger := log.FromContext(ctx)
	logger := baseLogger.WithValues(
		"tenant", req.NamespacedName,
	)

	result, err = appprovisioner.ReconcileOpenBaoTenant(ctx, req, logger, appprovisioner.TenantRuntime{
		Client:            r.Client,
		APIReader:         r.APIReader,
		Provisioner:       r.Provisioner,
		OperatorNamespace: r.OperatorNamespace,
	})
	if err != nil {
		recordError(err)
	}

	return result, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceProvisionerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoTenant{}).
		WithEventFilter(operatorpredicates.OpenBaoTenantPredicate()).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3,
			RateLimiter:             workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](1*time.Second, 60*time.Second),
		}).
		Named(constants.ControllerNameNamespaceProvisioner).
		Complete(r)
}
