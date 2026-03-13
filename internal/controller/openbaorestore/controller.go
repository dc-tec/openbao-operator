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
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaorestore "github.com/dc-tec/openbao-operator/internal/app/openbaorestore"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	observability "github.com/dc-tec/openbao-operator/internal/platform/observability"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	"github.com/dc-tec/openbao-operator/internal/service/restore"
)

// OpenBaoRestoreReconciler reconciles a OpenBaoRestore object.
type OpenBaoRestoreReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	AdmissionTracker      *admission.Tracker
	RestoreManager        *restore.Manager
	Recorder              events.EventRecorder
	OperatorImageVerifier imageverify.Verifier
	Platform              string
}

const controllerNameOpenBaoRestore = "openbaorestore"

type restoreManagerAdapter struct {
	manager *restore.Manager
}

func (a restoreManagerAdapter) Reconcile(ctx context.Context, logger logr.Logger, restoreResource *openbaov1alpha1.OpenBaoRestore) (recon.Result, error) {
	result, err := a.manager.Reconcile(ctx, logger, restoreResource)
	return recon.Result{RequeueAfter: result.RequeueAfter}, err
}

// SECURITY: RBAC is provided via namespace-scoped tenant Roles, not cluster-wide.
// The controller uses direct API calls for Jobs (GET, not list/watch) to check status,
// similar to the backup controller pattern. No cluster-wide permissions are needed.

// Reconcile is part of the main Kubernetes reconciliation loop.
func (r *OpenBaoRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	start := time.Now()
	reconcileMetrics := observability.NewReconcileMetrics(req.Namespace, req.Name, controllerNameOpenBaoRestore)
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

	logger := log.FromContext(ctx).WithName("openbaorestore")

	if r.RestoreManager == nil {
		return ctrl.Result{}, fmt.Errorf("restore manager is not configured")
	}
	if result, blocked := r.pauseForAdmissionDependencyLoss(ctx, logger); blocked {
		return result, nil
	}

	appResult, appErr := appopenbaorestore.ReconcileOpenBaoRestore(
		ctx,
		r.Client,
		req.NamespacedName,
		logger,
		restoreManagerAdapter{manager: r.RestoreManager},
	)
	result = ctrl.Result{RequeueAfter: appResult.RequeueAfter}
	err = appErr
	if err != nil {
		recordError(err)
		return result, err
	}

	return result, nil
}

func (r *OpenBaoRestoreReconciler) pauseForAdmissionDependencyLoss(ctx context.Context, logger logr.Logger) (ctrl.Result, bool) {
	if admission.UnsafeAdmissionDisabled() {
		return ctrl.Result{}, false
	}

	status, err := admission.RefreshStatus(ctx, r.AdmissionTracker, r.Client)
	if err != nil {
		logger.Info("Admission policy dependency refresh failed; pausing restore reconciliation", "error", err)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, true
	}
	if status != nil && !status.OverallReady {
		logger.Info("Admission policy dependencies not ready; pausing restore reconciliation", "summary", status.SummaryMessage())
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, true
	}

	return ctrl.Result{}, false
}

// SetupWithManager sets up the controller with the Manager.
// NOTE: This controller does NOT use Owns() or Watches() for Jobs because the operator
// uses namespace-scoped RBAC via tenant Roles. Owns/Watches would require cluster-wide
// list/watch permissions on Jobs, which we don't have. Instead, the restore manager
// uses direct API calls (GET) and RequeueAfter polling to monitor job status.
func (r *OpenBaoRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize the restore manager
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorder("openbaorestore")
	}
	if r.RestoreManager == nil {
		r.RestoreManager = restore.NewManager(r.Client, r.Scheme, r.Recorder, r.OperatorImageVerifier, r.Platform)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoRestore{}).
		Named(controllerNameOpenBaoRestore).
		Complete(r)
}
