package statusops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
)

func TestBuildAvailableCondition(t *testing.T) {
	tests := []struct {
		name          string
		replicas      int32
		readyReplicas int32
		wantStatus    metav1.ConditionStatus
		wantReason    string
	}{
		{
			name:          "all replicas ready",
			replicas:      3,
			readyReplicas: 3,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    ReasonAllReplicasReady,
		},
		{
			name:          "no replicas ready",
			replicas:      3,
			readyReplicas: 0,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    ReasonNoReplicasReady,
		},
		{
			name:          "partial replicas ready",
			replicas:      3,
			readyReplicas: 2,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    ReasonNotReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Replicas: tt.replicas,
				},
			}

			cond := buildAvailableCondition(cluster, tt.readyReplicas)

			assert.Equal(t, string(openbaov1alpha1.ConditionAvailable), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
		})
	}
}

func TestBuildDegradedCondition(t *testing.T) {
	tests := []struct {
		name            string
		cluster         *openbaov1alpha1.OpenBaoCluster
		admissionStatus *admission.Status
		upgradeFailed   bool
		wantStatus      metav1.ConditionStatus
		wantReason      string
		wantInMessage   string
	}{
		{
			name: "no degradation with selfInit enabled",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
				},
			},
			wantStatus: metav1.ConditionFalse,
		},
		{
			name: "degraded when selfInit disabled",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonRootTokenStored,
		},
		{
			name: "degraded when break glass active",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{Active: true},
				},
			},
			wantStatus:    metav1.ConditionTrue,
			wantInMessage: "spec.breakGlassAck",
		},
		{
			name: "degraded when rolling upgrade is paused",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						FromVersion:   "2.0.0",
						TargetVersion: "2.1.0",
						Failure: &openbaov1alpha1.ControllerErrorStatus{
							Reason:  "PodNotReady",
							Message: "Pod test-1 failed to become ready",
						},
					},
				},
			},
			upgradeFailed: true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    "PodNotReady",
			wantInMessage: upgradeRequestRetryFieldPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildDegradedCondition(tt.cluster, tt.upgradeFailed)

			assert.Equal(t, string(openbaov1alpha1.ConditionDegraded), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			if tt.wantReason != "" {
				assert.Equal(t, tt.wantReason, cond.Reason)
			}
			if tt.wantInMessage != "" {
				assert.Contains(t, cond.Message, tt.wantInMessage)
			}
		})
	}
}

func TestBuildUpgradingCondition(t *testing.T) {
	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		wantStatus metav1.ConditionStatus
		wantInMsg  string
	}{
		{
			name:       "no upgrade in progress",
			cluster:    &openbaov1alpha1.OpenBaoCluster{},
			wantStatus: metav1.ConditionFalse,
		},
		{
			name: "rolling upgrade in progress",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						FromVersion:   "2.0.0",
						TargetVersion: "2.1.0",
					},
				},
			},
			wantStatus: metav1.ConditionTrue,
			wantInMsg:  "Rolling upgrade from 2.0.0 to 2.1.0",
		},
		{
			name: "upgrade failed",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						FromVersion:   "2.0.0",
						TargetVersion: "2.1.0",
						Failure: &openbaov1alpha1.ControllerErrorStatus{
							Reason:  "PodNotReady",
							Message: "Pod failed to become ready",
						},
					},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantInMsg:  upgradeRequestRetryFieldPath,
		},
		{
			name: "blue green syncing with manual approval",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.1.0",
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
						BlueGreen: &openbaov1alpha1.BlueGreenConfig{
							AutoPromote: false,
						},
					},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.0.0",
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						Phase:                   openbaov1alpha1.PhaseSyncing,
						GreenRevision:           "green-abc",
						ManualPromotionRequired: true,
					},
				},
			},
			wantStatus: metav1.ConditionTrue,
			wantInMsg:  upgradeRequestPromoteFieldPath,
		},
		{
			name: "blue green rollback includes rollback reason",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.1.0",
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
					},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.0.0",
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						Phase:          openbaov1alpha1.PhaseRollingBack,
						BlueRevision:   "blue-123",
						RollbackReason: "quorum lost",
					},
				},
			},
			wantStatus: metav1.ConditionTrue,
			wantInMsg:  "Rollback reason: quorum lost.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildUpgradingCondition(tt.cluster)

			assert.Equal(t, string(openbaov1alpha1.ConditionUpgrading), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			if tt.wantInMsg != "" {
				assert.Contains(t, cond.Message, tt.wantInMsg)
			}
		})
	}
}

func TestBuildBackupCondition(t *testing.T) {
	tests := []struct {
		name             string
		backupInProgress bool
		backupJobName    string
		wantStatus       metav1.ConditionStatus
	}{
		{
			name:             "no backup in progress",
			backupInProgress: false,
			wantStatus:       metav1.ConditionFalse,
		},
		{
			name:             "backup in progress with job name",
			backupInProgress: true,
			backupJobName:    "my-backup-job",
			wantStatus:       metav1.ConditionTrue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildBackupCondition(tt.backupInProgress, tt.backupJobName)

			assert.Equal(t, string(openbaov1alpha1.ConditionBackingUp), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
		})
	}
}

func TestBuildReadReplicasReadyCondition(t *testing.T) {
	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		state      *clusterState
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name:       "no read replicas configured",
			cluster:    &openbaov1alpha1.OpenBaoCluster{},
			state:      &clusterState{},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonNoReadReplicasConfigured,
		},
		{
			name: "all read replicas ready",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
				},
			},
			state:      &clusterState{ReadReplicaReadyReplicas: 2},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonAllReadReplicasReady,
		},
		{
			name: "no ready read replicas",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
				},
			},
			state:      &clusterState{},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonNoReadyReadReplicas,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildReadReplicasReadyCondition(tt.cluster, tt.state)

			assert.Equal(t, string(openbaov1alpha1.ConditionReadReplicasReady), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
		})
	}
}

func TestBuildReadServingAvailableCondition(t *testing.T) {
	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		state      *clusterState
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name:       "no read replicas configured",
			cluster:    &openbaov1alpha1.OpenBaoCluster{},
			state:      &clusterState{},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonNoReadReplicasConfigured,
		},
		{
			name: "read serving without quorum",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 1},
				},
			},
			state: &clusterState{
				Available:                false,
				ReadReplicaReadyReplicas: 1,
				ReadServingKnown:         true,
				ReadServingAvailable:     true,
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonReadServingWithoutQuorum,
		},
		{
			name: "ready replicas not yet observed serving",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 1},
				},
			},
			state:      &clusterState{ReadReplicaReadyReplicas: 1},
			wantStatus: metav1.ConditionUnknown,
			wantReason: reasonUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildReadServingAvailableCondition(tt.cluster, tt.state)

			assert.Equal(t, string(openbaov1alpha1.ConditionReadServingAvailable), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
		})
	}
}

func TestBuildRaftMembershipReadyCondition(t *testing.T) {
	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		state      *clusterState
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name:       "no read replicas configured",
			cluster:    &openbaov1alpha1.OpenBaoCluster{},
			state:      &clusterState{},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonNoReadReplicasConfigured,
		},
		{
			name: "membership observed",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
				},
			},
			state: &clusterState{
				ReadReplicaRegisteredReplicas: 2,
				ReadReplicaMembershipKnown:    true,
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonRaftMembershipReady,
		},
		{
			name: "membership not yet observed",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
				},
			},
			state:      &clusterState{},
			wantStatus: metav1.ConditionUnknown,
			wantReason: reasonUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildRaftMembershipReadyCondition(tt.cluster, tt.state)

			assert.Equal(t, string(openbaov1alpha1.ConditionRaftMembershipReady), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
		})
	}
}

func TestBuildReadReplicasAutopilotHealthyCondition(t *testing.T) {
	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		state      *clusterState
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name:       "no read replicas configured",
			cluster:    &openbaov1alpha1.OpenBaoCluster{},
			state:      &clusterState{},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonNoReadReplicasConfigured,
		},
		{
			name: "all read replicas healthy in autopilot",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
				},
			},
			state: &clusterState{
				ReadReplicaHealthyReplicas: 2,
				ReadReplicaAutopilotKnown:  true,
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonReadReplicasAutopilotHealthy,
		},
		{
			name: "partial read replica autopilot health",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
				},
			},
			state: &clusterState{
				ReadReplicaHealthyReplicas: 1,
				ReadReplicaAutopilotKnown:  true,
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonReadReplicasAutopilotUnhealthy,
		},
		{
			name: "autopilot health not yet observed",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
				},
			},
			state:      &clusterState{},
			wantStatus: metav1.ConditionUnknown,
			wantReason: reasonUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildReadReplicasAutopilotHealthyCondition(tt.cluster, tt.state)

			assert.Equal(t, string(openbaov1alpha1.ConditionReadReplicasAutopilotHealthy), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
		})
	}
}

func TestBuildLeaderCondition(t *testing.T) {
	tests := []struct {
		name        string
		leaderCount int
		leaderName  string
		wantStatus  metav1.ConditionStatus
		wantReason  string
	}{
		{
			name:        "no leader",
			leaderCount: 0,
			wantStatus:  metav1.ConditionUnknown,
			wantReason:  ReasonLeaderUnknown,
		},
		{
			name:        "single leader",
			leaderCount: 1,
			leaderName:  "my-cluster-0",
			wantStatus:  metav1.ConditionTrue,
			wantReason:  ReasonLeaderFound,
		},
		{
			name:        "multiple leaders (split brain)",
			leaderCount: 2,
			wantStatus:  metav1.ConditionFalse,
			wantReason:  ReasonMultipleLeaders,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildLeaderCondition(tt.leaderCount, tt.leaderName)

			assert.Equal(t, string(openbaov1alpha1.ConditionOpenBaoLeader), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
		})
	}
}

func TestBuildInitializedCondition(t *testing.T) {
	tests := []struct {
		name        string
		initialized bool
		present     bool
		wantStatus  metav1.ConditionStatus
	}{
		{
			name:       "state not known",
			present:    false,
			wantStatus: metav1.ConditionUnknown,
		},
		{
			name:        "initialized",
			initialized: true,
			present:     true,
			wantStatus:  metav1.ConditionTrue,
		},
		{
			name:        "not initialized",
			initialized: false,
			present:     true,
			wantStatus:  metav1.ConditionFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildInitializedCondition(tt.initialized, tt.present)

			assert.Equal(t, string(openbaov1alpha1.ConditionOpenBaoInitialized), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
		})
	}
}

func TestComputePhase(t *testing.T) {
	tests := []struct {
		name      string
		state     *clusterState
		wantPhase openbaov1alpha1.ClusterPhase
	}{
		{
			name:      "initializing",
			state:     &clusterState{Available: false},
			wantPhase: openbaov1alpha1.ClusterPhaseInitializing,
		},
		{
			name:      "running",
			state:     &clusterState{Available: true},
			wantPhase: openbaov1alpha1.ClusterPhaseRunning,
		},
		{
			name:      "upgrading",
			state:     &clusterState{UpgradeInProgress: true},
			wantPhase: openbaov1alpha1.ClusterPhaseUpgrading,
		},
		{
			name:      "backing up",
			state:     &clusterState{BackupInProgress: true},
			wantPhase: openbaov1alpha1.ClusterPhaseBackingUp,
		},
		{
			name:      "failed",
			state:     &clusterState{UpgradeFailed: true},
			wantPhase: openbaov1alpha1.ClusterPhaseFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phase := computePhase(tt.state)
			assert.Equal(t, tt.wantPhase, phase)
		})
	}
}
