package openbaocluster

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	certmanager "github.com/dc-tec/openbao-operator/internal/certs"
	"github.com/dc-tec/openbao-operator/internal/constants"
	controllerdeps "github.com/dc-tec/openbao-operator/internal/controller/openbaocluster/deps"
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

func reconcileErrorReason(err error) string {
	return appopenbaocluster.ReconcileErrorReason(err)
}

func (r *OpenBaoClusterReconciler) loggerFor(ctx context.Context, _ ctrl.Request, _ string) logr.Logger {
	// controller-runtime already injects controller/reconcile identifiers, including
	// namespace/name for the reconciled object.
	return log.FromContext(ctx)
}

func (r *openBaoClusterWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	start := time.Now()
	reconcileMetrics := controllerdeps.NewReconcileMetrics(req.Namespace, req.Name, constants.ControllerNameOpenBaoClusterWorkload)
	recordedError := false
	recordError := func(e error) {
		if e == nil {
			return
		}
		reconcileMetrics.IncrementError(reconcileErrorReason(e))
		recordedError = true
	}
	defer func() {
		reconcileMetrics.ObserveDuration(time.Since(start).Seconds())
		if err != nil && !recordedError {
			recordError(err)
		}
	}()

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

	result, err = r.reconcileCluster(ctx, logger, cluster, recordError)
	return result, err
}

func shouldSkipWorkloadReconcile(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster == nil ||
		!cluster.DeletionTimestamp.IsZero() ||
		cluster.Spec.Paused ||
		cluster.Spec.Profile == ""
}

func (r *openBaoClusterWorkloadReconciler) reconcileCluster(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	recordError func(error),
) (ctrl.Result, error) {
	original := cluster.DeepCopy()
	reconcilers := []appopenbaocluster.SubReconciler{
		certmanager.NewManagerWithReloader(r.parent.Client, r.parent.Scheme, r.parent.TLSReload),
		appopenbaocluster.NewInfraReconciler(
			appopenbaocluster.InfraDependencies{
				Client:                r.parent.Client,
				APIReader:             r.parent.APIReader,
				Scheme:                r.parent.Scheme,
				RestConfig:            r.parent.RestConfig,
				OperatorNamespace:     r.parent.OperatorNamespace,
				OIDCIssuer:            r.parent.OIDCIssuer,
				OIDCJWTKeys:           r.parent.OIDCJWTKeys,
				OperatorImageVerifier: r.parent.OperatorImageVerifier,
				VerifyImageFunc:       r.parent.verifyImageRef,
				Recorder:              r.parent.Recorder,
				Platform:              r.parent.Platform,
				SmartClientConfig:     r.parent.SmartClientConfig,
			},
			appopenbaocluster.InfraReasonPolicy{
				GatewayAPIMissing:                   ReasonGatewayAPIMissing,
				PrerequisitesMissing:                ReasonPrerequisitesMissing,
				ACMEDomainNotResolvable:             ReasonACMEDomainNotResolvable,
				ACMEGatewayNotConfiguredPassthrough: ReasonACMEGatewayNotConfiguredForPassthrough,
				ImageVerificationFailed:             constants.ReasonImageVerificationFailed,
				InitContainerImageVerification:      constants.ReasonInitContainerImageVerificationFailed,
			},
		),
		appopenbaocluster.NewStorageReconciler(
			appopenbaocluster.StorageDependencies{
				Client:   r.parent.Client,
				Recorder: r.parent.Recorder,
			},
			appopenbaocluster.StorageReasonPolicy{
				InvalidSize:             ReasonStorageInvalidSize,
				ShrinkNotSupported:      ReasonStorageShrinkNotSupported,
				ResizeNotSupported:      ReasonStorageResizeNotSupported,
				StorageClassChangeError: ReasonStorageClassChangeNotSupported,
				RestartRequired:         ReasonStorageRestartRequired,
			},
		),
		appopenbaocluster.NewStorageResizeRestartReconciler(
			appopenbaocluster.StorageResizeRestartDependencies{
				Client:            r.parent.Client,
				APIReader:         r.parent.APIReader,
				Recorder:          r.parent.Recorder,
				SmartClientConfig: r.parent.SmartClientConfig,
			},
			appopenbaocluster.StorageReasonPolicy{
				InvalidSize:             ReasonStorageInvalidSize,
				ShrinkNotSupported:      ReasonStorageShrinkNotSupported,
				ResizeNotSupported:      ReasonStorageResizeNotSupported,
				StorageClassChangeError: ReasonStorageClassChangeNotSupported,
				RestartRequired:         ReasonStorageRestartRequired,
			},
		),
	}
	reconcilers = appopenbaocluster.AppendInitAndAutopilotReconcilers(reconcilers, r.parent.InitManager, r.parent.Recorder, constants.RequeueShort)

	policy := appopenbaocluster.WorkloadResultPolicy{
		PrerequisitesMissingReason: ReasonPrerequisitesMissing,
		GatewayAPIMissingReason:    ReasonGatewayAPIMissing,
		RequeueShort:               constants.RequeueShort,
		RequeueSafetyNetBase:       constants.RequeueSafetyNetBase,
		RequeueSafetyNetJitter:     constants.RequeueSafetyNetJitter,
		PermanentConfigurationReason: map[string]struct{}{
			ReasonStorageInvalidSize:             {},
			ReasonStorageShrinkNotSupported:      {},
			ReasonStorageResizeNotSupported:      {},
			ReasonStorageClassChangeNotSupported: {},
			ReasonStorageRestartRequired:         {},
		},
	}

	return appopenbaocluster.RunWorkloadReconcilers(
		ctx,
		r.parent.Client,
		logger,
		original,
		cluster,
		reconcilers,
		recordError,
		policy,
	)
}

func (r *openBaoClusterAdminOpsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	start := time.Now()
	reconcileMetrics := controllerdeps.NewReconcileMetrics(req.Namespace, req.Name, constants.ControllerNameOpenBaoClusterAdminOps)
	recordedError := false
	recordError := func(e error) {
		if e == nil {
			return
		}
		reconcileMetrics.IncrementError(reconcileErrorReason(e))
		recordedError = true
	}
	defer func() {
		reconcileMetrics.ObserveDuration(time.Since(start).Seconds())
		if err != nil && !recordedError {
			recordError(err)
		}
	}()

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
	return appopenbaocluster.ReconcileAdminOps(ctx, logger, appopenbaocluster.AdminOpsDependencies{
		Client:                r.parent.Client,
		APIReader:             r.parent.APIReader,
		Scheme:                r.parent.Scheme,
		OperatorNamespace:     r.parent.OperatorNamespace,
		OIDCIssuer:            r.parent.OIDCIssuer,
		OIDCJWTKeys:           r.parent.OIDCJWTKeys,
		SmartClientConfig:     r.parent.SmartClientConfig,
		ImageVerifier:         r.parent.ImageVerifier,
		OperatorImageVerifier: r.parent.OperatorImageVerifier,
		RequeueShort:          constants.RequeueShort,
		Platform:              r.parent.Platform,
	}, original, cluster, recordError)
}

func (r *openBaoClusterStatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	start := time.Now()
	reconcileMetrics := controllerdeps.NewReconcileMetrics(req.Namespace, req.Name, constants.ControllerNameOpenBaoClusterStatus)
	recordedError := false
	recordError := func(e error) {
		if e == nil {
			return
		}
		reconcileMetrics.IncrementError(reconcileErrorReason(e))
		recordedError = true
	}
	defer func() {
		reconcileMetrics.ObserveDuration(time.Since(start).Seconds())
		if err != nil && !recordedError {
			recordError(err)
		}
	}()

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
