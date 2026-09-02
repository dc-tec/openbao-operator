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
// currentVersion, and conditions.
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

	// Set prerequisite conditions before observed-state policy runs.
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

	// 2. Refresh admission state and apply normal status policy.
	now := metav1.Now()
	admissionStatus, refreshErr := r.ensureAdmissionStatusFresh(ctx)
	if refreshErr != nil {
		logger.Info("Failed to refresh admission dependency status during status reconciliation", "error", refreshErr)
	}
	if admissionStatus == nil {
		admissionStatus = r.currentAdmissionStatus()
	}
	policyResult := appopenbaocluster.ApplyStatusPolicy(logger, appopenbaocluster.StatusPolicyInput{
		Original:       original,
		Cluster:        cluster,
		State:          state,
		AdmissionState: admissionStatus,
		Now:            now,
	})
	// Rolling manager finalization only clears status.upgrade. The status
	// controller is the sole writer of CurrentVersion and advances it after
	// rollout convergence is observed from workload state.

	// 3. Update per-cluster metrics.
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

	logger.V(1).Info("Updated status for OpenBaoCluster",
		"readyReplicas", state.ReadyReplicas,
		"readReplicaReadyReplicas", state.ReadReplicaReadyReplicas,
		"phase", cluster.Status.Phase,
		"currentVersion", cluster.Status.CurrentVersion)

	// 5. Return the policy requeue decision.
	return ctrl.Result{RequeueAfter: policyResult.RequeueAfter}, nil
}
