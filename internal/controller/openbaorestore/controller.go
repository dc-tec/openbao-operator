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

package openbaorestore

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/restore"
)

// OpenBaoRestoreReconciler reconciles a OpenBaoRestore object.
type OpenBaoRestoreReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	RestoreManager *restore.Manager
	Recorder       record.EventRecorder
}

// SECURITY: RBAC is provided via namespace-scoped tenant Roles, not cluster-wide.
// The controller uses direct API calls for Jobs (GET, not list/watch) to check status,
// similar to the backup controller pattern. No cluster-wide permissions are needed.

// Reconcile is part of the main Kubernetes reconciliation loop.
func (r *OpenBaoRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("openbaorestore")

	// Fetch the OpenBaoRestore resource
	restoreResource := &openbaov1alpha1.OpenBaoRestore{}
	if err := r.Get(ctx, req.NamespacedName, restoreResource); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource deleted - nothing to do
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get OpenBaoRestore: %w", err)
	}

	// Add context fields for logging
	logger = logger.WithValues(
		"restore", req.Name,
		"namespace", req.Namespace,
		"cluster", restoreResource.Spec.Cluster,
		"phase", restoreResource.Status.Phase,
	)

	logger.Info("Reconciling OpenBaoRestore")

	// Create restore manager if not set
	if r.RestoreManager == nil {
		r.RestoreManager = restore.NewManager(r.Client, r.Scheme, r.Recorder)
	}

	// Delegate to restore manager
	result, err := r.RestoreManager.Reconcile(ctx, logger, restoreResource)
	if err != nil {
		logger.Error(err, "Reconciliation failed")
		return result, err
	}

	if result.RequeueAfter > 0 {
		logger.V(1).Info("Requeuing reconciliation", "requeueAfter", result.RequeueAfter)
	}

	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
// NOTE: This controller does NOT use Owns() or Watches() for Jobs because the operator
// uses namespace-scoped RBAC via tenant Roles. Owns/Watches would require cluster-wide
// list/watch permissions on Jobs, which we don't have. Instead, the restore manager
// uses direct API calls (GET) and RequeueAfter polling to monitor job status.
func (r *OpenBaoRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize the restore manager
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("openbaorestore")
	}
	r.RestoreManager = restore.NewManager(r.Client, r.Scheme, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoRestore{}).
		Named("openbaorestore").
		Complete(r)
}

// logr returns a logger for the reconciler.
func (r *OpenBaoRestoreReconciler) _(logger logr.Logger) logr.Logger {
	return logger.WithName("openbaorestore-controller")
}
