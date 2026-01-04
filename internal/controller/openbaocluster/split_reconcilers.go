package openbaocluster

import (
	"context"
	"fmt"
	"reflect"
	"strings"
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
	controllerutil "github.com/dc-tec/openbao-operator/internal/controller"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	inframanager "github.com/dc-tec/openbao-operator/internal/infra"
	upgrademanager "github.com/dc-tec/openbao-operator/internal/upgrade"
	"github.com/dc-tec/openbao-operator/internal/upgrade/bluegreen"
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
	if err := c.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to patch status (%s) for OpenBaoCluster %s/%s: %w", reason, cluster.Namespace, cluster.Name, err)
	}
	logger.V(1).Info("Patched OpenBaoCluster status", "reason", reason)
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

type workloadSentinelTrigger struct {
	fastPathAllowed bool
	isNewTrigger    bool
	resourceInfo    string
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

	sentinel := r.sentinelTrigger(cluster)
	if sentinel.isNewTrigger && sentinel.fastPathAllowed {
		recordSentinelTriggerMetrics(cluster, sentinel.resourceInfo)
	}

	reconcilers := []SubReconciler{
		certmanager.NewManagerWithReloader(r.parent.Client, r.parent.Scheme, r.parent.TLSReload),
		&infraReconciler{
			client:            r.parent.Client,
			scheme:            r.parent.Scheme,
			operatorNamespace: r.parent.OperatorNamespace,
			oidcIssuer:        r.parent.OIDCIssuer,
			oidcJWTKeys:       r.parent.OIDCJWTKeys,
			verifyImageFunc:   r.parent.verifyImage,
			recorder:          r.parent.Recorder,
			admissionStatus:   r.parent.AdmissionStatus,
		},
	}
	if r.parent.InitManager != nil {
		reconcilers = append(reconcilers, r.parent.InitManager)
	}

	if result, err := r.runWorkloadReconcilers(ctx, logger, original, cluster, reconcilers); err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	if sentinel.isNewTrigger && sentinel.fastPathAllowed {
		return r.applySentinelFastPath(ctx, logger, original, cluster, sentinel.resourceInfo)
	}

	// Clear previous workload error after a successful reconcile.
	cluster.Status.Workload.LastError = nil

	if err := patchStatusIfChanged(ctx, r.parent.Client, logger, original, cluster, "workload-complete"); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *openBaoClusterWorkloadReconciler) sentinelTrigger(cluster *openbaov1alpha1.OpenBaoCluster) workloadSentinelTrigger {
	sentinelFastPathAllowed := r.parent.AdmissionStatus == nil || r.parent.AdmissionStatus.SentinelReady
	trigger := workloadSentinelTrigger{
		fastPathAllowed: sentinelFastPathAllowed,
		resourceInfo:    unknownResource,
	}

	if cluster == nil || cluster.Status.Sentinel == nil || cluster.Status.Sentinel.TriggerID == "" {
		return trigger
	}

	trigger.isNewTrigger = cluster.Status.Sentinel.TriggerID != cluster.Status.Sentinel.LastHandledTriggerID
	if cluster.Status.Sentinel.TriggerResource != "" {
		trigger.resourceInfo = cluster.Status.Sentinel.TriggerResource
	}
	return trigger
}

func recordSentinelTriggerMetrics(cluster *openbaov1alpha1.OpenBaoCluster, sentinelResourceInfo string) {
	resourceKind := unknownResource
	if sentinelResourceInfo != "" && sentinelResourceInfo != unknownResource {
		parts := strings.Split(sentinelResourceInfo, "/")
		if len(parts) > 0 {
			resourceKind = parts[0]
		}
	}

	clusterMetrics := controllerutil.NewClusterMetrics(cluster.Namespace, cluster.Name)
	clusterMetrics.RecordDriftDetected(resourceKind)
	clusterMetrics.SetDriftLastDetectedTimestamp(float64(time.Now().Unix()))
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

func (r *openBaoClusterWorkloadReconciler) applySentinelFastPath(
	ctx context.Context,
	logger logr.Logger,
	original, cluster *openbaov1alpha1.OpenBaoCluster,
	sentinelResourceInfo string,
) (ctrl.Result, error) {
	now := metav1.Now()
	if cluster.Status.Drift == nil {
		cluster.Status.Drift = &openbaov1alpha1.DriftStatus{}
	}
	cluster.Status.Drift.LastDriftDetected = &now
	cluster.Status.Drift.DriftCorrectionCount++
	cluster.Status.Drift.LastDriftResource = sentinelResourceInfo
	cluster.Status.Drift.LastCorrectionTime = &now
	cluster.Status.Drift.ConsecutiveFastPaths++

	if cluster.Status.Sentinel != nil {
		cluster.Status.Sentinel.LastHandledTriggerID = cluster.Status.Sentinel.TriggerID
		cluster.Status.Sentinel.LastHandledAt = &now
	}

	clusterMetrics := controllerutil.NewClusterMetrics(cluster.Namespace, cluster.Name)
	clusterMetrics.RecordDriftCorrected()

	if err := patchStatusIfChanged(ctx, r.parent.Client, logger, original, cluster, "sentinel-fast-path"); err != nil {
		return ctrl.Result{}, err
	}

	// Prompt a quick follow-up. AdminOps will observe lastHandledTriggerID and run a full path reconcile.
	return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
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

	// Skip admin operations while a Sentinel trigger is active (unhandled) unless forced by rate-limiting rules.
	sentinelFastPathAllowed := r.parent.AdmissionStatus == nil || r.parent.AdmissionStatus.SentinelReady
	sentinelActive := false
	if sentinelFastPathAllowed && cluster.Status.Sentinel != nil && cluster.Status.Sentinel.TriggerID != "" {
		sentinelActive = cluster.Status.Sentinel.TriggerID != cluster.Status.Sentinel.LastHandledTriggerID
	}
	if sentinelActive {
		forceFullReconcile := false
		if cluster.Status.Drift == nil {
			cluster.Status.Drift = &openbaov1alpha1.DriftStatus{}
		}

		if cluster.Status.Drift.ConsecutiveFastPaths >= constants.SentinelMaxConsecutiveFastPaths {
			forceFullReconcile = true
		} else if cluster.Status.Drift.LastFullReconcileTime != nil {
			if time.Since(cluster.Status.Drift.LastFullReconcileTime.Time) >= constants.SentinelForceFullReconcileInterval {
				forceFullReconcile = true
			}
		}

		if !forceFullReconcile {
			logger.Info("Skipping Upgrade and Backup managers due to active Sentinel fast-path trigger",
				"consecutiveFastPaths", cluster.Status.Drift.ConsecutiveFastPaths,
				"maxBeforeForce", constants.SentinelMaxConsecutiveFastPaths,
			)
			return ctrl.Result{RequeueAfter: constants.RequeueStandard}, nil
		}
		logger.Info("Forcing full reconcile despite active Sentinel trigger",
			"consecutiveFastPaths", cluster.Status.Drift.ConsecutiveFastPaths,
		)
	}

	var reconcilers []SubReconciler
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen {
		infraMgr := inframanager.NewManagerWithSentinelAdmission(r.parent.Client, r.parent.Scheme, r.parent.OperatorNamespace, r.parent.OIDCIssuer, r.parent.OIDCJWTKeys, sentinelFastPathAllowed)
		reconcilers = append(reconcilers, bluegreen.NewManager(r.parent.Client, r.parent.Scheme, infraMgr))
	} else {
		reconcilers = append(reconcilers, upgrademanager.NewManager(r.parent.Client, r.parent.Scheme))
	}
	reconcilers = append(reconcilers, backupmanager.NewManager(r.parent.Client, r.parent.Scheme))

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

	// Record a successful full reconcile to reset fast-path counters.
	if cluster.Status.Drift == nil {
		cluster.Status.Drift = &openbaov1alpha1.DriftStatus{}
	}
	now := metav1.Now()
	cluster.Status.Drift.LastFullReconcileTime = &now
	cluster.Status.Drift.ConsecutiveFastPaths = 0

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
