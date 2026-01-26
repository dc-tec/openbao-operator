package openbaocluster

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	backupmanager "github.com/dc-tec/openbao-operator/internal/backup"
	certmanager "github.com/dc-tec/openbao-operator/internal/certs"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	inframanager "github.com/dc-tec/openbao-operator/internal/infra"
	initmanager "github.com/dc-tec/openbao-operator/internal/init"
	"github.com/dc-tec/openbao-operator/internal/kube"
	"github.com/dc-tec/openbao-operator/internal/raft"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
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

// autopilotConfigReconciler reconciles Raft Autopilot configuration for initialized clusters.
// This handles Day 2 operations like scaling replicas or changing autopilot settings.
type autopilotConfigReconciler struct {
	raftManager *raft.Manager
	recorder    events.EventRecorder
}

// Reconcile reconciles the Raft Autopilot configuration for an initialized cluster.
func (r *autopilotConfigReconciler) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	if err := r.raftManager.ReconcileAutopilotConfig(ctx, logger, cluster); err != nil {
		if operatorerrors.IsTransient(err) {
			logger.V(1).Info("Transient error reconciling autopilot config; will retry", "error", err)
			return recon.Result{RequeueAfter: constants.RequeueShort}, nil
		}

		// Check if this is a permanent prerequisites missing error
		if operatorerrors.IsPermanent(err) &&
			errors.Is(err, operatorerrors.ErrPermanentPrerequisitesMissing) {
			// Emit Warning event with actionable guidance
			// Note: SelfInit only works during initial startup, so if cluster is initialized,
			// users must configure JWT manually via API/CLI or configure autopilot in CRD
			var eventMsg string
			if cluster.Status.Initialized {
				eventMsg = "Autopilot configuration requires JWT authentication. " +
					"Since the cluster is already initialized, SelfInit is no longer available. " +
					"Manually configure JWT authentication via OpenBao API/CLI, " +
					"or manually configure autopilot settings in spec.configuration.raft.autopilot. " +
					"Error: %v"
			} else {
				eventMsg = "Autopilot configuration requires JWT authentication. " +
					"Enable JWT auth via spec.selfInit.oidc.enabled: true or configure JWT via self-init requests during initialization. " +
					"Alternatively, manually configure autopilot settings in spec.configuration.raft.autopilot. " +
					"Error: %v"
			}
			r.recorder.Eventf(cluster, nil, corev1.EventTypeWarning,
				"AutopilotConfigJWTPrerequisitesMissing", "",
				eventMsg, err)
			logger.Error(err, "Failed to reconcile autopilot config (permanent error - requires user intervention)")
			return recon.Result{}, nil
		}

		// Other non-transient errors
		logger.Error(err, "Failed to reconcile autopilot config (non-transient)")
		return recon.Result{}, nil
	}

	return recon.Result{}, nil
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

// patchWorkloadOwnedFields patches only the status fields owned by the Workload controller.
// This uses a controller-specific field owner to ensure proper SSA ownership tracking.
// Owned fields: initialized, selfInitialized, workload
func patchWorkloadOwnedFields(ctx context.Context, c client.Client, logger logr.Logger, original *openbaov1alpha1.OpenBaoCluster, cluster *openbaov1alpha1.OpenBaoCluster, reason string) error {
	if original == nil || cluster == nil {
		return nil
	}

	// Check if any owned field changed
	if original.Status.Initialized == cluster.Status.Initialized &&
		original.Status.SelfInitialized == cluster.Status.SelfInitialized &&
		reflect.DeepEqual(original.Status.Workload, cluster.Status.Workload) {
		return nil
	}

	// Ensure Workload is not nil to avoid null serialization
	workload := cluster.Status.Workload
	if workload == nil {
		workload = &openbaov1alpha1.WorkloadControllerStatus{}
	}

	applyCluster := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:     cluster.Status.Initialized,
			SelfInitialized: cluster.Status.SelfInitialized,
			Workload:        workload,
		},
	}

	applyConfig, err := kube.ToApplyConfiguration(applyCluster, c)
	if err != nil {
		return fmt.Errorf("failed to convert cluster to ApplyConfiguration: %w", err)
	}

	if err := c.Status().Apply(ctx, applyConfig, client.FieldOwner("openbao-workload-controller")); err != nil {
		return fmt.Errorf("failed to patch workload status (%s) for OpenBaoCluster %s/%s: %w", reason, cluster.Namespace, cluster.Name, err)
	}
	logger.V(1).Info("Patched OpenBaoCluster workload status (SSA)", "reason", reason, "fieldOwner", "openbao-workload-controller")
	return nil
}

// patchAdminOpsOwnedFields patches only the status fields owned by the AdminOps controller.
// This uses a controller-specific field owner to ensure proper SSA ownership tracking.
// Owned fields: upgrade, blueGreen, backup, operationLock, breakGlass, adminOps
func patchAdminOpsOwnedFields(ctx context.Context, c client.Client, logger logr.Logger, original *openbaov1alpha1.OpenBaoCluster, cluster *openbaov1alpha1.OpenBaoCluster, reason string) error {
	if original == nil || cluster == nil {
		return nil
	}

	// Check if any owned field changed
	if reflect.DeepEqual(original.Status.Upgrade, cluster.Status.Upgrade) &&
		reflect.DeepEqual(original.Status.BlueGreen, cluster.Status.BlueGreen) &&
		reflect.DeepEqual(original.Status.Backup, cluster.Status.Backup) &&
		reflect.DeepEqual(original.Status.OperationLock, cluster.Status.OperationLock) &&
		reflect.DeepEqual(original.Status.BreakGlass, cluster.Status.BreakGlass) &&
		reflect.DeepEqual(original.Status.AdminOps, cluster.Status.AdminOps) {
		return nil
	}

	// Ensure AdminOps is not nil to avoid null serialization
	adminOps := cluster.Status.AdminOps
	if adminOps == nil {
		adminOps = &openbaov1alpha1.AdminOpsControllerStatus{}
	}

	applyCluster := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Upgrade:       cluster.Status.Upgrade,
			BlueGreen:     cluster.Status.BlueGreen,
			Backup:        cluster.Status.Backup,
			OperationLock: cluster.Status.OperationLock,
			BreakGlass:    cluster.Status.BreakGlass,
			AdminOps:      adminOps,
		},
	}

	applyConfig, err := kube.ToApplyConfiguration(applyCluster, c)
	if err != nil {
		return fmt.Errorf("failed to convert cluster to ApplyConfiguration: %w", err)
	}

	if err := c.Status().Apply(ctx, applyConfig, client.FieldOwner("openbao-adminops-controller")); err != nil {
		return fmt.Errorf("failed to patch adminops status (%s) for OpenBaoCluster %s/%s: %w", reason, cluster.Namespace, cluster.Name, err)
	}
	logger.V(1).Info("Patched OpenBaoCluster adminops status (SSA)", "reason", reason, "fieldOwner", "openbao-adminops-controller")
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
			platform:              r.parent.Platform,
			smartClientConfig:     r.parent.SmartClientConfig,
		},
		&storageReconciler{
			client:   r.parent.Client,
			recorder: r.parent.Recorder,
		},
		&storageResizeRestartReconciler{
			client:            r.parent.Client,
			apiReader:         r.parent.APIReader,
			recorder:          r.parent.Recorder,
			smartClientConfig: r.parent.SmartClientConfig,
		},
	}
	if r.parent.InitManager != nil {
		reconcilers = append(reconcilers, r.parent.InitManager)
		// Add autopilot config reconciler for Day 2 operations
		// Get raft manager from InitManager if it's the concrete type
		var raftMgr *raft.Manager
		if initMgr, ok := r.parent.InitManager.(*initmanager.Manager); ok {
			raftMgr = initMgr.RaftManager()
		}
		if raftMgr != nil {
			reconcilers = append(reconcilers, &autopilotConfigReconciler{
				raftManager: raftMgr,
				recorder:    r.parent.Recorder,
			})
		}
	}

	if result, err := r.runWorkloadReconcilers(ctx, logger, original, cluster, reconcilers); err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	// Clear previous workload error after a successful reconcile.
	cluster.Status.Workload.LastError = nil

	if err := patchWorkloadOwnedFields(ctx, r.parent.Client, logger, original, cluster, "workload-complete"); err != nil {
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
			if statusErr := patchWorkloadOwnedFields(ctx, r.parent.Client, logger, original, cluster, "workload-error"); statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			if handled, ok := workloadResultForError(err, cluster.Status.Workload.LastError); ok {
				return handled, nil
			}

			return ctrl.Result{}, err
		}

		if result.RequeueAfter > 0 {
			if statusErr := patchWorkloadOwnedFields(ctx, r.parent.Client, logger, original, cluster, "workload-requeue"); statusErr != nil {
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
		case ReasonStorageInvalidSize, ReasonStorageShrinkNotSupported, ReasonStorageResizeNotSupported, ReasonStorageClassChangeNotSupported, ReasonStorageRestartRequired:
			// Permanent configuration issue; wait for user changes rather than hot-looping.
			return ctrl.Result{}, true
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
	infraMgr := inframanager.NewManagerWithReader(r.parent.Client, r.parent.APIReader, r.parent.Scheme, r.parent.OperatorNamespace, r.parent.OIDCIssuer, r.parent.OIDCJWTKeys, r.parent.Platform)
	// Blue/green upgrade strategy
	reconcilers = append(reconcilers, bluegreen.NewManager(r.parent.Client, r.parent.Scheme, infraMgr, r.parent.SmartClientConfig, r.parent.ImageVerifier, r.parent.OperatorImageVerifier, r.parent.Platform))

	// Rolling upgrade strategy
	reconcilers = append(reconcilers, rollingupgrade.NewManager(r.parent.Client, r.parent.Scheme, r.parent.SmartClientConfig, r.parent.OperatorImageVerifier, r.parent.Platform))

	// Backup
	reconcilers = append(reconcilers, backupmanager.NewManager(r.parent.Client, r.parent.Scheme, r.parent.SmartClientConfig, r.parent.OperatorImageVerifier, r.parent.Platform))

	for _, rec := range reconcilers {
		result, err := rec.Reconcile(ctx, logger, cluster)
		if err != nil {
			cluster.Status.AdminOps.LastError = controllerErrorStatus(err)

			if statusErr := patchAdminOpsOwnedFields(ctx, r.parent.Client, logger, original, cluster, "adminops-error"); statusErr != nil {
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
			if statusErr := patchAdminOpsOwnedFields(ctx, r.parent.Client, logger, original, cluster, "adminops-requeue"); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{RequeueAfter: result.RequeueAfter}, nil
		}
	}

	// Clear previous adminops error after a successful reconcile.
	cluster.Status.AdminOps.LastError = nil

	if err := patchAdminOpsOwnedFields(ctx, r.parent.Client, logger, original, cluster, "adminops-complete"); err != nil {
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
