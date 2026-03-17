package openbaocluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

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

func boolPtr(v bool) *bool {
	return &v
}
