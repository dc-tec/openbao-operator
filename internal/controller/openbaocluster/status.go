package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	controllerutil "github.com/dc-tec/openbao-operator/internal/controller"
)

// patchStatusSSA updates the cluster status using Server-Side Apply.
// SSA eliminates race conditions by having the API server merge changes,
// rather than requiring the client to refresh and merge manually.
func (r *OpenBaoClusterReconciler) patchStatusSSA(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
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

	return r.Status().Patch(ctx, applyCluster, client.Apply,
		client.FieldOwner("openbao-cluster-controller"),
		client.ForceOwnership,
	)
}

func (r *OpenBaoClusterReconciler) updateStatusForPaused(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	now := metav1.Now()

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAvailable),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             constants.ReasonPaused,
		Message:            "Reconciliation is paused; availability is not being evaluated",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             constants.ReasonPaused,
		Message:            "Cluster is paused; no new degradation has been evaluated",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             constants.ReasonPaused,
		Message:            "TLS readiness is not being evaluated while reconciliation is paused",
	})

	if err := r.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status for paused OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for paused OpenBaoCluster")

	return nil
}

func (r *OpenBaoClusterReconciler) updateStatusForProfileNotSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	now := metav1.Now()
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAvailable),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "spec.profile must be explicitly set to Hardened or Development; reconciliation is blocked until set",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "spec.profile is not set; defaults may be inappropriate for production and could lead to insecure deployment",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "TLS readiness is not being evaluated until spec.profile is set",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionProductionReady),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "Cluster cannot be considered production-ready until spec.profile is explicitly set",
	})

	if err := r.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status for missing profile on OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster missing profile")
	return nil
}

func (r *OpenBaoClusterReconciler) updateStatus(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (ctrl.Result, error) {
	// Capture original state to check for changes (e.g. ReadyReplicas), but NOT for patching merge
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

	// 4. Persist status (single API call via SSA)
	if err := r.patchStatusSSA(ctx, cluster); err != nil {
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
	now := metav1.Now()

	if !cluster.Spec.TLS.Enabled {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             "Disabled",
			Message:            "TLS is disabled",
		})
		return
	}

	tlsMode := cluster.Spec.TLS.Mode
	if tlsMode == "" {
		tlsMode = openbaov1alpha1.TLSModeOperatorManaged
	}

	if tlsMode == openbaov1alpha1.TLSModeACME {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             constants.ReasonUnknown,
			Message:            "TLS is managed by OpenBao via ACME; the operator does not evaluate certificate readiness",
		})
		return
	}

	// Check for server TLS secret
	serverSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + constants.SuffixTLSServer,
	}, serverSecret); err != nil {
		if apierrors.IsNotFound(err) {
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionTLSReady),
				Status:             metav1.ConditionFalse,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             ReasonTLSSecretMissing,
				Message:            "Server TLS Secret is not present yet",
			})
			return
		}
		// For other errors, mark as unknown
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             constants.ReasonUnknown,
			Message:            "Failed to get TLS secret",
		})
		return
	}

	hasCert := len(serverSecret.Data["tls.crt"]) > 0
	hasKey := len(serverSecret.Data["tls.key"]) > 0
	if hasCert && hasKey {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             constants.ReasonReady,
			Message:            "TLS assets are provisioned",
		})
	} else {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonTLSSecretInvalid,
			Message:            "Server TLS Secret is missing required keys (tls.crt/tls.key)",
		})
	}
}
