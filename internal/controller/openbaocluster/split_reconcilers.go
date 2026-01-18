package openbaocluster

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	backupmanager "github.com/dc-tec/openbao-operator/internal/backup"
	certmanager "github.com/dc-tec/openbao-operator/internal/certs"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	inframanager "github.com/dc-tec/openbao-operator/internal/infra"
	"github.com/dc-tec/openbao-operator/internal/upgrade/bluegreen"
	rollingupgrade "github.com/dc-tec/openbao-operator/internal/upgrade/rolling"
)

type openBaoClusterWorkloadReconciler struct {
	parent *OpenBaoClusterReconciler
}

type openBaoClusterAdminOpsReconciler struct {
	parent *OpenBaoClusterReconciler
}

type openBaoClusterStatusReconciler struct {
	parent *OpenBaoClusterReconciler
}

func (r *OpenBaoClusterReconciler) loggerFor(ctx context.Context, req ctrl.Request, controllerName string) logr.Logger {
	baseLogger := log.FromContext(ctx)
	return baseLogger.WithValues(
		"cluster_namespace", req.Namespace,
		"cluster_name", req.Name,
		"controller", controllerName,
		"reconcile_id", time.Now().UnixNano(),
	)
}

func patchStatusIfChanged(ctx context.Context, c client.Client, logger logr.Logger, original *openbaov1alpha1.OpenBaoCluster, cluster *openbaov1alpha1.OpenBaoCluster, reason string) error {
	if original == nil || cluster == nil {
		return nil
	}
	if reflect.DeepEqual(original.Status, cluster.Status) {
		return nil
	}

	// Create a minimal apply configuration with just the status fields we own
	applyCluster := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Status: cluster.Status,
	}

	if err := c.Status().Patch(ctx, applyCluster, client.Apply, client.FieldOwner("openbao-cluster-controller"), client.ForceOwnership); err != nil {
		return fmt.Errorf("failed to patch status (%s) for OpenBaoCluster %s/%s: %w", reason, cluster.Namespace, cluster.Name, err)
	}
	logger.V(1).Info("Patched OpenBaoCluster status (SSA)", "reason", reason)
	return nil
}

func controllerErrorStatus(err error) *openbaov1alpha1.ControllerErrorStatus {
	if err == nil {
		return nil
	}

	reason := "Error"
	if r, ok := operatorerrors.Reason(err); ok {
		reason = r
	}
	now := metav1.Now()
	return &openbaov1alpha1.ControllerErrorStatus{
		Reason:  reason,
		Message: err.Error(),
		At:      &now,
	}
}

func (r *openBaoClusterWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.parent.loggerFor(ctx, req, constants.ControllerNameOpenBaoClusterWorkload)
	logger.Info("Reconciling OpenBaoCluster workload")

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := r.parent.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get OpenBaoCluster %s/%s: %w", req.Namespace, req.Name, err)
	}

	if shouldSkipWorkloadReconcile(cluster) {
		return ctrl.Result{}, nil
	}

	return r.reconcileCluster(ctx, logger, cluster)
}

func shouldSkipWorkloadReconcile(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster == nil ||
		!cluster.DeletionTimestamp.IsZero() ||
		cluster.Spec.Paused ||
		cluster.Spec.Profile == ""
}

func (r *openBaoClusterWorkloadReconciler) reconcileCluster(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (ctrl.Result, error) {
	original := cluster.DeepCopy()
	if cluster.Status.Workload == nil {
		cluster.Status.Workload = &openbaov1alpha1.WorkloadControllerStatus{}
	}

	reconcilers := []SubReconciler{
		certmanager.NewManagerWithReloader(r.parent.Client, r.parent.Scheme, r.parent.TLSReload),
		&infraReconciler{
			client:                r.parent.Client,
			apiReader:             r.parent.APIReader,
			scheme:                r.parent.Scheme,
			operatorNamespace:     r.parent.OperatorNamespace,
			oidcIssuer:            r.parent.OIDCIssuer,
			oidcJWTKeys:           r.parent.OIDCJWTKeys,
			operatorImageVerifier: r.parent.OperatorImageVerifier,
			verifyImageFunc:       r.parent.verifyImage,
			recorder:              r.parent.Recorder,
			admissionStatus:       r.parent.AdmissionStatus,
		},
	}
	if r.parent.InitManager != nil {
		reconcilers = append(reconcilers, r.parent.InitManager)
	}

	if result, err := r.runWorkloadReconcilers(ctx, logger, original, cluster, reconcilers); err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	// Clear previous workload error after a successful reconcile.
	cluster.Status.Workload.LastError = nil

	if err := patchStatusIfChanged(ctx, r.parent.Client, logger, original, cluster, "workload-complete"); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *openBaoClusterWorkloadReconciler) runWorkloadReconcilers(
	ctx context.Context,
	logger logr.Logger,
	original, cluster *openbaov1alpha1.OpenBaoCluster,
	reconcilers []SubReconciler,
) (ctrl.Result, error) {
	for _, rec := range reconcilers {
		result, err := rec.Reconcile(ctx, logger, cluster)
		if err != nil {
			cluster.Status.Workload.LastError = controllerErrorStatus(err)

			// Persist status changes before returning to avoid losing in-memory updates.
			if statusErr := patchStatusIfChanged(ctx, r.parent.Client, logger, original, cluster, "workload-error"); statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			if handled, ok := workloadResultForError(err, cluster.Status.Workload.LastError); ok {
				return handled, nil
			}

			return ctrl.Result{}, err
		}

		if result.RequeueAfter > 0 {
			if statusErr := patchStatusIfChanged(ctx, r.parent.Client, logger, original, cluster, "workload-requeue"); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{RequeueAfter: result.RequeueAfter}, nil
		}
	}

	return ctrl.Result{}, nil
}

func workloadResultForError(err error, lastError *openbaov1alpha1.ControllerErrorStatus) (ctrl.Result, bool) {
	if lastError != nil {
		switch lastError.Reason {
		case ReasonPrerequisitesMissing:
			return ctrl.Result{RequeueAfter: constants.RequeueShort}, true
		case ReasonGatewayAPIMissing:
			jitterNanos := time.Now().UnixNano() % int64(constants.RequeueSafetyNetJitter)
			requeueAfter := constants.RequeueSafetyNetBase + time.Duration(jitterNanos)
			return ctrl.Result{RequeueAfter: requeueAfter}, true
		}
	}

	if operatorerrors.IsTransient(err) {
		shouldRequeue, requeueAfter := operatorerrors.ShouldRequeue(err)
		if shouldRequeue {
			if requeueAfter > 0 {
				return ctrl.Result{RequeueAfter: requeueAfter}, true
			}
			return ctrl.Result{RequeueAfter: constants.RequeueShort}, true
		}
	}

	return ctrl.Result{}, false
}

func (r *openBaoClusterAdminOpsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.parent.loggerFor(ctx, req, constants.ControllerNameOpenBaoClusterAdminOps)
	logger.Info("Reconciling OpenBaoCluster admin operations")

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := r.parent.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get OpenBaoCluster %s/%s: %w", req.Namespace, req.Name, err)
	}

	if !cluster.DeletionTimestamp.IsZero() || cluster.Spec.Paused || cluster.Spec.Profile == "" {
		return ctrl.Result{}, nil
	}

	original := cluster.DeepCopy()
	if cluster.Status.AdminOps == nil {
		cluster.Status.AdminOps = &openbaov1alpha1.AdminOpsControllerStatus{}
	}

	var reconcilers []SubReconciler
	infraMgr := inframanager.NewManagerWithReader(r.parent.Client, r.parent.APIReader, r.parent.Scheme, r.parent.OperatorNamespace, r.parent.OIDCIssuer, r.parent.OIDCJWTKeys)
	// Blue/green upgrade strategy
	reconcilers = append(reconcilers, bluegreen.NewManager(r.parent.Client, r.parent.Scheme, infraMgr, r.parent.SmartClientConfig, r.parent.ImageVerifier, r.parent.OperatorImageVerifier))

	// Rolling upgrade strategy
	reconcilers = append(reconcilers, rollingupgrade.NewManager(r.parent.Client, r.parent.Scheme, r.parent.SmartClientConfig, r.parent.OperatorImageVerifier))

	// Backup
	reconcilers = append(reconcilers, backupmanager.NewManager(r.parent.Client, r.parent.Scheme, r.parent.SmartClientConfig, r.parent.OperatorImageVerifier))

	for _, rec := range reconcilers {
		result, err := rec.Reconcile(ctx, logger, cluster)
		if err != nil {
			cluster.Status.AdminOps.LastError = controllerErrorStatus(err)

			if statusErr := patchStatusIfChanged(ctx, r.parent.Client, logger, original, cluster, "adminops-error"); statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			if operatorerrors.IsTransient(err) {
				shouldRequeue, requeueAfter := operatorerrors.ShouldRequeue(err)
				if shouldRequeue {
					if requeueAfter > 0 {
						return ctrl.Result{RequeueAfter: requeueAfter}, nil
					}
					return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
				}
			}
			if operatorerrors.IsPermanent(err) {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}

		if result.RequeueAfter > 0 {
			if statusErr := patchStatusIfChanged(ctx, r.parent.Client, logger, original, cluster, "adminops-requeue"); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{RequeueAfter: result.RequeueAfter}, nil
		}
	}

	// Clear previous adminops error after a successful reconcile.
	cluster.Status.AdminOps.LastError = nil

	if err := patchStatusIfChanged(ctx, r.parent.Client, logger, original, cluster, "adminops-complete"); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *openBaoClusterStatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.parent.loggerFor(ctx, req, constants.ControllerNameOpenBaoClusterStatus)
	logger.Info("Reconciling OpenBaoCluster status")

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := r.parent.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get OpenBaoCluster %s/%s: %w", req.Namespace, req.Name, err)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		logger.Info("OpenBaoCluster is marked for deletion")
		if containsFinalizer(cluster.Finalizers, openbaov1alpha1.OpenBaoClusterFinalizer) {
			if err := r.parent.handleDeletion(ctx, logger, cluster); err != nil {
				return ctrl.Result{}, err
			}
			cluster.Finalizers = removeFinalizer(cluster.Finalizers, openbaov1alpha1.OpenBaoClusterFinalizer)
			if err := r.parent.Update(ctx, cluster); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove finalizer from OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
			}
		}
		return ctrl.Result{}, nil
	}

	if !containsFinalizer(cluster.Finalizers, openbaov1alpha1.OpenBaoClusterFinalizer) {
		cluster.Finalizers = append(cluster.Finalizers, openbaov1alpha1.OpenBaoClusterFinalizer)
		if err := r.parent.Update(ctx, cluster); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer to OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}
		return ctrl.Result{}, nil
	}

	if cluster.Spec.Paused {
		if err := r.parent.updateStatusForPaused(ctx, logger, cluster); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if err := r.parent.emitSecurityWarningEvents(ctx, logger, cluster); err != nil {
		logger.Error(err, "Failed to emit security warning events")
	}

	if cluster.Spec.Profile == "" {
		if err := r.parent.updateStatusForProfileNotSet(ctx, logger, cluster); err != nil {
			return ctrl.Result{}, err
		}
		jitterNanos := time.Now().UnixNano() % int64(constants.RequeueSafetyNetJitter)
		requeueAfter := constants.RequeueSafetyNetBase + time.Duration(jitterNanos)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	statusUpdateResult, err := r.parent.updateStatus(ctx, logger, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	if statusUpdateResult.RequeueAfter > 0 {
		return statusUpdateResult, nil
	}

	if r.parent.InitManager != nil && !cluster.Status.Initialized {
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	jitterNanos := time.Now().UnixNano() % int64(constants.RequeueSafetyNetJitter)
	requeueAfter := constants.RequeueSafetyNetBase + time.Duration(jitterNanos)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}
