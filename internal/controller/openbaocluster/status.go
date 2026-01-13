package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	controllerutil "github.com/dc-tec/openbao-operator/internal/controller"
	"github.com/dc-tec/openbao-operator/internal/status"
)

func (r *OpenBaoClusterReconciler) updateStatusForPaused(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Capture original state for status patching to avoid optimistic locking conflicts
	original := cluster.DeepCopy()

	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	gen := cluster.Generation
	conditions := &cluster.Status.Conditions

	status.Unknown(conditions, gen, string(openbaov1alpha1.ConditionAvailable), constants.ReasonPaused, "Reconciliation is paused; availability is not being evaluated")
	status.False(conditions, gen, string(openbaov1alpha1.ConditionDegraded), constants.ReasonPaused, "Cluster is paused; no new degradation has been evaluated")
	status.Unknown(conditions, gen, string(openbaov1alpha1.ConditionTLSReady), constants.ReasonPaused, "TLS readiness is not being evaluated while reconciliation is paused")

	if err := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to update status for paused OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for paused OpenBaoCluster")

	return nil
}

func (r *OpenBaoClusterReconciler) updateStatusForProfileNotSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	original := cluster.DeepCopy()

	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	gen := cluster.Generation
	conditions := &cluster.Status.Conditions

	status.False(conditions, gen, string(openbaov1alpha1.ConditionAvailable), ReasonProfileNotSet, "spec.profile must be explicitly set to Hardened or Development; reconciliation is blocked until set")
	status.True(conditions, gen, string(openbaov1alpha1.ConditionDegraded), ReasonProfileNotSet, "spec.profile is not set; defaults may be inappropriate for production and could lead to insecure deployment")
	status.Unknown(conditions, gen, string(openbaov1alpha1.ConditionTLSReady), ReasonProfileNotSet, "TLS readiness is not being evaluated until spec.profile is set")
	status.False(conditions, gen, string(openbaov1alpha1.ConditionProductionReady), ReasonProfileNotSet, "Cluster cannot be considered production-ready until spec.profile is explicitly set")

	if err := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to update status for missing profile on OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster missing profile")
	return nil
}

func (r *OpenBaoClusterReconciler) updateStatus(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (ctrl.Result, error) {
	// Capture original state for status patching to avoid optimistic locking conflicts
	original := cluster.DeepCopy()

	// Set TLSReady early (evaluated separately from clusterState)
	r.setTLSReadyCondition(ctx, cluster)

	// 1. Gather all observed state (API calls)
	state, err := r.gatherClusterState(ctx, logger, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 2. Compute and set all conditions (pure logic)
	now := metav1.Now()
	applyAllConditions(cluster, state, r.AdmissionStatus, now)

	// 3. Update status fields
	cluster.Status.ReadyReplicas = state.ReadyReplicas
	cluster.Status.ActiveLeader = state.LeaderName
	cluster.Status.Phase = computePhase(state)

	// Set initial CurrentVersion if empty (first reconcile after initialization)
	if cluster.Status.CurrentVersion == "" && cluster.Status.Initialized {
		cluster.Status.CurrentVersion = cluster.Spec.Version
		logger.Info("Set initial CurrentVersion", "version", cluster.Spec.Version)
	}

	// Update per-cluster metrics
	clusterMetrics := controllerutil.NewClusterMetrics(cluster.Namespace, cluster.Name)
	clusterMetrics.SetReadyReplicas(state.ReadyReplicas)
	clusterMetrics.SetPhase(cluster.Status.Phase)

	// SECURITY: Warn when SelfInit is disabled - the operator will store the root token.
	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		logger.Info("SECURITY WARNING: SelfInit is disabled - root token will be stored in Secret",
			"cluster_namespace", cluster.Namespace,
			"cluster_name", cluster.Name,
			"secret_name", cluster.Name+"-root-token")
	}

	// 4. Persist status (single API call)
	if err := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster",
		"readyReplicas", state.ReadyReplicas,
		"phase", cluster.Status.Phase,
		"currentVersion", cluster.Status.CurrentVersion)

	// 5. Determine requeue
	previousReadyReplicas := original.Status.ReadyReplicas
	readyReplicasChanged := state.ReadyReplicas != previousReadyReplicas

	if state.StatusStale {
		logger.V(1).Info("StatefulSet status may be stale; requeuing to check status")
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	if !state.Available && state.ReadyReplicas > 0 {
		logger.V(1).Info("Not all replicas are ready; requeuing to check status",
			"readyReplicas", state.ReadyReplicas,
			"desiredReplicas", cluster.Spec.Replicas)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	if state.Available && readyReplicasChanged {
		logger.V(1).Info("All replicas became ready; requeuing once to ensure status is persisted",
			"readyReplicas", state.ReadyReplicas,
			"previousReadyReplicas", previousReadyReplicas)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	return ctrl.Result{}, nil
}

// setTLSReadyCondition evaluates and sets the TLSReady condition.
func (r *OpenBaoClusterReconciler) setTLSReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	gen := cluster.Generation
	conditions := &cluster.Status.Conditions
	condType := string(openbaov1alpha1.ConditionTLSReady)

	if !cluster.Spec.TLS.Enabled {
		status.True(conditions, gen, condType, "Disabled", "TLS is disabled")
		return
	}

	tlsMode := cluster.Spec.TLS.Mode
	if tlsMode == "" {
		tlsMode = openbaov1alpha1.TLSModeOperatorManaged
	}

	if tlsMode == openbaov1alpha1.TLSModeACME {
		status.Unknown(conditions, gen, condType, constants.ReasonUnknown, "TLS is managed by OpenBao via ACME; the operator does not evaluate certificate readiness")
		return
	}

	// Check for server TLS secret
	serverSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + constants.SuffixTLSServer,
	}, serverSecret); err != nil {
		if apierrors.IsNotFound(err) {
			status.False(conditions, gen, condType, ReasonTLSSecretMissing, "Server TLS Secret is not present yet")
			return
		}
		// For other errors, mark as unknown
		status.Unknown(conditions, gen, condType, constants.ReasonUnknown, "Failed to get TLS secret")
		return
	}

	hasCert := len(serverSecret.Data["tls.crt"]) > 0
	hasKey := len(serverSecret.Data["tls.key"]) > 0
	if hasCert && hasKey {
		status.True(conditions, gen, condType, constants.ReasonReady, "TLS assets are provisioned")
	} else {
		status.False(conditions, gen, condType, ReasonTLSSecretInvalid, "Server TLS Secret is missing required keys (tls.crt/tls.key)")
	}
}
