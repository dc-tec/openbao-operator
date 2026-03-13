package openbaocluster

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	initmanagerport "github.com/dc-tec/openbao-operator/internal/port/initmanager"
)

// WorkloadResultPolicy configures known workload error handling behavior.
type WorkloadResultPolicy struct {
	PrerequisitesMissingReason   string
	GatewayAPIMissingReason      string
	PermanentConfigurationReason map[string]struct{}
	RequeueShort                 time.Duration
	RequeueSafetyNetBase         time.Duration
	RequeueSafetyNetJitter       time.Duration
}

// AppendInitAndAutopilotReconcilers appends init and optional autopilot reconcilers.
func AppendInitAndAutopilotReconcilers(
	reconcilers []SubReconciler,
	initMgr initmanagerport.Manager,
	autopilotRuntime initmanagerport.AutopilotRuntime,
	recorder events.EventRecorder,
	requeueShort time.Duration,
) []SubReconciler {
	if initMgr == nil {
		return reconcilers
	}

	reconcilers = append(reconcilers, initMgr)

	// Add autopilot config reconciler for Day 2 operations when an autopilot runtime is available.
	if autopilotRuntime != nil {
		reconcilers = append(reconcilers, &autopilotConfigReconciler{
			autopilotRuntime: autopilotRuntime,
			recorder:         recorder,
			requeueShort: func() time.Duration {
				if requeueShort > 0 {
					return requeueShort
				}
				return 5 * time.Second
			}(),
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
) (recon.Result, error) {
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
				return recon.Result{}, statusErr
			}

			if handled, ok := workloadResultForError(err, cluster.Status.Workload.LastError, policy); ok {
				return handled, nil
			}

			return recon.Result{}, err
		}

		if result.RequeueAfter > 0 {
			if statusErr := PatchWorkloadOwnedFields(ctx, c, logger, original, cluster, "workload-requeue"); statusErr != nil {
				return recon.Result{}, statusErr
			}
			return recon.Result{RequeueAfter: result.RequeueAfter}, nil
		}
	}

	// Clear previous workload error after a successful reconcile.
	cluster.Status.Workload.LastError = nil
	if err := PatchWorkloadOwnedFields(ctx, c, logger, original, cluster, "workload-complete"); err != nil {
		return recon.Result{}, err
	}

	return recon.Result{}, nil
}

func workloadResultForError(
	err error,
	lastError *openbaov1alpha1.ControllerErrorStatus,
	policy WorkloadResultPolicy,
) (recon.Result, bool) {
	if lastError != nil {
		switch lastError.Reason {
		case policy.PrerequisitesMissingReason:
			return recon.Result{RequeueAfter: workloadRequeueShort(policy)}, true
		case policy.GatewayAPIMissingReason:
			safetyNetJitter := workloadRequeueSafetyNetJitter(policy)
			jitterNanos := time.Now().UnixNano() % int64(safetyNetJitter)
			requeueAfter := workloadRequeueSafetyNetBase(policy) + time.Duration(jitterNanos)
			return recon.Result{RequeueAfter: requeueAfter}, true
		default:
			if _, ok := policy.PermanentConfigurationReason[lastError.Reason]; ok {
				// Permanent configuration issue; wait for user changes rather than hot-looping.
				return recon.Result{}, true
			}
		}
	}

	if operatorerrors.IsTransient(err) {
		shouldRequeue, requeueAfter := operatorerrors.ShouldRequeue(err)
		if shouldRequeue {
			if requeueAfter > 0 {
				return recon.Result{RequeueAfter: requeueAfter}, true
			}
			return recon.Result{RequeueAfter: workloadRequeueShort(policy)}, true
		}
	}

	return recon.Result{}, false
}

// autopilotConfigReconciler reconciles Raft Autopilot configuration for initialized clusters.
// This handles Day 2 operations like scaling replicas or changing autopilot settings.
type autopilotConfigReconciler struct {
	autopilotRuntime initmanagerport.AutopilotRuntime
	recorder         events.EventRecorder
	requeueShort     time.Duration
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

	if err := r.autopilotRuntime.ReconcileAutopilotConfig(ctx, logger, cluster); err != nil {
		if operatorerrors.IsTransient(err) {
			logger.V(1).Info("Transient error reconciling autopilot config; will retry", "error", err)
			return recon.Result{RequeueAfter: r.requeueShort}, nil
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
					"Since this operator-managed cluster is already initialized, SelfInit is no longer available. " +
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

func workloadRequeueShort(policy WorkloadResultPolicy) time.Duration {
	if policy.RequeueShort > 0 {
		return policy.RequeueShort
	}
	return 5 * time.Second
}

func workloadRequeueSafetyNetBase(policy WorkloadResultPolicy) time.Duration {
	if policy.RequeueSafetyNetBase > 0 {
		return policy.RequeueSafetyNetBase
	}
	return 20 * time.Minute
}

func workloadRequeueSafetyNetJitter(policy WorkloadResultPolicy) time.Duration {
	if policy.RequeueSafetyNetJitter > 0 {
		return policy.RequeueSafetyNetJitter
	}
	return 5 * time.Minute
}
