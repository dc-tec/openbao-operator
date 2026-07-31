package openbaocluster

import (
	"fmt"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
)

func FuzzEvaluateProductionReady(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint8(0), true, true, false, "all good")
	f.Add(uint8(1), uint8(1), uint8(1), false, false, false, "missing admission")
	f.Add(uint8(2), uint8(2), uint8(0), true, false, true, "development")

	f.Fuzz(func(t *testing.T, profileSeed, tlsSeed, unsealSeed uint8, selfInitEnabled, admissionReady, unsafeAdmission bool, admissionSummary string) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.Profile = fuzzProfile(profileSeed)
		cluster.Spec.TLS = openbaov1alpha1.TLSConfig{
			Enabled: true,
			Mode:    fuzzTLSMode(tlsSeed),
		}
		cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
			Type: fuzzUnsealType(unsealSeed),
		}
		cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: selfInitEnabled}

		status, reason, message := evaluateProductionReady(cluster, admissionReady, strings.TrimSpace(admissionSummary), unsafeAdmission)
		if reason == "" {
			t.Fatalf("expected non-empty reason for production-ready evaluation")
		}
		if strings.TrimSpace(message) == "" {
			t.Fatalf("expected non-empty message for production-ready evaluation")
		}

		switch status {
		case metav1.ConditionTrue, metav1.ConditionFalse:
		default:
			t.Fatalf("unexpected condition status %q", status)
		}
	})
}

func FuzzApplyAllConditions(f *testing.F) {
	f.Add(
		uint8(0), uint8(0), uint8(0), uint8(0), uint8(0),
		int32(3), int32(3), int64(3),
		true, true, false, true, false, true, true,
		"fast-ssd", "fast-ssd", "leader-0", "admission ready", "apparmor denied",
	)
	f.Add(
		uint8(1), uint8(1), uint8(1), uint8(1), uint8(1),
		int32(0), int32(1), int64(0),
		false, false, true, false, true, false, false,
		"", "", "", "missing policy", "",
	)

	f.Fuzz(func(
		t *testing.T,
		profileSeed, tlsSeed, unsealSeed, upgradeSeed, blueGreenPhaseSeed uint8,
		replicas, readyReplicas int32,
		generation int64,
		selfInitEnabled, initializedKnown, initialized, sealedKnown, sealed, admissionReady, appArmorEnabled bool,
		storageClass, observedStorageClass, leaderName, admissionIssue, replicaFailureMessage string,
	) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Name = sanitizeClusterToken(leaderName, "example")
		cluster.Generation = generation
		cluster.Spec.Replicas = clampReplicas(replicas)
		cluster.Spec.Profile = fuzzProfile(profileSeed)
		cluster.Spec.TLS = openbaov1alpha1.TLSConfig{
			Enabled: true,
			Mode:    fuzzTLSMode(tlsSeed),
		}
		cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
			Type: fuzzUnsealType(unsealSeed),
		}
		cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: selfInitEnabled}
		cluster.Spec.WorkloadHardening = &openbaov1alpha1.WorkloadHardeningConfig{
			AppArmorEnabled: appArmorEnabled,
		}
		if strings.TrimSpace(storageClass) != "" {
			value := sanitizeClusterToken(storageClass, "sc")
			cluster.Spec.Storage.StorageClassName = &value
		} else {
			cluster.Spec.Storage.StorageClassName = nil
		}

		cluster.Status.BreakGlass = &openbaov1alpha1.BreakGlassStatus{Active: profileSeed%5 == 0}
		if upgradeSeed%2 == 0 {
			cluster.Status.Upgrade = &openbaov1alpha1.UpgradeProgress{
				FromVersion:      "2.0.0",
				TargetVersion:    "2.1.0",
				CurrentPartition: int32(upgradeSeed % 5),
				Failure: &openbaov1alpha1.ControllerErrorStatus{
					Reason:  fuzzUpgradeErrorReason(upgradeSeed),
					Message: sanitizeMessage(admissionIssue, "upgrade issue"),
				},
			}
		}
		if upgradeSeed%3 == 0 {
			cluster.Status.Workload = &openbaov1alpha1.WorkloadControllerStatus{
				LastError: &openbaov1alpha1.ControllerErrorStatus{
					Reason:  "WorkloadError",
					Message: sanitizeMessage(replicaFailureMessage, "workload error"),
				},
			}
		}
		if upgradeSeed%4 == 0 {
			cluster.Status.AdminOps = &openbaov1alpha1.AdminOpsControllerStatus{
				LastError: &openbaov1alpha1.ControllerErrorStatus{
					Reason:  "AdminOpsError",
					Message: sanitizeMessage(admissionIssue, "adminops error"),
				},
			}
		}
		if fuzzUpgradeStrategy(upgradeSeed) == openbaov1alpha1.UpdateStrategyBlueGreen {
			cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{
				Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
			}
			cluster.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
				Phase:        fuzzBlueGreenPhase(blueGreenPhaseSeed),
				BlueRevision: sanitizeClusterToken(observedStorageClass, "rev-a"),
				BlueImage:    "openbao/openbao:2.4.4",
			}
		}

		upgradeFailed, upgradeInProgress := fuzzUpgradeState(cluster.Status.Upgrade)
		state := &clusterState{
			ReadyReplicas:            clampReplicas(readyReplicas),
			Initialized:              initialized,
			InitializedKnown:         initializedKnown,
			Sealed:                   sealed,
			SealedKnown:              sealedKnown,
			LeaderCount:              int(profileSeed % 4),
			LeaderName:               sanitizeClusterToken(leaderName, "leader-0"),
			BackupInProgress:         upgradeSeed%3 == 1,
			BackupJobName:            sanitizeClusterToken(storageClass, "backup-job"),
			UpgradeFailed:            upgradeFailed,
			UpgradeInProgress:        upgradeInProgress,
			Available:                clampReplicas(readyReplicas) == clampReplicas(replicas) && clampReplicas(replicas) > 0,
			DataPVCCount:             int(profileSeed % 4),
			DataPVCStorageClassUnset: tlsSeed%3 == 0,
		}
		if state.DataPVCCount > 0 && strings.TrimSpace(observedStorageClass) != "" {
			state.DataPVCStorageClassNames = []string{sanitizeClusterToken(observedStorageClass, "default")}
		}
		if state.DataPVCCount > 0 && len(state.DataPVCStorageClassNames) == 0 {
			state.DataPVCStorageClassUnset = true
		}
		if appArmorEnabled {
			state.StatefulSet = &appsv1.StatefulSet{
				Status: appsv1.StatefulSetStatus{
					Conditions: []appsv1.StatefulSetCondition{{
						Type:    "ReplicaFailure",
						Status:  "True",
						Reason:  "FailedCreate",
						Message: sanitizeMessage(replicaFailureMessage, "AppArmor profile denied by node"),
					}},
				},
			}
		}

		var admissionStatus *admission.Status
		if admissionReady || admissionIssue != "" {
			admissionStatus = &admission.Status{
				OverallReady: admissionReady,
				Dependencies: []admission.DependencyStatus{{
					Dependency: admission.Dependency{Name: sanitizeClusterToken(admissionIssue, "dep")},
					Ready:      admissionReady,
					Issues:     []string{sanitizeMessage(admissionIssue, "dependency issue")},
				}},
			}
		}

		now := metav1.NewTime(time.Unix(1_700_000_000, 0).UTC())
		applyAllConditions(cluster, state, admissionStatus, now)

		seen := make(map[string]struct{}, len(cluster.Status.Conditions))
		for _, cond := range cluster.Status.Conditions {
			if _, exists := seen[cond.Type]; exists {
				t.Fatalf("duplicate condition type %q", cond.Type)
			}
			seen[cond.Type] = struct{}{}
			if cond.ObservedGeneration != cluster.Generation {
				t.Fatalf("condition %q has generation %d, want %d", cond.Type, cond.ObservedGeneration, cluster.Generation)
			}
		}

		required := []openbaov1alpha1.ConditionType{
			openbaov1alpha1.ConditionOpenBaoInitialized,
			openbaov1alpha1.ConditionOpenBaoSealed,
			openbaov1alpha1.ConditionOpenBaoLeader,
			openbaov1alpha1.ConditionAvailable,
			openbaov1alpha1.ConditionDegraded,
			openbaov1alpha1.ConditionUpgrading,
			openbaov1alpha1.ConditionBackingUp,
			openbaov1alpha1.ConditionStorageConfigured,
			openbaov1alpha1.ConditionReadReplicasReady,
			openbaov1alpha1.ConditionReadServingAvailable,
			openbaov1alpha1.ConditionRaftMembershipReady,
			openbaov1alpha1.ConditionReadReplicasAutopilotHealthy,
			openbaov1alpha1.ConditionReadReplicaStorageConfigured,
			openbaov1alpha1.ConditionEtcdEncryptionWarning,
			openbaov1alpha1.ConditionProductionReady,
		}
		for _, conditionType := range required {
			if _, ok := seen[string(conditionType)]; !ok {
				t.Fatalf("missing required condition %q", conditionType)
			}
		}

		_, hasSecurityRisk := seen[string(openbaov1alpha1.ConditionSecurityRisk)]
		if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment && !hasSecurityRisk {
			t.Fatalf("expected security-risk condition for development profile")
		}
		if cluster.Spec.Profile != openbaov1alpha1.ProfileDevelopment && hasSecurityRisk {
			t.Fatalf("unexpected security-risk condition for non-development profile")
		}

		_, hasNodeMismatch := seen[string(openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch)]
		if appArmorEnabled && !hasNodeMismatch {
			t.Fatalf("expected node-security mismatch condition when AppArmor is enabled")
		}
		if !appArmorEnabled && hasNodeMismatch {
			t.Fatalf("unexpected node-security mismatch condition when AppArmor is disabled")
		}
	})
}

func clampReplicas(value int32) int32 {
	if value < 0 {
		return 0
	}
	return value % 6
}

func fuzzProfile(seed uint8) openbaov1alpha1.Profile {
	switch seed % 3 {
	case 0:
		return ""
	case 1:
		return openbaov1alpha1.ProfileDevelopment
	default:
		return openbaov1alpha1.ProfileHardened
	}
}

func fuzzTLSMode(seed uint8) openbaov1alpha1.TLSMode {
	switch seed % 3 {
	case 0:
		return ""
	case 1:
		return openbaov1alpha1.TLSModeOperatorManaged
	default:
		return openbaov1alpha1.TLSModeExternal
	}
}

func fuzzUpgradeState(progress *openbaov1alpha1.UpgradeProgress) (failed, inProgress bool) {
	if progress == nil {
		return false, false
	}
	if progress.Failure == nil {
		return false, true
	}
	failed = strings.TrimSpace(progress.Failure.Reason) != ""
	return failed, !failed
}

func fuzzUnsealType(seed uint8) string {
	switch seed % 4 {
	case 0:
		return ""
	case 1:
		return unsealTypeStatic
	case 2:
		return "awskms"
	default:
		return "transit"
	}
}

func fuzzUpgradeStrategy(seed uint8) openbaov1alpha1.UpdateStrategyType {
	if seed%2 == 0 {
		return openbaov1alpha1.UpdateStrategyBlueGreen
	}
	return openbaov1alpha1.UpdateStrategyRollingUpdate
}

func fuzzBlueGreenPhase(seed uint8) openbaov1alpha1.BlueGreenPhase {
	switch seed % 5 {
	case 0:
		return ""
	case 1:
		return openbaov1alpha1.PhaseIdle
	case 2:
		return openbaov1alpha1.PhasePromoting
	case 3:
		return openbaov1alpha1.PhaseDemotingBlue
	default:
		return openbaov1alpha1.PhaseCleanup
	}
}

func fuzzUpgradeErrorReason(seed uint8) string {
	if seed%3 == 0 {
		return "PodNotReady"
	}
	return ""
}

func sanitizeClusterToken(input, fallback string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(input) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		}
		if b.Len() >= 32 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}

func sanitizeMessage(input, fallback string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return fallback
	}
	if len(trimmed) > 120 {
		return fmt.Sprintf("%s...", trimmed[:117])
	}
	return trimmed
}
