package openbaocluster

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/admission"
	"github.com/dc-tec/openbao-operator/internal/constants"
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

// buildAvailableCondition builds the Available condition based on replica counts.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildAvailableCondition(cluster *openbaov1alpha1.OpenBaoCluster, readyReplicas int32) metav1.Condition {
	available := readyReplicas == cluster.Spec.Replicas && readyReplicas > 0

	if available {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionAvailable),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonAllReplicasReady,
			Message: fmt.Sprintf("All %d replicas are ready", readyReplicas),
		}
	}

	if readyReplicas == 0 {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionAvailable),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonNoReplicasReady,
			Message: "No replicas are ready yet",
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionAvailable),
		Status:  metav1.ConditionFalse,
		Reason:  ReasonNotReady,
		Message: fmt.Sprintf("Only %d/%d replicas are ready", readyReplicas, cluster.Spec.Replicas),
	}
}

// buildDegradedCondition builds the Degraded condition based on cluster state.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildDegradedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	admissionStatus *admission.Status,
	upgradeFailed bool,
) metav1.Condition {
	// Check break glass first
	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  constants.ReasonBreakGlassRequired,
			Message: "Break glass required; see status.breakGlass for recovery steps",
		}
	}

	// Check admission policies for Sentinel
	sentinelEnabled := cluster.Spec.Sentinel != nil && cluster.Spec.Sentinel.Enabled
	if sentinelEnabled && admissionStatus != nil && !admissionStatus.SentinelReady {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonAdmissionPoliciesNotReady,
			Message: "Sentinel is enabled but required admission policies are missing or misbound; Sentinel will not be deployed. " + admissionStatus.SummaryMessage(),
		}
	}

	// Check upgrade failure
	if upgradeFailed && cluster.Status.Upgrade != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  cluster.Status.Upgrade.LastErrorReason,
			Message: cluster.Status.Upgrade.LastErrorMessage,
		}
	}

	// Check workload error
	if cluster.Status.Workload != nil && cluster.Status.Workload.LastError != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  cluster.Status.Workload.LastError.Reason,
			Message: cluster.Status.Workload.LastError.Message,
		}
	}

	// Check admin ops error
	if cluster.Status.AdminOps != nil && cluster.Status.AdminOps.LastError != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  cluster.Status.AdminOps.LastError.Reason,
			Message: cluster.Status.AdminOps.LastError.Message,
		}
	}

	// Check self-init disabled warning
	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonRootTokenStored,
			Message: "SelfInit is disabled. The operator is storing the root token in a Secret, which violates Zero Trust principles. Anyone with Secret read access in this namespace can access the root token. Strongly consider enabling SelfInit (spec.selfInit.enabled=true) for production deployments.",
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionDegraded),
		Status:  metav1.ConditionFalse,
		Reason:  constants.ReasonReconciling,
		Message: "No degradation has been recorded by the controller",
	}
}

// buildUpgradingCondition builds the Upgrading condition based on upgrade state.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildUpgradingCondition(cluster *openbaov1alpha1.OpenBaoCluster) metav1.Condition {
	rollingUpgradeInProgress := cluster.Status.Upgrade != nil
	upgradeFailed := rollingUpgradeInProgress && cluster.Status.Upgrade.LastErrorReason != ""

	blueGreenInProgress := cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.Phase != "" &&
		cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle

	if upgradeFailed && cluster.Status.Upgrade != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionUpgrading),
			Status:  metav1.ConditionFalse,
			Reason:  cluster.Status.Upgrade.LastErrorReason,
			Message: cluster.Status.Upgrade.LastErrorMessage,
		}
	}

	if rollingUpgradeInProgress && !upgradeFailed {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionUpgrading),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonInProgress,
			Message: fmt.Sprintf("Rolling upgrade from %s to %s (partition=%d)", cluster.Status.Upgrade.FromVersion, cluster.Status.Upgrade.TargetVersion, cluster.Status.Upgrade.CurrentPartition),
		}
	}

	if blueGreenInProgress && cluster.Status.BlueGreen != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionUpgrading),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonInProgress,
			Message: fmt.Sprintf("Blue/green upgrade phase %s", cluster.Status.BlueGreen.Phase),
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionUpgrading),
		Status:  metav1.ConditionFalse,
		Reason:  constants.ReasonIdle,
		Message: "No upgrade is currently in progress",
	}
}

// buildBackupCondition builds the BackingUp condition based on backup job state.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildBackupCondition(backupInProgress bool, backupJobName string) metav1.Condition {
	if backupInProgress {
		message := "Backup in progress"
		if backupJobName != "" {
			message = fmt.Sprintf("Backup Job %s is running", backupJobName)
		}
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionBackingUp),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonInProgress,
			Message: message,
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionBackingUp),
		Status:  metav1.ConditionFalse,
		Reason:  constants.ReasonIdle,
		Message: "No backup is currently in progress",
	}
}

// buildInitializedCondition builds the OpenBaoInitialized condition from pod labels.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildInitializedCondition(initialized, present bool) metav1.Condition {
	if !present {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoInitialized),
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonUnknown,
			Message: "OpenBao initialization state is not yet available via service registration",
		}
	}

	if initialized {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoInitialized),
			Status:  metav1.ConditionTrue,
			Reason:  "Initialized",
			Message: "OpenBao reports initialized",
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionOpenBaoInitialized),
		Status:  metav1.ConditionFalse,
		Reason:  "NotInitialized",
		Message: "OpenBao reports not initialized",
	}
}

// buildSealedCondition builds the OpenBaoSealed condition from pod labels.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildSealedCondition(sealed, present bool) metav1.Condition {
	if !present {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoSealed),
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonUnknown,
			Message: "OpenBao seal state is not yet available via service registration",
		}
	}

	if sealed {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoSealed),
			Status:  metav1.ConditionTrue,
			Reason:  "Sealed",
			Message: "OpenBao reports sealed",
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionOpenBaoSealed),
		Status:  metav1.ConditionFalse,
		Reason:  "Unsealed",
		Message: "OpenBao reports unsealed",
	}
}

// buildLeaderCondition builds the OpenBaoLeader condition from leader count.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildLeaderCondition(leaderCount int, leaderName string) metav1.Condition {
	switch leaderCount {
	case 0:
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonLeaderUnknown,
			Message: "No active leader label observed on pods",
		}
	case 1:
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonLeaderFound,
			Message: fmt.Sprintf("Leader is %s", leaderName),
		}
	default:
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonMultipleLeaders,
			Message: fmt.Sprintf("Multiple leaders detected via pod labels (%d)", leaderCount),
		}
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

	// OpenBao initialized condition (from pod0 labels)
	initCond := buildInitializedCondition(state.Initialized, state.InitializedKnown)
	initCond.ObservedGeneration = gen
	initCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, initCond)

	// OpenBao sealed condition (from pod0 labels)
	sealedCond := buildSealedCondition(state.Sealed, state.SealedKnown)
	sealedCond.ObservedGeneration = gen
	sealedCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, sealedCond)

	// Leader condition
	leaderCond := buildLeaderCondition(state.LeaderCount, state.LeaderName)
	leaderCond.ObservedGeneration = gen
	leaderCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, leaderCond)

	// Available condition
	availableCond := buildAvailableCondition(cluster, state.ReadyReplicas)
	availableCond.ObservedGeneration = gen
	availableCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, availableCond)

	// Degraded condition
	degradedCond := buildDegradedCondition(cluster, admissionStatus, state.UpgradeFailed)
	degradedCond.ObservedGeneration = gen
	degradedCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, degradedCond)

	// Upgrading condition
	upgradingCond := buildUpgradingCondition(cluster)
	upgradingCond.ObservedGeneration = gen
	upgradingCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, upgradingCond)

	// Backup condition
	backupCond := buildBackupCondition(state.BackupInProgress, state.BackupJobName)
	backupCond.ObservedGeneration = gen
	backupCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, backupCond)

	// Etcd encryption warning (always set)
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionEtcdEncryptionWarning),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gen,
		LastTransitionTime: now,
		Reason:             ReasonEtcdEncryptionUnknown,
		Message:            "The operator cannot verify etcd encryption status. Ensure etcd encryption at rest is enabled in your Kubernetes cluster to protect Secrets (including unseal keys and root tokens) stored in etcd.",
	})

	// Security risk condition for Development profile
	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionSecurityRisk),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gen,
			LastTransitionTime: now,
			Reason:             ReasonDevelopmentProfile,
			Message:            "Cluster is using Development profile with relaxed security. Not suitable for production.",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionSecurityRisk))
	}

	// Production ready condition
	admissionReady := admissionStatus == nil || admissionStatus.OverallReady
	admissionSummary := ""
	if admissionStatus != nil {
		admissionSummary = admissionStatus.SummaryMessage()
	}
	productionStatus, productionReason, productionMessage := evaluateProductionReady(cluster, admissionReady, admissionSummary)
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionProductionReady),
		Status:             productionStatus,
		ObservedGeneration: gen,
		LastTransitionTime: now,
		Reason:             productionReason,
		Message:            productionMessage,
	})
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
