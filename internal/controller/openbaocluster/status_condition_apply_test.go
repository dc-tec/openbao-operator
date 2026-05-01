package openbaocluster

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
)

func TestBuildSealedConditionAndApplyHelpers(t *testing.T) {
	t.Parallel()

	t.Run("sealed condition handles present and absent labels", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			sealed     bool
			present    bool
			wantStatus metav1.ConditionStatus
			wantReason string
		}{
			{name: "unknown", present: false, wantStatus: metav1.ConditionUnknown, wantReason: reasonUnknown},
			{name: "sealed", sealed: true, present: true, wantStatus: metav1.ConditionTrue, wantReason: "Sealed"},
			{name: "unsealed", sealed: false, present: true, wantStatus: metav1.ConditionFalse, wantReason: "Unsealed"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cond := buildSealedCondition(tt.sealed, tt.present)
				if cond.Status != tt.wantStatus || cond.Reason != tt.wantReason {
					t.Fatalf("buildSealedCondition() = %#v, want status=%s reason=%s", cond, tt.wantStatus, tt.wantReason)
				}
			})
		}
	})

	t.Run("applyAllConditions populates core conditions and security risk", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.Profile = openbaov1alpha1.ProfileDevelopment
		cluster.Spec.WorkloadHardening = &openbaov1alpha1.WorkloadHardeningConfig{AppArmorEnabled: true}
		readStorageClassName := "fast"
		cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{
			Replicas: 1,
			Storage: &openbaov1alpha1.ReadReplicaStorageConfig{
				StorageClassName: &readStorageClassName,
			},
		}
		state := &clusterState{
			ReadyReplicas:                       1,
			Available:                           true,
			Initialized:                         true,
			InitializedKnown:                    true,
			Sealed:                              false,
			SealedKnown:                         true,
			LeaderCount:                         1,
			LeaderName:                          "example-0",
			BackupInProgress:                    true,
			BackupJobName:                       "backup-job",
			DataPVCCount:                        1,
			DataPVCStorageClassNames:            []string{"fast"},
			ReadReplicaReadyReplicas:            1,
			ReadReplicaRegisteredReplicas:       1,
			ReadReplicaHealthyReplicas:          1,
			ReadReplicaMembershipKnown:          true,
			ReadReplicaAutopilotKnown:           true,
			ReadServingAvailable:                true,
			ReadServingKnown:                    true,
			ReadReplicaDataPVCCount:             1,
			ReadReplicaDataPVCStorageClassNames: []string{"fast"},
			StatefulSet: &appsv1.StatefulSet{
				Status: appsv1.StatefulSetStatus{
					Conditions: []appsv1.StatefulSetCondition{{
						Type:    "ReplicaFailure",
						Message: "AppArmor profile rejected by node",
					}},
				},
			},
		}
		admissionStatus := &admission.Status{OverallReady: false}
		now := metav1.Now()

		applyAllConditions(cluster, state, admissionStatus, now)

		for _, conditionType := range []openbaov1alpha1.ConditionType{
			openbaov1alpha1.ConditionOpenBaoInitialized,
			openbaov1alpha1.ConditionOpenBaoSealed,
			openbaov1alpha1.ConditionOpenBaoLeader,
			openbaov1alpha1.ConditionAvailable,
			openbaov1alpha1.ConditionDegraded,
			openbaov1alpha1.ConditionUpgrading,
			openbaov1alpha1.ConditionBackingUp,
			openbaov1alpha1.ConditionUserAccessBootstrap,
			openbaov1alpha1.ConditionStorageConfigured,
			openbaov1alpha1.ConditionReadReplicasReady,
			openbaov1alpha1.ConditionReadServingAvailable,
			openbaov1alpha1.ConditionRaftMembershipReady,
			openbaov1alpha1.ConditionReadReplicasAutopilotHealthy,
			openbaov1alpha1.ConditionReadReplicaStorageConfigured,
			openbaov1alpha1.ConditionEtcdEncryptionWarning,
			openbaov1alpha1.ConditionSecurityRisk,
			openbaov1alpha1.ConditionProductionReady,
			openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch,
		} {
			if cond := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType)); cond == nil {
				t.Fatalf("expected condition %s", conditionType)
			}
		}
		nodeMismatch := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch))
		if nodeMismatch == nil || nodeMismatch.Status != metav1.ConditionTrue {
			t.Fatalf("node mismatch condition = %#v, want true", nodeMismatch)
		}
	})

	t.Run("applyAllConditions surfaces unsafe admission mode", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeExternal
		cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
			Type: "transit",
			Transit: &openbaov1alpha1.TransitSealConfig{
				Address:   "https://infra-bao.example",
				KeyName:   "autounseal",
				MountPath: "transit/",
			},
		}
		admissionStatus := &admission.Status{
			OverallReady: true,
			UnsafeMode:   true,
		}

		applyAllConditions(cluster, &clusterState{}, admissionStatus, metav1.Now())

		securityRisk := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionSecurityRisk))
		if securityRisk == nil || securityRisk.Status != metav1.ConditionTrue || securityRisk.Reason != ReasonUnsafeAdmissionDisabled {
			t.Fatalf("SecurityRisk condition = %#v, want true %s", securityRisk, ReasonUnsafeAdmissionDisabled)
		}
		productionReady := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionProductionReady))
		if productionReady == nil || productionReady.Status != metav1.ConditionFalse || productionReady.Reason != ReasonUnsafeAdmissionDisabled {
			t.Fatalf("ProductionReady condition = %#v, want false %s", productionReady, ReasonUnsafeAdmissionDisabled)
		}
	})

	t.Run("applyNodeSecurityCapabilityMismatchCondition removes condition when apparmor disabled", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Status.Conditions = []metav1.Condition{{
			Type:   string(openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch),
			Status: metav1.ConditionTrue,
		}}

		applyNodeSecurityCapabilityMismatchCondition(cluster, &clusterState{}, cluster.Generation, metav1.Now())
		if cond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch)); cond != nil {
			t.Fatalf("expected node mismatch condition to be removed, got %#v", cond)
		}
	})
}
