// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package claimcontract

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const testSecretRefKind = "Secret"

const testAuditStdoutPath = "stdout"
const testBackupLocation = "payments-prod"
const testRenderedExposureHostname = "payments-bao.example.internal"
const testRenderedIngressClassName = "nginx"

func TestRenderSameClusterExecutionContract(t *testing.T) {
	t.Parallel()

	claim := validRenderedPrimaryClaimFixture()
	catalog := validRenderedPrimaryCatalogBundleFixture()

	rendered := mustRenderSameClusterExecutionContract(
		t,
		claim,
		catalog,
		SameClusterTransitUnsealDefaults{
			Address:               "https://transit.example.internal:8200",
			KeyName:               "openbao-unseal",
			MountPath:             "transit",
			Namespace:             "platform",
			TLSCACert:             "/etc/bao/seal-creds/ca.crt",
			TLSServerName:         "transit.example.internal",
			CredentialsSecretName: "transit-unseal-creds",
		},
	)
	assertRenderedPrimarySameClusterContract(t, rendered)
	assertRenderedBackupImplementationContract(t, rendered)
	if IdentityHash(mustBindApprovedServiceContract(t, claim, catalog)) == "" || IdentityHash(rendered) == "" {
		t.Fatal("expected non-empty contract identity hashes")
	}
}

func TestApplySameClusterNetworkDefaults(t *testing.T) {
	t.Parallel()

	rendered := &RenderedExecutionContract{
		Network: RenderedNetwork{
			DNSNamespace: "kube-system",
		},
	}
	ApplySameClusterNetworkDefaults(rendered, SameClusterNetworkDefaults{
		APIServerCIDR:        testAPIServerCIDR,
		APIServerEndpointIPs: []string{" 172.29.0.3 ", "172.29.0.2", "172.29.0.2"},
		DNSEndpointIPs:       []string{"169.254.20.10"},
	})

	if rendered.Network.APIServerCIDR != testAPIServerCIDR {
		t.Fatalf("apiServerCIDR = %q, want %s", rendered.Network.APIServerCIDR, testAPIServerCIDR)
	}
	if len(rendered.Network.APIServerEndpointIPs) != 3 || rendered.Network.APIServerEndpointIPs[0] != "172.29.0.3" {
		t.Fatalf("apiServerEndpointIPs = %v, want trimmed configured values", rendered.Network.APIServerEndpointIPs)
	}
	if len(rendered.Network.DNSEndpointIPs) != 1 || rendered.Network.DNSEndpointIPs[0] != "169.254.20.10" {
		t.Fatalf("dnsEndpointIPs = %v, want configured dns endpoint", rendered.Network.DNSEndpointIPs)
	}
	if rendered.Network.DNSNamespace != "kube-system" {
		t.Fatalf("dnsNamespace = %q, want profile value preserved", rendered.Network.DNSNamespace)
	}
}

func TestApplySameClusterNetworkDefaultsPreservesProfileValues(t *testing.T) {
	t.Parallel()

	rendered := &RenderedExecutionContract{
		Network: RenderedNetwork{
			APIServerCIDR:        "10.99.0.1/32",
			APIServerEndpointIPs: []string{"172.29.0.9"},
			DNSEndpointIPs:       []string{"169.254.20.20"},
		},
	}
	ApplySameClusterNetworkDefaults(rendered, SameClusterNetworkDefaults{
		APIServerCIDR:        testAPIServerCIDR,
		APIServerEndpointIPs: []string{"172.29.0.3"},
		DNSEndpointIPs:       []string{"169.254.20.10"},
	})

	if rendered.Network.APIServerCIDR != "10.99.0.1/32" {
		t.Fatalf("apiServerCIDR = %q, want profile value preserved", rendered.Network.APIServerCIDR)
	}
	if len(rendered.Network.APIServerEndpointIPs) != 1 || rendered.Network.APIServerEndpointIPs[0] != "172.29.0.9" {
		t.Fatalf("apiServerEndpointIPs = %v, want profile value preserved", rendered.Network.APIServerEndpointIPs)
	}
	if len(rendered.Network.DNSEndpointIPs) != 1 || rendered.Network.DNSEndpointIPs[0] != "169.254.20.20" {
		t.Fatalf("dnsEndpointIPs = %v, want profile value preserved", rendered.Network.DNSEndpointIPs)
	}
}

func TestRenderSameClusterExecutionContractUsesAllowedClaimHostname(t *testing.T) {
	t.Parallel()

	claim := validRenderedPrimaryClaimFixture()
	claim.Spec.ServiceParameters.Exposure = &openbaov1alpha1.OpenBaoClusterClaimExposureServiceParametersSpec{
		Hostname: "custom.example.internal",
	}
	catalog := validRenderedPrimaryCatalogBundleFixture()
	catalog.ExposureClass.Spec.HostnamePolicy.Claim = &openbaov1alpha1.OpenBaoExposureClaimHostnamePolicySpec{
		Enabled:         true,
		AllowedSuffixes: []string{"example.internal"},
	}

	rendered := mustRenderSameClusterExecutionContract(
		t,
		claim,
		catalog,
		SameClusterTransitUnsealDefaults{
			Address:               "https://transit.example.internal:8200",
			KeyName:               "openbao-unseal",
			MountPath:             "transit",
			TLSCACert:             "/etc/bao/seal-creds/ca.crt",
			CredentialsSecretName: "transit-unseal-creds",
		},
	)
	if rendered.Exposure.HostnamePolicy.Value != "custom.example.internal" {
		t.Fatalf("hostname = %q, want requested claim hostname", rendered.Exposure.HostnamePolicy.Value)
	}
}

func TestRenderSameClusterExecutionContractRejectsDisallowedClaimHostname(t *testing.T) {
	t.Parallel()

	claim := validRenderedPrimaryClaimFixture()
	claim.Spec.ServiceParameters.Exposure = &openbaov1alpha1.OpenBaoClusterClaimExposureServiceParametersSpec{
		Hostname: "custom.example.com",
	}
	catalog := validRenderedPrimaryCatalogBundleFixture()
	catalog.ExposureClass.Spec.HostnamePolicy.Claim = &openbaov1alpha1.OpenBaoExposureClaimHostnamePolicySpec{
		Enabled:         true,
		AllowedSuffixes: []string{"example.internal"},
	}

	rendered, result := RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		mustBindApprovedServiceContract(t, claim, catalog),
		catalog,
		SameClusterTransitUnsealDefaults{},
		SameClusterBootstrapResolvedInputs{},
	)
	if result.Valid {
		t.Fatalf("RenderSameClusterExecutionContract() = %#v, want invalid", result)
	}
	if rendered != nil {
		t.Fatalf("rendered = %#v, want nil", rendered)
	}
}

func TestRenderSameClusterExecutionContractRendersIngressInputs(t *testing.T) {
	t.Parallel()

	claim := validRenderedIngressClaimFixture()
	catalog := validRenderedIngressCatalogBundleFixture()

	rendered := mustRenderSameClusterExecutionContract(
		t,
		claim,
		catalog,
		SameClusterTransitUnsealDefaults{},
	)
	if rendered.Exposure.Ingress == nil {
		t.Fatal("rendered ingress = nil, want concrete ingress execution inputs")
	}
	if rendered.Exposure.Ingress.PolicyRef == nil || rendered.Exposure.Ingress.PolicyRef.UID != "ingress-policy-uid" {
		t.Fatalf("unexpected rendered ingress policy ref: %#v", rendered.Exposure.Ingress.PolicyRef)
	}
	if rendered.Exposure.Ingress.ClassName != testRenderedIngressClassName {
		t.Fatalf("className = %q, want nginx", rendered.Exposure.Ingress.ClassName)
	}
	if rendered.Exposure.Ingress.PathType != openbaov1alpha1.IngressPathTypePrefix {
		t.Fatalf("pathType = %q, want Prefix", rendered.Exposure.Ingress.PathType)
	}
	if rendered.Exposure.Ingress.ReadinessMode != openbaov1alpha1.IngressReadinessModeLoadBalancerPublished {
		t.Fatalf("readinessMode = %q, want LoadBalancerPublished", rendered.Exposure.Ingress.ReadinessMode)
	}
	if rendered.Exposure.Ingress.BackendTLSPublicationMode != openbaov1alpha1.OpenBaoIngressBackendTLSPublicationModeAnnotation {
		t.Fatalf("backendTLSPublicationMode = %q, want Annotation", rendered.Exposure.Ingress.BackendTLSPublicationMode)
	}
	if rendered.Exposure.Ingress.Annotations["nginx.ingress.kubernetes.io/backend-protocol"] != "HTTPS" {
		t.Fatalf("annotations = %#v, want backend TLS annotation", rendered.Exposure.Ingress.Annotations)
	}
}

func assertRenderedPrimarySameClusterContract(t *testing.T, rendered *RenderedExecutionContract) {
	t.Helper()

	if rendered.TargetNamespace != "payments" {
		t.Fatalf("TargetNamespace = %q, want %q", rendered.TargetNamespace, "payments")
	}
	if rendered.Cluster.Version != "2.6.0" || rendered.Cluster.Replicas != 3 || rendered.Cluster.ReadReplicas != 1 {
		t.Fatalf("unexpected rendered cluster contract: %#v", rendered.Cluster)
	}
	if rendered.Unseal.Mode != UnsealPostureModeExternal {
		t.Fatalf("unexpected rendered unseal contract: %#v", rendered.Unseal)
	}
	if rendered.Unseal.Transit == nil || rendered.Unseal.Transit.Address != "https://transit.example.internal:8200" {
		t.Fatalf("unexpected rendered transit unseal contract: %#v", rendered.Unseal)
	}
	if rendered.Bootstrap.OperatorLifecycleAuth.JWT == nil || rendered.Bootstrap.OperatorLifecycleAuth.JWT.Audience != "openbao-operator" {
		t.Fatalf("unexpected rendered bootstrap contract: %#v", rendered.Bootstrap)
	}
	if rendered.Exposure.HostnamePolicy.Value != testRenderedExposureHostname {
		t.Fatalf("unexpected rendered hostname policy: %#v", rendered.Exposure.HostnamePolicy)
	}
	if rendered.Exposure.Entrypoint == nil {
		t.Fatal("rendered exposure entrypoint = nil, want concrete gateway entrypoint")
	}
	if rendered.Exposure.Entrypoint.Ref == nil || rendered.Exposure.Entrypoint.Ref.UID != "entrypoint-uid" {
		t.Fatalf("unexpected rendered exposure entrypoint ref: %#v", rendered.Exposure.Entrypoint)
	}
	if rendered.Exposure.Entrypoint.ObjectRef.Name != "internal-gateway" {
		t.Fatalf("unexpected rendered exposure entrypoint objectRef: %#v", rendered.Exposure.Entrypoint.ObjectRef)
	}
	if rendered.Exposure.Entrypoint.ListenerPolicy == nil || rendered.Exposure.Entrypoint.ListenerPolicy.SectionName != "https" {
		t.Fatalf("unexpected rendered exposure listener policy: %#v", rendered.Exposure.Entrypoint.ListenerPolicy)
	}
}

func assertRenderedBackupImplementationContract(t *testing.T, rendered *RenderedExecutionContract) {
	t.Helper()

	if rendered.Backup.TargetRef == nil || rendered.Backup.TargetRef.Name != "primary-object-backup-v1" || rendered.Backup.TargetRef.UID != "backup-target-uid" {
		t.Fatalf("unexpected rendered backup target ref: %#v", rendered.Backup)
	}
	if rendered.Backup.BackendRef == nil || rendered.Backup.BackendRef.Name != "s3-primary-v1" || rendered.Backup.BackendRef.UID != "backup-backend-uid" {
		t.Fatalf("unexpected rendered backup backend ref: %#v", rendered.Backup)
	}
	if rendered.Backup.AuthProfileRef == nil || rendered.Backup.AuthProfileRef.Name != "aws-irsa-backup-v1" || rendered.Backup.AuthProfileRef.UID != "backup-auth-uid" {
		t.Fatalf("unexpected rendered backup auth ref: %#v", rendered.Backup)
	}
	if rendered.Backup.TransferProfileRef == nil || rendered.Backup.TransferProfileRef.Name != "multipart-standard-v1" || rendered.Backup.TransferProfileRef.UID != "transfer-profile-uid" {
		t.Fatalf("unexpected rendered backup transfer ref: %#v", rendered.Backup)
	}
	if rendered.Backup.Location != testBackupLocation {
		t.Fatalf("rendered backup location = %q, want %q", rendered.Backup.Location, testBackupLocation)
	}
	if rendered.Backup.KeyPrefix != "claims/payments/payments-bao/finance" {
		t.Fatalf("rendered backup keyPrefix = %q, want %q", rendered.Backup.KeyPrefix, "claims/payments/payments-bao/finance")
	}
	if rendered.Backup.Backend == nil || rendered.Backup.Backend.Provider != openbaov1alpha1.OpenBaoObjectStorageProviderS3 || rendered.Backup.Backend.Region != "eu-west-1" {
		t.Fatalf("unexpected rendered backup backend contract: %#v", rendered.Backup.Backend)
	}
	if rendered.Backup.Auth == nil || rendered.Backup.Auth.Mode != openbaov1alpha1.OpenBaoBackupAuthModeWorkloadIdentity || rendered.Backup.Auth.RoleARN != "arn:aws:iam::123456789012:role/openbao-backup" {
		t.Fatalf("unexpected rendered backup auth contract: %#v", rendered.Backup.Auth)
	}
	if rendered.Backup.Auth.WorkloadIdentity == nil || rendered.Backup.Auth.WorkloadIdentity.ServiceAccountAnnotations["eks.amazonaws.com/role-arn"] == "" {
		t.Fatalf("unexpected rendered backup workload identity contract: %#v", rendered.Backup.Auth)
	}
	if rendered.Backup.Transfer == nil || rendered.Backup.Transfer.PartSize != 16777216 || rendered.Backup.Transfer.Concurrency != 5 {
		t.Fatalf("unexpected rendered backup transfer contract: %#v", rendered.Backup.Transfer)
	}
	assertRenderedBackupEgressContract(t, rendered)
}

func assertRenderedBackupEgressContract(t *testing.T, rendered *RenderedExecutionContract) {
	t.Helper()

	if len(rendered.Network.RequiredEgressRules) != 1 {
		t.Fatalf("rendered network egress rules = %#v, want one rendered rule", rendered.Network.RequiredEgressRules)
	}
	if rendered.Network.RequiredEgressRules[0].To[0].IPBlock == nil || rendered.Network.RequiredEgressRules[0].To[0].IPBlock.CIDR != "10.10.0.0/16" {
		t.Fatalf("unexpected rendered backup egress destination: %#v", rendered.Network.RequiredEgressRules)
	}
	if len(rendered.Network.RequiredEgressRules[0].Ports) != 1 || rendered.Network.RequiredEgressRules[0].Ports[0].Port == nil || rendered.Network.RequiredEgressRules[0].Ports[0].Port.IntVal != 443 {
		t.Fatalf("unexpected rendered backup egress ports: %#v", rendered.Network.RequiredEgressRules)
	}
}

func TestRenderSameClusterExecutionContractRejectsDisallowedBackupPartition(t *testing.T) {
	t.Parallel()

	claim := validRenderedPrimaryClaimFixture()
	claim.Spec.ServiceProfileRef.Name = "standard-dev-v1"
	catalog := validRenderedDevelopmentCatalogBundleFixture()
	catalog.ServiceProfile.Spec.Bootstrap.ProfileRef = nil
	catalog.ServiceProfile.Spec.Backup.ProfileRef = openbaov1alpha1.LocalReference{Name: "backup-enabled-v1"}
	catalog.BackupProfile = &openbaov1alpha1.OpenBaoBackupProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "backup-enabled-v1", UID: types.UID("backup-uid")},
		Spec: openbaov1alpha1.OpenBaoBackupProfileSpec{
			Schedule:  "0 3 * * *",
			TargetRef: &openbaov1alpha1.LocalReference{Name: "primary-object-backup-v1"},
		},
	}
	catalog.BackupTarget = &openbaov1alpha1.OpenBaoBackupTarget{
		ObjectMeta: metav1.ObjectMeta{Name: "primary-object-backup-v1", UID: types.UID("backup-target-uid")},
		Spec: openbaov1alpha1.OpenBaoBackupTargetSpec{
			BackendRef: openbaov1alpha1.LocalReference{Name: "s3-primary-v1"},
			LocationPolicy: openbaov1alpha1.OpenBaoBackupLocationPolicySpec{
				Location: openbaov1alpha1.OpenBaoBackupLocationSelectionSpec{
					Mode:              openbaov1alpha1.OpenBaoBackupLocationModeClaimValue,
					ValidationPattern: "^[a-z0-9-]+$",
				},
				KeyPrefix: openbaov1alpha1.OpenBaoBackupKeyPrefixPolicySpec{
					Template: "claims/{{ claim.namespace }}/{{ claim.name }}",
				},
			},
		},
	}
	catalog.BackupBackend = &openbaov1alpha1.OpenBaoBackupBackend{
		ObjectMeta: metav1.ObjectMeta{Name: "s3-primary-v1", UID: types.UID("backup-backend-uid")},
		Spec: openbaov1alpha1.OpenBaoBackupBackendSpec{
			Driver: openbaov1alpha1.OpenBaoBackupBackendDriverObjectStorage,
			ObjectStorage: &openbaov1alpha1.OpenBaoBackupBackendObjectStorageSpec{
				Provider: openbaov1alpha1.OpenBaoObjectStorageProviderS3,
			},
		},
	}

	rendered, renderValidation := RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		mustBindApprovedServiceContract(t, claim, catalog),
		catalog,
		SameClusterTransitUnsealDefaults{},
		SameClusterBootstrapResolvedInputs{},
	)
	if renderValidation.Valid {
		t.Fatalf("RenderSameClusterExecutionContract() = %#v, want invalid", renderValidation)
	}
	if rendered != nil {
		t.Fatalf("rendered = %#v, want nil", rendered)
	}
}

func TestRenderSameClusterExecutionContractRequiresTransitDefaultsForHardened(t *testing.T) {
	t.Parallel()

	claim := validRenderedPrimaryClaimFixture()
	claim.Spec.ServiceParameters = nil
	catalog := validRenderedPrimaryCatalogBundleFixture()
	catalog.ServiceProfile.Spec.Bootstrap.ProfileRef = nil
	catalog.BootstrapProfile = nil
	catalog.ExposureClass.Spec.PublishMode = openbaov1alpha1.OpenBaoExposurePublishModeClusterInternal
	catalog.ExposureClass.Spec.EntrypointRef = nil
	catalog.ExposureClass.Spec.HostnamePolicy = openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{}
	catalog.ExposureClass.Spec.TLSPolicy = nil
	catalog.ExposureClass.Spec.ServicePolicy = nil
	catalog.Entrypoint = nil

	rendered, renderResult := RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		mustBindApprovedServiceContract(t, claim, catalog),
		catalog,
		SameClusterTransitUnsealDefaults{},
		SameClusterBootstrapResolvedInputs{},
	)
	if renderResult.Valid {
		t.Fatalf("RenderSameClusterExecutionContract() = %#v, want invalid", renderResult)
	}
	if rendered != nil {
		t.Fatalf("rendered = %#v, want nil", rendered)
	}
}

func TestRenderSameClusterExecutionContractRendersAuthMethodConfig(t *testing.T) {
	t.Parallel()

	claim := validRenderedDevelopmentClaimFixture()
	catalog := validRenderedDevelopmentCatalogBundleFixture()
	catalog.BootstrapProfile.Spec.Auth = &openbaov1alpha1.OpenBaoBootstrapAuthSpec{
		Methods: []openbaov1alpha1.OpenBaoBootstrapAuthMethodSpec{{
			Type: "kubernetes",
			Path: "kubernetes",
			ConfigRef: &openbaov1alpha1.TypedObjectReference{
				Kind: "ConfigMap",
				Name: "kubernetes-auth-default",
			},
		}},
	}

	rendered, renderValidation := RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		mustBindApprovedServiceContract(t, claim, catalog),
		catalog,
		SameClusterTransitUnsealDefaults{},
		SameClusterBootstrapResolvedInputs{
			AuthMethodConfigs: map[string]ProjectedBootstrapArtifact{
				BootstrapAuthMethodIdentity("kubernetes", "kubernetes"): {
					Ref: openbaov1alpha1.TypedObjectReference{
						Kind: testSecretRefKind,
						Name: "claim-bootstrap-authcfg-a1b2c3d4",
					},
					SecretData: map[string][]byte{
						"kubernetes_host": []byte("https://kubernetes.default.svc"),
						"issuer":          []byte("https://kubernetes.default.svc.cluster.local"),
					},
				},
			},
		},
	)
	if !renderValidation.Valid {
		t.Fatalf("RenderSameClusterExecutionContract() = %#v, want valid", renderValidation)
	}
	if rendered.Bootstrap.Auth == nil || len(rendered.Bootstrap.Auth.Methods) != 1 {
		t.Fatalf("rendered.Bootstrap.Auth = %#v, want one auth method", rendered.Bootstrap.Auth)
	}
	if got := rendered.Bootstrap.Auth.Methods[0].ConfigFromRef; got == nil || got.Kind != testSecretRefKind || got.Name != "claim-bootstrap-authcfg-a1b2c3d4" {
		t.Fatalf("rendered auth configFromRef = %#v, want projected Secret ref", got)
	}
}

func TestRenderSameClusterExecutionContractRendersPolicyBundles(t *testing.T) {
	t.Parallel()

	claim := validRenderedDevelopmentClaimFixture()
	catalog := validRenderedDevelopmentCatalogBundleFixture()
	catalog.BootstrapProfile.Spec.Policies = &openbaov1alpha1.OpenBaoBootstrapPoliciesSpec{
		Bundles: []openbaov1alpha1.OpenBaoBootstrapPolicyBundleSpec{{
			Name: "app-readwrite",
			ContentRef: openbaov1alpha1.TypedObjectReference{
				Kind: "ConfigMap",
				Name: "app-readwrite-policy",
			},
		}},
	}

	rendered, renderValidation := RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		mustBindApprovedServiceContract(t, claim, catalog),
		catalog,
		SameClusterTransitUnsealDefaults{},
		SameClusterBootstrapResolvedInputs{
			PolicyBundleContents: map[string]ProjectedBootstrapArtifact{
				BootstrapPolicyBundleIdentity(catalog.BootstrapProfile.Spec.Policies.Bundles[0]): {
					Ref: openbaov1alpha1.TypedObjectReference{
						Kind: "ConfigMap",
						Name: "claim-bootstrap-policy-a1b2c3d4",
					},
					ConfigMapData: map[string]string{
						"content": `path "kv/data/*" { capabilities = ["read"] }`,
					},
				},
			},
		},
	)
	if !renderValidation.Valid {
		t.Fatalf("RenderSameClusterExecutionContract() = %#v, want valid", renderValidation)
	}
	if rendered.Bootstrap.Policies == nil || len(rendered.Bootstrap.Policies.Bundles) != 1 {
		t.Fatalf("rendered.Bootstrap.Policies = %#v, want one policy bundle", rendered.Bootstrap.Policies)
	}
	if got := rendered.Bootstrap.Policies.Bundles[0].ContentFromRef; got.Kind != "ConfigMap" || got.Name != "claim-bootstrap-policy-a1b2c3d4" {
		t.Fatalf("rendered policy contentFromRef = %#v, want projected ConfigMap ref", got)
	}
}

func TestRenderSameClusterExecutionContractRendersAuditDevices(t *testing.T) {
	t.Parallel()

	claim := validRenderedDevelopmentClaimFixture()
	catalog := validRenderedDevelopmentCatalogBundleFixture()
	catalog.BootstrapProfile.Spec.Audit = &openbaov1alpha1.OpenBaoBootstrapAuditSpec{
		Devices: []openbaov1alpha1.OpenBaoBootstrapAuditDeviceSpec{{
			Type: "file",
			SinkRef: &openbaov1alpha1.TypedObjectReference{
				Kind: "ConfigMap",
				Name: "audit-file-default",
			},
		}},
	}

	rendered, renderValidation := RenderSameClusterExecutionContract(
		claim,
		&openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
		mustBindApprovedServiceContract(t, claim, catalog),
		catalog,
		SameClusterTransitUnsealDefaults{},
		SameClusterBootstrapResolvedInputs{
			AuditDeviceSinks: map[string]ProjectedBootstrapAuditSink{
				BootstrapAuditDeviceIdentity(catalog.BootstrapProfile.Spec.Audit.Devices[0]): {
					Artifact: ProjectedBootstrapArtifact{
						Ref: openbaov1alpha1.TypedObjectReference{
							Kind: testSecretRefKind,
							Name: "claim-bootstrap-audit-a1b2c3d4",
						},
						SecretData: map[string][]byte{
							"sink.json": []byte(`{"path":"stdout","fileOptions":{"filePath":"stdout"}}`),
						},
					},
					Path: testAuditStdoutPath,
				},
			},
		},
	)
	if !renderValidation.Valid {
		t.Fatalf("RenderSameClusterExecutionContract() = %#v, want valid", renderValidation)
	}
	if rendered.Bootstrap.Audit == nil || len(rendered.Bootstrap.Audit.Devices) != 1 {
		t.Fatalf("rendered.Bootstrap.Audit = %#v, want one audit device", rendered.Bootstrap.Audit)
	}
	if got := rendered.Bootstrap.Audit.Devices[0].Path; got != testAuditStdoutPath {
		t.Fatalf("rendered audit path = %q, want stdout", got)
	}
	if got := rendered.Bootstrap.Audit.Devices[0].SinkFromRef; got == nil || got.Kind != testSecretRefKind || got.Name != "claim-bootstrap-audit-a1b2c3d4" {
		t.Fatalf("rendered audit sinkFromRef = %#v, want projected Secret ref", got)
	}
}
