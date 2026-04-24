package openbaoclusterclaim

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const testSecretKind = "Secret"

func sameClusterCatalogObjects() []client.Object {
	return []client.Object{
		sameClusterServiceProfile(),
		sameClusterBootstrapProfile(),
		sameClusterExposureClass(),
		sameClusterBackupProfile(),
	}
}

func sameClusterGatewayCatalogObjects() []client.Object {
	return []client.Object{
		sameClusterGatewayServiceProfile(),
		sameClusterBootstrapProfile(),
		sameClusterGatewayExposureClass(),
		validEntrypoint(),
		sameClusterBackupProfile(),
	}
}

func sameClusterIngressCatalogObjects() []client.Object {
	return []client.Object{
		sameClusterIngressServiceProfile(),
		sameClusterBootstrapProfile(),
		sameClusterIngressExposureClass(),
		validIngressEntrypoint(),
		validIngressPolicy(),
		sameClusterBackupProfile(),
	}
}

func sameClusterConfigRefCatalogObjects() []client.Object {
	return []client.Object{
		sameClusterConfigRefServiceProfile(),
		sameClusterConfigRefBootstrapProfile(),
		sameClusterExposureClass(),
		sameClusterBackupProfile(),
	}
}

func sameClusterSecretConfigRefCatalogObjects() []client.Object {
	return []client.Object{
		sameClusterConfigRefServiceProfile(),
		sameClusterSecretConfigRefBootstrapProfile(),
		sameClusterExposureClass(),
		sameClusterBackupProfile(),
	}
}

func sameClusterPolicyCatalogObjects() []client.Object {
	return []client.Object{
		sameClusterPolicyServiceProfile(),
		sameClusterPolicyBootstrapProfile(),
		sameClusterExposureClass(),
		sameClusterBackupProfile(),
	}
}

func sameClusterSecretPolicyCatalogObjects() []client.Object {
	return []client.Object{
		sameClusterPolicyServiceProfile(),
		sameClusterSecretPolicyBootstrapProfile(),
		sameClusterExposureClass(),
		sameClusterBackupProfile(),
	}
}

func sameClusterAuditCatalogObjects() []client.Object {
	return []client.Object{
		sameClusterAuditServiceProfile(),
		sameClusterAuditBootstrapProfile(),
		sameClusterExposureClass(),
		sameClusterBackupProfile(),
	}
}

func sameClusterSecretAuditCatalogObjects() []client.Object {
	return []client.Object{
		sameClusterAuditServiceProfile(),
		sameClusterSecretAuditBootstrapProfile(),
		sameClusterExposureClass(),
		sameClusterBackupProfile(),
	}
}

func sameClusterHardenedCatalogObjects() []client.Object {
	return []client.Object{
		sameClusterHardenedServiceProfile(),
		sameClusterBootstrapProfile(),
		sameClusterHardenedExposureClass(),
		sameClusterBackupProfile(),
	}
}

func sameClusterBackupEnabledCatalogObjects() []client.Object {
	return []client.Object{
		sameClusterBackupEnabledServiceProfile(),
		sameClusterBootstrapProfile(),
		sameClusterExposureClass(),
		sameClusterBackupEnabledProfile(),
		validSameClusterBackupTarget(),
		validSameClusterBackupBackend(),
		validSameClusterBackupAuthProfile(),
		validSameClusterTransferProfile(),
	}
}

func sameClusterHardenedBackupCatalogObjects() []client.Object {
	return []client.Object{
		sameClusterHardenedBackupServiceProfile(),
		sameClusterBootstrapProfile(),
		sameClusterHardenedExposureClass(),
		sameClusterBackupEnabledProfile(),
		validSameClusterBackupTarget(),
		validSameClusterBackupBackend(),
		validSameClusterBackupAuthProfile(),
		validSameClusterTransferProfile(),
	}
}

func validTenant() *openbaov1alpha1.OpenBaoTenant {
	return &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "payments"},
		Spec:       openbaov1alpha1.OpenBaoTenantSpec{TargetNamespace: "payments"},
	}
}

func validServiceOfferingForReconcile(name, revision string) *openbaov1alpha1.OpenBaoServiceOffering {
	return &openbaov1alpha1.OpenBaoServiceOffering{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openbaov1alpha1.OpenBaoServiceOfferingSpec{
			CurrentRevisionRef: openbaov1alpha1.LocalReference{Name: revision},
		},
	}
}

func validServiceProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	preUpgradeSnapshot := true

	return &openbaov1alpha1.OpenBaoServiceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-ha-v1", UID: types.UID("service-profile-uid")},
		Spec: openbaov1alpha1.OpenBaoServiceProfileSpec{
			Cluster: openbaov1alpha1.OpenBaoServiceProfileClusterSpec{
				Version:         "2.6.0",
				Voters:          3,
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
	}
}

func validBootstrapProfile() *openbaov1alpha1.OpenBaoBootstrapProfile {
	return &openbaov1alpha1.OpenBaoBootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "oidc-standard-users-v1", UID: types.UID("bootstrap-profile-uid")},
		Spec: openbaov1alpha1.OpenBaoBootstrapProfileSpec{
			OperatorLifecycleAuth: openbaov1alpha1.OpenBaoBootstrapLifecycleAuthSpec{
				Mode: openbaov1alpha1.OpenBaoBootstrapLifecycleAuthModeJWT,
				JWT:  &openbaov1alpha1.OpenBaoBootstrapLifecycleJWTSpec{Audience: "openbao-operator"},
			},
		},
	}
}

func validExposureClass() *openbaov1alpha1.OpenBaoExposureClass {
	return &openbaov1alpha1.OpenBaoExposureClass{
		ObjectMeta: metav1.ObjectMeta{Name: "internal-tls-v1", UID: types.UID("exposure-class-uid")},
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
	}
}

func validBackupProfile() *openbaov1alpha1.OpenBaoBackupProfile {
	return &openbaov1alpha1.OpenBaoBackupProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-daily-v1", UID: types.UID("backup-profile-uid")},
		Spec: openbaov1alpha1.OpenBaoBackupProfileSpec{
			Schedule: "0 3 * * *",
		},
	}
}

func sameClusterServiceProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	preUpgradeSnapshot := false
	readReplicas := int32(1)

	return &openbaov1alpha1.OpenBaoServiceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-ha-v1", UID: types.UID("service-profile-uid")},
		Spec: openbaov1alpha1.OpenBaoServiceProfileSpec{
			Cluster: openbaov1alpha1.OpenBaoServiceProfileClusterSpec{
				Version:         "2.6.0",
				Voters:          3,
				ReadReplicas:    &readReplicas,
				SecurityProfile: openbaov1alpha1.ProfileDevelopment,
			},
			Storage: openbaov1alpha1.OpenBaoServiceProfileStorageSpec{
				PrimarySize:     "20Gi",
				ReadReplicaSize: "10Gi",
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
	}
}

func sameClusterHardenedServiceProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	profile := sameClusterServiceProfile()
	profile.Name = "standard-ha-hardened-v1"
	profile.UID = types.UID("service-profile-hardened-uid")
	profile.Spec.Cluster.SecurityProfile = openbaov1alpha1.ProfileHardened
	return profile
}

func sameClusterConfigRefServiceProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	profile := sameClusterServiceProfile()
	profile.Name = "standard-ha-configref-v1"
	profile.UID = types.UID("service-profile-configref-uid")
	return profile
}

func sameClusterGatewayServiceProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	profile := sameClusterServiceProfile()
	profile.Name = "standard-ha-gateway-v1"
	profile.UID = types.UID("service-profile-gateway-uid")
	profile.Spec.Exposure.ClassRef = openbaov1alpha1.LocalReference{Name: "shared-gateway-v1"}
	return profile
}

func sameClusterIngressServiceProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	profile := sameClusterServiceProfile()
	profile.Name = "standard-ha-ingress-v1"
	profile.UID = types.UID("service-profile-ingress-uid")
	profile.Spec.Exposure.ClassRef = openbaov1alpha1.LocalReference{Name: "edge-ingress-v1"}
	return profile
}

func sameClusterPolicyServiceProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	profile := sameClusterServiceProfile()
	profile.Name = "standard-ha-policy-v1"
	profile.UID = types.UID("service-profile-policy-uid")
	return profile
}

func sameClusterAuditServiceProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	profile := sameClusterServiceProfile()
	profile.Name = "standard-ha-audit-v1"
	profile.UID = types.UID("service-profile-audit-uid")
	return profile
}

func sameClusterBackupEnabledServiceProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	profile := sameClusterServiceProfile()
	profile.Name = "standard-ha-backup-v1"
	profile.UID = types.UID("service-profile-backup-uid")
	profile.Spec.Backup.ProfileRef = openbaov1alpha1.LocalReference{Name: "standard-daily-backed-v1"}
	return profile
}

func sameClusterHardenedBackupServiceProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	profile := sameClusterHardenedServiceProfile()
	profile.Name = "standard-ha-hardened-backup-v1"
	profile.UID = types.UID("service-profile-hardened-backup-uid")
	profile.Spec.Backup.ProfileRef = openbaov1alpha1.LocalReference{Name: "standard-daily-backed-v1"}
	return profile
}

func sameClusterBootstrapProfile() *openbaov1alpha1.OpenBaoBootstrapProfile {
	return &openbaov1alpha1.OpenBaoBootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "oidc-standard-users-v1", UID: types.UID("bootstrap-profile-uid")},
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
	}
}

func sameClusterConfigRefBootstrapProfile() *openbaov1alpha1.OpenBaoBootstrapProfile {
	profile := sameClusterBootstrapProfile()
	profile.Spec.Auth = &openbaov1alpha1.OpenBaoBootstrapAuthSpec{
		Methods: []openbaov1alpha1.OpenBaoBootstrapAuthMethodSpec{{
			Type: "kubernetes",
			Path: "kubernetes",
			ConfigRef: &openbaov1alpha1.TypedObjectReference{
				Kind: "ConfigMap",
				Name: "kubernetes-auth-default",
			},
		}},
	}
	return profile
}

func sameClusterSecretConfigRefBootstrapProfile() *openbaov1alpha1.OpenBaoBootstrapProfile {
	profile := sameClusterBootstrapProfile()
	profile.Spec.Auth = &openbaov1alpha1.OpenBaoBootstrapAuthSpec{
		Methods: []openbaov1alpha1.OpenBaoBootstrapAuthMethodSpec{{
			Type: "kubernetes",
			Path: "kubernetes",
			ConfigRef: &openbaov1alpha1.TypedObjectReference{
				Kind: "Secret",
				Name: "kubernetes-auth-default",
			},
		}},
	}
	return profile
}

func sameClusterPolicyBootstrapProfile() *openbaov1alpha1.OpenBaoBootstrapProfile {
	profile := sameClusterBootstrapProfile()
	profile.Spec.Policies = &openbaov1alpha1.OpenBaoBootstrapPoliciesSpec{
		Bundles: []openbaov1alpha1.OpenBaoBootstrapPolicyBundleSpec{{
			Name: "app-readwrite",
			ContentRef: openbaov1alpha1.TypedObjectReference{
				Kind: "ConfigMap",
				Name: "app-readwrite-policy",
			},
		}},
	}
	return profile
}

func sameClusterSecretPolicyBootstrapProfile() *openbaov1alpha1.OpenBaoBootstrapProfile {
	profile := sameClusterBootstrapProfile()
	profile.Spec.Policies = &openbaov1alpha1.OpenBaoBootstrapPoliciesSpec{
		Bundles: []openbaov1alpha1.OpenBaoBootstrapPolicyBundleSpec{{
			Name: "app-readwrite",
			ContentRef: openbaov1alpha1.TypedObjectReference{
				Kind: "Secret",
				Name: "app-readwrite-policy",
			},
		}},
	}
	return profile
}

func sameClusterAuditBootstrapProfile() *openbaov1alpha1.OpenBaoBootstrapProfile {
	profile := sameClusterBootstrapProfile()
	profile.Spec.Audit = &openbaov1alpha1.OpenBaoBootstrapAuditSpec{
		Devices: []openbaov1alpha1.OpenBaoBootstrapAuditDeviceSpec{{
			Type: "file",
			SinkRef: &openbaov1alpha1.TypedObjectReference{
				Kind: "ConfigMap",
				Name: "audit-file-default",
			},
		}},
	}
	return profile
}

func sameClusterSecretAuditBootstrapProfile() *openbaov1alpha1.OpenBaoBootstrapProfile {
	profile := sameClusterBootstrapProfile()
	profile.Spec.Audit = &openbaov1alpha1.OpenBaoBootstrapAuditSpec{
		Devices: []openbaov1alpha1.OpenBaoBootstrapAuditDeviceSpec{{
			Type: "file",
			SinkRef: &openbaov1alpha1.TypedObjectReference{
				Kind: "Secret",
				Name: "audit-file-default",
			},
		}},
	}
	return profile
}

func sameClusterExposureClass() *openbaov1alpha1.OpenBaoExposureClass {
	return &openbaov1alpha1.OpenBaoExposureClass{
		ObjectMeta: metav1.ObjectMeta{Name: "internal-tls-v1", UID: types.UID("exposure-class-uid")},
		Spec: openbaov1alpha1.OpenBaoExposureClassSpec{
			PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeClusterInternal,
			HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
				Mode: openbaov1alpha1.OpenBaoExposureHostnamePolicyModeGenerated,
			},
			ServicePolicy: &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
				Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
				BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
			},
		},
	}
}

func sameClusterGatewayExposureClass() *openbaov1alpha1.OpenBaoExposureClass {
	return &openbaov1alpha1.OpenBaoExposureClass{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-gateway-v1", UID: types.UID("exposure-class-gateway-uid")},
		Spec: openbaov1alpha1.OpenBaoExposureClassSpec{
			PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeGateway,
			HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
				Mode:         openbaov1alpha1.OpenBaoExposureHostnamePolicyModeGenerated,
				DomainSuffix: "example.internal",
			},
			TLSPolicy: &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
				Mode:       openbaov1alpha1.OpenBaoExposureTLSModeOperatorManaged,
				MinVersion: openbaov1alpha1.OpenBaoExposureTLSMinimumVersionTLS12,
			},
			EntrypointRef: &openbaov1alpha1.LocalReference{Name: "internal-gateway-v1"},
			Routing: &openbaov1alpha1.OpenBaoExposureRoutingSpec{
				Path: "/",
			},
			ServicePolicy: &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
				Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
				BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
			},
		},
	}
}

func sameClusterIngressExposureClass() *openbaov1alpha1.OpenBaoExposureClass {
	return &openbaov1alpha1.OpenBaoExposureClass{
		ObjectMeta: metav1.ObjectMeta{Name: "edge-ingress-v1", UID: types.UID("exposure-class-ingress-uid")},
		Spec: openbaov1alpha1.OpenBaoExposureClassSpec{
			PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeIngress,
			HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
				Mode:         openbaov1alpha1.OpenBaoExposureHostnamePolicyModeGenerated,
				DomainSuffix: "example.internal",
			},
			TLSPolicy: &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
				Mode:       openbaov1alpha1.OpenBaoExposureTLSModeOperatorManaged,
				MinVersion: openbaov1alpha1.OpenBaoExposureTLSMinimumVersionTLS12,
			},
			EntrypointRef:    &openbaov1alpha1.LocalReference{Name: "internal-ingress-v1"},
			IngressPolicyRef: &openbaov1alpha1.LocalReference{Name: "nginx-backend-tls-v1"},
			Routing: &openbaov1alpha1.OpenBaoExposureRoutingSpec{
				Path: "/",
			},
			ServicePolicy: &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
				Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
				BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
			},
		},
	}
}

func sameClusterHardenedExposureClass() *openbaov1alpha1.OpenBaoExposureClass {
	exposure := sameClusterExposureClass()
	exposure.Spec.TLSPolicy = &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
		Mode:       openbaov1alpha1.OpenBaoExposureTLSModeExternal,
		MinVersion: openbaov1alpha1.OpenBaoExposureTLSMinimumVersionTLS12,
	}
	return exposure
}

func sameClusterBackupProfile() *openbaov1alpha1.OpenBaoBackupProfile {
	return &openbaov1alpha1.OpenBaoBackupProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-daily-v1", UID: types.UID("backup-profile-uid")},
	}
}

func sameClusterBackupEnabledProfile() *openbaov1alpha1.OpenBaoBackupProfile {
	return &openbaov1alpha1.OpenBaoBackupProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-daily-backed-v1", UID: types.UID("backup-profile-backed-uid")},
		Spec: openbaov1alpha1.OpenBaoBackupProfileSpec{
			Schedule:  "0 3 * * *",
			Retention: &openbaov1alpha1.BackupRetention{MaxCount: 30, MaxAge: "720h"},
			TargetRef: &openbaov1alpha1.LocalReference{Name: "primary-object-backup-v1"},
		},
	}
}

func validSameClusterBackupTarget() *openbaov1alpha1.OpenBaoBackupTarget {
	return &openbaov1alpha1.OpenBaoBackupTarget{
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
	}
}

func validSameClusterBackupBackend() *openbaov1alpha1.OpenBaoBackupBackend {
	return &openbaov1alpha1.OpenBaoBackupBackend{
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
	}
}

func validSameClusterBackupAuthProfile() *openbaov1alpha1.OpenBaoBackupAuthProfile {
	return &openbaov1alpha1.OpenBaoBackupAuthProfile{
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
	}
}

func validSameClusterTransferProfile() *openbaov1alpha1.OpenBaoTransferProfile {
	return &openbaov1alpha1.OpenBaoTransferProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "multipart-standard-v1", UID: types.UID("transfer-profile-uid")},
		Spec: openbaov1alpha1.OpenBaoTransferProfileSpec{
			PartSize:    16777216,
			Concurrency: 5,
		},
	}
}

func validEntrypoint() *openbaov1alpha1.OpenBaoEntrypoint {
	return &openbaov1alpha1.OpenBaoEntrypoint{
		ObjectMeta: metav1.ObjectMeta{Name: "internal-gateway-v1", UID: types.UID("entrypoint-uid")},
		Spec: openbaov1alpha1.OpenBaoEntrypointSpec{
			Mode: openbaov1alpha1.OpenBaoEntrypointModeGateway,
			ObjectRef: openbaov1alpha1.OpenBaoEntrypointObjectReference{
				APIGroup:  "gateway.networking.k8s.io",
				Kind:      "Gateway",
				Name:      "internal-gateway",
				Namespace: "networking",
			},
		},
	}
}

func validIngressEntrypoint() *openbaov1alpha1.OpenBaoEntrypoint {
	return &openbaov1alpha1.OpenBaoEntrypoint{
		ObjectMeta: metav1.ObjectMeta{Name: "internal-ingress-v1", UID: types.UID("entrypoint-ingress-uid")},
		Spec: openbaov1alpha1.OpenBaoEntrypointSpec{
			Mode: openbaov1alpha1.OpenBaoEntrypointModeIngress,
			ObjectRef: openbaov1alpha1.OpenBaoEntrypointObjectReference{
				APIGroup: "networking.k8s.io",
				Kind:     "IngressClass",
				Name:     "nginx",
			},
		},
	}
}

func validIngressPolicy() *openbaov1alpha1.OpenBaoIngressPolicy {
	return &openbaov1alpha1.OpenBaoIngressPolicy{
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
	}
}
