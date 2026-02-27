package openbaocluster

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	initmanager "github.com/dc-tec/openbao-operator/internal/init"
	initmanagerport "github.com/dc-tec/openbao-operator/internal/port/initmanager"
	"github.com/dc-tec/openbao-operator/internal/raft"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
)

// WorkloadResultPolicy configures known workload error handling behavior.
type WorkloadResultPolicy struct {
	PrerequisitesMissingReason   string
	GatewayAPIMissingReason      string
	PermanentConfigurationReason map[string]struct{}
}

// AppendInitAndAutopilotReconcilers appends init and optional autopilot reconcilers.
func AppendInitAndAutopilotReconcilers(reconcilers []SubReconciler, initMgr initmanagerport.Manager, recorder events.EventRecorder) []SubReconciler {
	if initMgr == nil {
		return reconcilers
	}

	reconcilers = append(reconcilers, initMgr)

	// Add autopilot config reconciler for Day 2 operations only when the concrete
	// init manager exposes a raft manager.
	var raftMgr *raft.Manager
	if typedInitMgr, ok := initMgr.(*initmanager.Manager); ok {
		raftMgr = typedInitMgr.RaftManager()
	}
	if raftMgr != nil {
		reconcilers = append(reconcilers, &autopilotConfigReconciler{
			raftManager: raftMgr,
			recorder:    recorder,
		})
	}

	return reconcilers
}

// RunWorkloadReconcilers executes workload orchestration with consistent status patching.
func RunWorkloadReconcilers(
	ctx context.Context,
	c client.Client,
	logger logr.Logger,
	original *openbaov1alpha1.OpenBaoCluster,
	cluster *openbaov1alpha1.OpenBaoCluster,
	reconcilers []SubReconciler,
	recordError ErrorRecorder,
	policy WorkloadResultPolicy,
) (ctrl.Result, error) {
	if cluster.Status.Workload == nil {
		cluster.Status.Workload = &openbaov1alpha1.WorkloadControllerStatus{}
	}

	for _, rec := range reconcilers {
		result, err := rec.Reconcile(ctx, logger, cluster)
		if err != nil {
			if recordError != nil {
				recordError(err)
			}
			cluster.Status.Workload.LastError = controllerErrorStatus(err)

			// Persist status changes before returning to avoid losing in-memory updates.
			if statusErr := PatchWorkloadOwnedFields(ctx, c, logger, original, cluster, "workload-error"); statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			if handled, ok := workloadResultForError(err, cluster.Status.Workload.LastError, policy); ok {
				return handled, nil
			}

			return ctrl.Result{}, err
		}

		if result.RequeueAfter > 0 {
			if statusErr := PatchWorkloadOwnedFields(ctx, c, logger, original, cluster, "workload-requeue"); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{RequeueAfter: result.RequeueAfter}, nil
		}
	}

	// Clear previous workload error after a successful reconcile.
	cluster.Status.Workload.LastError = nil
	if err := PatchWorkloadOwnedFields(ctx, c, logger, original, cluster, "workload-complete"); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func workloadResultForError(
	err error,
	lastError *openbaov1alpha1.ControllerErrorStatus,
	policy WorkloadResultPolicy,
) (ctrl.Result, bool) {
	if lastError != nil {
		switch lastError.Reason {
		case policy.PrerequisitesMissingReason:
			return ctrl.Result{RequeueAfter: constants.RequeueShort}, true
		case policy.GatewayAPIMissingReason:
			jitterNanos := time.Now().UnixNano() % int64(constants.RequeueSafetyNetJitter)
			requeueAfter := constants.RequeueSafetyNetBase + time.Duration(jitterNanos)
			return ctrl.Result{RequeueAfter: requeueAfter}, true
		default:
			if _, ok := policy.PermanentConfigurationReason[lastError.Reason]; ok {
				// Permanent configuration issue; wait for user changes rather than hot-looping.
				return ctrl.Result{}, true
			}
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

// autopilotConfigReconciler reconciles Raft Autopilot configuration for initialized clusters.
// This handles Day 2 operations like scaling replicas or changing autopilot settings.
type autopilotConfigReconciler struct {
	raftManager *raft.Manager
	recorder    events.EventRecorder
}

// Reconcile reconciles the Raft Autopilot configuration for an initialized cluster.
func (r *autopilotConfigReconciler) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	// During upgrades we expect transient API instability (pod restarts, leader changes).
	// Autopilot reconciliation uses authenticated API calls against the stable public
	// Service and can create unnecessary load and noisy failures while the cluster
	// is rolling. Skip it until the upgrade finishes.
	if cluster != nil && cluster.Status.Upgrade != nil {
		logger.V(1).Info("Skipping autopilot config reconciliation during upgrade")
		return recon.Result{}, nil
	}

	if err := r.raftManager.ReconcileAutopilotConfig(ctx, logger, cluster); err != nil {
		if operatorerrors.IsTransient(err) {
			logger.V(1).Info("Transient error reconciling autopilot config; will retry", "error", err)
			return recon.Result{RequeueAfter: constants.RequeueShort}, nil
		}

		// Check if this is a permanent prerequisites missing error.
		if operatorerrors.IsPermanent(err) &&
			errors.Is(err, operatorerrors.ErrPermanentPrerequisitesMissing) {
			// Emit Warning event with actionable guidance
			// Note: SelfInit only works during initial startup, so if cluster is initialized,
			// users must configure JWT manually via API/CLI or configure autopilot in CRD.
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
				"AutopilotConfigJWTPrerequisitesMissing", "AutopilotConfigJWTPrerequisitesMissing",
				eventMsg, err)
			logger.Error(err, "Failed to reconcile autopilot config (permanent error - requires user intervention)")
			return recon.Result{}, nil
		}

		// Other non-transient errors.
		logger.Error(err, "Failed to reconcile autopilot config (non-transient)")
		return recon.Result{}, nil
	}

	return recon.Result{}, nil
}
