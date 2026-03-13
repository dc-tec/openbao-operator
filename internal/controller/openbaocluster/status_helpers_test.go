package openbaocluster

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
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
						FromVersion:      "2.0.0",
						TargetVersion:    "2.1.0",
						LastErrorReason:  "PodNotReady",
						LastErrorMessage: "Pod test-1 failed to become ready",
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

func TestBuildUserAccessBootstrapCondition(t *testing.T) {
	tests := []struct {
		name          string
		cluster       *openbaov1alpha1.OpenBaoCluster
		wantStatus    metav1.ConditionStatus
		wantReason    string
		wantInMessage string
	}{
		{
			name:       "self init disabled",
			cluster:    &openbaov1alpha1.OpenBaoCluster{},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonDisabled,
		},
		{
			name: "self init enabled but only operator oidc",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						OIDC:    &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true},
						Requests: []openbaov1alpha1.SelfInitRequest{
							{
								Name:      "operator-role",
								Operation: openbaov1alpha1.SelfInitOperationCreate,
								Path:      "auth/jwt-operator/role/openbao-operator",
							},
						},
					},
				},
			},
			wantStatus:    metav1.ConditionUnknown,
			wantReason:    ReasonUserAccessUnverified,
			wantInMessage: "spec.selfInit.oidc only bootstraps operator authentication",
		},
		{
			name: "structured auth method recognized",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						Requests: []openbaov1alpha1.SelfInitRequest{
							{
								Name:      "enable-userpass",
								Operation: openbaov1alpha1.SelfInitOperationCreate,
								Path:      "sys/auth/userpass",
								AuthMethod: &openbaov1alpha1.SelfInitAuthMethod{
									Type: "userpass",
								},
							},
						},
					},
				},
			},
			wantStatus:    metav1.ConditionTrue,
			wantReason:    ReasonUserAccessConfigured,
			wantInMessage: "auth/userpass",
		},
		{
			name: "auth request path recognized",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						Requests: []openbaov1alpha1.SelfInitRequest{
							{
								Name:      "configure-admin-role",
								Operation: openbaov1alpha1.SelfInitOperationCreate,
								Path:      "auth/jwt/role/admin",
							},
						},
					},
				},
			},
			wantStatus:    metav1.ConditionTrue,
			wantReason:    ReasonUserAccessConfigured,
			wantInMessage: "auth/jwt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildUserAccessBootstrapCondition(tt.cluster)

			assert.Equal(t, string(openbaov1alpha1.ConditionUserAccessBootstrap), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
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
						FromVersion:      "2.0.0",
						TargetVersion:    "2.1.0",
						LastErrorReason:  "PodNotReady",
						LastErrorMessage: "Pod failed to become ready",
					},
				},
			},
			wantStatus: metav1.ConditionFalse, // Failed upgrade shows as not upgrading
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

func TestBuildStorageConfiguredCondition(t *testing.T) {
	className := "fast-ssd"

	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		state      *clusterState
		wantStatus metav1.ConditionStatus
		wantReason string
		wantInMsg  string
	}{
		{
			name: "explicit storage class before pvc creation",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Storage: openbaov1alpha1.StorageConfig{
						StorageClassName: &className,
					},
				},
			},
			state:      &clusterState{},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonStorageClassConfigured,
			wantInMsg:  "Configured to request",
		},
		{
			name: "default storage class pending",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			state:      &clusterState{},
			wantStatus: metav1.ConditionUnknown,
			wantReason: ReasonStorageClassPending,
			wantInMsg:  "rely on the default StorageClass",
		},
		{
			name: "default storage class resolved from pvcs",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			state: &clusterState{
				DataPVCCount:             3,
				DataPVCStorageClassNames: []string{"gp3"},
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonStorageClassDefaulted,
			wantInMsg:  "Using default StorageClass",
		},
		{
			name: "configured storage class mismatch",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Storage: openbaov1alpha1.StorageConfig{
						StorageClassName: &className,
					},
				},
			},
			state: &clusterState{
				DataPVCCount:             1,
				DataPVCStorageClassNames: []string{"gp3"},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonStorageClassMismatch,
			wantInMsg:  "does not match",
		},
		{
			name: "inconsistent storage classes across pvcs",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			state: &clusterState{
				DataPVCCount:             3,
				DataPVCStorageClassNames: []string{"fast", "slow"},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonStorageClassInconsistent,
			wantInMsg:  "inconsistent StorageClass values",
		},
		{
			name: "pvcs created without storage class",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			state: &clusterState{
				DataPVCCount:             2,
				DataPVCStorageClassUnset: true,
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonStorageClassUnset,
			wantInMsg:  "without a StorageClass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildStorageConfiguredCondition(tt.cluster, tt.state)

			assert.Equal(t, string(openbaov1alpha1.ConditionStorageConfigured), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
			assert.Contains(t, cond.Message, tt.wantInMsg)
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
			name: "hardened with invalid api server network config",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeExternal,
					},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Conditions: []metav1.Condition{{
						Type:   string(openbaov1alpha1.ConditionAPIServerNetworkReady),
						Status: metav1.ConditionFalse,
						Reason: ReasonAPIServerNetworkConfigurationInvalid,
					}},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonAPIServerNetworkConfigurationInvalid,
		},
		{
			name: "hardened with api server network unknown does not block",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeExternal,
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "transit",
						Transit: &openbaov1alpha1.TransitSealConfig{
							Address:   "https://infra-bao.example",
							KeyName:   "autounseal",
							MountPath: "transit/",
						},
					},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Conditions: []metav1.Condition{{
						Type:   string(openbaov1alpha1.ConditionAPIServerNetworkReady),
						Status: metav1.ConditionUnknown,
						Reason: ReasonAPIServerEndpointIPsRecommended,
					}},
				},
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonProductionReady,
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
		{
			name: "hardened transit with tls skip verify",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeExternal,
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "transit",
						Transit: &openbaov1alpha1.TransitSealConfig{
							Address:       "https://infra-bao.example",
							KeyName:       "autounseal",
							MountPath:     "transit/",
							TLSSkipVerify: boolPtr(true),
						},
					},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonUnsealTLSSkipVerify,
		},
		{
			name: "hardened transit with inline token",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeExternal,
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "transit",
						Transit: &openbaov1alpha1.TransitSealConfig{
							Address:   "https://infra-bao.example",
							KeyName:   "autounseal",
							MountPath: "transit/",
							Token:     "s.inline",
						},
					},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonTransitInlineToken,
		},
		{
			name: "hardened transit without https",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeExternal,
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "transit",
						Transit: &openbaov1alpha1.TransitSealConfig{
							Address:   "http://infra-bao.example",
							KeyName:   "autounseal",
							MountPath: "transit/",
						},
					},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonTransitAddressNotHTTPS,
		},
		{
			name: "hardened cloud kms without ready unseal identity condition",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeExternal,
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "awskms",
						AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{
							Region:   "eu-central-1",
							KMSKeyID: "alias/openbao",
						},
					},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Conditions: []metav1.Condition{{
						Type:   string(openbaov1alpha1.ConditionCloudUnsealIdentityReady),
						Status: metav1.ConditionFalse,
						Reason: constants.ReasonCredentialsSecretMissing,
					}},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: constants.ReasonCredentialsSecretMissing,
		},
		{
			name: "hardened acme without integration readiness",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
						ACME: &openbaov1alpha1.ACMEConfig{
							DirectoryURL: "https://acme.example/directory",
						},
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "transit",
						Transit: &openbaov1alpha1.TransitSealConfig{
							Address:   "https://infra-bao.example",
							KeyName:   "autounseal",
							MountPath: "transit/",
						},
					},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Conditions: []metav1.Condition{{
						Type:   string(openbaov1alpha1.ConditionACMEIntegrationReady),
						Status: metav1.ConditionFalse,
						Reason: ReasonACMEGatewayNotConfiguredForPassthrough,
					}},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonACMEGatewayNotConfiguredForPassthrough,
		},
		{
			name: "hardened acme without ready shared cache",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					Replicas: 3,
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
						ACME: &openbaov1alpha1.ACMEConfig{
							DirectoryURL: "https://acme.example/directory",
							SharedCache: &openbaov1alpha1.ACMESharedCacheConfig{
								Mode: openbaov1alpha1.ACMESharedCacheModeManagedPVC,
								Size: "1Gi",
							},
						},
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "transit",
						Transit: &openbaov1alpha1.TransitSealConfig{
							Address:   "https://infra-bao.example",
							KeyName:   "autounseal",
							MountPath: "transit/",
						},
					},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(openbaov1alpha1.ConditionACMEIntegrationReady),
							Status: metav1.ConditionTrue,
							Reason: ReasonACMEIntegrationReady,
						},
						{
							Type:   string(openbaov1alpha1.ConditionACMECacheReady),
							Status: metav1.ConditionFalse,
							Reason: ReasonACMECachePending,
						},
					},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonACMECachePending,
		},
		{
			name: "hardened gateway without ready gateway integration",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeExternal,
					},
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled:  true,
						Hostname: "bao.example.test",
						GatewayRef: openbaov1alpha1.GatewayReference{
							Name: "shared-gateway",
						},
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "transit",
						Transit: &openbaov1alpha1.TransitSealConfig{
							Address:   "https://infra-bao.example",
							KeyName:   "autounseal",
							MountPath: "transit/",
						},
					},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Conditions: []metav1.Condition{{
						Type:   string(openbaov1alpha1.ConditionGatewayIntegrationReady),
						Status: metav1.ConditionFalse,
						Reason: ReasonGatewayNotProgrammed,
					}},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonGatewayNotProgrammed,
		},
		{
			name: "gateway integration unknown does not block hardened production ready",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeExternal,
					},
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled:  true,
						Hostname: "bao.example.test",
						GatewayRef: openbaov1alpha1.GatewayReference{
							Name: "shared-gateway",
						},
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "transit",
						Transit: &openbaov1alpha1.TransitSealConfig{
							Address:   "https://infra-bao.example",
							KeyName:   "autounseal",
							MountPath: "transit/",
						},
					},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Conditions: []metav1.Condition{{
						Type:   string(openbaov1alpha1.ConditionGatewayIntegrationReady),
						Status: metav1.ConditionUnknown,
						Reason: ReasonGatewayCapabilitiesUnknown,
					}},
				},
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonProductionReady,
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

func TestReconcileCurrentVersion_SkipsWhenRollingUpgradeStatusExists(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.3",
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				TargetVersion:   "2.4.4",
				LastErrorReason: "UpgradeFailed",
			},
		},
	}

	state := &clusterState{
		RollingUpgradeInProgress: true,
		UpgradeInProgress:        false,
		UpgradeFailed:            true,
	}

	appopenbaocluster.ReconcileCurrentVersion(logr.Discard(), cluster, state, "2.4.4")
	assert.Equal(t, "2.4.3", cluster.Status.CurrentVersion)
}

func TestReconcileCurrentVersion_DoesNotRegressWhenObservedVersionIsLower(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.4",
		},
	}

	state := &clusterState{
		RollingUpgradeInProgress: false,
		BlueGreenInProgress:      false,
		UpgradeInProgress:        false,
	}

	appopenbaocluster.ReconcileCurrentVersion(logr.Discard(), cluster, state, "2.4.3")
	assert.Equal(t, "2.4.4", cluster.Status.CurrentVersion)
}

func TestReconcileCurrentVersion_AdvancesWhenObservedVersionIsHigher(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.3",
		},
	}

	state := &clusterState{
		RollingUpgradeInProgress: false,
		BlueGreenInProgress:      false,
		UpgradeInProgress:        false,
	}

	appopenbaocluster.ReconcileCurrentVersion(logr.Discard(), cluster, state, "2.4.4")
	assert.Equal(t, "2.4.4", cluster.Status.CurrentVersion)
}

func boolPtr(v bool) *bool {
	return &v
}
