package claimcontract

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func validRenderedPrimaryClaimFixture() *openbaov1alpha1.OpenBaoClusterClaim {
	return &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-v1"},
			ServiceParameters: &openbaov1alpha1.OpenBaoClusterClaimServiceParametersSpec{
				Backup: &openbaov1alpha1.OpenBaoClusterClaimBackupServiceParametersSpec{
					Location:  testBackupLocation,
					Partition: "finance",
				},
			},
		},
	}
}

func validRenderedDevelopmentClaimFixture() *openbaov1alpha1.OpenBaoClusterClaim {
	return &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-dev-v1"},
		},
	}
}

func validRenderedIngressClaimFixture() *openbaov1alpha1.OpenBaoClusterClaim {
	return &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-ingress-v1"},
		},
	}
}

func validRenderedPrimaryCatalogBundleFixture() *CatalogBundle {
	readReplicas := int32(1)
	preUpgradeSnapshot := true
	return &CatalogBundle{
		ServiceProfile: &openbaov1alpha1.OpenBaoServiceProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "standard-ha-v1", UID: types.UID("service-profile-uid")},
			Spec: openbaov1alpha1.OpenBaoServiceProfileSpec{
				Cluster: openbaov1alpha1.OpenBaoServiceProfileClusterSpec{
					Version:         "2.6.0",
					Voters:          3,
					ReadReplicas:    &readReplicas,
					SecurityProfile: openbaov1alpha1.ProfileHardened,
				},
				Storage: openbaov1alpha1.OpenBaoServiceProfileStorageSpec{
					PrimarySize:     "20Gi",
					ReadReplicaSize: "20Gi",
				},
				Bootstrap: openbaov1alpha1.OpenBaoServiceProfileBootstrapSpec{
					Mode:       openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
					ProfileRef: &openbaov1alpha1.LocalReference{Name: "oidc-standard-users-v1"},
				},
				Exposure: openbaov1alpha1.OpenBaoServiceProfileExposureSpec{
					ClassRef: openbaov1alpha1.LocalReference{Name: "internal-tls-v1"},
				},
				Backup: openbaov1alpha1.OpenBaoServiceProfileBackupSpec{
					ProfileRef: openbaov1alpha1.LocalReference{Name: "standard-daily-v1"},
				},
				Lifecycle: openbaov1alpha1.OpenBaoServiceProfileLifecycleSpec{
					UpgradeStrategy:    openbaov1alpha1.UpdateStrategyRollingUpdate,
					PreUpgradeSnapshot: &preUpgradeSnapshot,
				},
			},
		},
		BootstrapProfile: &openbaov1alpha1.OpenBaoBootstrapProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "oidc-standard-users-v1", UID: types.UID("bootstrap-uid")},
			Spec: openbaov1alpha1.OpenBaoBootstrapProfileSpec{
				OperatorLifecycleAuth: openbaov1alpha1.OpenBaoBootstrapLifecycleAuthSpec{
					Mode: openbaov1alpha1.OpenBaoBootstrapLifecycleAuthModeJWT,
					JWT:  &openbaov1alpha1.OpenBaoBootstrapLifecycleJWTSpec{Audience: "openbao-operator"},
				},
				Auth: &openbaov1alpha1.OpenBaoBootstrapAuthSpec{
					Methods: []openbaov1alpha1.OpenBaoBootstrapAuthMethodSpec{{Type: "kubernetes", Path: "kubernetes"}},
				},
			},
		},
		ExposureClass: &openbaov1alpha1.OpenBaoExposureClass{
			ObjectMeta: metav1.ObjectMeta{Name: "internal-tls-v1", UID: types.UID("exposure-uid")},
			Spec: openbaov1alpha1.OpenBaoExposureClassSpec{
				PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeGateway,
				HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
					Mode:         openbaov1alpha1.OpenBaoExposureHostnamePolicyModeGenerated,
					DomainSuffix: "example.internal",
				},
				TLSPolicy: &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
					Mode:       openbaov1alpha1.OpenBaoExposureTLSModeExternal,
					MinVersion: openbaov1alpha1.OpenBaoExposureTLSMinimumVersionTLS12,
				},
				EntrypointRef: &openbaov1alpha1.LocalReference{Name: "internal-gateway-v1"},
				ServicePolicy: &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
					Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
					BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
				},
			},
		},
		Entrypoint: &openbaov1alpha1.OpenBaoEntrypoint{
			ObjectMeta: metav1.ObjectMeta{Name: "internal-gateway-v1", UID: types.UID("entrypoint-uid")},
			Spec: openbaov1alpha1.OpenBaoEntrypointSpec{
				Mode: openbaov1alpha1.OpenBaoEntrypointModeGateway,
				ObjectRef: openbaov1alpha1.OpenBaoEntrypointObjectReference{
					APIGroup:  "gateway.networking.k8s.io",
					Kind:      "Gateway",
					Name:      "internal-gateway",
					Namespace: "networking",
				},
				ListenerPolicy: &openbaov1alpha1.OpenBaoEntrypointListenerPolicySpec{
					SectionName: "https",
				},
			},
		},
		BackupProfile: &openbaov1alpha1.OpenBaoBackupProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "standard-daily-v1", UID: types.UID("backup-uid")},
			Spec: openbaov1alpha1.OpenBaoBackupProfileSpec{
				Schedule:  "0 3 * * *",
				Retention: &openbaov1alpha1.BackupRetention{MaxCount: 30, MaxAge: "720h"},
				TargetRef: &openbaov1alpha1.LocalReference{Name: "primary-object-backup-v1"},
			},
		},
		BackupTarget: &openbaov1alpha1.OpenBaoBackupTarget{
			ObjectMeta: metav1.ObjectMeta{Name: "primary-object-backup-v1", UID: types.UID("backup-target-uid")},
			Spec: openbaov1alpha1.OpenBaoBackupTargetSpec{
				BackendRef:          openbaov1alpha1.LocalReference{Name: "s3-primary-v1"},
				AuthProfileRef:      &openbaov1alpha1.LocalReference{Name: "aws-irsa-backup-v1"},
				TransportProfileRef: &openbaov1alpha1.LocalReference{Name: "multipart-standard-v1"},
				LocationPolicy: openbaov1alpha1.OpenBaoBackupLocationPolicySpec{
					Location: openbaov1alpha1.OpenBaoBackupLocationSelectionSpec{
						Mode:              openbaov1alpha1.OpenBaoBackupLocationModeClaimValue,
						ValidationPattern: "^[a-z0-9-]+$",
					},
					KeyPrefix: openbaov1alpha1.OpenBaoBackupKeyPrefixPolicySpec{
						Template:            "claims/{{ claim.namespace }}/{{ claim.name }}",
						AllowClaimPartition: true,
					},
				},
			},
		},
		BackupBackend: &openbaov1alpha1.OpenBaoBackupBackend{
			ObjectMeta: metav1.ObjectMeta{Name: "s3-primary-v1", UID: types.UID("backup-backend-uid")},
			Spec: openbaov1alpha1.OpenBaoBackupBackendSpec{
				Driver: openbaov1alpha1.OpenBaoBackupBackendDriverObjectStorage,
				ObjectStorage: &openbaov1alpha1.OpenBaoBackupBackendObjectStorageSpec{
					Provider:     openbaov1alpha1.OpenBaoObjectStorageProviderS3,
					Endpoint:     "https://s3.example.internal",
					Region:       "eu-west-1",
					UsePathStyle: true,
					RequiredEgressRules: []networkingv1.NetworkPolicyEgressRule{
						{
							To: []networkingv1.NetworkPolicyPeer{{
								IPBlock: &networkingv1.IPBlock{CIDR: "10.10.0.0/16"},
							}},
							Ports: []networkingv1.NetworkPolicyPort{{
								Protocol: ptr.To(corev1.ProtocolTCP),
								Port:     ptr.To(intstr.FromInt(443)),
							}},
						},
					},
				},
			},
		},
		BackupAuth: &openbaov1alpha1.OpenBaoBackupAuthProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "aws-irsa-backup-v1", UID: types.UID("backup-auth-uid")},
			Spec: openbaov1alpha1.OpenBaoBackupAuthProfileSpec{
				Mode: openbaov1alpha1.OpenBaoBackupAuthModeWorkloadIdentity,
				WorkloadIdentity: &openbaov1alpha1.WorkloadIdentityConfig{
					ServiceAccountAnnotations: map[string]string{
						"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/openbao-backup",
					},
				},
				RoleARN: "arn:aws:iam::123456789012:role/openbao-backup",
			},
		},
		TransferProfile: &openbaov1alpha1.OpenBaoTransferProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "multipart-standard-v1", UID: types.UID("transfer-profile-uid")},
			Spec: openbaov1alpha1.OpenBaoTransferProfileSpec{
				PartSize:    16777216,
				Concurrency: 5,
			},
		},
	}
}

func validRenderedDevelopmentCatalogBundleFixture() *CatalogBundle {
	return &CatalogBundle{
		ServiceProfile: &openbaov1alpha1.OpenBaoServiceProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "standard-dev-v1", UID: types.UID("service-profile-uid")},
			Spec: openbaov1alpha1.OpenBaoServiceProfileSpec{
				Cluster: openbaov1alpha1.OpenBaoServiceProfileClusterSpec{
					Version:         "2.6.0",
					Voters:          1,
					SecurityProfile: openbaov1alpha1.ProfileDevelopment,
				},
				Storage: openbaov1alpha1.OpenBaoServiceProfileStorageSpec{PrimarySize: "20Gi"},
				Bootstrap: openbaov1alpha1.OpenBaoServiceProfileBootstrapSpec{
					Mode:       openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
					ProfileRef: &openbaov1alpha1.LocalReference{Name: "bootstrap-dev-v1"},
				},
				Exposure: openbaov1alpha1.OpenBaoServiceProfileExposureSpec{
					ClassRef: openbaov1alpha1.LocalReference{Name: "cluster-internal-v1"},
				},
				Backup: openbaov1alpha1.OpenBaoServiceProfileBackupSpec{
					ProfileRef: openbaov1alpha1.LocalReference{Name: "backup-disabled-v1"},
				},
			},
		},
		BootstrapProfile: &openbaov1alpha1.OpenBaoBootstrapProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-dev-v1", UID: types.UID("bootstrap-uid")},
			Spec: openbaov1alpha1.OpenBaoBootstrapProfileSpec{
				OperatorLifecycleAuth: openbaov1alpha1.OpenBaoBootstrapLifecycleAuthSpec{
					Mode: openbaov1alpha1.OpenBaoBootstrapLifecycleAuthModeJWT,
					JWT:  &openbaov1alpha1.OpenBaoBootstrapLifecycleJWTSpec{Audience: "openbao-operator"},
				},
				SecretEngines: &openbaov1alpha1.OpenBaoBootstrapSecretEnginesSpec{
					Mounts: []openbaov1alpha1.OpenBaoBootstrapSecretEngineMountSpec{{
						Type: "kv",
						Path: "secret",
					}},
				},
			},
		},
		ExposureClass: &openbaov1alpha1.OpenBaoExposureClass{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-internal-v1", UID: types.UID("exposure-uid")},
			Spec: openbaov1alpha1.OpenBaoExposureClassSpec{
				PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeClusterInternal,
			},
		},
		BackupProfile: &openbaov1alpha1.OpenBaoBackupProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "backup-disabled-v1", UID: types.UID("backup-uid")},
		},
	}
}

func validRenderedIngressCatalogBundleFixture() *CatalogBundle {
	return &CatalogBundle{
		ServiceProfile: &openbaov1alpha1.OpenBaoServiceProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "standard-ha-ingress-v1", UID: types.UID("service-profile-ingress-uid")},
			Spec: openbaov1alpha1.OpenBaoServiceProfileSpec{
				Cluster: openbaov1alpha1.OpenBaoServiceProfileClusterSpec{
					Version:         "2.6.0",
					Voters:          1,
					SecurityProfile: openbaov1alpha1.ProfileDevelopment,
				},
				Storage: openbaov1alpha1.OpenBaoServiceProfileStorageSpec{PrimarySize: "20Gi"},
				Bootstrap: openbaov1alpha1.OpenBaoServiceProfileBootstrapSpec{
					Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
				},
				Exposure: openbaov1alpha1.OpenBaoServiceProfileExposureSpec{
					ClassRef: openbaov1alpha1.LocalReference{Name: "edge-ingress-v1"},
				},
				Backup: openbaov1alpha1.OpenBaoServiceProfileBackupSpec{
					ProfileRef: openbaov1alpha1.LocalReference{Name: "backup-disabled-v1"},
				},
			},
		},
		ExposureClass: &openbaov1alpha1.OpenBaoExposureClass{
			ObjectMeta: metav1.ObjectMeta{Name: "edge-ingress-v1", UID: types.UID("exposure-ingress-uid")},
			Spec: openbaov1alpha1.OpenBaoExposureClassSpec{
				PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeIngress,
				HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
					Mode:  openbaov1alpha1.OpenBaoExposureHostnamePolicyModeFixed,
					Value: testRenderedExposureHostname,
				},
				TLSPolicy: &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
					Mode: openbaov1alpha1.OpenBaoExposureTLSModeOperatorManaged,
				},
				EntrypointRef:    &openbaov1alpha1.LocalReference{Name: "nginx-v1"},
				IngressPolicyRef: &openbaov1alpha1.LocalReference{Name: "nginx-backend-tls-v1"},
				Routing: &openbaov1alpha1.OpenBaoExposureRoutingSpec{
					Path: "/vault",
				},
				ServicePolicy: &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
					Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
					BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
				},
			},
		},
		Entrypoint: &openbaov1alpha1.OpenBaoEntrypoint{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx-v1", UID: types.UID("entrypoint-ingress-uid")},
			Spec: openbaov1alpha1.OpenBaoEntrypointSpec{
				Mode: openbaov1alpha1.OpenBaoEntrypointModeIngress,
				ObjectRef: openbaov1alpha1.OpenBaoEntrypointObjectReference{
					APIGroup: "networking.k8s.io",
					Kind:     "IngressClass",
					Name:     testRenderedIngressClassName,
				},
			},
		},
		IngressPolicy: &openbaov1alpha1.OpenBaoIngressPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx-backend-tls-v1", UID: types.UID("ingress-policy-uid")},
			Spec: openbaov1alpha1.OpenBaoIngressPolicySpec{
				PathType:      openbaov1alpha1.IngressPathTypePrefix,
				ReadinessMode: openbaov1alpha1.IngressReadinessModeLoadBalancerPublished,
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/backend-protocol": "HTTPS",
				},
				BackendTLS: &openbaov1alpha1.OpenBaoIngressPolicyBackendTLSSpec{
					PublicationMode: openbaov1alpha1.OpenBaoIngressBackendTLSPublicationModeAnnotation,
				},
			},
		},
		BackupProfile: &openbaov1alpha1.OpenBaoBackupProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "backup-disabled-v1", UID: types.UID("backup-disabled-uid")},
		},
	}
}

func mustBindApprovedServiceContract(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	catalog *CatalogBundle,
) *ApprovedServiceContract {
	t.Helper()

	approved, result := BindApprovedServiceContract(claim, catalog)
	if !result.Valid {
		t.Fatalf("BindApprovedServiceContract() = %#v, want valid", result)
	}
	return approved
}

func mustRenderSameClusterExecutionContract(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	catalog *CatalogBundle,
	transitDefaults SameClusterTransitUnsealDefaults,
) *RenderedExecutionContract {
	t.Helper()

	rendered, result := RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		mustBindApprovedServiceContract(t, claim, catalog),
		catalog,
		transitDefaults,
		SameClusterBootstrapResolvedInputs{},
	)
	if !result.Valid {
		t.Fatalf("RenderSameClusterExecutionContract() = %#v, want valid", result)
	}
	if rendered == nil {
		t.Fatal("RenderSameClusterExecutionContract() returned nil contract")
	}
	return rendered
}
