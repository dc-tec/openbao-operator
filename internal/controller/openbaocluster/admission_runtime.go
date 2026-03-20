package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func (r *OpenBaoClusterReconciler) currentAdmissionStatus() *admission.Status {
	if r.AdmissionTracker == nil {
		return nil
	}
	return r.AdmissionTracker.Current()
}

func (r *OpenBaoClusterReconciler) ensureAdmissionStatusFresh(ctx context.Context) (*admission.Status, error) {
	if r.AdmissionTracker == nil {
		return nil, nil
	}
	return r.AdmissionTracker.EnsureFresh(ctx)
}

func (r *OpenBaoClusterReconciler) refreshAdmissionStatus(ctx context.Context) (*admission.Status, error) {
	if r.AdmissionTracker == nil {
		return nil, nil
	}
	return r.AdmissionTracker.Refresh(ctx)
}

func (r *OpenBaoClusterReconciler) pauseForAdmissionDependencyLoss(ctx context.Context, logger logr.Logger, controllerName string) (ctrl.Result, bool) {
	if admission.UnsafeAdmissionDisabled() {
		return ctrl.Result{}, false
	}

	status, err := r.refreshAdmissionStatus(ctx)
	if err != nil {
		logger.Info("Admission policy dependency refresh failed; pausing reconciliation", "controller", controllerName, "error", err)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, true
	}
	if status != nil && !status.OverallReady {
		logger.Info("Admission policy dependencies not ready; pausing reconciliation", "controller", controllerName, "summary", status.SummaryMessage())
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, true
	}

	return ctrl.Result{}, false
}
