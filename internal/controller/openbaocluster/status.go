package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/platform/observability"
)

// patchStatusSSA updates the cluster status using Server-Side Apply.
// SSA eliminates race conditions by having the API server merge changes,
// rather than requiring the client to refresh and merge manually.
// This function patches only the fields owned by the Status controller:
// observedGeneration, phase, activeLeader, readyReplicas, readReplicas,
// currentVersion, conditions, lastBackupTime.
func (r *OpenBaoClusterReconciler) patchStatusSSA(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	cluster.Status.ObservedGeneration = cluster.Generation
	return appopenbaocluster.PatchStatusOwnedFields(ctx, r.Client, cluster)
}

func (r *OpenBaoClusterReconciler) updateStatus(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (ctrl.Result, error) {
	if r.Applications == nil {
		return ctrl.Result{}, fmt.Errorf("OpenBaoCluster applications are not configured")
	}

	// Capture original state to check for changes (e.g. ReadyReplicas), but NOT for patching merge.
	original := cluster.DeepCopy()

	// Set TLSReady early (evaluated separately from clusterState).
	r.setAPIServerNetworkReadyCondition(ctx, cluster)
	r.setTLSReadyCondition(ctx, cluster)
	r.setACMEIntegrationReadyCondition(ctx, cluster)
	r.setACMECacheReadyCondition(ctx, cluster)
	r.setAuditFileStorageReadyCondition(ctx, cluster)
	r.setGatewayIntegrationReadyCondition(ctx, cluster)
	r.setIngressIntegrationReadyCondition(ctx, cluster)
	r.setBackupConfigurationReadyCondition(ctx, cluster)
	r.setCloudUnsealIdentityReadyCondition(ctx, cluster)

	// 1. Gather all observed state (API calls).
	state, err := r.Applications.GatherStatusState(ctx, logger, cluster)
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
	cluster.Status.ReadReplicas = buildReadReplicaStatus(cluster, state)
	cluster.Status.ActiveLeader = state.LeaderName
	cluster.Status.Phase = computePhase(state)

	appopenbaocluster.ReconcileCurrentVersion(logger, cluster, state, observedVersion)
	appopenbaocluster.MaybeAdvanceCurrentVersionForBlueGreen(logger, cluster, observedVersion)
	// Rolling manager finalization only clears status.upgrade. The status
	// controller is the sole writer of CurrentVersion and advances it after
	// rollout convergence is observed from workload state.

	// Update per-cluster metrics.
	clusterMetrics := observability.NewClusterMetrics(cluster.Namespace, cluster.Name)
	clusterMetrics.SetReadyReplicas(state.ReadyReplicas)
	if cluster.Status.ReadReplicas != nil {
		clusterMetrics.SetReadReplicaCounts(
			cluster.Status.ReadReplicas.DesiredReplicas,
			cluster.Status.ReadReplicas.ReadyReplicas,
			cluster.Status.ReadReplicas.RegisteredReplicas,
			cluster.Status.ReadReplicas.HealthyReplicas,
		)
	} else {
		clusterMetrics.SetReadReplicaCounts(0, 0, 0, 0)
	}
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
		"readReplicaReadyReplicas", state.ReadReplicaReadyReplicas,
		"phase", cluster.Status.Phase,
		"currentVersion", cluster.Status.CurrentVersion)

	// 5. Determine requeue.
	return r.determineStatusRequeue(logger, state, original, cluster), nil
}

func buildReadReplicaStatus(cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState) *openbaov1alpha1.ReadReplicaStatus {
	if cluster.Spec.ReadReplicas == nil {
		return nil
	}

	status := &openbaov1alpha1.ReadReplicaStatus{
		DesiredReplicas: cluster.Spec.ReadReplicas.Replicas,
	}

	if state != nil {
		status.ReadyReplicas = state.ReadReplicaReadyReplicas
		status.RegisteredReplicas = state.ReadReplicaRegisteredReplicas
		status.HealthyReplicas = state.ReadReplicaHealthyReplicas
		status.Storage.DesiredPVCs = cluster.Spec.ReadReplicas.Replicas
		status.Storage.BoundPVCs = int32(state.ReadReplicaDataPVCCount)
		switch {
		case len(state.ReadReplicaDataPVCStorageClassNames) == 1:
			status.Storage.StorageClassName = state.ReadReplicaDataPVCStorageClassNames[0]
		case cluster.Spec.ReadReplicas.Storage != nil && cluster.Spec.ReadReplicas.Storage.StorageClassName != nil:
			status.Storage.StorageClassName = *cluster.Spec.ReadReplicas.Storage.StorageClassName
		}
		return status
	}

	status.Storage.DesiredPVCs = cluster.Spec.ReadReplicas.Replicas
	if cluster.Spec.ReadReplicas.Storage != nil && cluster.Spec.ReadReplicas.Storage.StorageClassName != nil {
		status.Storage.StorageClassName = *cluster.Spec.ReadReplicas.Storage.StorageClassName
	}
	return status
}
