package openbaocluster

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
)

// applyAllConditions computes and sets all status conditions from cluster state.
// This consolidates condition logic to eliminate duplicate code paths.
func applyAllConditions(
	cluster *openbaov1alpha1.OpenBaoCluster,
	state *clusterState,
	admissionStatus *admission.Status,
	now metav1.Time,
) {
	gen := cluster.Generation

	initCond := buildInitializedCondition(state.Initialized, state.InitializedKnown)
	initCond.ObservedGeneration = gen
	initCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, initCond)

	sealedCond := buildSealedCondition(state.Sealed, state.SealedKnown)
	sealedCond.ObservedGeneration = gen
	sealedCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, sealedCond)

	leaderCond := buildLeaderCondition(state.LeaderCount, state.LeaderName)
	leaderCond.ObservedGeneration = gen
	leaderCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, leaderCond)

	availableCond := buildAvailableCondition(cluster, state.ReadyReplicas)
	availableCond.ObservedGeneration = gen
	availableCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, availableCond)

	degradedCond := buildDegradedCondition(cluster, state.UpgradeFailed)
	degradedCond.ObservedGeneration = gen
	degradedCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, degradedCond)

	upgradingCond := buildUpgradingCondition(cluster)
	upgradingCond.ObservedGeneration = gen
	upgradingCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, upgradingCond)

	backupCond := buildBackupCondition(state.BackupInProgress, state.BackupJobName)
	backupCond.ObservedGeneration = gen
	backupCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, backupCond)

	userAccessCond := buildUserAccessBootstrapCondition(cluster)
	userAccessCond.ObservedGeneration = gen
	userAccessCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, userAccessCond)

	storageCond := buildStorageConfiguredCondition(cluster, state)
	storageCond.ObservedGeneration = gen
	storageCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, storageCond)

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionEtcdEncryptionWarning),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gen,
		LastTransitionTime: now,
		Reason:             ReasonEtcdEncryptionUnknown,
		Message:            "The operator cannot verify etcd encryption status. Ensure etcd encryption at rest is enabled in your Kubernetes cluster to protect Secrets (including unseal keys and root tokens) stored in etcd.",
	})

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

	applyNodeSecurityCapabilityMismatchCondition(cluster, state, gen, now)
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
