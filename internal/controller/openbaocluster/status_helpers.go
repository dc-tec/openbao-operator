package openbaocluster

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/admission"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/status"
)

func evaluateProductionReady(cluster *openbaov1alpha1.OpenBaoCluster, admissionReady bool, admissionSummary string) (metav1.ConditionStatus, string, string) {
	if cluster.Spec.Profile == "" {
		return metav1.ConditionFalse, ReasonProfileNotSet, "spec.profile must be explicitly set to Hardened or Development"
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		return metav1.ConditionFalse, ReasonDevelopmentProfile, "Development profile is not suitable for production"
	}

	if !admissionReady {
		if admissionSummary != "" {
			return metav1.ConditionFalse, ReasonAdmissionPoliciesNotReady, "Required admission policies are not ready: " + admissionSummary
		}
		return metav1.ConditionFalse, ReasonAdmissionPoliciesNotReady, "Required admission policies are not ready"
	}

	if cluster.Spec.TLS.Mode == "" || cluster.Spec.TLS.Mode == openbaov1alpha1.TLSModeOperatorManaged {
		return metav1.ConditionFalse, ReasonOperatorManagedTLS, "Hardened profile requires TLS mode External or ACME; OperatorManaged TLS is not considered production-ready"
	}

	if isStaticUnseal(cluster) {
		return metav1.ConditionFalse, ReasonStaticUnsealInUse, "Hardened profile requires a non-static unseal configuration (external KMS/Transit); static unseal is not considered production-ready"
	}

	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		return metav1.ConditionFalse, ReasonRootTokenStored, "Hardened profile requires self-init; manual bootstrap stores a root token Secret and is not considered production-ready"
	}

	return metav1.ConditionTrue, ReasonProductionReady, "Cluster meets Hardened profile production-ready requirements"
}

func isStaticUnseal(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster.Spec.Unseal == nil {
		return true
	}
	if cluster.Spec.Unseal.Type == "" {
		return true
	}
	return cluster.Spec.Unseal.Type == unsealTypeStatic
}

// setAvailableCondition sets the Available condition based on replica counts.
func setAvailableCondition(conditions *[]metav1.Condition, generation int64, cluster *openbaov1alpha1.OpenBaoCluster, readyReplicas int32) {
	available := readyReplicas == cluster.Spec.Replicas && readyReplicas > 0
	condType := string(openbaov1alpha1.ConditionAvailable)

	if available {
		status.True(conditions, generation, condType, ReasonAllReplicasReady, fmt.Sprintf("All %d replicas are ready", readyReplicas))
		return
	}

	if readyReplicas == 0 {
		status.False(conditions, generation, condType, ReasonNoReplicasReady, "No replicas are ready yet")
		return
	}

	status.False(conditions, generation, condType, ReasonNotReady, fmt.Sprintf("Only %d/%d replicas are ready", readyReplicas, cluster.Spec.Replicas))
}

// setDegradedCondition sets the Degraded condition based on cluster state.
func setDegradedCondition(
	conditions *[]metav1.Condition,
	generation int64,
	cluster *openbaov1alpha1.OpenBaoCluster,
	admissionStatus *admission.Status,
	upgradeFailed bool,
) {
	condType := string(openbaov1alpha1.ConditionDegraded)

	// Check break glass first
	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active {
		status.True(conditions, generation, condType, constants.ReasonBreakGlassRequired, "Break glass required; see status.breakGlass for recovery steps")
		return
	}

	// Check admission policies for Sentinel
	sentinelEnabled := cluster.Spec.Sentinel != nil && cluster.Spec.Sentinel.Enabled
	if sentinelEnabled && admissionStatus != nil && !admissionStatus.SentinelReady {
		status.True(conditions, generation, condType, ReasonAdmissionPoliciesNotReady, "Sentinel is enabled but required admission policies are missing or misbound; Sentinel will not be deployed. "+admissionStatus.SummaryMessage())
		return
	}

	// Check upgrade failure
	if upgradeFailed && cluster.Status.Upgrade != nil {
		status.True(conditions, generation, condType, cluster.Status.Upgrade.LastErrorReason, cluster.Status.Upgrade.LastErrorMessage)
		return
	}

	// Check workload error
	if cluster.Status.Workload != nil && cluster.Status.Workload.LastError != nil {
		status.True(conditions, generation, condType, cluster.Status.Workload.LastError.Reason, cluster.Status.Workload.LastError.Message)
		return
	}

	// Check admin ops error
	if cluster.Status.AdminOps != nil && cluster.Status.AdminOps.LastError != nil {
		status.True(conditions, generation, condType, cluster.Status.AdminOps.LastError.Reason, cluster.Status.AdminOps.LastError.Message)
		return
	}

	// Check self-init disabled warning
	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		status.True(conditions, generation, condType, ReasonRootTokenStored, "SelfInit is disabled. The operator is storing the root token in a Secret, which violates Zero Trust principles. Anyone with Secret read access in this namespace can access the root token. Strongly consider enabling SelfInit (spec.selfInit.enabled=true) for production deployments.")
		return
	}

	status.False(conditions, generation, condType, constants.ReasonReconciling, "No degradation has been recorded by the controller")
}

// setUpgradingCondition sets the Upgrading condition based on upgrade state.
func setUpgradingCondition(conditions *[]metav1.Condition, generation int64, cluster *openbaov1alpha1.OpenBaoCluster) {
	rollingUpgradeInProgress := cluster.Status.Upgrade != nil
	upgradeFailed := rollingUpgradeInProgress && cluster.Status.Upgrade.LastErrorReason != ""

	blueGreenInProgress := cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.Phase != "" &&
		cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle

	condType := string(openbaov1alpha1.ConditionUpgrading)

	if upgradeFailed && cluster.Status.Upgrade != nil {
		status.False(conditions, generation, condType, cluster.Status.Upgrade.LastErrorReason, cluster.Status.Upgrade.LastErrorMessage)
		return
	}

	if rollingUpgradeInProgress && !upgradeFailed {
		status.True(conditions, generation, condType, ReasonInProgress, fmt.Sprintf("Rolling upgrade from %s to %s (partition=%d)", cluster.Status.Upgrade.FromVersion, cluster.Status.Upgrade.TargetVersion, cluster.Status.Upgrade.CurrentPartition))
		return
	}

	if blueGreenInProgress && cluster.Status.BlueGreen != nil {
		status.True(conditions, generation, condType, ReasonInProgress, fmt.Sprintf("Blue/green upgrade phase %s", cluster.Status.BlueGreen.Phase))
		return
	}

	status.False(conditions, generation, condType, constants.ReasonIdle, "No upgrade is currently in progress")
}

// setBackupCondition sets the BackingUp condition based on backup job state.
func setBackupCondition(conditions *[]metav1.Condition, generation int64, backupInProgress bool, backupJobName string) {
	condType := string(openbaov1alpha1.ConditionBackingUp)
	if backupInProgress {
		message := "Backup in progress"
		if backupJobName != "" {
			message = fmt.Sprintf("Backup Job %s is running", backupJobName)
		}
		status.True(conditions, generation, condType, ReasonInProgress, message)
		return
	}

	status.False(conditions, generation, condType, constants.ReasonIdle, "No backup is currently in progress")
}

// setInitializedCondition sets the OpenBaoInitialized condition from pod labels.
func setInitializedCondition(conditions *[]metav1.Condition, generation int64, initialized, present bool) {
	condType := string(openbaov1alpha1.ConditionOpenBaoInitialized)
	if !present {
		status.Unknown(conditions, generation, condType, constants.ReasonUnknown, "OpenBao initialization state is not yet available via service registration")
		return
	}

	if initialized {
		status.True(conditions, generation, condType, "Initialized", "OpenBao reports initialized")
		return
	}

	status.False(conditions, generation, condType, "NotInitialized", "OpenBao reports not initialized")
}

// setSealedCondition sets the OpenBaoSealed condition from pod labels.
func setSealedCondition(conditions *[]metav1.Condition, generation int64, sealed, present bool) {
	condType := string(openbaov1alpha1.ConditionOpenBaoSealed)
	if !present {
		status.Unknown(conditions, generation, condType, constants.ReasonUnknown, "OpenBao seal state is not yet available via service registration")
		return
	}

	if sealed {
		status.True(conditions, generation, condType, "Sealed", "OpenBao reports sealed")
		return
	}

	status.False(conditions, generation, condType, "Unsealed", "OpenBao reports unsealed")
}

// setLeaderCondition sets the OpenBaoLeader condition from leader count.
func setLeaderCondition(conditions *[]metav1.Condition, generation int64, leaderCount int, leaderName string) {
	condType := string(openbaov1alpha1.ConditionOpenBaoLeader)
	switch leaderCount {
	case 0:
		status.Unknown(conditions, generation, condType, ReasonLeaderUnknown, "No active leader label observed on pods")
	case 1:
		status.True(conditions, generation, condType, ReasonLeaderFound, fmt.Sprintf("Leader is %s", leaderName))
	default:
		status.False(conditions, generation, condType, ReasonMultipleLeaders, fmt.Sprintf("Multiple leaders detected via pod labels (%d)", leaderCount))
	}
}

// applyAllConditions computes and sets all status conditions from cluster state.
// This consolidates condition logic to eliminate duplicate code paths.
func applyAllConditions(
	cluster *openbaov1alpha1.OpenBaoCluster,
	state *clusterState,
	admissionStatus *admission.Status,
	now metav1.Time,
) {
	gen := cluster.Generation
	conditions := &cluster.Status.Conditions

	// OpenBao initialized condition (from pod0 labels)
	setInitializedCondition(conditions, gen, state.Initialized, state.InitializedKnown)

	// OpenBao sealed condition (from pod0 labels)
	setSealedCondition(conditions, gen, state.Sealed, state.SealedKnown)

	// Leader condition
	setLeaderCondition(conditions, gen, state.LeaderCount, state.LeaderName)

	// Available condition
	setAvailableCondition(conditions, gen, cluster, state.ReadyReplicas)

	// Degraded condition
	setDegradedCondition(conditions, gen, cluster, admissionStatus, state.UpgradeFailed)

	// Upgrading condition
	setUpgradingCondition(conditions, gen, cluster)

	// Backup condition
	setBackupCondition(conditions, gen, state.BackupInProgress, state.BackupJobName)

	// Etcd encryption warning (always set)
	status.True(conditions, gen, string(openbaov1alpha1.ConditionEtcdEncryptionWarning), ReasonEtcdEncryptionUnknown, "The operator cannot verify etcd encryption status. Ensure etcd encryption at rest is enabled in your Kubernetes cluster to protect Secrets (including unseal keys and root tokens) stored in etcd.")

	// Security risk condition for Development profile
	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		status.True(conditions, gen, string(openbaov1alpha1.ConditionSecurityRisk), ReasonDevelopmentProfile, "Cluster is using Development profile with relaxed security. Not suitable for production.")
	} else {
		status.Remove(conditions, string(openbaov1alpha1.ConditionSecurityRisk))
	}

	// Production ready condition
	admissionReady := admissionStatus == nil || admissionStatus.OverallReady
	admissionSummary := ""
	if admissionStatus != nil {
		admissionSummary = admissionStatus.SummaryMessage()
	}
	productionStatus, productionReason, productionMessage := evaluateProductionReady(cluster, admissionReady, admissionSummary)
	status.Set(conditions, gen, string(openbaov1alpha1.ConditionProductionReady), productionStatus, productionReason, productionMessage)
}

// computePhase determines the cluster phase from state.
func computePhase(state *clusterState) openbaov1alpha1.ClusterPhase {
	if state.UpgradeFailed {
		return openbaov1alpha1.ClusterPhaseFailed
	}
	if state.UpgradeInProgress {
		return openbaov1alpha1.ClusterPhaseUpgrading
	}
	if state.BackupInProgress {
		return openbaov1alpha1.ClusterPhaseBackingUp
	}
	if state.Available {
		return openbaov1alpha1.ClusterPhaseRunning
	}
	return openbaov1alpha1.ClusterPhaseInitializing
}
