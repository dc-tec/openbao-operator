package statusops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestEvaluateProductionReady(t *testing.T) {
	tests := []struct {
		name            string
		cluster         *openbaov1alpha1.OpenBaoCluster
		unsafeAdmission bool
		wantStatus      metav1.ConditionStatus
		wantReason      string
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
			name: "hardened with unsafe admission mode",
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
			},
			unsafeAdmission: true,
			wantStatus:      metav1.ConditionFalse,
			wantReason:      ReasonUnsafeAdmissionDisabled,
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
						Reason: constants.ReasonAPIServerNetworkConfigurationInvalid,
					}},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: constants.ReasonAPIServerNetworkConfigurationInvalid,
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
						Reason: constants.ReasonAPIServerEndpointIPsRecommended,
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
			name: "hardened with security context weakening",
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
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(false),
					},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonSecurityContextWeakening,
		},
		{
			name: "hardened with tls disabled",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newProductionReadyHardenedCluster()
				cluster.Spec.TLS.Enabled = false
				return cluster
			}(),
			wantStatus: metav1.ConditionFalse,
			wantReason: constants.ReasonSecurityViolation,
		},
		{
			name: "hardened with backup ambient storage identity",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newProductionReadyHardenedCluster()
				cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
					Target: openbaov1alpha1.BackupTarget{
						Provider: "s3",
						Bucket:   "backups",
					},
				}
				return cluster
			}(),
			wantStatus: metav1.ConditionFalse,
			wantReason: constants.ReasonSecurityViolation,
		},
		{
			name: "hardened with raw ingress rules",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newProductionReadyHardenedCluster()
				cluster.Spec.Network = &openbaov1alpha1.NetworkConfig{
					IngressRules: []networkingv1.NetworkPolicyIngressRule{{}},
				}
				return cluster
			}(),
			wantStatus: metav1.ConditionFalse,
			wantReason: constants.ReasonSecurityViolation,
		},
		{
			name: "hardened with wildcard egress rule",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newProductionReadyHardenedCluster()
				port := intstr.FromInt32(443)
				cluster.Spec.Network = &openbaov1alpha1.NetworkConfig{
					EgressRules: []networkingv1.NetworkPolicyEgressRule{
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"},
								},
							},
							Ports: []networkingv1.NetworkPolicyPort{
								{
									Protocol: ptr.To(corev1.ProtocolTCP),
									Port:     &port,
								},
							},
						},
					},
				}
				return cluster
			}(),
			wantStatus: metav1.ConditionFalse,
			wantReason: constants.ReasonSecurityViolation,
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
						Reason: constants.ReasonACMEGatewayNotConfiguredForPassthrough,
					}},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: constants.ReasonACMEGatewayNotConfiguredForPassthrough,
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
							Reason: constants.ReasonACMEIntegrationReady,
						},
						{
							Type:   string(openbaov1alpha1.ConditionACMECacheReady),
							Status: metav1.ConditionFalse,
							Reason: reasonACMECachePending,
						},
					},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: reasonACMECachePending,
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
						Reason: constants.ReasonGatewayNotProgrammed,
					}},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: constants.ReasonGatewayNotProgrammed,
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
						Reason: constants.ReasonGatewayCapabilitiesUnknown,
					}},
				},
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonProductionReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reason, _ := evaluateProductionReady(tt.cluster, true, "", tt.unsafeAdmission)
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
}

func newProductionReadyHardenedCluster() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
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
	}
}

func TestHardenedSecurityContextWeakensPodSecurity(t *testing.T) {
	tests := []struct {
		name            string
		securityContext *corev1.PodSecurityContext
		want            bool
	}{
		{
			name:            "nil security context",
			securityContext: nil,
			want:            false,
		},
		{
			name: "safe non-root overrides",
			securityContext: &corev1.PodSecurityContext{
				RunAsNonRoot:       ptr.To(true),
				RunAsUser:          ptr.To(int64(1001)),
				RunAsGroup:         ptr.To(int64(1001)),
				FSGroup:            ptr.To(int64(1001)),
				SupplementalGroups: []int64{1002},
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			want: false,
		},
		{
			name: "run as root allowed",
			securityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(false),
			},
			want: true,
		},
		{
			name: "root run as user",
			securityContext: &corev1.PodSecurityContext{
				RunAsUser: ptr.To(int64(0)),
			},
			want: true,
		},
		{
			name: "root run as group",
			securityContext: &corev1.PodSecurityContext{
				RunAsGroup: ptr.To(int64(0)),
			},
			want: true,
		},
		{
			name: "root fs group",
			securityContext: &corev1.PodSecurityContext{
				FSGroup: ptr.To(int64(0)),
			},
			want: true,
		},
		{
			name: "root supplemental group",
			securityContext: &corev1.PodSecurityContext{
				SupplementalGroups: []int64{0},
			},
			want: true,
		},
		{
			name: "unconfined seccomp",
			securityContext: &corev1.PodSecurityContext{
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeUnconfined,
				},
			},
			want: true,
		},
		{
			name: "pod sysctls",
			securityContext: &corev1.PodSecurityContext{
				Sysctls: []corev1.Sysctl{{
					Name:  "kernel.shm_rmid_forced",
					Value: "1",
				}},
			},
			want: true,
		},
		{
			name: "windows options",
			securityContext: &corev1.PodSecurityContext{
				WindowsOptions: &corev1.WindowsSecurityContextOptions{},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SecurityContext: tt.securityContext,
				},
			}

			assert.Equal(t, tt.want, hardenedSecurityContextWeakensPodSecurity(cluster))
		})
	}
}

func boolPtr(v bool) *bool {
	return &v
}
