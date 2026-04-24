package openbaoclusterclaim

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/claimcontract"
	"github.com/dc-tec/openbao-operator/internal/service/connectionpublishing"
)

func validSameClusterPublicService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:              connectionpublishing.LocalPublicServiceName("payments-bao"),
			Namespace:         "payments",
			CreationTimestamp: metav1.NewTime(time.Date(2026, time.April, 20, 17, 0, 0, 0, time.UTC)),
		},
	}
}

func validSameClusterCASecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              connectionpublishing.LocalCASecretName("payments-bao"),
			Namespace:         "payments",
			CreationTimestamp: metav1.NewTime(time.Date(2026, time.April, 20, 18, 0, 0, 0, time.UTC)),
		},
		Data: map[string][]byte{
			"ca.crt": []byte("same-cluster-ca"),
		},
	}
}

func validSameClusterEndpoint() string {
	return "https://payments-bao-public.payments.svc:8200"
}

func validSameClusterGatewayEndpoint() string {
	return "https://payments-bao.example.internal"
}

func validSameClusterAppliedStatus() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	approved, validation := claimcontract.BindApprovedServiceContract(validClaim(), sameClusterCatalogBundle())
	if !validation.Valid || approved == nil {
		panic("expected valid same-cluster approved contract test fixture")
	}
	rendered, renderValidation := claimcontract.RenderSameClusterExecutionContract(
		validClaim(),
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		approved,
		sameClusterCatalogBundle(),
		claimcontract.SameClusterTransitUnsealDefaults{},
		claimcontract.SameClusterBootstrapResolvedInputs{},
	)
	if !renderValidation.Valid || rendered == nil {
		panic("expected valid same-cluster rendered contract test fixture")
	}
	status := claimcontract.AppliedStatus(approved)
	status.ApprovedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(approved))
	status.RenderedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(rendered))
	status.RenderedDependencies = claimcontract.AppliedRenderedDependencies(rendered)
	return &status
}

func validSameClusterAppliedStatusWithOffering(name string) *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	status := validSameClusterAppliedStatus()
	status.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: name}
	return status
}

func validSameClusterGatewayAppliedStatus() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-gateway-v1"}
	approved, validation := claimcontract.BindApprovedServiceContract(claim, sameClusterGatewayCatalogBundle())
	if !validation.Valid || approved == nil {
		panic("expected valid same-cluster gateway approved contract test fixture")
	}
	rendered, renderValidation := claimcontract.RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		approved,
		sameClusterGatewayCatalogBundle(),
		claimcontract.SameClusterTransitUnsealDefaults{},
		claimcontract.SameClusterBootstrapResolvedInputs{},
	)
	if !renderValidation.Valid || rendered == nil {
		panic("expected valid same-cluster gateway rendered contract test fixture")
	}
	status := claimcontract.AppliedStatus(approved)
	status.ApprovedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(approved))
	status.RenderedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(rendered))
	status.RenderedDependencies = claimcontract.AppliedRenderedDependencies(rendered)
	return &status
}

func validSameClusterConfigRefAppliedStatus() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	return validSameClusterConfigRefAppliedStatusForKind(kindConfigMap)
}

func validSameClusterSecretConfigRefAppliedStatus() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	return validSameClusterConfigRefAppliedStatusForKind(kindSecret)
}

func validSameClusterConfigRefAppliedStatusForKind(kind string) *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-configref-v1"}
	catalogBundle := sameClusterConfigRefCatalogBundle()
	if kind == kindSecret {
		catalogBundle = sameClusterSecretConfigRefCatalogBundle()
	}
	approved, validation := claimcontract.BindApprovedServiceContract(claim, catalogBundle)
	if !validation.Valid || approved == nil {
		panic("expected valid same-cluster config-ref approved contract test fixture")
	}
	var artifact claimcontract.ProjectedBootstrapArtifact
	switch kind {
	case kindConfigMap:
		artifact = claimcontract.ProjectedBootstrapArtifact{
			Ref: openbaov1alpha1.TypedObjectReference{
				Kind: kindConfigMap,
				Name: projectedBootstrapArtifactName(claim, "authcfg", claimcontract.BootstrapAuthMethodIdentity("kubernetes", "kubernetes"), projectedBootstrapArtifactIdentity{
					Kind:       kindConfigMap,
					StringData: validSameClusterAuthMethodConfig(),
				}),
			},
			ConfigMapData: validSameClusterAuthMethodConfig(),
		}
	case kindSecret:
		artifact = claimcontract.ProjectedBootstrapArtifact{
			Ref: openbaov1alpha1.TypedObjectReference{
				Kind: kindSecret,
				Name: projectedBootstrapArtifactName(claim, "authcfg", claimcontract.BootstrapAuthMethodIdentity("kubernetes", "kubernetes"), projectedBootstrapArtifactIdentity{
					Kind:       kindSecret,
					SecretData: stringifySecretData(validSameClusterAuthMethodSecret().Data),
				}),
			},
			SecretData: copySecretData(validSameClusterAuthMethodSecret().Data),
		}
	default:
		panic("unsupported auth config ref kind")
	}
	rendered, renderValidation := claimcontract.RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		approved,
		catalogBundle,
		claimcontract.SameClusterTransitUnsealDefaults{},
		claimcontract.SameClusterBootstrapResolvedInputs{
			AuthMethodConfigs: map[string]claimcontract.ProjectedBootstrapArtifact{
				claimcontract.BootstrapAuthMethodIdentity("kubernetes", "kubernetes"): artifact,
			},
		},
	)
	if !renderValidation.Valid || rendered == nil {
		panic("expected valid same-cluster config-ref rendered contract test fixture")
	}
	status := claimcontract.AppliedStatus(approved)
	status.ApprovedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(approved))
	status.RenderedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(rendered))
	status.RenderedDependencies = claimcontract.AppliedRenderedDependencies(rendered)
	return &status
}

func validSameClusterPolicyAppliedStatus() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	return validSameClusterPolicyAppliedStatusForKind(kindConfigMap)
}

func validSameClusterSecretPolicyAppliedStatus() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	return validSameClusterPolicyAppliedStatusForKind(kindSecret)
}

func validSameClusterPolicyAppliedStatusForKind(kind string) *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-policy-v1"}
	catalogBundle := sameClusterPolicyCatalogBundle()
	if kind == kindSecret {
		catalogBundle = sameClusterSecretPolicyCatalogBundle()
	}
	policyBundle := catalogBundle.BootstrapProfile.Spec.Policies.Bundles[0]
	approved, validation := claimcontract.BindApprovedServiceContract(claim, catalogBundle)
	if !validation.Valid || approved == nil {
		panic("expected valid same-cluster policy approved contract test fixture")
	}
	var artifact claimcontract.ProjectedBootstrapArtifact
	switch kind {
	case kindConfigMap:
		artifact = claimcontract.ProjectedBootstrapArtifact{
			Ref: openbaov1alpha1.TypedObjectReference{
				Kind: kindConfigMap,
				Name: projectedBootstrapArtifactName(claim, "policy", claimcontract.BootstrapPolicyBundleIdentity(sameClusterPolicyBootstrapProfile().Spec.Policies.Bundles[0]), projectedBootstrapArtifactIdentity{
					Kind: kindConfigMap,
					StringData: map[string]string{
						"content": validSameClusterPolicyContent(),
					},
				}),
			},
			ConfigMapData: map[string]string{"content": validSameClusterPolicyContent()},
		}
	case kindSecret:
		artifact = claimcontract.ProjectedBootstrapArtifact{
			Ref: openbaov1alpha1.TypedObjectReference{
				Kind: kindSecret,
				Name: projectedBootstrapArtifactName(claim, "policy", claimcontract.BootstrapPolicyBundleIdentity(sameClusterSecretPolicyBootstrapProfile().Spec.Policies.Bundles[0]), projectedBootstrapArtifactIdentity{
					Kind: kindSecret,
					SecretData: map[string]string{
						"content": validSameClusterPolicyContent(),
					},
				}),
			},
			SecretData: map[string][]byte{"content": []byte(validSameClusterPolicyContent())},
		}
	default:
		panic("unsupported policy ref kind")
	}
	rendered, renderValidation := claimcontract.RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		approved,
		catalogBundle,
		claimcontract.SameClusterTransitUnsealDefaults{},
		claimcontract.SameClusterBootstrapResolvedInputs{
			PolicyBundleContents: map[string]claimcontract.ProjectedBootstrapArtifact{
				claimcontract.BootstrapPolicyBundleIdentity(policyBundle): artifact,
			},
		},
	)
	if !renderValidation.Valid || rendered == nil {
		panic("expected valid same-cluster policy rendered contract test fixture")
	}
	status := claimcontract.AppliedStatus(approved)
	status.ApprovedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(approved))
	status.RenderedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(rendered))
	status.RenderedDependencies = claimcontract.AppliedRenderedDependencies(rendered)
	return &status
}

func validSameClusterAuditAppliedStatus() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	return validSameClusterAuditAppliedStatusForKind(kindConfigMap)
}

func validSameClusterSecretAuditAppliedStatus() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	return validSameClusterAuditAppliedStatusForKind(kindSecret)
}

func validSameClusterAuditAppliedStatusForKind(kind string) *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-audit-v1"}
	catalogBundle := sameClusterAuditCatalogBundle()
	if kind == kindSecret {
		catalogBundle = sameClusterSecretAuditCatalogBundle()
	}
	auditDevice := catalogBundle.BootstrapProfile.Spec.Audit.Devices[0]
	approved, validation := claimcontract.BindApprovedServiceContract(claim, catalogBundle)
	if !validation.Valid || approved == nil {
		panic("expected valid same-cluster audit approved contract test fixture")
	}
	var artifact claimcontract.ProjectedBootstrapArtifact
	switch kind {
	case kindConfigMap:
		artifact = claimcontract.ProjectedBootstrapArtifact{
			Ref: openbaov1alpha1.TypedObjectReference{
				Kind: kindConfigMap,
				Name: projectedBootstrapArtifactName(claim, "audit", claimcontract.BootstrapAuditDeviceIdentity(sameClusterAuditBootstrapProfile().Spec.Audit.Devices[0]), projectedBootstrapArtifactIdentity{
					Kind: kindConfigMap,
					StringData: map[string]string{
						"sink.json": `{"path":"stdout","fileOptions":{"filePath":"stdout"}}`,
					},
				}),
			},
			ConfigMapData: map[string]string{
				"sink.json": `{"path":"stdout","fileOptions":{"filePath":"stdout"}}`,
			},
		}
	case kindSecret:
		artifact = claimcontract.ProjectedBootstrapArtifact{
			Ref: openbaov1alpha1.TypedObjectReference{
				Kind: kindSecret,
				Name: projectedBootstrapArtifactName(claim, "audit", claimcontract.BootstrapAuditDeviceIdentity(sameClusterSecretAuditBootstrapProfile().Spec.Audit.Devices[0]), projectedBootstrapArtifactIdentity{
					Kind: kindSecret,
					SecretData: map[string]string{
						"sink.json": `{"path":"stdout","fileOptions":{"filePath":"stdout"}}`,
					},
				}),
			},
			SecretData: map[string][]byte{
				"sink.json": []byte(`{"path":"stdout","fileOptions":{"filePath":"stdout"}}`),
			},
		}
	default:
		panic("unsupported audit ref kind")
	}
	rendered, renderValidation := claimcontract.RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		approved,
		catalogBundle,
		claimcontract.SameClusterTransitUnsealDefaults{},
		claimcontract.SameClusterBootstrapResolvedInputs{
			AuditDeviceSinks: map[string]claimcontract.ProjectedBootstrapAuditSink{
				claimcontract.BootstrapAuditDeviceIdentity(auditDevice): {
					Artifact: artifact,
					Path:     "stdout",
				},
			},
		},
	)
	if !renderValidation.Valid || rendered == nil {
		panic("expected valid same-cluster audit rendered contract test fixture")
	}
	status := claimcontract.AppliedStatus(approved)
	status.ApprovedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(approved))
	status.RenderedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(rendered))
	status.RenderedDependencies = claimcontract.AppliedRenderedDependencies(rendered)
	return &status
}

func validSameClusterHardenedAppliedStatus() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-hardened-v1"}
	approved, validation := claimcontract.BindApprovedServiceContract(claim, sameClusterHardenedCatalogBundle())
	if !validation.Valid || approved == nil {
		panic("expected valid same-cluster hardened approved contract test fixture")
	}
	rendered, renderValidation := claimcontract.RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		approved,
		sameClusterHardenedCatalogBundle(),
		claimcontract.SameClusterTransitUnsealDefaults{
			Address:               "https://transit.example.internal:8200",
			KeyName:               "openbao-unseal",
			MountPath:             "transit",
			Namespace:             "platform",
			TLSServerName:         "transit.example.internal",
			CredentialsSecretName: "transit-unseal-creds",
		},
		claimcontract.SameClusterBootstrapResolvedInputs{},
	)
	if !renderValidation.Valid || rendered == nil {
		panic("expected valid same-cluster hardened rendered contract test fixture")
	}
	status := claimcontract.AppliedStatus(approved)
	status.ApprovedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(approved))
	status.RenderedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(rendered))
	status.RenderedDependencies = claimcontract.AppliedRenderedDependencies(rendered)
	return &status
}

func validSameClusterBackupEnabledAppliedStatus() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-backup-v1"}
	claim.Spec.ServiceParameters = &openbaov1alpha1.OpenBaoClusterClaimServiceParametersSpec{
		Backup: &openbaov1alpha1.OpenBaoClusterClaimBackupServiceParametersSpec{
			Location:  "payments-prod",
			Partition: "finance",
		},
	}
	approved, validation := claimcontract.BindApprovedServiceContract(claim, sameClusterBackupEnabledCatalogBundle())
	if !validation.Valid || approved == nil {
		panic("expected valid same-cluster backup-enabled approved contract test fixture")
	}
	rendered, renderValidation := claimcontract.RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		approved,
		sameClusterBackupEnabledCatalogBundle(),
		claimcontract.SameClusterTransitUnsealDefaults{},
		claimcontract.SameClusterBootstrapResolvedInputs{},
	)
	if !renderValidation.Valid || rendered == nil {
		panic("expected valid same-cluster backup-enabled rendered contract test fixture")
	}
	status := claimcontract.AppliedStatus(approved)
	status.ApprovedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(approved))
	status.RenderedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(rendered))
	status.RenderedDependencies = claimcontract.AppliedRenderedDependencies(rendered)
	return &status
}

func validSameClusterHardenedBackupAppliedStatus() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-hardened-backup-v1"}
	claim.Spec.ServiceParameters = &openbaov1alpha1.OpenBaoClusterClaimServiceParametersSpec{
		Backup: &openbaov1alpha1.OpenBaoClusterClaimBackupServiceParametersSpec{
			Location:  "payments-prod",
			Partition: "finance",
		},
	}
	approved, validation := claimcontract.BindApprovedServiceContract(claim, sameClusterHardenedBackupCatalogBundle())
	if !validation.Valid || approved == nil {
		panic("expected valid same-cluster hardened backup approved contract test fixture")
	}
	rendered, renderValidation := claimcontract.RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		approved,
		sameClusterHardenedBackupCatalogBundle(),
		claimcontract.SameClusterTransitUnsealDefaults{
			Address:               "https://transit.example.internal:8200",
			KeyName:               "openbao-unseal",
			MountPath:             "transit",
			Namespace:             "platform",
			TLSServerName:         "transit.example.internal",
			CredentialsSecretName: "transit-unseal-creds",
		},
		claimcontract.SameClusterBootstrapResolvedInputs{},
	)
	if !renderValidation.Valid || rendered == nil {
		panic("expected valid same-cluster hardened backup rendered contract test fixture")
	}
	status := claimcontract.AppliedStatus(approved)
	status.ApprovedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(approved))
	status.RenderedContract = claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(rendered))
	status.RenderedDependencies = claimcontract.AppliedRenderedDependencies(rendered)
	return &status
}

func sameClusterCatalogBundle() *claimcontract.CatalogBundle {
	return &claimcontract.CatalogBundle{
		ServiceProfile:   sameClusterServiceProfile(),
		BootstrapProfile: sameClusterBootstrapProfile(),
		ExposureClass:    sameClusterExposureClass(),
		BackupProfile:    sameClusterBackupProfile(),
	}
}

func sameClusterGatewayCatalogBundle() *claimcontract.CatalogBundle {
	return &claimcontract.CatalogBundle{
		ServiceProfile:   sameClusterGatewayServiceProfile(),
		BootstrapProfile: sameClusterBootstrapProfile(),
		ExposureClass:    sameClusterGatewayExposureClass(),
		Entrypoint:       validEntrypoint(),
		BackupProfile:    sameClusterBackupProfile(),
	}
}

func sameClusterConfigRefCatalogBundle() *claimcontract.CatalogBundle {
	return &claimcontract.CatalogBundle{
		ServiceProfile:   sameClusterConfigRefServiceProfile(),
		BootstrapProfile: sameClusterConfigRefBootstrapProfile(),
		ExposureClass:    sameClusterExposureClass(),
		BackupProfile:    sameClusterBackupProfile(),
	}
}

func sameClusterSecretConfigRefCatalogBundle() *claimcontract.CatalogBundle {
	return &claimcontract.CatalogBundle{
		ServiceProfile:   sameClusterConfigRefServiceProfile(),
		BootstrapProfile: sameClusterSecretConfigRefBootstrapProfile(),
		ExposureClass:    sameClusterExposureClass(),
		BackupProfile:    sameClusterBackupProfile(),
	}
}

func sameClusterPolicyCatalogBundle() *claimcontract.CatalogBundle {
	return &claimcontract.CatalogBundle{
		ServiceProfile:   sameClusterPolicyServiceProfile(),
		BootstrapProfile: sameClusterPolicyBootstrapProfile(),
		ExposureClass:    sameClusterExposureClass(),
		BackupProfile:    sameClusterBackupProfile(),
	}
}

func sameClusterSecretPolicyCatalogBundle() *claimcontract.CatalogBundle {
	return &claimcontract.CatalogBundle{
		ServiceProfile:   sameClusterPolicyServiceProfile(),
		BootstrapProfile: sameClusterSecretPolicyBootstrapProfile(),
		ExposureClass:    sameClusterExposureClass(),
		BackupProfile:    sameClusterBackupProfile(),
	}
}

func sameClusterAuditCatalogBundle() *claimcontract.CatalogBundle {
	return &claimcontract.CatalogBundle{
		ServiceProfile:   sameClusterAuditServiceProfile(),
		BootstrapProfile: sameClusterAuditBootstrapProfile(),
		ExposureClass:    sameClusterExposureClass(),
		BackupProfile:    sameClusterBackupProfile(),
	}
}

func sameClusterSecretAuditCatalogBundle() *claimcontract.CatalogBundle {
	return &claimcontract.CatalogBundle{
		ServiceProfile:   sameClusterAuditServiceProfile(),
		BootstrapProfile: sameClusterSecretAuditBootstrapProfile(),
		ExposureClass:    sameClusterExposureClass(),
		BackupProfile:    sameClusterBackupProfile(),
	}
}

func sameClusterHardenedCatalogBundle() *claimcontract.CatalogBundle {
	return &claimcontract.CatalogBundle{
		ServiceProfile:   sameClusterHardenedServiceProfile(),
		BootstrapProfile: sameClusterBootstrapProfile(),
		ExposureClass:    sameClusterHardenedExposureClass(),
		BackupProfile:    sameClusterBackupProfile(),
	}
}

func sameClusterBackupEnabledCatalogBundle() *claimcontract.CatalogBundle {
	return &claimcontract.CatalogBundle{
		ServiceProfile:   sameClusterBackupEnabledServiceProfile(),
		BootstrapProfile: sameClusterBootstrapProfile(),
		ExposureClass:    sameClusterExposureClass(),
		BackupProfile:    sameClusterBackupEnabledProfile(),
		BackupTarget:     validSameClusterBackupTarget(),
		BackupBackend:    validSameClusterBackupBackend(),
		BackupAuth:       validSameClusterBackupAuthProfile(),
		TransferProfile:  validSameClusterTransferProfile(),
	}
}

func sameClusterHardenedBackupCatalogBundle() *claimcontract.CatalogBundle {
	return &claimcontract.CatalogBundle{
		ServiceProfile:   sameClusterHardenedBackupServiceProfile(),
		BootstrapProfile: sameClusterBootstrapProfile(),
		ExposureClass:    sameClusterHardenedExposureClass(),
		BackupProfile:    sameClusterBackupEnabledProfile(),
		BackupTarget:     validSameClusterBackupTarget(),
		BackupBackend:    validSameClusterBackupBackend(),
		BackupAuth:       validSameClusterBackupAuthProfile(),
		TransferProfile:  validSameClusterTransferProfile(),
	}
}

func validSameClusterTransitUnsealConfig() SameClusterTransitUnsealConfig {
	return SameClusterTransitUnsealConfig{
		Address:               "https://transit.example.internal:8200",
		KeyName:               "openbao-unseal",
		MountPath:             "transit",
		Namespace:             "platform",
		TLSServerName:         "transit.example.internal",
		CredentialsSecretName: "transit-unseal-creds",
	}
}

func validSameClusterAuthMethodConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubernetes-auth-default",
			Namespace: "payments",
		},
		Data: validSameClusterAuthMethodConfig(),
	}
}

func validSameClusterAuthMethodSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubernetes-auth-default",
			Namespace: "payments",
		},
		Data: map[string][]byte{
			"kubernetes_host": []byte("https://kubernetes.default.svc"),
			"issuer":          []byte("https://kubernetes.default.svc.cluster.local"),
		},
	}
}

func validSameClusterAuthMethodConfig() map[string]string {
	return map[string]string{
		"kubernetes_host": "https://kubernetes.default.svc",
		"issuer":          "https://kubernetes.default.svc.cluster.local",
	}
}

func validSameClusterPolicyContentConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-readwrite-policy",
			Namespace: "payments",
		},
		Data: map[string]string{
			"policy.hcl": validSameClusterPolicyContent(),
		},
	}
}

func validSameClusterPolicyContentSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-readwrite-policy",
			Namespace: "payments",
		},
		Data: map[string][]byte{
			"policy.hcl": []byte(validSameClusterPolicyContent()),
		},
	}
}

func validSameClusterPolicyContent() string {
	return `path "kv/data/*" { capabilities = ["read"] }`
}

func validSameClusterAuditSinkConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "audit-file-default",
			Namespace: "payments",
		},
		Data: map[string]string{
			"audit.json": `{"path":"stdout","fileOptions":{"filePath":"stdout"}}`,
		},
	}
}

func validSameClusterAuditSinkSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "audit-file-default",
			Namespace: "payments",
		},
		Data: map[string][]byte{
			"audit.json": []byte(`{"path":"stdout","fileOptions":{"filePath":"stdout"}}`),
		},
	}
}

func validExistingSameClusterConcreteSpec() openbaov1alpha1.OpenBaoClusterSpec {
	return openbaov1alpha1.OpenBaoClusterSpec{
		Version:  "2.5.0",
		Image:    "openbao/openbao:2.5.0",
		Replicas: 1,
		Profile:  openbaov1alpha1.ProfileDevelopment,
		TLS: openbaov1alpha1.TLSConfig{
			Enabled:        true,
			RotationPeriod: "720h",
		},
		Storage: openbaov1alpha1.StorageConfig{
			Size: "5Gi",
		},
		Service: &openbaov1alpha1.ServiceConfig{},
	}
}
