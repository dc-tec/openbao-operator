package claimcontract

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func mustDesiredSameClusterCluster(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	rendered *RenderedExecutionContract,
) *openbaov1alpha1.OpenBaoCluster {
	t.Helper()

	cluster, result := DesiredSameClusterCluster(claim, rendered)
	if !result.Valid {
		t.Fatalf("DesiredSameClusterCluster() = %#v, want valid", result)
	}
	if cluster == nil {
		t.Fatal("DesiredSameClusterCluster() returned nil cluster")
	}
	return cluster
}

func validRenderedGatewayExecutionContractFixture() *RenderedExecutionContract {
	return &RenderedExecutionContract{
		TargetNamespace: "payments",
		Cluster: RenderedCluster{
			Version:         "2.6.0",
			Replicas:        1,
			SecurityProfile: openbaov1alpha1.ProfileDevelopment,
		},
		Storage: RenderedStorage{PrimarySize: "20Gi"},
		Bootstrap: RenderedBootstrap{
			Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
			SecretEngines: &openbaov1alpha1.OpenBaoBootstrapSecretEnginesSpec{
				Mounts: []openbaov1alpha1.OpenBaoBootstrapSecretEngineMountSpec{{Type: "kv", Path: "secret"}},
			},
		},
		Exposure: RenderedExposure{
			PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeGateway,
			HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
				Mode:  openbaov1alpha1.OpenBaoExposureHostnamePolicyModeFixed,
				Value: "payments-bao.example.internal",
			},
			TLSPolicy: &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
				Mode: openbaov1alpha1.OpenBaoExposureTLSModeOperatorManaged,
			},
			Entrypoint: &RenderedExposureEntrypoint{
				Ref:  &RenderedBoundReference{Name: "shared-gateway-v1", UID: "entrypoint-uid"},
				Mode: openbaov1alpha1.OpenBaoEntrypointModeGateway,
				ObjectRef: openbaov1alpha1.OpenBaoEntrypointObjectReference{
					APIGroup:  "gateway.networking.k8s.io",
					Kind:      "Gateway",
					Name:      "shared-gateway",
					Namespace: "networking",
				},
				ListenerPolicy: &openbaov1alpha1.OpenBaoEntrypointListenerPolicySpec{
					SectionName: "https",
				},
			},
			Routing: &openbaov1alpha1.OpenBaoExposureRoutingSpec{
				Path: "/vault",
			},
			ServicePolicy: &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
				Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
				BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
			},
		},
	}
}

func validRenderedIngressExecutionContractFixture() *RenderedExecutionContract {
	return &RenderedExecutionContract{
		TargetNamespace: "payments",
		Cluster: RenderedCluster{
			Version:         "2.6.0",
			Replicas:        1,
			SecurityProfile: openbaov1alpha1.ProfileDevelopment,
		},
		Storage: RenderedStorage{PrimarySize: "20Gi"},
		Bootstrap: RenderedBootstrap{
			Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
			SecretEngines: &openbaov1alpha1.OpenBaoBootstrapSecretEnginesSpec{
				Mounts: []openbaov1alpha1.OpenBaoBootstrapSecretEngineMountSpec{{Type: "kv", Path: "secret"}},
			},
		},
		Exposure: RenderedExposure{
			PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeIngress,
			HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
				Mode:  openbaov1alpha1.OpenBaoExposureHostnamePolicyModeFixed,
				Value: "payments-bao.example.internal",
			},
			TLSPolicy: &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
				Mode: openbaov1alpha1.OpenBaoExposureTLSModeOperatorManaged,
			},
			Entrypoint: &RenderedExposureEntrypoint{
				Mode: openbaov1alpha1.OpenBaoEntrypointModeIngress,
				ObjectRef: openbaov1alpha1.OpenBaoEntrypointObjectReference{
					APIGroup: "networking.k8s.io",
					Kind:     "IngressClass",
					Name:     "nginx",
				},
			},
			Ingress: &RenderedExposureIngress{
				PolicyRef:                 &RenderedBoundReference{Name: "nginx-backend-tls-v1", UID: "ingress-policy-uid"},
				ClassName:                 "nginx",
				PathType:                  openbaov1alpha1.IngressPathTypePrefix,
				Annotations:               map[string]string{"nginx.ingress.kubernetes.io/backend-protocol": "HTTPS"},
				BackendTLSPublicationMode: openbaov1alpha1.OpenBaoIngressBackendTLSPublicationModeAnnotation,
				ReadinessMode:             openbaov1alpha1.IngressReadinessModeLoadBalancerPublished,
			},
			ServicePolicy: &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
				Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
				BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
			},
			Routing: &openbaov1alpha1.OpenBaoExposureRoutingSpec{
				Path: "/vault",
			},
		},
	}
}

func validRenderedHardenedExecutionContractFixture() *RenderedExecutionContract {
	return &RenderedExecutionContract{
		TargetNamespace: "payments",
		Cluster: RenderedCluster{
			Version:         "2.6.0",
			Replicas:        3,
			SecurityProfile: openbaov1alpha1.ProfileHardened,
		},
		Storage: RenderedStorage{
			PrimarySize: "20Gi",
		},
		Unseal: RenderedUnseal{
			Mode: UnsealPostureModeExternal,
			Transit: &RenderedTransitUnseal{
				Address:               "https://transit.example.internal:8200",
				KeyName:               "openbao-unseal",
				MountPath:             "transit",
				Namespace:             "platform",
				TLSCACert:             "/etc/bao/seal-creds/ca.crt",
				TLSServerName:         "transit.example.internal",
				CredentialsSecretName: "transit-unseal-creds",
			},
		},
		Bootstrap: RenderedBootstrap{
			Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
			SecretEngines: &openbaov1alpha1.OpenBaoBootstrapSecretEnginesSpec{
				Mounts: []openbaov1alpha1.OpenBaoBootstrapSecretEngineMountSpec{{Type: "kv", Path: "secret"}},
			},
		},
		Exposure: RenderedExposure{
			PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeClusterInternal,
			TLSPolicy: &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
				Mode: openbaov1alpha1.OpenBaoExposureTLSModeExternal,
			},
		},
	}
}

func validRenderedBackupEgressRules() []networkingv1.NetworkPolicyEgressRule {
	return []networkingv1.NetworkPolicyEgressRule{
		{
			To: []networkingv1.NetworkPolicyPeer{{
				IPBlock: &networkingv1.IPBlock{CIDR: "10.10.0.0/16"},
			}},
			Ports: []networkingv1.NetworkPolicyPort{{
				Protocol: ptr.To(corev1.ProtocolTCP),
				Port:     ptr.To(intstr.FromInt(443)),
			}},
		},
	}
}

func validRenderedBackupExecutionContract(profile openbaov1alpha1.Profile, replicas int32) *RenderedExecutionContract {
	rendered := &RenderedExecutionContract{
		TargetNamespace: "payments",
		Cluster: RenderedCluster{
			Version:         "2.6.0",
			Replicas:        replicas,
			SecurityProfile: profile,
		},
		Storage: RenderedStorage{PrimarySize: "20Gi"},
		Bootstrap: RenderedBootstrap{
			Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
			OperatorLifecycleAuth: openbaov1alpha1.OpenBaoBootstrapLifecycleAuthSpec{
				Mode: openbaov1alpha1.OpenBaoBootstrapLifecycleAuthModeJWT,
				JWT:  &openbaov1alpha1.OpenBaoBootstrapLifecycleJWTSpec{Audience: "openbao-operator"},
			},
			SecretEngines: &openbaov1alpha1.OpenBaoBootstrapSecretEnginesSpec{
				Mounts: []openbaov1alpha1.OpenBaoBootstrapSecretEngineMountSpec{{Type: "kv", Path: "secret"}},
			},
		},
		Exposure: RenderedExposure{
			PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeClusterInternal,
		},
		Backup: RenderedBackup{
			Schedule: "0 3 * * *",
			Retention: &openbaov1alpha1.BackupRetention{
				MaxCount: 30,
				MaxAge:   "720h",
			},
			TargetRef:          &RenderedBoundReference{Name: "primary-object-backup-v1", UID: "backup-target-uid"},
			BackendRef:         &RenderedBoundReference{Name: "s3-primary-v1", UID: "backup-backend-uid"},
			AuthProfileRef:     &RenderedBoundReference{Name: "aws-irsa-backup-v1", UID: "backup-auth-uid"},
			TransferProfileRef: &RenderedBoundReference{Name: "multipart-standard-v1", UID: "transfer-profile-uid"},
			Location:           "payments-prod",
			Partition:          "finance",
			KeyPrefix:          "claims/payments/payments-bao/finance",
			Backend: &RenderedBackupBackend{
				Driver:       openbaov1alpha1.OpenBaoBackupBackendDriverObjectStorage,
				Provider:     openbaov1alpha1.OpenBaoObjectStorageProviderS3,
				Endpoint:     "https://s3.example.internal",
				Region:       "eu-west-1",
				UsePathStyle: true,
			},
			Auth: &RenderedBackupAuth{
				Mode: openbaov1alpha1.OpenBaoBackupAuthModeWorkloadIdentity,
				WorkloadIdentity: &openbaov1alpha1.WorkloadIdentityConfig{
					ServiceAccountAnnotations: map[string]string{
						"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/openbao-backup",
					},
				},
				RoleARN: "arn:aws:iam::123456789012:role/openbao-backup",
			},
			Transfer: &RenderedBackupTransfer{
				PartSize:    16777216,
				Concurrency: 5,
			},
		},
		Network: RenderedNetwork{
			RequiredEgressRules: validRenderedBackupEgressRules(),
		},
	}

	if profile == openbaov1alpha1.ProfileHardened {
		rendered.Unseal = RenderedUnseal{
			Mode: UnsealPostureModeExternal,
			Transit: &RenderedTransitUnseal{
				Address:               "https://transit.example.internal:8200",
				KeyName:               "openbao-unseal",
				MountPath:             "transit",
				Namespace:             "platform",
				TLSCACert:             "/etc/bao/seal-creds/ca.crt",
				TLSServerName:         "transit.example.internal",
				CredentialsSecretName: "transit-unseal-creds",
			},
		}
		rendered.Exposure.TLSPolicy = &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
			Mode: openbaov1alpha1.OpenBaoExposureTLSModeExternal,
		}
	}

	return rendered
}
