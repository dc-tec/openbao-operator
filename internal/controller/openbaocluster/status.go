package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/platform/observability"
)

// patchStatusSSA updates the cluster status using Server-Side Apply.
// SSA eliminates race conditions by having the API server merge changes,
// rather than requiring the client to refresh and merge manually.
// This function patches only the fields owned by the Status controller:
// phase, activeLeader, readyReplicas, currentVersion, conditions, lastBackupTime.
func (r *OpenBaoClusterReconciler) patchStatusSSA(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Create an apply configuration with just the status fields owned by Status controller.
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
			Phase:          cluster.Status.Phase,
			ActiveLeader:   cluster.Status.ActiveLeader,
			ReadyReplicas:  cluster.Status.ReadyReplicas,
			CurrentVersion: cluster.Status.CurrentVersion,
			LastBackupTime: cluster.Status.LastBackupTime,
			Conditions:     cluster.Status.Conditions,
		},
	}

	applyConfig, err := toApplyConfiguration(applyCluster, r.Client)
	if err != nil {
		return fmt.Errorf("failed to convert cluster to ApplyConfiguration: %w", err)
	}

	return r.Status().Apply(ctx, applyConfig,
		client.FieldOwner("openbao-status-controller"),
	)
}

func (r *OpenBaoClusterReconciler) updateStatus(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (ctrl.Result, error) {
	// Capture original state to check for changes (e.g. ReadyReplicas), but NOT for patching merge.
	original := cluster.DeepCopy()

	// Set TLSReady early (evaluated separately from clusterState).
	r.setAPIServerNetworkReadyCondition(ctx, cluster)
	r.setTLSReadyCondition(ctx, cluster)
	r.setACMEIntegrationReadyCondition(ctx, cluster)
	r.setACMECacheReadyCondition(ctx, cluster)
	r.setGatewayIntegrationReadyCondition(ctx, cluster)
	r.setBackupConfigurationReadyCondition(ctx, cluster)
	r.setCloudUnsealIdentityReadyCondition(ctx, cluster)

	// 1. Gather all observed state (API calls).
	state, err := appopenbaocluster.GatherStatusState(ctx, logger, r.statusDependencies(), cluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	observedVersion := appopenbaocluster.ObservedVersionFromPods(state)

	// 2. Compute and set all conditions (pure logic).
	now := metav1.Now()
	admissionStatus, refreshErr := r.ensureAdmissionStatusFresh(ctx)
	if refreshErr != nil {
		logger.Info("Failed to refresh admission dependency status during status reconciliation", "error", refreshErr)
	}
	if admissionStatus == nil {
		admissionStatus = r.currentAdmissionStatus()
	}
	applyAllConditions(cluster, state, admissionStatus, now)

	// 3. Update status fields (computed locally).
	cluster.Status.ReadyReplicas = state.ReadyReplicas
	cluster.Status.ActiveLeader = state.LeaderName
	cluster.Status.Phase = computePhase(state)

	appopenbaocluster.ReconcileCurrentVersion(logger, cluster, state, observedVersion)
	appopenbaocluster.MaybeAdvanceCurrentVersionForBlueGreen(logger, cluster, observedVersion)
	// Rolling upgrade completion is finalized by the AdminOps rolling manager.
	// The status controller must not independently advance CurrentVersion for rolling,
	// otherwise it can race with in-progress partitioned rollouts.

	// Update per-cluster metrics.
	clusterMetrics := observability.NewClusterMetrics(cluster.Namespace, cluster.Name)
	clusterMetrics.SetReadyReplicas(state.ReadyReplicas)
	clusterMetrics.SetPhase(cluster.Status.Phase)

	if appopenbaocluster.ShouldWarnSelfInitDisabled(cluster) {
		logger.Info("SECURITY WARNING: SelfInit is disabled - root token will be stored in Secret",
			"cluster_namespace", cluster.Namespace,
			"cluster_name", cluster.Name,
			"secret_name", cluster.Name+"-root-token")
	}

	// 4. Persist status (single API call via SSA).
	if err := r.patchStatusSSA(ctx, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster",
		"readyReplicas", state.ReadyReplicas,
		"phase", cluster.Status.Phase,
		"currentVersion", cluster.Status.CurrentVersion)

	// 5. Determine requeue.
	return r.determineStatusRequeue(logger, state, original, cluster), nil
}
