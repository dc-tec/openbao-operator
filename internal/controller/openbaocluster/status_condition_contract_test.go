package openbaocluster

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestSetACMEIntegrationReadyEvaluatedCondition_AllowsKnownReasonStatusPairs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status metav1.ConditionStatus
		reason string
	}{
		{name: "ready", status: metav1.ConditionTrue, reason: ReasonACMEIntegrationReady},
		{name: "gateway api missing", status: metav1.ConditionFalse, reason: ReasonGatewayAPIMissing},
		{name: "gateway passthrough missing", status: metav1.ConditionFalse, reason: ReasonACMEGatewayNotConfiguredForPassthrough},
		{name: "domain not resolvable", status: metav1.ConditionFalse, reason: ReasonACMEDomainNotResolvable},
		{name: "prerequisites missing", status: metav1.ConditionFalse, reason: ReasonPrerequisitesMissing},
		{name: "paused", status: metav1.ConditionUnknown, reason: reasonPaused},
		{name: "profile not set", status: metav1.ConditionUnknown, reason: ReasonProfileNotSet},
		{name: "unknown", status: metav1.ConditionUnknown, reason: reasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newOpenBaoClusterStatusTestObject()
			cluster.Generation = 17

			setACMEIntegrationReadyEvaluatedCondition(cluster, appopenbaocluster.ACMEIntegrationResult{
				Status:  tt.status,
				Reason:  tt.reason,
				Message: "contract message",
			})

			assertClusterCondition(
				t,
				cluster,
				openbaov1alpha1.ConditionACMEIntegrationReady,
				true,
				tt.status,
				tt.reason,
				"contract message",
			)
		})
	}
}

func TestSetGatewayIntegrationReadyEvaluatedCondition_AllowsKnownReasonStatusPairs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status metav1.ConditionStatus
		reason string
	}{
		{name: "ready", status: metav1.ConditionTrue, reason: ReasonGatewayIntegrationReady},
		{name: "gateway api missing", status: metav1.ConditionFalse, reason: ReasonGatewayAPIMissing},
		{name: "gateway reference missing", status: metav1.ConditionFalse, reason: ReasonGatewayReferenceMissing},
		{name: "gateway class missing", status: metav1.ConditionFalse, reason: ReasonGatewayClassMissing},
		{name: "gateway listener incompatible", status: metav1.ConditionFalse, reason: ReasonGatewayListenerIncompatible},
		{name: "gateway class not accepted", status: metav1.ConditionFalse, reason: ReasonGatewayClassNotAccepted},
		{name: "gateway version unsupported", status: metav1.ConditionFalse, reason: ReasonGatewayVersionUnsupported},
		{name: "gateway feature unsupported", status: metav1.ConditionFalse, reason: ReasonGatewayFeatureUnsupported},
		{name: "gateway not programmed", status: metav1.ConditionFalse, reason: ReasonGatewayNotProgrammed},
		{name: "gateway class pending", status: metav1.ConditionUnknown, reason: ReasonGatewayClassPending},
		{name: "gateway capabilities unknown", status: metav1.ConditionUnknown, reason: ReasonGatewayCapabilitiesUnknown},
		{name: "gateway programming pending", status: metav1.ConditionUnknown, reason: ReasonGatewayProgrammingPending},
		{name: "paused", status: metav1.ConditionUnknown, reason: reasonPaused},
		{name: "profile not set", status: metav1.ConditionUnknown, reason: ReasonProfileNotSet},
		{name: "unknown", status: metav1.ConditionUnknown, reason: reasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newOpenBaoClusterStatusTestObject()
			cluster.Generation = 17

			setGatewayIntegrationReadyEvaluatedCondition(cluster, appopenbaocluster.GatewayIntegrationResult{
				Status:  tt.status,
				Reason:  tt.reason,
				Message: "contract message",
			})

			assertClusterCondition(
				t,
				cluster,
				openbaov1alpha1.ConditionGatewayIntegrationReady,
				true,
				tt.status,
				tt.reason,
				"contract message",
			)
		})
	}
}

func TestSetAPIServerNetworkReadyEvaluatedCondition_AllowsKnownReasonStatusPairs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status metav1.ConditionStatus
		reason string
	}{
		{name: "ready", status: metav1.ConditionTrue, reason: ReasonAPIServerNetworkReady},
		{name: "recommended", status: metav1.ConditionUnknown, reason: ReasonAPIServerEndpointIPsRecommended},
		{name: "configuration invalid", status: metav1.ConditionFalse, reason: ReasonAPIServerNetworkConfigurationInvalid},
		{name: "paused", status: metav1.ConditionUnknown, reason: reasonPaused},
		{name: "profile not set", status: metav1.ConditionUnknown, reason: ReasonProfileNotSet},
		{name: "unknown", status: metav1.ConditionUnknown, reason: reasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newOpenBaoClusterStatusTestObject()
			cluster.Generation = 17

			setAPIServerNetworkReadyEvaluatedCondition(cluster, appopenbaocluster.APIServerNetworkResult{
				Status:  tt.status,
				Reason:  tt.reason,
				Message: "contract message",
			})

			assertClusterCondition(
				t,
				cluster,
				openbaov1alpha1.ConditionAPIServerNetworkReady,
				true,
				tt.status,
				tt.reason,
				"contract message",
			)
		})
	}
}

func TestSetTLSReadyEvaluatedCondition_AllowsKnownReasonStatusPairs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status metav1.ConditionStatus
		reason string
	}{
		{name: "disabled", status: metav1.ConditionTrue, reason: ReasonDisabled},
		{name: "ready", status: metav1.ConditionTrue, reason: reasonReady},
		{name: "missing secret", status: metav1.ConditionFalse, reason: ReasonTLSSecretMissing},
		{name: "invalid secret", status: metav1.ConditionFalse, reason: ReasonTLSSecretInvalid},
		{name: "paused", status: metav1.ConditionUnknown, reason: reasonPaused},
		{name: "profile not set", status: metav1.ConditionUnknown, reason: ReasonProfileNotSet},
		{name: "unknown", status: metav1.ConditionUnknown, reason: reasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newOpenBaoClusterStatusTestObject()
			cluster.Generation = 17

			setTLSReadyEvaluatedCondition(cluster, statusConditionResult{
				Status:  tt.status,
				Reason:  tt.reason,
				Message: "contract message",
			})

			assertClusterCondition(
				t,
				cluster,
				openbaov1alpha1.ConditionTLSReady,
				true,
				tt.status,
				tt.reason,
				"contract message",
			)
		})
	}
}

func TestSetACMECacheReadyEvaluatedCondition_AllowsKnownReasonStatusPairs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status metav1.ConditionStatus
		reason string
	}{
		{name: "ready", status: metav1.ConditionTrue, reason: ReasonACMECacheReady},
		{name: "not configured", status: metav1.ConditionFalse, reason: ReasonACMECacheNotConfigured},
		{name: "missing", status: metav1.ConditionFalse, reason: ReasonACMECacheMissing},
		{name: "pending", status: metav1.ConditionFalse, reason: ReasonACMECachePending},
		{name: "invalid access mode", status: metav1.ConditionFalse, reason: ReasonACMECacheInvalidAccessMode},
		{name: "unknown", status: metav1.ConditionUnknown, reason: reasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newOpenBaoClusterStatusTestObject()
			cluster.Generation = 17

			setACMECacheReadyEvaluatedCondition(cluster, statusConditionResult{
				Status:  tt.status,
				Reason:  tt.reason,
				Message: "contract message",
			})

			assertClusterCondition(
				t,
				cluster,
				openbaov1alpha1.ConditionACMECacheReady,
				true,
				tt.status,
				tt.reason,
				"contract message",
			)
		})
	}
}

func TestSetBackupConfigurationReadyEvaluatedCondition_AllowsKnownReasonStatusPairs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status metav1.ConditionStatus
		reason string
	}{
		{name: "ready", status: metav1.ConditionTrue, reason: reasonReady},
		{name: "ambient identity assumed", status: metav1.ConditionTrue, reason: constants.ReasonAmbientIdentityAssumed},
		{name: "workload identity configured", status: metav1.ConditionTrue, reason: constants.ReasonWorkloadIdentityConfigured},
		{name: "authentication required", status: metav1.ConditionFalse, reason: constants.ReasonAuthenticationRequired},
		{name: "token secret missing", status: metav1.ConditionFalse, reason: constants.ReasonTokenSecretMissing},
		{name: "credentials secret missing", status: metav1.ConditionFalse, reason: constants.ReasonCredentialsSecretMissing},
		{name: "network egress rules required", status: metav1.ConditionFalse, reason: constants.ReasonNetworkEgressRulesRequired},
		{name: "paused", status: metav1.ConditionUnknown, reason: reasonPaused},
		{name: "profile not set", status: metav1.ConditionUnknown, reason: ReasonProfileNotSet},
		{name: "unknown", status: metav1.ConditionUnknown, reason: reasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newOpenBaoClusterStatusTestObject()
			cluster.Generation = 17

			setBackupConfigurationReadyEvaluatedCondition(cluster, appopenbaocluster.BackupConfigurationResult{
				Status:  tt.status,
				Reason:  tt.reason,
				Message: "contract message",
			})

			assertClusterCondition(
				t,
				cluster,
				openbaov1alpha1.ConditionBackupConfigurationReady,
				true,
				tt.status,
				tt.reason,
				"contract message",
			)
		})
	}
}

func TestSetCloudUnsealIdentityReadyEvaluatedCondition_AllowsKnownReasonStatusPairs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status metav1.ConditionStatus
		reason string
	}{
		{name: "ready", status: metav1.ConditionTrue, reason: reasonReady},
		{name: "ambient identity assumed", status: metav1.ConditionTrue, reason: constants.ReasonAmbientIdentityAssumed},
		{name: "workload identity configured", status: metav1.ConditionTrue, reason: constants.ReasonWorkloadIdentityConfigured},
		{name: "credentials secret missing", status: metav1.ConditionFalse, reason: constants.ReasonCredentialsSecretMissing},
		{name: "prerequisites missing", status: metav1.ConditionFalse, reason: constants.ReasonPrerequisitesMissing},
		{name: "paused", status: metav1.ConditionUnknown, reason: reasonPaused},
		{name: "profile not set", status: metav1.ConditionUnknown, reason: ReasonProfileNotSet},
		{name: "unknown", status: metav1.ConditionUnknown, reason: reasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newOpenBaoClusterStatusTestObject()
			cluster.Generation = 17

			setCloudUnsealIdentityReadyEvaluatedCondition(cluster, statusConditionResult{
				Status:  tt.status,
				Reason:  tt.reason,
				Message: "contract message",
			})

			assertClusterCondition(
				t,
				cluster,
				openbaov1alpha1.ConditionCloudUnsealIdentityReady,
				true,
				tt.status,
				tt.reason,
				"contract message",
			)
		})
	}
}

func TestEvaluatedConditionContracts_RejectUnexpectedReasonAndStatusPairs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		conditionType openbaov1alpha1.ConditionType
		apply         func(*openbaov1alpha1.OpenBaoCluster)
	}{
		{
			name:          "acme rejects gateway ready reason",
			conditionType: openbaov1alpha1.ConditionACMEIntegrationReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setACMEIntegrationReadyEvaluatedCondition(cluster, appopenbaocluster.ACMEIntegrationResult{
					Status:  metav1.ConditionFalse,
					Reason:  ReasonGatewayIntegrationReady,
					Message: "wrong reason for condition",
				})
			},
		},
		{
			name:          "acme rejects ready reason with false status",
			conditionType: openbaov1alpha1.ConditionACMEIntegrationReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setACMEIntegrationReadyEvaluatedCondition(cluster, appopenbaocluster.ACMEIntegrationResult{
					Status:  metav1.ConditionFalse,
					Reason:  ReasonACMEIntegrationReady,
					Message: "wrong status for reason",
				})
			},
		},
		{
			name:          "gateway rejects acme ready reason",
			conditionType: openbaov1alpha1.ConditionGatewayIntegrationReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setGatewayIntegrationReadyEvaluatedCondition(cluster, appopenbaocluster.GatewayIntegrationResult{
					Status:  metav1.ConditionFalse,
					Reason:  ReasonACMEIntegrationReady,
					Message: "wrong reason for condition",
				})
			},
		},
		{
			name:          "gateway rejects pending reason with false status",
			conditionType: openbaov1alpha1.ConditionGatewayIntegrationReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setGatewayIntegrationReadyEvaluatedCondition(cluster, appopenbaocluster.GatewayIntegrationResult{
					Status:  metav1.ConditionFalse,
					Reason:  ReasonGatewayClassPending,
					Message: "wrong status for reason",
				})
			},
		},
		{
			name:          "api server rejects gateway reason",
			conditionType: openbaov1alpha1.ConditionAPIServerNetworkReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setAPIServerNetworkReadyEvaluatedCondition(cluster, appopenbaocluster.APIServerNetworkResult{
					Status:  metav1.ConditionTrue,
					Reason:  ReasonGatewayIntegrationReady,
					Message: "wrong reason for condition",
				})
			},
		},
		{
			name:          "api server rejects ready reason with false status",
			conditionType: openbaov1alpha1.ConditionAPIServerNetworkReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setAPIServerNetworkReadyEvaluatedCondition(cluster, appopenbaocluster.APIServerNetworkResult{
					Status:  metav1.ConditionFalse,
					Reason:  ReasonAPIServerNetworkReady,
					Message: "wrong status for reason",
				})
			},
		},
		{
			name:          "tls rejects acme cache reason",
			conditionType: openbaov1alpha1.ConditionTLSReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setTLSReadyEvaluatedCondition(cluster, statusConditionResult{
					Status:  metav1.ConditionTrue,
					Reason:  ReasonACMECacheReady,
					Message: "wrong reason for condition",
				})
			},
		},
		{
			name:          "tls rejects ready reason with false status",
			conditionType: openbaov1alpha1.ConditionTLSReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setTLSReadyEvaluatedCondition(cluster, statusConditionResult{
					Status:  metav1.ConditionFalse,
					Reason:  reasonReady,
					Message: "wrong status for reason",
				})
			},
		},
		{
			name:          "acme cache rejects tls reason",
			conditionType: openbaov1alpha1.ConditionACMECacheReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setACMECacheReadyEvaluatedCondition(cluster, statusConditionResult{
					Status:  metav1.ConditionTrue,
					Reason:  ReasonTLSSecretInvalid,
					Message: "wrong reason for condition",
				})
			},
		},
		{
			name:          "acme cache rejects ready reason with false status",
			conditionType: openbaov1alpha1.ConditionACMECacheReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setACMECacheReadyEvaluatedCondition(cluster, statusConditionResult{
					Status:  metav1.ConditionFalse,
					Reason:  ReasonACMECacheReady,
					Message: "wrong status for reason",
				})
			},
		},
		{
			name:          "backup configuration rejects gateway reason",
			conditionType: openbaov1alpha1.ConditionBackupConfigurationReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setBackupConfigurationReadyEvaluatedCondition(cluster, appopenbaocluster.BackupConfigurationResult{
					Status:  metav1.ConditionTrue,
					Reason:  ReasonGatewayIntegrationReady,
					Message: "wrong reason for condition",
				})
			},
		},
		{
			name:          "backup configuration rejects ready reason with false status",
			conditionType: openbaov1alpha1.ConditionBackupConfigurationReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setBackupConfigurationReadyEvaluatedCondition(cluster, appopenbaocluster.BackupConfigurationResult{
					Status:  metav1.ConditionFalse,
					Reason:  reasonReady,
					Message: "wrong status for reason",
				})
			},
		},
		{
			name:          "cloud unseal rejects gateway reason",
			conditionType: openbaov1alpha1.ConditionCloudUnsealIdentityReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setCloudUnsealIdentityReadyEvaluatedCondition(cluster, statusConditionResult{
					Status:  metav1.ConditionTrue,
					Reason:  ReasonGatewayIntegrationReady,
					Message: "wrong reason for condition",
				})
			},
		},
		{
			name:          "cloud unseal rejects ready reason with false status",
			conditionType: openbaov1alpha1.ConditionCloudUnsealIdentityReady,
			apply: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				setCloudUnsealIdentityReadyEvaluatedCondition(cluster, statusConditionResult{
					Status:  metav1.ConditionFalse,
					Reason:  reasonReady,
					Message: "wrong status for reason",
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newOpenBaoClusterStatusTestObject()
			cluster.Generation = 29

			tt.apply(cluster)

			cond := mustFindClusterCondition(t, cluster, tt.conditionType)
			if cond.Status != metav1.ConditionUnknown || cond.Reason != reasonUnknown {
				t.Fatalf("condition = %#v, want status=%s reason=%s", cond, metav1.ConditionUnknown, reasonUnknown)
			}
			if !strings.Contains(cond.Message, "Controller rejected unexpected") {
				t.Fatalf("message = %q, want contract violation prefix", cond.Message)
			}
		})
	}
}

func mustFindClusterCondition(
	t *testing.T,
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
) *metav1.Condition {
	t.Helper()

	for i := range cluster.Status.Conditions {
		cond := &cluster.Status.Conditions[i]
		if cond.Type == string(conditionType) {
			return cond
		}
	}

	t.Fatalf("expected %s condition", conditionType)
	return nil
}
