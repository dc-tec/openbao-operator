package openbaocluster

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

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
	upgradeFailed bool,
) metav1.Condition {
	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  reasonBreakGlassRequired,
			Message: buildBreakGlassConditionMessage(cluster),
		}
	}

	if upgradeFailed && cluster.Status.Upgrade != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  rollingUpgradeFailureReason(cluster.Status.Upgrade),
			Message: buildRollingUpgradeFailedMessage(cluster),
		}
	}

	if cluster.Status.Workload != nil && cluster.Status.Workload.LastError != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  cluster.Status.Workload.LastError.Reason,
			Message: cluster.Status.Workload.LastError.Message,
		}
	}

	if cluster.Status.AdminOps != nil && cluster.Status.AdminOps.LastError != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  cluster.Status.AdminOps.LastError.Reason,
			Message: cluster.Status.AdminOps.LastError.Message,
		}
	}

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
		Reason:  reasonReconciling,
		Message: "No degradation has been recorded by the controller",
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
		Reason:  reasonIdle,
		Message: "No backup is currently in progress",
	}
}

func desiredReadReplicaCount(cluster *openbaov1alpha1.OpenBaoCluster) int32 {
	if cluster.Spec.ReadReplicas == nil {
		return 0
	}
	return cluster.Spec.ReadReplicas.Replicas
}

func desiredReadReplicaStorageClassName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Spec.ReadReplicas == nil || cluster.Spec.ReadReplicas.Storage == nil || cluster.Spec.ReadReplicas.Storage.StorageClassName == nil {
		return ""
	}
	return strings.TrimSpace(*cluster.Spec.ReadReplicas.Storage.StorageClassName)
}

// buildReadReplicasReadyCondition reports whether the read-replica pool has the
// desired number of Ready Pods.
func buildReadReplicasReadyCondition(cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState) metav1.Condition {
	desired := desiredReadReplicaCount(cluster)
	if desired == 0 {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadReplicasReady),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonNoReadReplicasConfigured,
			Message: "No steady-state read replicas are configured",
		}
	}

	if state == nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadReplicasReady),
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "Read-replica readiness has not been observed yet",
		}
	}

	if state.ReadReplicaReadyReplicas == desired {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadReplicasReady),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonAllReadReplicasReady,
			Message: fmt.Sprintf("All %d read replicas are ready", desired),
		}
	}

	if state.ReadReplicaReadyReplicas == 0 {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadReplicasReady),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonNoReadyReadReplicas,
			Message: "No read replicas are ready yet",
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionReadReplicasReady),
		Status:  metav1.ConditionFalse,
		Reason:  ReasonReadReplicasNotReady,
		Message: fmt.Sprintf("Only %d/%d read replicas are ready", state.ReadReplicaReadyReplicas, desired),
	}
}

// buildReadServingAvailableCondition reports whether at least one read-replica
// Pod is observed in a read-serving state.
func buildReadServingAvailableCondition(cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState) metav1.Condition {
	desired := desiredReadReplicaCount(cluster)
	if desired == 0 {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadServingAvailable),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonNoReadReplicasConfigured,
			Message: "No steady-state read replicas are configured",
		}
	}

	if state == nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadServingAvailable),
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "Read-serving availability has not been observed yet",
		}
	}

	if state.ReadReplicaReadyReplicas == 0 {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadServingAvailable),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonNoReadyReadReplicas,
			Message: "No ready read replicas are available to serve reads",
		}
	}

	if !state.ReadServingKnown {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadServingAvailable),
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "Read-serving availability has not been observed yet",
		}
	}

	if state.ReadServingAvailable {
		reason := ReasonReadServingAvailable
		message := "At least one read replica is serving reads"
		if !state.Available {
			reason = ReasonReadServingWithoutQuorum
			message = "At least one read replica is serving reads even though the voter pool is not fully available"
		}
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadServingAvailable),
			Status:  metav1.ConditionTrue,
			Reason:  reason,
			Message: message,
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionReadServingAvailable),
		Status:  metav1.ConditionFalse,
		Reason:  ReasonPodsNotServingReads,
		Message: "Read replicas are ready but none are currently observed in a read-serving state",
	}
}

// buildRaftMembershipReadyCondition reports whether observed non-voter
// membership matches the declared read-replica pool size.
func buildRaftMembershipReadyCondition(cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState) metav1.Condition {
	desired := desiredReadReplicaCount(cluster)
	if desired == 0 {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionRaftMembershipReady),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonNoReadReplicasConfigured,
			Message: "No steady-state read replicas are configured, so no non-voter membership is expected",
		}
	}

	if state == nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionRaftMembershipReady),
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "Raft membership has not been observed yet",
		}
	}

	if state.ReadReplicaRegisteredReplicas == desired {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionRaftMembershipReady),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonRaftMembershipReady,
			Message: fmt.Sprintf("Observed %d/%d read replicas registered in Raft membership", desired, desired),
		}
	}

	if !state.ReadReplicaMembershipKnown {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionRaftMembershipReady),
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "Raft membership has not been observed yet",
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionRaftMembershipReady),
		Status:  metav1.ConditionFalse,
		Reason:  ReasonReadReplicasNotReady,
		Message: fmt.Sprintf("Observed %d/%d read replicas registered in Raft membership", state.ReadReplicaRegisteredReplicas, desired),
	}
}

// buildStorageConfiguredCondition reports whether the workload is using an explicit
// or consistently resolved storage class, so users can see the effective one-shot choice.
func buildStorageConfiguredCondition(cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState) metav1.Condition {
	desiredStorageClassName := ""
	if cluster.Spec.Storage.StorageClassName != nil {
		desiredStorageClassName = strings.TrimSpace(*cluster.Spec.Storage.StorageClassName)
	}

	if state == nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionStorageConfigured),
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "Storage configuration has not been observed yet",
		}
	}

	if state.DataPVCCount == 0 {
		if desiredStorageClassName != "" {
			return metav1.Condition{
				Type:    string(openbaov1alpha1.ConditionStorageConfigured),
				Status:  metav1.ConditionTrue,
				Reason:  ReasonStorageClassConfigured,
				Message: fmt.Sprintf("Configured to request StorageClass %q when data PVCs are created. This choice becomes effectively immutable after PVC creation.", desiredStorageClassName),
			}
		}
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionStorageConfigured),
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonStorageClassPending,
			Message: "No data PVCs are present yet and spec.storage.storageClassName is unset. The cluster will rely on the default StorageClass when PVCs are created; set it explicitly on new clusters if you need a specific class.",
		}
	}

	if state.DataPVCStorageClassUnset && len(state.DataPVCStorageClassNames) == 0 {
		if desiredStorageClassName != "" {
			return metav1.Condition{
				Type:    string(openbaov1alpha1.ConditionStorageConfigured),
				Status:  metav1.ConditionFalse,
				Reason:  ReasonStorageClassMismatch,
				Message: fmt.Sprintf("spec.storage.storageClassName=%q does not match the observed data PVCs, which were created without a StorageClass. Storage class selection is effectively immutable after PVC creation.", desiredStorageClassName),
			}
		}
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionStorageConfigured),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonStorageClassUnset,
			Message: fmt.Sprintf("All %d data PVCs were created without a StorageClass. Set spec.storage.storageClassName explicitly on new clusters if you need a specific class; the effective storage path is immutable after PVC creation.", state.DataPVCCount),
		}
	}

	if state.DataPVCStorageClassUnset || len(state.DataPVCStorageClassNames) > 1 {
		observed := append([]string{}, state.DataPVCStorageClassNames...)
		if state.DataPVCStorageClassUnset {
			observed = append(observed, "<unset>")
		}
		sort.Strings(observed)
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionStorageConfigured),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonStorageClassInconsistent,
			Message: fmt.Sprintf("Observed inconsistent StorageClass values across %d data PVCs: %s. All OpenBao data PVCs should use one effective storage class.", state.DataPVCCount, strings.Join(observed, ", ")),
		}
	}

	observedStorageClassName := state.DataPVCStorageClassNames[0]
	if desiredStorageClassName == "" {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionStorageConfigured),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonStorageClassDefaulted,
			Message: fmt.Sprintf("Using default StorageClass %q on %d data PVCs. Set spec.storage.storageClassName explicitly on new clusters if you need a specific class; this choice is effectively immutable after PVC creation.", observedStorageClassName, state.DataPVCCount),
		}
	}
	if desiredStorageClassName != observedStorageClassName {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionStorageConfigured),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonStorageClassMismatch,
			Message: fmt.Sprintf("spec.storage.storageClassName=%q does not match the observed data PVC StorageClass %q. Storage class selection is effectively immutable after PVC creation.", desiredStorageClassName, observedStorageClassName),
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionStorageConfigured),
		Status:  metav1.ConditionTrue,
		Reason:  ReasonStorageClassConfigured,
		Message: fmt.Sprintf("Using configured StorageClass %q on %d data PVCs. This choice is effectively immutable after PVC creation.", observedStorageClassName, state.DataPVCCount),
	}
}

// buildReadReplicaStorageConfiguredCondition reports whether the read-replica
// pool is using an explicit or consistently resolved storage class.
func buildReadReplicaStorageConfiguredCondition(cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState) metav1.Condition {
	desiredReplicas := desiredReadReplicaCount(cluster)
	if desiredReplicas == 0 {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadReplicaStorageConfigured),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonNoReadReplicasConfigured,
			Message: "No steady-state read replicas are configured",
		}
	}

	desiredStorageClassName := desiredReadReplicaStorageClassName(cluster)
	if state == nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadReplicaStorageConfigured),
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "Read-replica storage configuration has not been observed yet",
		}
	}

	if state.ReadReplicaDataPVCCount == 0 {
		if desiredStorageClassName != "" {
			return metav1.Condition{
				Type:    string(openbaov1alpha1.ConditionReadReplicaStorageConfigured),
				Status:  metav1.ConditionTrue,
				Reason:  ReasonStorageClassConfigured,
				Message: fmt.Sprintf("Configured to request StorageClass %q when read-replica data PVCs are created. This choice becomes effectively immutable after PVC creation.", desiredStorageClassName),
			}
		}
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadReplicaStorageConfigured),
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonStorageClassPending,
			Message: "No read-replica data PVCs are present yet and spec.readReplicas.storage.storageClassName is unset. The read pool will rely on the default StorageClass when PVCs are created; set it explicitly on new clusters if you need a specific class.",
		}
	}

	if state.ReadReplicaDataPVCStorageClassUnset && len(state.ReadReplicaDataPVCStorageClassNames) == 0 {
		if desiredStorageClassName != "" {
			return metav1.Condition{
				Type:    string(openbaov1alpha1.ConditionReadReplicaStorageConfigured),
				Status:  metav1.ConditionFalse,
				Reason:  ReasonStorageClassMismatch,
				Message: fmt.Sprintf("spec.readReplicas.storage.storageClassName=%q does not match the observed read-replica data PVCs, which were created without a StorageClass. Storage class selection is effectively immutable after PVC creation.", desiredStorageClassName),
			}
		}
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadReplicaStorageConfigured),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonStorageClassUnset,
			Message: fmt.Sprintf("All %d read-replica data PVCs were created without a StorageClass. Set spec.readReplicas.storage.storageClassName explicitly on new clusters if you need a specific class; the effective storage path is immutable after PVC creation.", state.ReadReplicaDataPVCCount),
		}
	}

	if state.ReadReplicaDataPVCStorageClassUnset || len(state.ReadReplicaDataPVCStorageClassNames) > 1 {
		observed := append([]string{}, state.ReadReplicaDataPVCStorageClassNames...)
		if state.ReadReplicaDataPVCStorageClassUnset {
			observed = append(observed, "<unset>")
		}
		sort.Strings(observed)
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadReplicaStorageConfigured),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonStorageClassInconsistent,
			Message: fmt.Sprintf("Observed inconsistent StorageClass values across %d read-replica data PVCs: %s. All read-replica data PVCs should use one effective storage class.", state.ReadReplicaDataPVCCount, strings.Join(observed, ", ")),
		}
	}

	observedStorageClassName := state.ReadReplicaDataPVCStorageClassNames[0]
	if desiredStorageClassName == "" {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadReplicaStorageConfigured),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonStorageClassDefaulted,
			Message: fmt.Sprintf("Using default StorageClass %q on %d read-replica data PVCs. Set spec.readReplicas.storage.storageClassName explicitly on new clusters if you need a specific class; this choice is effectively immutable after PVC creation.", observedStorageClassName, state.ReadReplicaDataPVCCount),
		}
	}
	if desiredStorageClassName != observedStorageClassName {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionReadReplicaStorageConfigured),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonStorageClassMismatch,
			Message: fmt.Sprintf("spec.readReplicas.storage.storageClassName=%q does not match the observed read-replica data PVC StorageClass %q. Storage class selection is effectively immutable after PVC creation.", desiredStorageClassName, observedStorageClassName),
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionReadReplicaStorageConfigured),
		Status:  metav1.ConditionTrue,
		Reason:  ReasonStorageClassConfigured,
		Message: fmt.Sprintf("Using configured StorageClass %q on %d read-replica data PVCs. This choice is effectively immutable after PVC creation.", observedStorageClassName, state.ReadReplicaDataPVCCount),
	}
}

// buildInitializedCondition builds the OpenBaoInitialized condition from pod labels.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildInitializedCondition(initialized, present bool) metav1.Condition {
	if !present {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoInitialized),
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "OpenBao initialization state is not yet available via service registration",
		}
	}

	if initialized {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoInitialized),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonInitialized,
			Message: "OpenBao reports initialized",
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionOpenBaoInitialized),
		Status:  metav1.ConditionFalse,
		Reason:  ReasonNotInitialized,
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
			Reason:  reasonUnknown,
			Message: "OpenBao seal state is not yet available via service registration",
		}
	}

	if sealed {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoSealed),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonSealed,
			Message: "OpenBao reports sealed",
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionOpenBaoSealed),
		Status:  metav1.ConditionFalse,
		Reason:  ReasonUnsealed,
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

func applyNodeSecurityCapabilityMismatchCondition(cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState, gen int64, now metav1.Time) {
	appArmorEnabled := cluster.Spec.WorkloadHardening != nil && cluster.Spec.WorkloadHardening.AppArmorEnabled
	if !appArmorEnabled {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch))
		return
	}

	cond := metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: gen,
		LastTransitionTime: now,
		Reason:             reasonReady,
		Message:            "No node security capability mismatch detected for enabled workload hardening settings",
	}

	if state != nil && state.StatefulSet != nil {
		for _, ssCond := range state.StatefulSet.Status.Conditions {
			if ssCond.Type != "ReplicaFailure" {
				continue
			}
			msg := strings.ToLower(ssCond.Message)
			if strings.Contains(msg, "apparmor") {
				cond.Status = metav1.ConditionTrue
				cond.Reason = ReasonAppArmorUnsupported
				cond.Message = "AppArmor is enabled (spec.workloadHardening.appArmorEnabled=true) but the workload cannot be admitted/scheduled due to AppArmor support mismatch: " + ssCond.Message
				break
			}
		}
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, cond)
}
