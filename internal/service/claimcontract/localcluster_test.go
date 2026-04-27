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

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestDesiredSameClusterCluster(t *testing.T) {
	t.Parallel()

	claim := validRenderedDevelopmentClaimFixture()
	catalog := validRenderedDevelopmentCatalogBundleFixture()
	catalog.ServiceProfile.Spec.Cluster.Voters = 3
	catalog.ServiceProfile.Spec.Cluster.ReadReplicas = ptr.To(int32(1))
	catalog.ServiceProfile.Spec.Storage.ReadReplicaSize = "10Gi"
	catalog.ExposureClass.Spec.HostnamePolicy.Mode = openbaov1alpha1.OpenBaoExposureHostnamePolicyModeGenerated
	catalog.ExposureClass.Spec.ServicePolicy = &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
		Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
		BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
	}

	cluster := mustDesiredSameClusterCluster(t, claim, mustRenderSameClusterExecutionContract(
		t,
		claim,
		catalog,
		SameClusterTransitUnsealDefaults{},
	))
	if cluster.Namespace != "payments" || cluster.Name != "payments-bao" {
		t.Fatalf("cluster key = %s/%s, want payments/payments-bao", cluster.Namespace, cluster.Name)
	}
	if cluster.Spec.Profile != openbaov1alpha1.ProfileDevelopment {
		t.Fatalf("profile = %q, want %q", cluster.Spec.Profile, openbaov1alpha1.ProfileDevelopment)
	}
	if cluster.Spec.Replicas != 3 {
		t.Fatalf("replicas = %d, want 3", cluster.Spec.Replicas)
	}
	if cluster.Spec.ReadReplicas == nil || cluster.Spec.ReadReplicas.Replicas != 1 {
		t.Fatalf("readReplicas = %#v, want one read replica", cluster.Spec.ReadReplicas)
	}
	if cluster.Spec.SelfInit == nil || !cluster.Spec.SelfInit.Enabled || len(cluster.Spec.SelfInit.Requests) != 1 {
		t.Fatalf("selfInit = %#v, want enabled with one request", cluster.Spec.SelfInit)
	}
	if cluster.Spec.SelfInit.OIDC == nil || !cluster.Spec.SelfInit.OIDC.Enabled || cluster.Spec.SelfInit.OIDC.Audience != "openbao-operator" {
		t.Fatalf("selfInit.oidc = %#v, want enabled with operator audience", cluster.Spec.SelfInit.OIDC)
	}
	if cluster.Spec.SelfInit.Requests[0].Path != "sys/mounts/secret" {
		t.Fatalf("selfInit request path = %q, want %q", cluster.Spec.SelfInit.Requests[0].Path, "sys/mounts/secret")
	}
	if cluster.Spec.Upgrade == nil || cluster.Spec.Upgrade.Strategy != openbaov1alpha1.UpdateStrategyRollingUpdate {
		t.Fatalf("upgrade = %#v, want rolling update", cluster.Spec.Upgrade)
	}
	if cluster.Spec.Unseal != nil {
		t.Fatalf("unseal = %#v, want nil static default", cluster.Spec.Unseal)
	}
}

func TestDesiredSameClusterClusterBoundsLongClaimNames(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "guarded-claim-e2e-claims-guardrails-1776869695-4492b1e7",
			Namespace: "payments",
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-dev-v1"},
		},
	}
	catalog := validRenderedDevelopmentCatalogBundleFixture()
	catalog.ServiceProfile.Spec.Storage.PrimarySize = "5Gi"
	cluster := mustDesiredSameClusterCluster(t, claim, mustRenderSameClusterExecutionContract(
		t,
		claim,
		catalog,
		SameClusterTransitUnsealDefaults{},
	))
	if cluster.Name != ClaimManagedLocalClusterName(claim.Name) {
		t.Fatalf("cluster name = %q, want %q", cluster.Name, ClaimManagedLocalClusterName(claim.Name))
	}
	if len(cluster.Name) > maxClaimManagedLocalClusterNameLength {
		t.Fatalf("len(cluster.Name) = %d, want <= %d", len(cluster.Name), maxClaimManagedLocalClusterNameLength)
	}
}

func TestDesiredSameClusterClusterRejectsPreUpgradeSnapshotWithoutBackup(t *testing.T) {
	t.Parallel()

	claim := validRenderedDevelopmentClaimFixture()
	catalog := validRenderedDevelopmentCatalogBundleFixture()
	catalog.ServiceProfile.Spec.Cluster.ReadReplicas = ptr.To(int32(1))
	catalog.ServiceProfile.Spec.Storage.ReadReplicaSize = "10Gi"
	preUpgradeSnapshot := true
	catalog.ServiceProfile.Spec.Lifecycle = openbaov1alpha1.OpenBaoServiceProfileLifecycleSpec{
		UpgradeStrategy:    openbaov1alpha1.UpdateStrategyRollingUpdate,
		PreUpgradeSnapshot: &preUpgradeSnapshot,
	}

	cluster, result := DesiredSameClusterCluster(claim, mustRenderSameClusterExecutionContract(
		t,
		claim,
		catalog,
		SameClusterTransitUnsealDefaults{},
	))
	if result.Valid {
		t.Fatalf("DesiredSameClusterCluster() = %#v, want invalid", result)
	}
	if cluster != nil {
		t.Fatalf("cluster = %#v, want nil", cluster)
	}
}

func TestDesiredSameClusterClusterProjectsCatalogImplementationProfiles(t *testing.T) {
	t.Parallel()

	claim := validRenderedDevelopmentClaimFixture()
	catalog, expectations := catalogImplementationProfileFixture()

	cluster := mustDesiredSameClusterCluster(t, claim, mustRenderSameClusterExecutionContract(
		t,
		claim,
		catalog,
		SameClusterTransitUnsealDefaults{},
	))

	assertCatalogStorageProjection(t, cluster, expectations.primaryClass, expectations.acmeCacheClass)
	assertCatalogUnsealProjection(t, cluster)
	assertCatalogRuntimeProjection(t, cluster, expectations.fsGroup)
	assertCatalogObservabilityProjection(t, cluster)
	assertCatalogUsabilityProjection(t, cluster)
}

type catalogImplementationProfileExpectations struct {
	primaryClass   string
	acmeCacheClass string
	fsGroup        int64
}

func catalogImplementationProfileFixture() (*CatalogBundle, catalogImplementationProfileExpectations) {
	catalog := validRenderedDevelopmentCatalogBundleFixture()
	expectations := catalogImplementationProfileExpectations{
		primaryClass:   "fast-rwo",
		acmeCacheClass: "shared-rwx",
		fsGroup:        1000710000,
	}
	appArmorEnabled := true
	readReplicas := int32(1)

	catalog.ServiceProfile.Spec.Cluster.SecurityProfile = openbaov1alpha1.ProfileHardened
	catalog.ServiceProfile.Spec.Cluster.Voters = 3
	catalog.ServiceProfile.Spec.Cluster.ReadReplicas = &readReplicas
	catalog.ServiceProfile.Spec.Storage.ProfileRef = &openbaov1alpha1.LocalReference{Name: "production-storage-v1"}
	catalog.ServiceProfile.Spec.Unseal = &openbaov1alpha1.OpenBaoServiceProfileUnsealSpec{
		ProfileRef: &openbaov1alpha1.LocalReference{Name: "aws-kms-unseal-v1"},
	}
	catalog.ServiceProfile.Spec.Runtime = &openbaov1alpha1.OpenBaoServiceProfileRuntimeSpec{
		ProfileRef: &openbaov1alpha1.LocalReference{Name: "aws-workload-v1"},
	}
	catalog.ServiceProfile.Spec.Observability = &openbaov1alpha1.OpenBaoServiceProfileObservabilitySpec{
		ProfileRef: &openbaov1alpha1.LocalReference{Name: "metrics-v1"},
	}
	catalog.ServiceProfile.Spec.Network = &openbaov1alpha1.OpenBaoServiceProfileNetworkSpec{
		ProfileRef: &openbaov1alpha1.LocalReference{Name: "network-dependencies-v1"},
	}
	catalog.ServiceProfile.Spec.Lifecycle = openbaov1alpha1.OpenBaoServiceProfileLifecycleSpec{
		PolicyRef:       &openbaov1alpha1.LocalReference{Name: "blue-green-conservative-v1"},
		UpgradeStrategy: openbaov1alpha1.UpdateStrategyBlueGreen,
	}
	catalog.ExposureClass.Spec.TLSPolicy = &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
		Mode: openbaov1alpha1.OpenBaoExposureTLSModeACME,
		ACME: &openbaov1alpha1.OpenBaoExposureACMEPolicySpec{
			DirectoryURL: "https://acme.example.internal/directory",
			Domains:      []string{"payments-bao.example.internal"},
			Email:        "platform@example.internal",
		},
	}
	catalog.ExposureClass.Spec.ReadReplicaServicePolicy = &openbaov1alpha1.OpenBaoExposureReadReplicaServicePolicySpec{
		Enabled:     true,
		Type:        openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
		Annotations: map[string]string{"service.beta.kubernetes.io/aws-load-balancer-scheme": "internal"},
	}
	catalog.StorageProfile = &openbaov1alpha1.OpenBaoStorageProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "production-storage-v1"},
		Spec: openbaov1alpha1.OpenBaoStorageProfileSpec{
			Primary: &openbaov1alpha1.OpenBaoStorageProfileVolumeSpec{
				StorageClassName: &expectations.primaryClass,
			},
			ACMECache: &openbaov1alpha1.OpenBaoStorageProfileACMECacheSpec{
				Mode:             openbaov1alpha1.ACMESharedCacheModeManagedPVC,
				Size:             "1Gi",
				StorageClassName: &expectations.acmeCacheClass,
			},
		},
	}
	catalog.UnsealProfile = &openbaov1alpha1.OpenBaoUnsealProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-kms-unseal-v1"},
		Spec: openbaov1alpha1.OpenBaoUnsealProfileSpec{
			Mode: openbaov1alpha1.OpenBaoUnsealProfileModeAWSKMS,
			AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{
				Region:   "eu-west-1",
				KMSKeyID: "alias/openbao",
			},
			CredentialsSecretRef: &corev1.LocalObjectReference{Name: "aws-kms-credentials"},
		},
	}
	catalog.RuntimeProfile = &openbaov1alpha1.OpenBaoRuntimeProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-workload-v1"},
		Spec: openbaov1alpha1.OpenBaoRuntimeProfileSpec{
			ServiceAccount: &openbaov1alpha1.ServiceAccountConfig{
				Name:        "openbao-workload",
				Annotations: map[string]string{"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/openbao"},
			},
			PodMetadata: &openbaov1alpha1.PodMetadataConfig{
				Labels: map[string]string{"azure.workload.identity/use": "true"},
			},
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "private-registry"}},
			WorkloadHardening: &openbaov1alpha1.WorkloadHardeningConfig{
				AppArmorEnabled: appArmorEnabled,
			},
			SecurityContext: &corev1.PodSecurityContext{FSGroup: &expectations.fsGroup},
			HelperImages: &openbaov1alpha1.OpenBaoRuntimeProfileHelperImagesSpec{
				Init:    "registry.example.com/openbao-init:2.6.0",
				Restore: "registry.example.com/openbao-restore:2.6.0",
				Upgrade: "registry.example.com/openbao-upgrade:2.6.0",
			},
			ReadReplica: &openbaov1alpha1.OpenBaoRuntimeProfileReadReplicaSpec{
				Template: &openbaov1alpha1.ReadReplicaTemplateConfig{
					Metadata: &openbaov1alpha1.PodMetadataConfig{
						Labels: map[string]string{"openbao.org/read-replica": "true"},
					},
				},
			},
		},
	}
	catalog.ObservabilityProfile = &openbaov1alpha1.OpenBaoObservabilityProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-v1"},
		Spec: openbaov1alpha1.OpenBaoObservabilityProfileSpec{
			Observability: &openbaov1alpha1.ObservabilityConfig{
				Metrics: &openbaov1alpha1.MetricsConfig{
					Enabled: true,
					ServiceMonitor: &openbaov1alpha1.ServiceMonitorConfig{
						Enabled:       true,
						Interval:      "15s",
						ScrapeTimeout: "5s",
					},
				},
			},
		},
	}
	catalog.NetworkProfile = &openbaov1alpha1.OpenBaoNetworkProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "network-dependencies-v1"},
		Spec: openbaov1alpha1.OpenBaoNetworkProfileSpec{
			APIServerCIDR:  testAPIServerCIDR,
			DNSNamespace:   "kube-system",
			DNSEndpointIPs: []string{"169.254.20.10"},
			EgressRules: []networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: "10.20.0.0/16"}}},
			}},
			TrustedIngressPeers: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": "ingress-system"}},
			}},
		},
	}
	autoPromote := false
	maxFailures := int32(2)
	catalog.UpgradePolicy = &openbaov1alpha1.OpenBaoUpgradePolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "blue-green-conservative-v1"},
		Spec: openbaov1alpha1.OpenBaoUpgradePolicySpec{
			BlueGreen: &openbaov1alpha1.OpenBaoUpgradePolicyBlueGreenSpec{
				AutoPromote:     &autoPromote,
				MinSyncDuration: "2m",
				MaxJobFailures:  &maxFailures,
			},
		},
	}

	return catalog, expectations
}

func assertCatalogStorageProjection(
	t *testing.T,
	cluster *openbaov1alpha1.OpenBaoCluster,
	primaryClass string,
	acmeCacheClass string,
) {
	t.Helper()

	if cluster.Spec.TLS.Mode != openbaov1alpha1.TLSModeACME || cluster.Spec.TLS.ACME == nil {
		t.Fatalf("tls = %#v, want ACME config", cluster.Spec.TLS)
	}
	if cluster.Spec.TLS.ACME.DirectoryURL != "https://acme.example.internal/directory" ||
		cluster.Spec.TLS.ACME.SharedCache == nil ||
		cluster.Spec.TLS.ACME.SharedCache.StorageClassName == nil ||
		*cluster.Spec.TLS.ACME.SharedCache.StorageClassName != acmeCacheClass {
		t.Fatalf("tls.acme = %#v, want configured ACME with managed shared cache", cluster.Spec.TLS.ACME)
	}
	if cluster.Spec.Storage.StorageClassName == nil || *cluster.Spec.Storage.StorageClassName != primaryClass {
		t.Fatalf("storage.storageClassName = %#v, want %q", cluster.Spec.Storage.StorageClassName, primaryClass)
	}
	if cluster.Spec.ReadReplicas == nil ||
		cluster.Spec.ReadReplicas.Storage == nil ||
		cluster.Spec.ReadReplicas.Storage.Size != nil ||
		cluster.Spec.ReadReplicas.Storage.StorageClassName == nil ||
		*cluster.Spec.ReadReplicas.Storage.StorageClassName != primaryClass {
		t.Fatalf("readReplica storage = %#v, want inherited primary storage class without size override", cluster.Spec.ReadReplicas)
	}
}

func assertCatalogUnsealProjection(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster) {
	t.Helper()

	if cluster.Spec.Unseal == nil ||
		cluster.Spec.Unseal.Type != unsealTypeAWSKMS ||
		cluster.Spec.Unseal.AWSKMS == nil ||
		cluster.Spec.Unseal.CredentialsSecretRef == nil ||
		cluster.Spec.Unseal.CredentialsSecretRef.Name != "aws-kms-credentials" {
		t.Fatalf("unseal = %#v, want AWS KMS profile projection", cluster.Spec.Unseal)
	}
}

func assertCatalogRuntimeProjection(
	t *testing.T,
	cluster *openbaov1alpha1.OpenBaoCluster,
	fsGroup int64,
) {
	t.Helper()

	if cluster.Spec.ServiceAccount == nil ||
		cluster.Spec.ServiceAccount.Name != "openbao-workload" ||
		cluster.Spec.ServiceAccount.Annotations["eks.amazonaws.com/role-arn"] == "" {
		t.Fatalf("serviceAccount = %#v, want runtime profile projection", cluster.Spec.ServiceAccount)
	}
	if cluster.Spec.PodMetadata == nil || cluster.Spec.PodMetadata.Labels["azure.workload.identity/use"] != "true" {
		t.Fatalf("podMetadata = %#v, want runtime pod labels", cluster.Spec.PodMetadata)
	}
	if len(cluster.Spec.ImagePullSecrets) != 1 || cluster.Spec.ImagePullSecrets[0].Name != "private-registry" {
		t.Fatalf("imagePullSecrets = %#v, want private-registry", cluster.Spec.ImagePullSecrets)
	}
	if cluster.Spec.WorkloadHardening == nil || !cluster.Spec.WorkloadHardening.AppArmorEnabled {
		t.Fatalf("workloadHardening = %#v, want AppArmor enabled", cluster.Spec.WorkloadHardening)
	}
	if cluster.Spec.SecurityContext == nil || cluster.Spec.SecurityContext.FSGroup == nil || *cluster.Spec.SecurityContext.FSGroup != fsGroup {
		t.Fatalf("securityContext = %#v, want fsGroup %d", cluster.Spec.SecurityContext, fsGroup)
	}
}

func assertCatalogObservabilityProjection(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster) {
	t.Helper()

	if cluster.Spec.Observability == nil ||
		cluster.Spec.Observability.Metrics == nil ||
		!cluster.Spec.Observability.Metrics.Enabled ||
		cluster.Spec.Observability.Metrics.ServiceMonitor.Interval != "15s" {
		t.Fatalf("observability = %#v, want metrics ServiceMonitor profile projection", cluster.Spec.Observability)
	}
}

func assertCatalogUsabilityProjection(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster) {
	t.Helper()

	if cluster.Spec.InitContainer == nil || cluster.Spec.InitContainer.Image != "registry.example.com/openbao-init:2.6.0" {
		t.Fatalf("initContainer = %#v, want helper image projection", cluster.Spec.InitContainer)
	}
	if cluster.Spec.Restore == nil || cluster.Spec.Restore.Image != "registry.example.com/openbao-restore:2.6.0" {
		t.Fatalf("restore = %#v, want restore helper image projection", cluster.Spec.Restore)
	}
	if cluster.Spec.Upgrade == nil ||
		cluster.Spec.Upgrade.Strategy != openbaov1alpha1.UpdateStrategyBlueGreen ||
		cluster.Spec.Upgrade.Image != "registry.example.com/openbao-upgrade:2.6.0" ||
		cluster.Spec.Upgrade.BlueGreen == nil ||
		cluster.Spec.Upgrade.BlueGreen.AutoPromote ||
		cluster.Spec.Upgrade.BlueGreen.Verification == nil ||
		cluster.Spec.Upgrade.BlueGreen.Verification.MinSyncDuration != "2m" ||
		cluster.Spec.Upgrade.BlueGreen.MaxJobFailures == nil ||
		*cluster.Spec.Upgrade.BlueGreen.MaxJobFailures != 2 {
		t.Fatalf("upgrade = %#v, want blue/green upgrade policy and helper image", cluster.Spec.Upgrade)
	}
	if cluster.Spec.Network == nil ||
		cluster.Spec.Network.APIServerCIDR != testAPIServerCIDR ||
		cluster.Spec.Network.DNSNamespace != "kube-system" ||
		len(cluster.Spec.Network.DNSEndpointIPs) != 1 ||
		len(cluster.Spec.Network.EgressRules) != 1 ||
		len(cluster.Spec.Network.TrustedIngressPeers) != 1 {
		t.Fatalf("network = %#v, want network profile projection", cluster.Spec.Network)
	}
	if cluster.Spec.ReadReplicas == nil ||
		cluster.Spec.ReadReplicas.Service == nil ||
		!cluster.Spec.ReadReplicas.Service.Enabled ||
		cluster.Spec.ReadReplicas.Service.Annotations["service.beta.kubernetes.io/aws-load-balancer-scheme"] != "internal" ||
		cluster.Spec.ReadReplicas.Template == nil ||
		cluster.Spec.ReadReplicas.Template.Metadata.Labels["openbao.org/read-replica"] != "true" {
		t.Fatalf("readReplicas = %#v, want read-replica service and template projection", cluster.Spec.ReadReplicas)
	}
}

func TestDesiredSameClusterClusterRejectsACMEHAWithoutSharedCache(t *testing.T) {
	t.Parallel()

	rendered := validRenderedGatewayExecutionContractFixture()
	rendered.Cluster.Replicas = 3
	rendered.Exposure.TLSPolicy = &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
		Mode: openbaov1alpha1.OpenBaoExposureTLSModeACME,
		ACME: &openbaov1alpha1.OpenBaoExposureACMEPolicySpec{
			DirectoryURL: "https://acme.example.internal/directory",
		},
	}

	cluster, result := DesiredSameClusterCluster(
		&openbaov1alpha1.OpenBaoClusterClaim{ObjectMeta: metav1.ObjectMeta{Name: "payments-bao"}},
		rendered,
	)
	if result.Valid {
		t.Fatalf("DesiredSameClusterCluster() = %#v, want invalid", result)
	}
	if cluster != nil {
		t.Fatalf("cluster = %#v, want nil", cluster)
	}
}

func TestDesiredSameClusterClusterProjectsGatewayExposure(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
	}

	cluster := mustDesiredSameClusterCluster(t, claim, validRenderedGatewayExecutionContractFixture())
	if cluster.Spec.Gateway == nil {
		t.Fatalf("cluster/spec.gateway = %#v, want projected gateway config", cluster)
	}
	if cluster.Spec.Ingress != nil {
		t.Fatalf("cluster/spec.ingress = %#v, want nil for gateway exposure", cluster.Spec.Ingress)
	}
	if !cluster.Spec.Gateway.Enabled {
		t.Fatal("gateway.enabled = false, want true")
	}
	if cluster.Spec.Gateway.GatewayRef.Name != "shared-gateway" || cluster.Spec.Gateway.GatewayRef.Namespace != "networking" {
		t.Fatalf("gatewayRef = %#v, want rendered gateway object ref", cluster.Spec.Gateway.GatewayRef)
	}
	if cluster.Spec.Gateway.ListenerName != "https" {
		t.Fatalf("listenerName = %q, want https", cluster.Spec.Gateway.ListenerName)
	}
	if cluster.Spec.Gateway.Hostname != "payments-bao.example.internal" {
		t.Fatalf("hostname = %q, want payments-bao.example.internal", cluster.Spec.Gateway.Hostname)
	}
	if cluster.Spec.Gateway.Path != "/vault" {
		t.Fatalf("path = %q, want /vault", cluster.Spec.Gateway.Path)
	}
	if cluster.Spec.Gateway.BackendTLS == nil || cluster.Spec.Gateway.BackendTLS.Enabled == nil || !*cluster.Spec.Gateway.BackendTLS.Enabled {
		t.Fatalf("backendTLS = %#v, want enabled", cluster.Spec.Gateway.BackendTLS)
	}
}

func TestDesiredSameClusterClusterProjectsIngressExposure(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
	}

	cluster := mustDesiredSameClusterCluster(t, claim, validRenderedIngressExecutionContractFixture())
	if cluster.Spec.Ingress == nil {
		t.Fatalf("cluster/spec.ingress = %#v, want projected ingress config", cluster)
	}
	if cluster.Spec.Gateway != nil {
		t.Fatalf("cluster/spec.gateway = %#v, want nil for ingress exposure", cluster.Spec.Gateway)
	}
	if !cluster.Spec.Ingress.Enabled {
		t.Fatal("ingress.enabled = false, want true")
	}
	if cluster.Spec.Ingress.ClassName == nil || *cluster.Spec.Ingress.ClassName != "nginx" {
		t.Fatalf("className = %#v, want nginx", cluster.Spec.Ingress.ClassName)
	}
	if cluster.Spec.Ingress.Host != "payments-bao.example.internal" {
		t.Fatalf("host = %q, want payments-bao.example.internal", cluster.Spec.Ingress.Host)
	}
	if cluster.Spec.Ingress.Path != "/vault" {
		t.Fatalf("path = %q, want /vault", cluster.Spec.Ingress.Path)
	}
	if cluster.Spec.Ingress.PathType != openbaov1alpha1.IngressPathTypePrefix {
		t.Fatalf("pathType = %q, want Prefix", cluster.Spec.Ingress.PathType)
	}
	if cluster.Spec.Ingress.ReadinessMode != openbaov1alpha1.IngressReadinessModeLoadBalancerPublished {
		t.Fatalf("readinessMode = %q, want LoadBalancerPublished", cluster.Spec.Ingress.ReadinessMode)
	}
	if cluster.Spec.Ingress.Annotations["nginx.ingress.kubernetes.io/backend-protocol"] != "HTTPS" {
		t.Fatalf("annotations = %#v, want backend TLS annotation", cluster.Spec.Ingress.Annotations)
	}
}

func TestDesiredSameClusterClusterProjectsBootstrapRefs(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
	}

	for _, tt := range []struct {
		name         string
		rendered     RenderedBootstrap
		wantPath     string
		assertSingle func(t *testing.T, req openbaov1alpha1.SelfInitRequest)
	}{
		{
			name: "auth method config",
			rendered: RenderedBootstrap{
				Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
				Auth: &RenderedBootstrapAuthSpec{
					Methods: []RenderedBootstrapAuthMethodSpec{{
						Type: "kubernetes",
						Path: "kubernetes",
						ConfigFromRef: &openbaov1alpha1.TypedObjectReference{
							Kind: "ConfigMap",
							Name: "claim-bootstrap-authcfg-a1b2c3d4",
						},
					}},
				},
			},
			wantPath: "sys/auth/kubernetes",
			assertSingle: func(t *testing.T, req openbaov1alpha1.SelfInitRequest) {
				t.Helper()
				if got := req.AuthMethod; got == nil || got.ConfigFromRef == nil || got.ConfigFromRef.Name != "claim-bootstrap-authcfg-a1b2c3d4" {
					t.Fatalf("auth request authMethod = %#v, want projected configFromRef", got)
				}
			},
		},
		{
			name: "policy bundle",
			rendered: RenderedBootstrap{
				Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
				Policies: &RenderedBootstrapPoliciesSpec{
					Bundles: []RenderedBootstrapPolicyBundleSpec{{
						Name: "app-readwrite",
						ContentFromRef: openbaov1alpha1.TypedObjectReference{
							Kind: "ConfigMap",
							Name: "claim-bootstrap-policy-a1b2c3d4",
						},
					}},
				},
			},
			wantPath: "sys/policies/acl/app-readwrite",
			assertSingle: func(t *testing.T, req openbaov1alpha1.SelfInitRequest) {
				t.Helper()
				if got := req.Policy; got == nil || got.ContentFromRef == nil || got.ContentFromRef.Name != "claim-bootstrap-policy-a1b2c3d4" {
					t.Fatalf("policy request = %#v, want projected contentFromRef", got)
				}
			},
		},
		{
			name: "audit device",
			rendered: RenderedBootstrap{
				Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
				Audit: &RenderedBootstrapAuditSpec{
					Devices: []RenderedBootstrapAuditDeviceSpec{{
						Type: "file",
						Path: "stdout",
						SinkFromRef: &openbaov1alpha1.TypedObjectReference{
							Kind: "Secret",
							Name: "claim-bootstrap-audit-a1b2c3d4",
						},
					}},
				},
			},
			wantPath: "sys/audit/stdout",
			assertSingle: func(t *testing.T, req openbaov1alpha1.SelfInitRequest) {
				t.Helper()
				if got := req.AuditDevice; got == nil || got.SinkFromRef == nil || got.SinkFromRef.Name != "claim-bootstrap-audit-a1b2c3d4" {
					t.Fatalf("audit request = %#v, want projected sinkFromRef", got)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rendered := &RenderedExecutionContract{
				TargetNamespace: "payments",
				Cluster: RenderedCluster{
					Version:         "2.6.0",
					Replicas:        1,
					SecurityProfile: openbaov1alpha1.ProfileDevelopment,
				},
				Storage:   RenderedStorage{PrimarySize: "20Gi"},
				Bootstrap: tt.rendered,
				Exposure: RenderedExposure{
					PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeClusterInternal,
				},
			}

			cluster := mustDesiredSameClusterCluster(t, claim, rendered)
			if cluster.Spec.SelfInit == nil {
				t.Fatalf("cluster/selfInit = %#v, want projected self-init config", cluster)
			}
			if len(cluster.Spec.SelfInit.Requests) != 1 {
				t.Fatalf("selfInit requests = %#v, want one projected request", cluster.Spec.SelfInit.Requests)
			}
			if cluster.Spec.SelfInit.Requests[0].Path != tt.wantPath {
				t.Fatalf("request path = %q, want %q", cluster.Spec.SelfInit.Requests[0].Path, tt.wantPath)
			}
			tt.assertSingle(t, cluster.Spec.SelfInit.Requests[0])
		})
	}
}

func TestDesiredSameClusterClusterProjectsHardenedTransitUnseal(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
	}

	cluster := mustDesiredSameClusterCluster(t, claim, validRenderedHardenedExecutionContractFixture())
	if cluster.Spec.Profile != openbaov1alpha1.ProfileHardened {
		t.Fatalf("profile = %q, want %q", cluster.Spec.Profile, openbaov1alpha1.ProfileHardened)
	}
	if cluster.Spec.TLS.Mode != openbaov1alpha1.TLSModeExternal {
		t.Fatalf("tls mode = %q, want %q", cluster.Spec.TLS.Mode, openbaov1alpha1.TLSModeExternal)
	}
	if cluster.Spec.Unseal == nil || cluster.Spec.Unseal.Type != "transit" {
		t.Fatalf("unseal = %#v, want transit", cluster.Spec.Unseal)
	}
	if cluster.Spec.Unseal.Transit == nil || cluster.Spec.Unseal.Transit.Address != "https://transit.example.internal:8200" {
		t.Fatalf("transit = %#v, want rendered transit config", cluster.Spec.Unseal.Transit)
	}
	if cluster.Spec.Unseal.Transit.TLSCACert != "/etc/bao/seal-creds/ca.crt" {
		t.Fatalf("transit tlsCACert = %q, want mounted transit CA path", cluster.Spec.Unseal.Transit.TLSCACert)
	}
	if cluster.Spec.Unseal.CredentialsSecretRef == nil || cluster.Spec.Unseal.CredentialsSecretRef.Name != "transit-unseal-creds" {
		t.Fatalf("credentialsSecretRef = %#v, want transit-unseal-creds", cluster.Spec.Unseal.CredentialsSecretRef)
	}
}

func TestDesiredSameClusterClusterProjectsBackupSchedule(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
	}

	rendered := validRenderedBackupExecutionContract(openbaov1alpha1.ProfileDevelopment, 1)
	rendered.Runtime.HelperImages = &openbaov1alpha1.OpenBaoRuntimeProfileHelperImagesSpec{
		Backup: "registry.example.com/openbao-backup:2.6.0",
	}

	cluster, result := DesiredSameClusterCluster(claim, rendered)
	if !result.Valid {
		t.Fatalf("DesiredSameClusterCluster() = %#v, want valid", result)
	}
	if cluster == nil || cluster.Spec.Backup == nil {
		t.Fatalf("cluster/spec.backup = %#v, want projected backup config", cluster)
	}
	if cluster.Spec.Backup.Schedule != "0 3 * * *" {
		t.Fatalf("backup schedule = %q, want 0 3 * * *", cluster.Spec.Backup.Schedule)
	}
	if cluster.Spec.Backup.JWTAuthRole != "openbao-operator-backup" {
		t.Fatalf("backup jwtAuthRole = %q, want openbao-operator-backup", cluster.Spec.Backup.JWTAuthRole)
	}
	if cluster.Spec.Backup.Image != "registry.example.com/openbao-backup:2.6.0" {
		t.Fatalf("backup image = %q, want helper image projection", cluster.Spec.Backup.Image)
	}
	if cluster.Spec.Backup.Target.Provider != "s3" {
		t.Fatalf("backup provider = %q, want s3", cluster.Spec.Backup.Target.Provider)
	}
	if cluster.Spec.Backup.Target.Bucket != "payments-prod" {
		t.Fatalf("backup bucket = %q, want payments-prod", cluster.Spec.Backup.Target.Bucket)
	}
	if cluster.Spec.Backup.Target.PathPrefix != "claims/payments/payments-bao/finance" {
		t.Fatalf("backup pathPrefix = %q, want rendered prefix", cluster.Spec.Backup.Target.PathPrefix)
	}
	if cluster.Spec.Backup.Target.RoleARN != "arn:aws:iam::123456789012:role/openbao-backup" {
		t.Fatalf("backup roleArn = %q, want projected role", cluster.Spec.Backup.Target.RoleARN)
	}
	if cluster.Spec.Backup.Target.WorkloadIdentity == nil || cluster.Spec.Backup.Target.WorkloadIdentity.ServiceAccountAnnotations["eks.amazonaws.com/role-arn"] == "" {
		t.Fatalf("backup workload identity = %#v, want projected annotations", cluster.Spec.Backup.Target.WorkloadIdentity)
	}
	if cluster.Spec.Backup.Target.PartSize != 16777216 || cluster.Spec.Backup.Target.Concurrency != 5 {
		t.Fatalf("backup transfer settings = %#v, want rendered partSize/concurrency", cluster.Spec.Backup.Target)
	}
	if cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) != 1 {
		t.Fatalf("network = %#v, want one rendered egress rule", cluster.Spec.Network)
	}
	if cluster.Spec.Network.EgressRules[0].To[0].IPBlock == nil || cluster.Spec.Network.EgressRules[0].To[0].IPBlock.CIDR != "10.10.0.0/16" {
		t.Fatalf("network egress destination = %#v, want rendered ipBlock", cluster.Spec.Network.EgressRules)
	}
	if len(cluster.Spec.Network.EgressRules[0].Ports) != 1 || cluster.Spec.Network.EgressRules[0].Ports[0].Port == nil || cluster.Spec.Network.EgressRules[0].Ports[0].Port.IntVal != 443 {
		t.Fatalf("network egress ports = %#v, want rendered tcp/443", cluster.Spec.Network.EgressRules)
	}
}

func TestDesiredSameClusterClusterRejectsUnsupportedHardenedProjection(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
	}

	rendered := &RenderedExecutionContract{
		TargetNamespace: "payments",
		Cluster: RenderedCluster{
			Version:         "2.6.0",
			Replicas:        3,
			SecurityProfile: openbaov1alpha1.ProfileHardened,
		},
		Unseal: RenderedUnseal{
			Mode: UnsealPostureModeExternal,
		},
		Bootstrap: RenderedBootstrap{
			Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
			SecretEngines: &openbaov1alpha1.OpenBaoBootstrapSecretEnginesSpec{
				Mounts: []openbaov1alpha1.OpenBaoBootstrapSecretEngineMountSpec{{Type: "kv", Path: "secret"}},
			},
		},
		Exposure: RenderedExposure{
			PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeClusterInternal,
		},
	}

	cluster, result := DesiredSameClusterCluster(claim, rendered)
	if result.Valid {
		t.Fatalf("DesiredSameClusterCluster() = %#v, want invalid", result)
	}
	if cluster != nil {
		t.Fatalf("cluster = %#v, want nil", cluster)
	}
}

func TestDesiredSameClusterClusterProjectsHardenedBackupSchedule(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
	}

	rendered := validRenderedBackupExecutionContract(openbaov1alpha1.ProfileHardened, 3)

	cluster, result := DesiredSameClusterCluster(claim, rendered)
	if !result.Valid {
		t.Fatalf("DesiredSameClusterCluster() = %#v, want valid", result)
	}
	if cluster == nil || cluster.Spec.Backup == nil {
		t.Fatalf("cluster/spec.backup = %#v, want projected hardened backup config", cluster)
	}
	if cluster.Spec.Profile != openbaov1alpha1.ProfileHardened {
		t.Fatalf("profile = %q, want %q", cluster.Spec.Profile, openbaov1alpha1.ProfileHardened)
	}
	if cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) != 1 {
		t.Fatalf("network = %#v, want rendered egress rule", cluster.Spec.Network)
	}
	if cluster.Spec.Backup.Target.Provider != "s3" {
		t.Fatalf("backup provider = %q, want s3", cluster.Spec.Backup.Target.Provider)
	}
}

func TestDesiredSameClusterClusterProjectsExplicitNetworkDefaults(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
	}

	rendered := validRenderedBackupExecutionContract(openbaov1alpha1.ProfileDevelopment, 1)
	ApplySameClusterNetworkDefaults(rendered, SameClusterNetworkDefaults{
		APIServerCIDR:        testAPIServerCIDR,
		APIServerEndpointIPs: []string{"172.29.0.2"},
		DNSEndpointIPs:       []string{"169.254.20.10"},
	})

	cluster, result := DesiredSameClusterCluster(claim, rendered)
	if !result.Valid {
		t.Fatalf("DesiredSameClusterCluster() = %#v, want valid", result)
	}
	if cluster == nil || cluster.Spec.Network == nil {
		t.Fatalf("cluster/spec.network = %#v, want explicit network defaults", cluster)
	}
	if cluster.Spec.Network.APIServerCIDR != testAPIServerCIDR {
		t.Fatalf("apiServerCIDR = %q, want %s", cluster.Spec.Network.APIServerCIDR, testAPIServerCIDR)
	}
	if len(cluster.Spec.Network.APIServerEndpointIPs) != 1 || cluster.Spec.Network.APIServerEndpointIPs[0] != "172.29.0.2" {
		t.Fatalf("apiServerEndpointIPs = %v, want 172.29.0.2", cluster.Spec.Network.APIServerEndpointIPs)
	}
	if len(cluster.Spec.Network.DNSEndpointIPs) != 1 || cluster.Spec.Network.DNSEndpointIPs[0] != "169.254.20.10" {
		t.Fatalf("dnsEndpointIPs = %v, want 169.254.20.10", cluster.Spec.Network.DNSEndpointIPs)
	}
	if len(cluster.Spec.Network.EgressRules) != 1 {
		t.Fatalf("egressRules = %#v, want rendered backup egress rule to remain intact", cluster.Spec.Network.EgressRules)
	}
}

func TestDesiredSameClusterClusterRejectsHardenedBackupProjection(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
	}

	rendered := validRenderedBackupExecutionContract(openbaov1alpha1.ProfileHardened, 1)
	rendered.Network.RequiredEgressRules = nil

	cluster, result := DesiredSameClusterCluster(claim, rendered)
	if result.Valid {
		t.Fatalf("DesiredSameClusterCluster() = %#v, want invalid", result)
	}
	if cluster != nil {
		t.Fatalf("cluster = %#v, want nil", cluster)
	}
}
