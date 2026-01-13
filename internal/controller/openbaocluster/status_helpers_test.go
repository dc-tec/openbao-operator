package openbaocluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/admission"
)

func TestSetAvailableCondition(t *testing.T) {
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
			var conditions []metav1.Condition
			setAvailableCondition(&conditions, 1, cluster, tt.readyReplicas)

			cond := meta.FindStatusCondition(conditions, string(openbaov1alpha1.ConditionAvailable))
			assert.NotNil(t, cond)
			assert.Equal(t, string(openbaov1alpha1.ConditionAvailable), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
		})
	}
}

func TestSetDegradedCondition(t *testing.T) {
	tests := []struct {
		name            string
		cluster         *openbaov1alpha1.OpenBaoCluster
		admissionStatus *admission.Status
		upgradeFailed   bool
		wantStatus      metav1.ConditionStatus
		wantReason      string
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
			wantStatus: metav1.ConditionTrue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var conditions []metav1.Condition
			setDegradedCondition(&conditions, 1, tt.cluster, tt.admissionStatus, tt.upgradeFailed)

			cond := meta.FindStatusCondition(conditions, string(openbaov1alpha1.ConditionDegraded))
			assert.NotNil(t, cond)
			assert.Equal(t, string(openbaov1alpha1.ConditionDegraded), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			if tt.wantReason != "" {
				assert.Equal(t, tt.wantReason, cond.Reason)
			}
		})
	}
}

func TestSetUpgradingCondition(t *testing.T) {
	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		wantStatus metav1.ConditionStatus
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
		},
		{
			name: "upgrade failed",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Upgrade: &openbaov1alpha1.UpgradeProgress{
						LastErrorReason:  "PodNotReady",
						LastErrorMessage: "Pod failed to become ready",
					},
				},
			},
			wantStatus: metav1.ConditionFalse, // Failed upgrade shows as not upgrading
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var conditions []metav1.Condition
			setUpgradingCondition(&conditions, 1, tt.cluster)

			cond := meta.FindStatusCondition(conditions, string(openbaov1alpha1.ConditionUpgrading))
			assert.NotNil(t, cond)
			assert.Equal(t, string(openbaov1alpha1.ConditionUpgrading), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
		})
	}
}

func TestSetBackupCondition(t *testing.T) {
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
			var conditions []metav1.Condition
			setBackupCondition(&conditions, 1, tt.backupInProgress, tt.backupJobName)

			cond := meta.FindStatusCondition(conditions, string(openbaov1alpha1.ConditionBackingUp))
			assert.NotNil(t, cond)
			assert.Equal(t, string(openbaov1alpha1.ConditionBackingUp), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
		})
	}
}

func TestSetLeaderCondition(t *testing.T) {
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
			var conditions []metav1.Condition
			setLeaderCondition(&conditions, 1, tt.leaderCount, tt.leaderName)

			cond := meta.FindStatusCondition(conditions, string(openbaov1alpha1.ConditionOpenBaoLeader))
			assert.NotNil(t, cond)
			assert.Equal(t, string(openbaov1alpha1.ConditionOpenBaoLeader), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
		})
	}
}

func TestSetInitializedCondition(t *testing.T) {
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
			var conditions []metav1.Condition
			setInitializedCondition(&conditions, 1, tt.initialized, tt.present)

			cond := meta.FindStatusCondition(conditions, string(openbaov1alpha1.ConditionOpenBaoInitialized))
			assert.NotNil(t, cond)
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

func TestEvaluateProductionReady(t *testing.T) {
	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name: "profile not set",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonProfileNotSet,
		},
		{
			name: "development profile",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile: openbaov1alpha1.ProfileDevelopment,
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonDevelopmentProfile,
		},
		{
			name: "hardened but static unseal",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeExternal,
					},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonStaticUnsealInUse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reason, _ := evaluateProductionReady(tt.cluster, true, "")
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
}
