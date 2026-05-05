//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
)

var _ = Describe("Claims Functional", Label("claims", "claims-functional"), func() {
	ctx := context.Background()

	It("publishes the external ingress hostname once ingress integration is ready", Label(
		"case:claims-functional-ingress",
		"covers:claim-ingress",
		"covers:claim-external-endpoint-publication",
	), func() {
		if !serviceClaimsE2EEnabled() {
			Skip("claim functional suite requires E2E_ENABLE_SERVICE_CLAIMS=true")
		}

		f, err := framework.NewSetup(ctx, "claims-functional-ingress", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		c := f.Client
		catalog := newSameClusterClaimCatalog(f.Namespace)
		ingressHost := claimScopedName("vault", f.Namespace) + ".claims.example.internal"
		ingressClassName := claimScopedName("ingress-class", f.Namespace)
		entrypointName := claimScopedName("entrypoint", f.Namespace)
		ingressPolicyName := claimScopedName("ingress-policy", f.Namespace)
		networkProfileName := claimScopedName("network", f.Namespace)
		serviceProfile := catalog.serviceProfile()
		attachClaimNetworkProfile(serviceProfile, networkProfileName)

		var claim *openbaov1alpha1.OpenBaoClusterClaim

		DeferCleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			if claim != nil {
				latest := &openbaov1alpha1.OpenBaoClusterClaim{}
				if err := c.Get(cleanupCtx, client.ObjectKeyFromObject(claim), latest); err == nil {
					_ = c.Delete(cleanupCtx, latest)
					_ = waitForClaimDeleted(cleanupCtx, c, latest.Namespace, latest.Name, 2*time.Minute, framework.DefaultPollInterval)
				}
			}

			_ = deleteObjects(
				cleanupCtx,
				c,
				&networkingv1.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: ingressClassName}},
				catalog.serviceOffering(),
				serviceProfile,
				catalog.secretBootstrapProfile(),
				&openbaov1alpha1.OpenBaoExposureClass{ObjectMeta: metav1.ObjectMeta{Name: catalog.ExposureName}},
				&openbaov1alpha1.OpenBaoEntrypoint{ObjectMeta: metav1.ObjectMeta{Name: entrypointName}},
				&openbaov1alpha1.OpenBaoIngressPolicy{ObjectMeta: metav1.ObjectMeta{Name: ingressPolicyName}},
				&openbaov1alpha1.OpenBaoNetworkProfile{ObjectMeta: metav1.ObjectMeta{Name: networkProfileName}},
				catalog.backupProfile(),
			)
			_ = f.Cleanup(cleanupCtx)
		})

		Expect(createObjects(
			ctx,
			c,
			catalog.bootstrapAuthSecret(f.Namespace),
			catalog.backupProfile(),
			catalog.secretBootstrapProfile(),
			serviceProfile,
			catalog.serviceOffering(),
			claimTrustedIngressNetworkProfile(networkProfileName),
			&networkingv1.IngressClass{
				ObjectMeta: metav1.ObjectMeta{Name: ingressClassName},
				Spec: networkingv1.IngressClassSpec{
					Controller: "openbao.org/e2e-ingress",
				},
			},
			&openbaov1alpha1.OpenBaoEntrypoint{
				ObjectMeta: metav1.ObjectMeta{Name: entrypointName},
				Spec: openbaov1alpha1.OpenBaoEntrypointSpec{
					Mode: openbaov1alpha1.OpenBaoEntrypointModeIngress,
					ObjectRef: openbaov1alpha1.OpenBaoEntrypointObjectReference{
						APIGroup: "networking.k8s.io",
						Kind:     "IngressClass",
						Name:     ingressClassName,
					},
				},
			},
			&openbaov1alpha1.OpenBaoIngressPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: ingressPolicyName},
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
			&openbaov1alpha1.OpenBaoExposureClass{
				ObjectMeta: metav1.ObjectMeta{Name: catalog.ExposureName},
				Spec: openbaov1alpha1.OpenBaoExposureClassSpec{
					PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeIngress,
					HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
						Mode:         openbaov1alpha1.OpenBaoExposureHostnamePolicyModeGenerated,
						DomainSuffix: "claims.example.internal",
						Claim: &openbaov1alpha1.OpenBaoExposureClaimHostnamePolicySpec{
							Enabled:         true,
							AllowedSuffixes: []string{"claims.example.internal"},
						},
					},
					TLSPolicy: &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
						Mode: openbaov1alpha1.OpenBaoExposureTLSModeOperatorManaged,
					},
					EntrypointRef:    &openbaov1alpha1.LocalReference{Name: entrypointName},
					IngressPolicyRef: &openbaov1alpha1.LocalReference{Name: ingressPolicyName},
					Routing: &openbaov1alpha1.OpenBaoExposureRoutingSpec{
						Path: "/",
					},
					ServicePolicy: &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
						Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
						BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
					},
				},
			},
		)).To(Succeed())

		claim = catalog.sameClusterClaim(operatorNamespace, claimScopedName("claim", f.Namespace), f.TenantName)
		claim.Spec.ServiceParameters = &openbaov1alpha1.OpenBaoClusterClaimServiceParametersSpec{
			Exposure: &openbaov1alpha1.OpenBaoClusterClaimExposureServiceParametersSpec{
				Hostname: ingressHost,
			},
		}
		Expect(c.Create(ctx, claim)).To(Succeed())

		localRef, err := waitForClaimLocalClusterRef(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			3*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())

		ingress := &networkingv1.Ingress{}
		Eventually(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Namespace: localRef.Namespace, Name: localRef.Name}, ingress)).To(Succeed())
		}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("publishing a load balancer address on the managed Ingress")
		original := ingress.DeepCopy()
		ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{
			Hostname: "lb.claims.example.internal",
		}}
		Expect(c.Status().Patch(ctx, ingress, client.MergeFrom(original))).To(Succeed())

		expectedEndpoint := "https://" + ingressHost
		updated, err := waitForClaimEndpoint(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			expectedEndpoint,
			8*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Connection.SecretRef).NotTo(BeNil())
	})

	It("publishes the external gateway hostname once gateway integration is ready", Label(
		"case:claims-functional-gateway",
		"covers:claim-gateway",
		"covers:claim-external-endpoint-publication",
		"requires-gateway-api",
	), func() {
		if !serviceClaimsE2EEnabled() {
			Skip("claim functional suite requires E2E_ENABLE_SERVICE_CLAIMS=true")
		}

		f, err := framework.NewSetup(ctx, "claims-functional-gateway", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		c := f.Client
		catalog := newSameClusterClaimCatalog(f.Namespace)
		gatewayClassName := claimScopedName("gateway-class", f.Namespace)
		gatewayName := claimScopedName("gateway", f.Namespace)
		entrypointName := claimScopedName("entrypoint", f.Namespace)
		gatewayHost := claimScopedName("vault", f.Namespace) + ".gateway.example.internal"
		networkProfileName := claimScopedName("network", f.Namespace)
		serviceProfile := catalog.serviceProfile()
		attachClaimNetworkProfile(serviceProfile, networkProfileName)

		Expect(f.InstallGatewayAPI()).To(Succeed())

		var claim *openbaov1alpha1.OpenBaoClusterClaim

		DeferCleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			if claim != nil {
				latest := &openbaov1alpha1.OpenBaoClusterClaim{}
				if err := c.Get(cleanupCtx, client.ObjectKeyFromObject(claim), latest); err == nil {
					_ = c.Delete(cleanupCtx, latest)
					_ = waitForClaimDeleted(cleanupCtx, c, latest.Namespace, latest.Name, 2*time.Minute, framework.DefaultPollInterval)
				}
			}

			_ = deleteObjects(
				cleanupCtx,
				c,
				&gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: gatewayName, Namespace: f.Namespace}},
				&gatewayv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: gatewayClassName}},
				catalog.serviceOffering(),
				serviceProfile,
				catalog.secretBootstrapProfile(),
				&openbaov1alpha1.OpenBaoExposureClass{ObjectMeta: metav1.ObjectMeta{Name: catalog.ExposureName}},
				&openbaov1alpha1.OpenBaoEntrypoint{ObjectMeta: metav1.ObjectMeta{Name: entrypointName}},
				&openbaov1alpha1.OpenBaoNetworkProfile{ObjectMeta: metav1.ObjectMeta{Name: networkProfileName}},
				catalog.backupProfile(),
			)
			_ = f.Cleanup(cleanupCtx)
		})

		ensureGatewayClassReady(ctx, c, gatewayClassName, gatewayv1.FeatureName("HTTPRoute"), gatewayv1.FeatureName("BackendTLSPolicy"))

		Expect(createObjects(
			ctx,
			c,
			catalog.bootstrapAuthSecret(f.Namespace),
			catalog.backupProfile(),
			catalog.secretBootstrapProfile(),
			serviceProfile,
			catalog.serviceOffering(),
			claimTrustedIngressNetworkProfile(networkProfileName),
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayName,
					Namespace: f.Namespace,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: gatewayv1.ObjectName(gatewayClassName),
					Listeners: []gatewayv1.Listener{{
						Name:     gatewayv1.SectionName("https"),
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     gatewayv1.PortNumber(443),
						Hostname: (*gatewayv1.Hostname)(&gatewayHost),
					}},
				},
			},
			&openbaov1alpha1.OpenBaoEntrypoint{
				ObjectMeta: metav1.ObjectMeta{Name: entrypointName},
				Spec: openbaov1alpha1.OpenBaoEntrypointSpec{
					Mode: openbaov1alpha1.OpenBaoEntrypointModeGateway,
					ObjectRef: openbaov1alpha1.OpenBaoEntrypointObjectReference{
						APIGroup:  "gateway.networking.k8s.io",
						Kind:      "Gateway",
						Name:      gatewayName,
						Namespace: f.Namespace,
					},
					ListenerPolicy: &openbaov1alpha1.OpenBaoEntrypointListenerPolicySpec{
						SectionName: "https",
					},
				},
			},
			&openbaov1alpha1.OpenBaoExposureClass{
				ObjectMeta: metav1.ObjectMeta{Name: catalog.ExposureName},
				Spec: openbaov1alpha1.OpenBaoExposureClassSpec{
					PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeGateway,
					HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
						Mode:  openbaov1alpha1.OpenBaoExposureHostnamePolicyModeFixed,
						Value: gatewayHost,
					},
					TLSPolicy: &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
						Mode: openbaov1alpha1.OpenBaoExposureTLSModeOperatorManaged,
					},
					EntrypointRef: &openbaov1alpha1.LocalReference{Name: entrypointName},
					Routing: &openbaov1alpha1.OpenBaoExposureRoutingSpec{
						Path: "/",
					},
					ServicePolicy: &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
						Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
						BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
					},
				},
			},
		)).To(Succeed())

		claim = catalog.sameClusterClaim(operatorNamespace, claimScopedName("claim", f.Namespace), f.TenantName)
		Expect(c.Create(ctx, claim)).To(Succeed())

		_, err = waitForClaimLocalClusterRef(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			3*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())

		By("marking the referenced Gateway as programmed")
		markGatewayProgrammed(ctx, c, f.Namespace, gatewayName)

		expectedEndpoint := "https://" + gatewayHost
		updated, err := waitForClaimEndpoint(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			expectedEndpoint,
			8*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Connection.SecretRef).NotTo(BeNil())
	})

	It("projects production catalog profiles into the claim-managed cluster", Label(
		"case:claims-functional-catalog-profiles",
		"covers:claim-catalog-unseal-profile",
		"covers:claim-catalog-runtime-profile",
		"covers:claim-catalog-observability-profile",
		"covers:claim-catalog-storage-profile",
		"covers:claim-catalog-network-profile",
		"covers:claim-catalog-upgrade-policy",
		"covers:claim-catalog-read-replica-profile",
	), func() {
		if !serviceClaimsE2EEnabled() {
			Skip("claim functional suite requires E2E_ENABLE_SERVICE_CLAIMS=true")
		}

		f, err := framework.NewSetup(ctx, "claims-functional-catalog", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		c := f.Client
		catalog := newSameClusterClaimCatalog(f.Namespace)
		storageProfileName := claimScopedName("storage", f.Namespace)
		unsealProfileName := claimScopedName("unseal", f.Namespace)
		runtimeProfileName := claimScopedName("runtime", f.Namespace)
		observabilityProfileName := claimScopedName("observability", f.Namespace)
		networkProfileName := claimScopedName("network", f.Namespace)
		upgradePolicyName := claimScopedName("upgrade-policy", f.Namespace)
		primaryStorageClass := claimScopedName("primary-storage", f.Namespace)

		serviceProfile := catalog.serviceProfile()
		readReplicas := int32(1)
		serviceProfile.Spec.Cluster.ReadReplicas = &readReplicas
		serviceProfile.Spec.Storage.ReadReplicaSize = "5Gi"
		serviceProfile.Spec.Storage.ProfileRef = &openbaov1alpha1.LocalReference{Name: storageProfileName}
		serviceProfile.Spec.Unseal = &openbaov1alpha1.OpenBaoServiceProfileUnsealSpec{
			ProfileRef: &openbaov1alpha1.LocalReference{Name: unsealProfileName},
		}
		serviceProfile.Spec.Runtime = &openbaov1alpha1.OpenBaoServiceProfileRuntimeSpec{
			ProfileRef: &openbaov1alpha1.LocalReference{Name: runtimeProfileName},
		}
		serviceProfile.Spec.Observability = &openbaov1alpha1.OpenBaoServiceProfileObservabilitySpec{
			ProfileRef: &openbaov1alpha1.LocalReference{Name: observabilityProfileName},
		}
		serviceProfile.Spec.Network = &openbaov1alpha1.OpenBaoServiceProfileNetworkSpec{
			ProfileRef: &openbaov1alpha1.LocalReference{Name: networkProfileName},
		}
		serviceProfile.Spec.Lifecycle = openbaov1alpha1.OpenBaoServiceProfileLifecycleSpec{
			PolicyRef:       &openbaov1alpha1.LocalReference{Name: upgradePolicyName},
			UpgradeStrategy: openbaov1alpha1.UpdateStrategyBlueGreen,
		}
		exposureClass := catalog.internalExposureClass()
		exposureClass.Spec.ReadReplicaServicePolicy = &openbaov1alpha1.OpenBaoExposureReadReplicaServicePolicySpec{
			Enabled: true,
			Type:    openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
			Annotations: map[string]string{
				"openbao.org/e2e-read-service": "true",
			},
		}
		maxJobFailures := int32(2)

		var claim *openbaov1alpha1.OpenBaoClusterClaim

		DeferCleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			if claim != nil {
				latest := &openbaov1alpha1.OpenBaoClusterClaim{}
				if err := c.Get(cleanupCtx, client.ObjectKeyFromObject(claim), latest); err == nil {
					_ = c.Delete(cleanupCtx, latest)
					_ = waitForClaimDeleted(cleanupCtx, c, latest.Namespace, latest.Name, 2*time.Minute, framework.DefaultPollInterval)
				}
			}

			_ = deleteObjects(
				cleanupCtx,
				c,
				catalog.serviceOffering(),
				serviceProfile,
				catalog.secretBootstrapProfile(),
				exposureClass,
				catalog.backupProfile(),
				&openbaov1alpha1.OpenBaoStorageProfile{ObjectMeta: metav1.ObjectMeta{Name: storageProfileName}},
				&openbaov1alpha1.OpenBaoUnsealProfile{ObjectMeta: metav1.ObjectMeta{Name: unsealProfileName}},
				&openbaov1alpha1.OpenBaoRuntimeProfile{ObjectMeta: metav1.ObjectMeta{Name: runtimeProfileName}},
				&openbaov1alpha1.OpenBaoObservabilityProfile{ObjectMeta: metav1.ObjectMeta{Name: observabilityProfileName}},
				&openbaov1alpha1.OpenBaoNetworkProfile{ObjectMeta: metav1.ObjectMeta{Name: networkProfileName}},
				&openbaov1alpha1.OpenBaoUpgradePolicy{ObjectMeta: metav1.ObjectMeta{Name: upgradePolicyName}},
			)
			_ = f.Cleanup(cleanupCtx)
		})

		Expect(createObjects(
			ctx,
			c,
			catalog.bootstrapAuthSecret(f.Namespace),
			catalog.backupProfile(),
			catalog.secretBootstrapProfile(),
			exposureClass,
			&openbaov1alpha1.OpenBaoStorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: storageProfileName},
				Spec: openbaov1alpha1.OpenBaoStorageProfileSpec{
					Primary: &openbaov1alpha1.OpenBaoStorageProfileVolumeSpec{
						StorageClassName: ptr.To(primaryStorageClass),
					},
					ReadReplica: &openbaov1alpha1.OpenBaoStorageProfileReadReplicaSpec{
						UsePrimaryStorageClass: ptr.To(true),
					},
				},
			},
			&openbaov1alpha1.OpenBaoUnsealProfile{
				ObjectMeta: metav1.ObjectMeta{Name: unsealProfileName},
				Spec: openbaov1alpha1.OpenBaoUnsealProfileSpec{
					Mode: openbaov1alpha1.OpenBaoUnsealProfileModeOperatorManagedStatic,
					Static: &openbaov1alpha1.StaticSealConfig{
						CurrentKey:   "file:///etc/bao/unseal/key",
						CurrentKeyID: "e2e-static",
					},
				},
			},
			&openbaov1alpha1.OpenBaoRuntimeProfile{
				ObjectMeta: metav1.ObjectMeta{Name: runtimeProfileName},
				Spec: openbaov1alpha1.OpenBaoRuntimeProfileSpec{
					ServiceAccount: &openbaov1alpha1.ServiceAccountConfig{
						Name: claimScopedName("workload-sa", f.Namespace),
						Annotations: map[string]string{
							"openbao.org/e2e-runtime-profile": "true",
						},
					},
					PodMetadata: &openbaov1alpha1.PodMetadataConfig{
						Labels: map[string]string{
							"openbao.org/e2e-runtime-profile": "true",
						},
					},
					HelperImages: &openbaov1alpha1.OpenBaoRuntimeProfileHelperImagesSpec{
						Init:    configInitImage,
						Restore: "example.com/openbao-restore:e2e",
						Upgrade: "example.com/openbao-upgrade:e2e",
					},
					ReadReplica: &openbaov1alpha1.OpenBaoRuntimeProfileReadReplicaSpec{
						Template: &openbaov1alpha1.ReadReplicaTemplateConfig{
							Metadata: &openbaov1alpha1.PodMetadataConfig{
								Labels: map[string]string{"openbao.org/e2e-read-replica": "true"},
							},
						},
					},
				},
			},
			&openbaov1alpha1.OpenBaoObservabilityProfile{
				ObjectMeta: metav1.ObjectMeta{Name: observabilityProfileName},
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
			},
			&openbaov1alpha1.OpenBaoNetworkProfile{
				ObjectMeta: metav1.ObjectMeta{Name: networkProfileName},
				Spec: openbaov1alpha1.OpenBaoNetworkProfileSpec{
					DNSNamespace: "kube-system",
				},
			},
			&openbaov1alpha1.OpenBaoUpgradePolicy{
				ObjectMeta: metav1.ObjectMeta{Name: upgradePolicyName},
				Spec: openbaov1alpha1.OpenBaoUpgradePolicySpec{
					BlueGreen: &openbaov1alpha1.OpenBaoUpgradePolicyBlueGreenSpec{
						MinSyncDuration: "1s",
						MaxJobFailures:  &maxJobFailures,
					},
				},
			},
			serviceProfile,
			catalog.serviceOffering(),
		)).To(Succeed())

		claim = catalog.sameClusterClaim(operatorNamespace, claimScopedName("claim", f.Namespace), f.TenantName)
		Expect(c.Create(ctx, claim)).To(Succeed())

		localRef, err := waitForClaimLocalClusterRef(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			3*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())

		cluster := &openbaov1alpha1.OpenBaoCluster{}
		Eventually(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Namespace: localRef.Namespace, Name: localRef.Name}, cluster)).To(Succeed())
			g.Expect(cluster.Spec.ReadReplicas).NotTo(BeNil())
			g.Expect(cluster.Spec.ReadReplicas.Service).NotTo(BeNil())
			g.Expect(cluster.Spec.ReadReplicas.Template).NotTo(BeNil())
			g.Expect(cluster.Spec.Unseal).NotTo(BeNil())
			g.Expect(cluster.Spec.ServiceAccount).NotTo(BeNil())
			g.Expect(cluster.Spec.PodMetadata).NotTo(BeNil())
			g.Expect(cluster.Spec.Observability).NotTo(BeNil())
			g.Expect(cluster.Spec.Network).NotTo(BeNil())
			g.Expect(cluster.Spec.Upgrade).NotTo(BeNil())
			g.Expect(cluster.Spec.Restore).NotTo(BeNil())
		}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		Expect(cluster.Spec.Storage.StorageClassName).NotTo(BeNil())
		Expect(*cluster.Spec.Storage.StorageClassName).To(Equal(primaryStorageClass))
		Expect(cluster.Spec.ReadReplicas.Replicas).To(Equal(int32(1)))
		Expect(cluster.Spec.ReadReplicas.Service.Enabled).To(BeTrue())
		Expect(cluster.Spec.ReadReplicas.Service.Annotations).To(HaveKeyWithValue("openbao.org/e2e-read-service", "true"))
		Expect(cluster.Spec.ReadReplicas.Template.Metadata.Labels).To(
			HaveKeyWithValue("openbao.org/e2e-read-replica", "true"),
		)
		Expect(cluster.Spec.Unseal.Type).To(Equal("static"))
		Expect(cluster.Spec.Unseal.Static).NotTo(BeNil())
		Expect(cluster.Spec.Unseal.Static.CurrentKeyID).To(Equal("e2e-static"))
		Expect(cluster.Spec.ServiceAccount.Name).To(Equal(claimScopedName("workload-sa", f.Namespace)))
		Expect(cluster.Spec.ServiceAccount.Annotations).To(HaveKeyWithValue("openbao.org/e2e-runtime-profile", "true"))
		Expect(cluster.Spec.PodMetadata.Labels).To(HaveKeyWithValue("openbao.org/e2e-runtime-profile", "true"))
		Expect(cluster.Spec.Observability.Metrics).NotTo(BeNil())
		Expect(cluster.Spec.Observability.Metrics.Enabled).To(BeTrue())
		Expect(cluster.Spec.Observability.Metrics.ServiceMonitor.Interval).To(Equal("15s"))
		Expect(cluster.Spec.Network.DNSNamespace).To(Equal("kube-system"))
		Expect(cluster.Spec.InitContainer).NotTo(BeNil())
		Expect(cluster.Spec.InitContainer.Image).To(Equal(configInitImage))
		Expect(cluster.Spec.Upgrade.Strategy).To(Equal(openbaov1alpha1.UpdateStrategyBlueGreen))
		Expect(cluster.Spec.Upgrade.Image).To(Equal("example.com/openbao-upgrade:e2e"))
		Expect(cluster.Spec.Upgrade.BlueGreen).NotTo(BeNil())
		Expect(cluster.Spec.Upgrade.BlueGreen.Verification.MinSyncDuration).To(Equal("1s"))
		Expect(*cluster.Spec.Upgrade.BlueGreen.MaxJobFailures).To(Equal(int32(2)))
		Expect(cluster.Spec.Restore.Image).To(Equal("example.com/openbao-restore:e2e"))

		updated, err := waitForClaim(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			framework.DefaultLongWaitTimeout,
			framework.DefaultPollInterval,
			func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
				return claim.Status.Applied.StorageProfileRef != nil &&
					claim.Status.Applied.UnsealProfileRef != nil &&
					claim.Status.Applied.RuntimeProfileRef != nil &&
					claim.Status.Applied.ObservabilityProfileRef != nil &&
					claim.Status.Applied.NetworkProfileRef != nil &&
					claim.Status.Applied.UpgradePolicyRef != nil, nil
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Applied.UnsealProfileRef.Name).To(Equal(unsealProfileName))
		Expect(updated.Status.Applied.ObservabilityProfileRef.Name).To(Equal(observabilityProfileName))
		Expect(updated.Status.Applied.NetworkProfileRef.Name).To(Equal(networkProfileName))
		Expect(updated.Status.Applied.UpgradePolicyRef.Name).To(Equal(upgradePolicyName))
	})

	It("executes an in-place claim upgrade request against a new service-profile revision", Label(
		"case:claims-functional-upgrade-request",
		"covers:claim-upgrade-request",
		"covers:claim-upgrade-rollout",
		"covers:claim-upgrade-version-rollout",
		"covers:claim-upgrade-blocked-incompatible",
		"claims-upgrade",
	), func() {
		if !serviceClaimsE2EEnabled() {
			Skip("claim functional suite requires E2E_ENABLE_SERVICE_CLAIMS=true")
		}
		initialVersion := envOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
		targetVersion := envOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)
		if initialVersion == targetVersion {
			Skip(fmt.Sprintf("claim upgrade functional case requires different versions: from=%s to=%s", initialVersion, targetVersion))
		}
		targetImage := fmt.Sprintf("%s:%s", envOrDefault("RELATED_IMAGE_OPENBAO", "openbao/openbao"), targetVersion)

		f, err := framework.NewSetup(ctx, "claims-functional-upgrade", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		c := f.Client
		catalog := newSameClusterClaimCatalog(f.Namespace)
		backupBackend := catalog.backupBackend()
		backupTarget := catalog.backupTarget()
		backupProfileV1 := catalog.backupProfile()
		backupProfileV1.Spec.Schedule = "0 3 * * *"
		backupProfileV1.Spec.TargetRef = &openbaov1alpha1.LocalReference{Name: backupTarget.Name}
		backupProfileV2 := catalog.backupProfile()
		backupProfileV2.Name = claimScopedName("backup-v2", f.Namespace)
		backupProfileV2.Spec.Schedule = "15 4 * * *"
		backupProfileV2.Spec.TargetRef = &openbaov1alpha1.LocalReference{Name: backupTarget.Name}

		serviceProfileV1 := catalog.serviceProfile()
		serviceProfileV1.Spec.Cluster.Version = initialVersion
		serviceProfileV2 := catalog.serviceProfile()
		serviceProfileV2.Name = claimScopedName("service-v2", f.Namespace)
		serviceProfileV2.Spec.Cluster.Version = targetVersion
		serviceProfileV2.Spec.Backup.ProfileRef.Name = backupProfileV2.Name
		blockedServiceProfile := catalog.serviceProfile()
		blockedServiceProfile.Name = claimScopedName("service-blocked", f.Namespace)
		blockedServiceProfile.Spec.Cluster.Version = targetVersion
		blockedServiceProfile.Spec.Cluster.Voters = 3

		serviceOffering := catalog.serviceOffering()
		claim := catalog.sameClusterClaim(operatorNamespace, claimScopedName("claim", f.Namespace), f.TenantName)
		upgradeRequestName := claimScopedName("upgrade", f.Namespace)
		blockedUpgradeRequestName := claimScopedName("upgrade-blocked", f.Namespace)

		DeferCleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			_ = deleteObjects(
				cleanupCtx,
				c,
				&openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: claim.Namespace,
						Name:      upgradeRequestName,
					},
				},
				&openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: claim.Namespace,
						Name:      blockedUpgradeRequestName,
					},
				},
			)
			latest := &openbaov1alpha1.OpenBaoClusterClaim{}
			if err := c.Get(cleanupCtx, client.ObjectKeyFromObject(claim), latest); err == nil {
				_ = c.Delete(cleanupCtx, latest)
				_ = waitForClaimDeleted(cleanupCtx, c, latest.Namespace, latest.Name, 2*time.Minute, framework.DefaultPollInterval)
			}

			_ = deleteObjects(
				cleanupCtx,
				c,
				serviceOffering,
				serviceProfileV1,
				serviceProfileV2,
				blockedServiceProfile,
				catalog.secretBootstrapProfile(),
				catalog.internalExposureClass(),
				backupBackend,
				backupTarget,
				backupProfileV1,
				backupProfileV2,
			)
			_ = f.Cleanup(cleanupCtx)
		})

		Expect(createObjects(
			ctx,
			c,
			catalog.bootstrapAuthSecret(f.Namespace),
			backupBackend,
			backupTarget,
			backupProfileV1,
			backupProfileV2,
			catalog.internalExposureClass(),
			catalog.secretBootstrapProfile(),
			serviceProfileV1,
			serviceProfileV2,
			blockedServiceProfile,
			serviceOffering,
		)).To(Succeed())
		Expect(c.Create(ctx, claim)).To(Succeed())

		_, err = waitForClaimPinnedBinding(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			catalog.OfferingName,
			serviceProfileV1.Name,
			6*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		updated, err := waitForClaimPhase(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			openbaov1alpha1.OpenBaoClusterClaimPhaseReady,
			8*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Connection.SecretRef).NotTo(BeNil())

		localRef, err := waitForClaimLocalClusterRef(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			3*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(f.WaitForClusterPhase(
			ctx,
			localRef.Name,
			openbaov1alpha1.ClusterPhaseRunning,
			8*time.Minute,
			framework.DefaultPollInterval,
		)).To(Succeed())

		By("blocking an incompatible service-shape upgrade request before mutating the claim")
		Expect(c.Get(ctx, client.ObjectKeyFromObject(serviceOffering), serviceOffering)).To(Succeed())
		originalOffering := serviceOffering.DeepCopy()
		serviceOffering.Spec.CurrentRevisionRef.Name = blockedServiceProfile.Name
		Expect(c.Patch(ctx, serviceOffering, client.MergeFrom(originalOffering))).To(Succeed())

		blockedUpgradeRequest := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: claim.Namespace,
				Name:      blockedUpgradeRequestName,
			},
			Spec: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestSpec{
				ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
				Target: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
					ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: catalog.OfferingName},
				},
			},
		}
		Expect(c.Create(ctx, blockedUpgradeRequest)).To(Succeed())
		blockedResult, err := waitForClaimUpgradeRequestState(
			ctx,
			c,
			blockedUpgradeRequest.Namespace,
			blockedUpgradeRequest.Name,
			openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked,
			5*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(blockedResult.Status.Classification).NotTo(BeNil())
		Expect(blockedResult.Status.Classification.Class).To(Equal(
			openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked,
		))
		Expect(blockedResult.Status.Classification.Reason).To(Equal("UnsupportedServiceShapeChange"))
		_, err = waitForClaimPinnedBinding(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			catalog.OfferingName,
			serviceProfileV1.Name,
			3*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())

		By("publishing the next immutable service-profile revision on the same offering alias")
		Expect(c.Get(ctx, client.ObjectKeyFromObject(serviceOffering), serviceOffering)).To(Succeed())
		originalOffering = serviceOffering.DeepCopy()
		serviceOffering.Spec.CurrentRevisionRef.Name = serviceProfileV2.Name
		Expect(c.Patch(ctx, serviceOffering, client.MergeFrom(originalOffering))).To(Succeed())

		upgradeRequest := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: claim.Namespace,
				Name:      upgradeRequestName,
			},
			Spec: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestSpec{
				ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
				Target: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
					ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: catalog.OfferingName},
				},
			},
		}
		Expect(c.Create(ctx, upgradeRequest)).To(Succeed())

		By("waiting for the request to enter rollout")
		request, err := waitForClaimUpgradeRequest(
			ctx,
			c,
			upgradeRequest.Namespace,
			upgradeRequest.Name,
			5*time.Minute,
			framework.DefaultPollInterval,
			func(request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) (bool, error) {
				return request.Status.State == openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut ||
					request.Status.State == openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded, nil
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(request.Status.Target).NotTo(BeNil())
		Expect(request.Status.Target.ServiceProfileRef).NotTo(BeNil())
		Expect(request.Status.Target.ServiceProfileRef.Name).To(Equal(serviceProfileV2.Name))

		if request.Status.State == openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut {
			By("waiting for the claim to publish an active maintenance summary")
			updated, err = waitForClaim(
				ctx,
				c,
				claim.Namespace,
				claim.Name,
				5*time.Minute,
				framework.DefaultPollInterval,
				func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
					return claim.Status.Phase == openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded &&
						claim.Status.Summary != nil &&
						claim.Status.Summary.Reason == string(openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut) &&
						claim.Status.Summary.SourceRef != nil &&
						claim.Status.Summary.SourceRef.Kind == "OpenBaoClusterClaimUpgradeRequest" &&
						claim.Status.Summary.SourceRef.Name == upgradeRequest.Name, nil
				},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.Status.Summary).NotTo(BeNil())
			Expect(updated.Status.Summary.Severity).To(Equal(openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo))
		}

		By("waiting for the claim to converge onto the upgraded revision")
		updated, err = waitForClaimPinnedBinding(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			catalog.OfferingName,
			serviceProfileV2.Name,
			8*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Applied.ServiceProfileRef).NotTo(BeNil())
		Expect(updated.Status.Applied.ServiceProfileRef.Name).To(Equal(serviceProfileV2.Name))

		By("waiting for the upgrade request to complete and the claim workflow summary to clear")
		request, err = waitForClaimUpgradeRequestState(
			ctx,
			c,
			upgradeRequest.Namespace,
			upgradeRequest.Name,
			openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded,
			8*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(request.Status.Classification).NotTo(BeNil())
		Expect(request.Status.Classification.Class).To(Equal(
			openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace,
		))

		updated, err = waitForClaimUpgradeCleared(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			5*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Phase).To(Equal(openbaov1alpha1.OpenBaoClusterClaimPhaseReady))
		Expect(updated.Status.Summary).To(BeNil())

		By("projecting the upgraded backup schedule onto the local OpenBaoCluster")
		Eventually(func(g Gomega) {
			localCluster := &openbaov1alpha1.OpenBaoCluster{}
			key := types.NamespacedName{Namespace: localRef.Namespace, Name: localRef.Name}
			g.Expect(c.Get(ctx, key, localCluster)).To(Succeed())
			g.Expect(localCluster.Spec.Version).To(Equal(targetVersion))
			g.Expect(localCluster.Spec.Image).To(Equal(targetImage))
			g.Expect(localCluster.Status.CurrentVersion).To(Equal(targetVersion))
			g.Expect(localCluster.Spec.Backup).NotTo(BeNil())
			g.Expect(localCluster.Spec.Backup.Schedule).To(Equal("15 4 * * *"))
		}, 5*time.Minute, framework.DefaultPollInterval).Should(Succeed())
	})

	It("executes a Blue/Green claim upgrade request against a new service-profile revision", Label(
		"case:claims-functional-bluegreen-upgrade-request",
		"covers:claim-bluegreen-upgrade",
		"covers:claim-upgrade-bluegreen",
		"claims-bluegreen",
		"claims-upgrade",
	), func() {
		if !serviceClaimsE2EEnabled() {
			Skip("claim functional suite requires E2E_ENABLE_SERVICE_CLAIMS=true")
		}
		initialVersion := envOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
		targetVersion := envOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)
		if initialVersion == targetVersion {
			Skip(fmt.Sprintf("claim Blue/Green upgrade functional case requires different versions: from=%s to=%s", initialVersion, targetVersion))
		}
		targetImage := fmt.Sprintf("%s:%s", envOrDefault("RELATED_IMAGE_OPENBAO", "openbao/openbao"), targetVersion)

		f, err := framework.NewSetup(ctx, "claims-functional-bluegreen", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		c := f.Client
		catalog := newSameClusterClaimCatalog(f.Namespace)
		upgradePolicyName := claimScopedName("bluegreen-policy", f.Namespace)
		autoPromote := true
		maxJobFailures := int32(2)

		backupProfile := catalog.backupProfile()
		serviceProfileV1 := catalog.serviceProfile()
		serviceProfileV1.Spec.Cluster.Version = initialVersion
		serviceProfileV1.Spec.Cluster.Voters = 3
		serviceProfileV1.Spec.Lifecycle.UpgradeStrategy = openbaov1alpha1.UpdateStrategyBlueGreen
		serviceProfileV1.Spec.Lifecycle.PolicyRef = &openbaov1alpha1.LocalReference{Name: upgradePolicyName}
		serviceProfileV2 := catalog.serviceProfile()
		serviceProfileV2.Name = claimScopedName("service-bluegreen-v2", f.Namespace)
		serviceProfileV2.Spec.Cluster.Version = targetVersion
		serviceProfileV2.Spec.Cluster.Voters = 3
		serviceProfileV2.Spec.Lifecycle.UpgradeStrategy = openbaov1alpha1.UpdateStrategyBlueGreen
		serviceProfileV2.Spec.Lifecycle.PolicyRef = &openbaov1alpha1.LocalReference{Name: upgradePolicyName}
		upgradePolicy := &openbaov1alpha1.OpenBaoUpgradePolicy{
			ObjectMeta: metav1.ObjectMeta{Name: upgradePolicyName},
			Spec: openbaov1alpha1.OpenBaoUpgradePolicySpec{
				BlueGreen: &openbaov1alpha1.OpenBaoUpgradePolicyBlueGreenSpec{
					AutoPromote:     &autoPromote,
					MinSyncDuration: "1s",
					MaxJobFailures:  &maxJobFailures,
				},
			},
		}
		serviceOffering := catalog.serviceOffering()
		claim := catalog.sameClusterClaim(operatorNamespace, claimScopedName("claim", f.Namespace), f.TenantName)
		upgradeRequestName := claimScopedName("bluegreen-upgrade", f.Namespace)

		DeferCleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()

			_ = deleteObjects(
				cleanupCtx,
				c,
				&openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: claim.Namespace,
						Name:      upgradeRequestName,
					},
				},
			)
			latest := &openbaov1alpha1.OpenBaoClusterClaim{}
			if err := c.Get(cleanupCtx, client.ObjectKeyFromObject(claim), latest); err == nil {
				_ = c.Delete(cleanupCtx, latest)
				_ = waitForClaimDeleted(cleanupCtx, c, latest.Namespace, latest.Name, 3*time.Minute, framework.DefaultPollInterval)
			}

			_ = deleteObjects(
				cleanupCtx,
				c,
				serviceOffering,
				serviceProfileV1,
				serviceProfileV2,
				upgradePolicy,
				catalog.secretBootstrapProfile(),
				catalog.internalExposureClass(),
				backupProfile,
			)
			_ = f.Cleanup(cleanupCtx)
		})

		Expect(createObjects(
			ctx,
			c,
			catalog.bootstrapAuthSecret(f.Namespace),
			backupProfile,
			catalog.internalExposureClass(),
			catalog.secretBootstrapProfile(),
			upgradePolicy,
			serviceProfileV1,
			serviceProfileV2,
			serviceOffering,
		)).To(Succeed())
		Expect(c.Create(ctx, claim)).To(Succeed())

		_, err = waitForClaimPinnedBinding(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			catalog.OfferingName,
			serviceProfileV1.Name,
			6*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		_, err = waitForClaimPhase(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			openbaov1alpha1.OpenBaoClusterClaimPhaseReady,
			10*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		localRef, err := waitForClaimLocalClusterRef(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			3*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(f.WaitForClusterPhase(
			ctx,
			localRef.Name,
			openbaov1alpha1.ClusterPhaseRunning,
			10*time.Minute,
			framework.DefaultPollInterval,
		)).To(Succeed())
		Eventually(func(g Gomega) {
			localCluster := &openbaov1alpha1.OpenBaoCluster{}
			key := types.NamespacedName{Namespace: localRef.Namespace, Name: localRef.Name}
			g.Expect(c.Get(ctx, key, localCluster)).To(Succeed())
			g.Expect(localCluster.Spec.Upgrade).NotTo(BeNil())
			g.Expect(localCluster.Spec.Upgrade.Strategy).To(Equal(openbaov1alpha1.UpdateStrategyBlueGreen))
			g.Expect(localCluster.Spec.Upgrade.BlueGreen).NotTo(BeNil())
			g.Expect(localCluster.Spec.Upgrade.BlueGreen.Verification).NotTo(BeNil())
			g.Expect(localCluster.Spec.Upgrade.BlueGreen.Verification.MinSyncDuration).To(Equal("1s"))
			g.Expect(localCluster.Status.BlueGreen).NotTo(BeNil())
			g.Expect(localCluster.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle))
		}, 8*time.Minute, framework.DefaultPollInterval).Should(Succeed())

		By("publishing the Blue/Green target revision and requesting the claim upgrade")
		Expect(c.Get(ctx, client.ObjectKeyFromObject(serviceOffering), serviceOffering)).To(Succeed())
		originalOffering := serviceOffering.DeepCopy()
		serviceOffering.Spec.CurrentRevisionRef.Name = serviceProfileV2.Name
		Expect(c.Patch(ctx, serviceOffering, client.MergeFrom(originalOffering))).To(Succeed())

		upgradeRequest := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: claim.Namespace,
				Name:      upgradeRequestName,
			},
			Spec: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestSpec{
				ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
				Target: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
					ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: catalog.OfferingName},
				},
			},
		}
		Expect(c.Create(ctx, upgradeRequest)).To(Succeed())

		By("waiting for the claim upgrade request to drive the Blue/Green rollout")
		request, err := waitForClaimUpgradeRequest(
			ctx,
			c,
			upgradeRequest.Namespace,
			upgradeRequest.Name,
			6*time.Minute,
			framework.DefaultPollInterval,
			func(request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) (bool, error) {
				return request.Status.State == openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut ||
					request.Status.State == openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded, nil
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(request.Status.Target).NotTo(BeNil())
		Expect(request.Status.Target.ServiceProfileRef).NotTo(BeNil())
		Expect(request.Status.Target.ServiceProfileRef.Name).To(Equal(serviceProfileV2.Name))
		Expect(request.Status.Classification).NotTo(BeNil())
		Expect(request.Status.Classification.Class).To(Equal(
			openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace,
		))

		By("waiting for the Blue/Green-managed cluster to converge to the target version")
		request, err = waitForClaimUpgradeRequestState(
			ctx,
			c,
			upgradeRequest.Namespace,
			upgradeRequest.Name,
			openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded,
			20*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(request.Status.Classification).NotTo(BeNil())
		Expect(request.Status.Classification.Class).To(Equal(
			openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace,
		))
		_, err = waitForClaimPinnedBinding(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			catalog.OfferingName,
			serviceProfileV2.Name,
			10*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		_, err = waitForClaimUpgradeCleared(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			5*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			localCluster := &openbaov1alpha1.OpenBaoCluster{}
			key := types.NamespacedName{Namespace: localRef.Namespace, Name: localRef.Name}
			g.Expect(c.Get(ctx, key, localCluster)).To(Succeed())
			g.Expect(localCluster.Spec.Version).To(Equal(targetVersion))
			g.Expect(localCluster.Spec.Image).To(Equal(targetImage))
			g.Expect(localCluster.Spec.Upgrade).NotTo(BeNil())
			g.Expect(localCluster.Spec.Upgrade.Strategy).To(Equal(openbaov1alpha1.UpdateStrategyBlueGreen))
			g.Expect(localCluster.Status.CurrentVersion).To(Equal(targetVersion))
			g.Expect(localCluster.Status.BlueGreen).NotTo(BeNil())
			g.Expect(localCluster.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle))
			g.Expect(localCluster.Status.BlueGreen.GreenRevision).To(BeEmpty())
		}, 10*time.Minute, framework.DefaultPollInterval).Should(Succeed())
	})

	It("rolls out a service-offering revision across claims with bounded concurrency", Label(
		"case:claims-functional-service-offering-rollout",
		"covers:claim-service-offering-rollout",
		"covers:claim-service-offering-rollout-concurrency",
		"claims-rollout",
		"claims-concurrency",
		"claims-upgrade",
	), func() {
		if !serviceClaimsE2EEnabled() {
			Skip("claim functional suite requires E2E_ENABLE_SERVICE_CLAIMS=true")
		}
		initialVersion := envOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
		targetVersion := envOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)
		if initialVersion == targetVersion {
			Skip(fmt.Sprintf("claim rollout functional case requires different versions: from=%s to=%s", initialVersion, targetVersion))
		}
		targetImage := fmt.Sprintf("%s:%s", envOrDefault("RELATED_IMAGE_OPENBAO", "openbao/openbao"), targetVersion)

		f, err := framework.NewSetup(ctx, "claims-functional-rollout", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		c := f.Client
		catalog := newSameClusterClaimCatalog(f.Namespace)
		backupBackend := catalog.backupBackend()
		backupTarget := catalog.backupTarget()
		backupProfileV1 := catalog.backupProfile()
		backupProfileV1.Spec.Schedule = "0 3 * * *"
		backupProfileV1.Spec.TargetRef = &openbaov1alpha1.LocalReference{Name: backupTarget.Name}
		backupProfileV2 := catalog.backupProfile()
		backupProfileV2.Name = claimScopedName("backup-rollout-v2", f.Namespace)
		backupProfileV2.Spec.Schedule = "45 4 * * *"
		backupProfileV2.Spec.TargetRef = &openbaov1alpha1.LocalReference{Name: backupTarget.Name}

		serviceProfileV1 := catalog.serviceProfile()
		serviceProfileV1.Spec.Cluster.Version = initialVersion
		serviceProfileV2 := catalog.serviceProfile()
		serviceProfileV2.Name = claimScopedName("service-rollout-v2", f.Namespace)
		serviceProfileV2.Spec.Cluster.Version = targetVersion
		serviceProfileV2.Spec.Backup.ProfileRef.Name = backupProfileV2.Name
		serviceOffering := catalog.serviceOffering()
		claimA := catalog.sameClusterClaim(operatorNamespace, claimScopedName("claim-a", f.Namespace), f.TenantName)
		claimB := catalog.sameClusterClaim(operatorNamespace, claimScopedName("claim-b", f.Namespace), f.TenantName)
		rolloutName := claimScopedName("offering-rollout", f.Namespace)
		maxConcurrent := int32(1)
		rollout := &openbaov1alpha1.OpenBaoServiceOfferingRollout{
			ObjectMeta: metav1.ObjectMeta{Name: rolloutName},
			Spec: openbaov1alpha1.OpenBaoServiceOfferingRolloutSpec{
				OfferingRef:       openbaov1alpha1.LocalReference{Name: catalog.OfferingName},
				TargetRevisionRef: openbaov1alpha1.LocalReference{Name: serviceProfileV2.Name},
				Selector: &openbaov1alpha1.OpenBaoServiceOfferingRolloutSelectorSpec{
					Namespaces: []string{operatorNamespace},
				},
				Strategy: &openbaov1alpha1.OpenBaoServiceOfferingRolloutStrategySpec{
					MaxConcurrent: &maxConcurrent,
					Mode:          openbaov1alpha1.OpenBaoServiceOfferingRolloutModeInPlaceOnly,
				},
			},
		}

		DeferCleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			_ = deleteObjects(
				cleanupCtx,
				c,
				&openbaov1alpha1.OpenBaoServiceOfferingRollout{
					ObjectMeta: metav1.ObjectMeta{Name: rolloutName},
				},
			)
			requests, err := listClaimUpgradeRequestsForRollout(cleanupCtx, c, rolloutName)
			if err == nil {
				for i := range requests {
					_ = c.Delete(cleanupCtx, &requests[i])
				}
			}
			for _, claim := range []*openbaov1alpha1.OpenBaoClusterClaim{claimA, claimB} {
				latest := &openbaov1alpha1.OpenBaoClusterClaim{}
				if err := c.Get(cleanupCtx, client.ObjectKeyFromObject(claim), latest); err == nil {
					_ = c.Delete(cleanupCtx, latest)
					_ = waitForClaimDeleted(cleanupCtx, c, latest.Namespace, latest.Name, 3*time.Minute, framework.DefaultPollInterval)
				}
			}

			_ = deleteObjects(
				cleanupCtx,
				c,
				serviceOffering,
				serviceProfileV1,
				serviceProfileV2,
				catalog.secretBootstrapProfile(),
				catalog.internalExposureClass(),
				backupBackend,
				backupTarget,
				backupProfileV1,
				backupProfileV2,
			)
			_ = f.Cleanup(cleanupCtx)
		})

		Expect(createObjects(
			ctx,
			c,
			catalog.bootstrapAuthSecret(f.Namespace),
			backupBackend,
			backupTarget,
			backupProfileV1,
			backupProfileV2,
			catalog.internalExposureClass(),
			catalog.secretBootstrapProfile(),
			serviceProfileV1,
			serviceProfileV2,
			serviceOffering,
		)).To(Succeed())
		Expect(c.Create(ctx, claimA)).To(Succeed())
		Expect(c.Create(ctx, claimB)).To(Succeed())

		localRefs := make(map[string]*openbaov1alpha1.NamespacedReference, 2)
		for _, claim := range []*openbaov1alpha1.OpenBaoClusterClaim{claimA, claimB} {
			_, err = waitForClaimPinnedBinding(
				ctx,
				c,
				claim.Namespace,
				claim.Name,
				catalog.OfferingName,
				serviceProfileV1.Name,
				6*time.Minute,
				framework.DefaultPollInterval,
			)
			Expect(err).NotTo(HaveOccurred())
			_, err = waitForClaimPhase(
				ctx,
				c,
				claim.Namespace,
				claim.Name,
				openbaov1alpha1.OpenBaoClusterClaimPhaseReady,
				8*time.Minute,
				framework.DefaultPollInterval,
			)
			Expect(err).NotTo(HaveOccurred())
			localRef, err := waitForClaimLocalClusterRef(
				ctx,
				c,
				claim.Namespace,
				claim.Name,
				3*time.Minute,
				framework.DefaultPollInterval,
			)
			Expect(err).NotTo(HaveOccurred())
			localRefs[claim.Name] = localRef
			Expect(f.WaitForClusterPhase(
				ctx,
				localRef.Name,
				openbaov1alpha1.ClusterPhaseRunning,
				8*time.Minute,
				framework.DefaultPollInterval,
			)).To(Succeed())
		}

		By("publishing the next offering revision and creating a bounded rollout intent")
		Expect(c.Get(ctx, client.ObjectKeyFromObject(serviceOffering), serviceOffering)).To(Succeed())
		originalOffering := serviceOffering.DeepCopy()
		serviceOffering.Spec.CurrentRevisionRef.Name = serviceProfileV2.Name
		Expect(c.Patch(ctx, serviceOffering, client.MergeFrom(originalOffering))).To(Succeed())
		Expect(c.Create(ctx, rollout)).To(Succeed())

		By("observing bounded rollout-owned claim upgrade request concurrency")
		Eventually(func(g Gomega) {
			requests, err := listClaimUpgradeRequestsForRollout(ctx, c, rolloutName)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(requests).NotTo(BeEmpty())

			updatedRollout := &openbaov1alpha1.OpenBaoServiceOfferingRollout{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: rolloutName}, updatedRollout)).To(Succeed())
			g.Expect(updatedRollout.Status.Total).To(Equal(int32(2)))
			for _, claimStatus := range updatedRollout.Status.Claims {
				if claimStatus.RequestRef == nil {
					continue
				}
				g.Expect(claimStatus.RequestRef.Namespace).To(Equal(operatorNamespace))
			}
		}, 2*time.Minute, framework.DefaultPollInterval).Should(Succeed())
		Consistently(func(g Gomega) {
			requests, err := listClaimUpgradeRequestsForRollout(ctx, c, rolloutName)
			g.Expect(err).NotTo(HaveOccurred())

			active := 0
			for _, request := range requests {
				switch request.Status.State {
				case "",
					openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending,
					openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut:
					active++
				}
			}
			g.Expect(active).To(BeNumerically("<=", 1))
		}, 90*time.Second, framework.DefaultPollInterval).Should(Succeed())

		By("waiting for the rollout to promote both selected claims")
		updatedRollout, err := waitForServiceOfferingRolloutState(
			ctx,
			c,
			rolloutName,
			openbaov1alpha1.OpenBaoServiceOfferingRolloutStateSucceeded,
			20*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updatedRollout.Status.Total).To(Equal(int32(2)))
		Expect(updatedRollout.Status.Succeeded).To(Equal(int32(2)))
		Expect(updatedRollout.Status.Failed).To(Equal(int32(0)))
		Expect(updatedRollout.Status.Blocked).To(Equal(int32(0)))
		requests, err := listClaimUpgradeRequestsForRollout(ctx, c, rolloutName)
		Expect(err).NotTo(HaveOccurred())
		Expect(requests).To(HaveLen(2))

		for _, claim := range []*openbaov1alpha1.OpenBaoClusterClaim{claimA, claimB} {
			_, err = waitForClaimPinnedBinding(
				ctx,
				c,
				claim.Namespace,
				claim.Name,
				catalog.OfferingName,
				serviceProfileV2.Name,
				10*time.Minute,
				framework.DefaultPollInterval,
			)
			Expect(err).NotTo(HaveOccurred())
			_, err = waitForClaimUpgradeCleared(
				ctx,
				c,
				claim.Namespace,
				claim.Name,
				5*time.Minute,
				framework.DefaultPollInterval,
			)
			Expect(err).NotTo(HaveOccurred())

			localRef := localRefs[claim.Name]
			Eventually(func(g Gomega) {
				localCluster := &openbaov1alpha1.OpenBaoCluster{}
				key := types.NamespacedName{Namespace: localRef.Namespace, Name: localRef.Name}
				g.Expect(c.Get(ctx, key, localCluster)).To(Succeed())
				g.Expect(localCluster.Spec.Version).To(Equal(targetVersion))
				g.Expect(localCluster.Spec.Image).To(Equal(targetImage))
				g.Expect(localCluster.Status.CurrentVersion).To(Equal(targetVersion))
				g.Expect(localCluster.Spec.Backup).NotTo(BeNil())
				g.Expect(localCluster.Spec.Backup.Schedule).To(Equal("45 4 * * *"))
			}, 5*time.Minute, framework.DefaultPollInterval).Should(Succeed())
		}
	})

	It("executes a manual claim backup request and projects the result onto claim status", Label(
		"case:claims-functional-backup-request",
		"covers:claim-backup-request",
		"covers:claim-backup-status-projection",
	), func() {
		if !serviceClaimsE2EEnabled() {
			Skip("claim functional suite requires E2E_ENABLE_SERVICE_CLAIMS=true")
		}

		f, err := framework.NewSetup(ctx, "claims-functional-backup", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())

		restCfg, err := ctrlconfig.GetConfig()
		Expect(err).NotTo(HaveOccurred())
		rustfsNamespace := claimScopedName("rustfs", f.Namespace)
		rustfsEndpoint := fmt.Sprintf("http://rustfs-svc.%s.svc.cluster.local:9000", rustfsNamespace)
		if err := ensureClaimRustFS(ctx, f.Client, restCfg, rustfsNamespace); err != nil {
			Skip("claim backup functional case requires RustFS: " + err.Error())
		}
		Expect(err).NotTo(HaveOccurred())
		c := f.Client
		catalog := newSameClusterClaimCatalog(f.Namespace)

		credentialsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      claimScopedName("rustfs-secret", f.Namespace),
				Namespace: f.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"accessKeyId":     []byte(rustfsAccessKey),
				"secretAccessKey": []byte(rustfsSecretKey),
			},
		}
		backupAuthProfile := &openbaov1alpha1.OpenBaoBackupAuthProfile{
			ObjectMeta: metav1.ObjectMeta{Name: claimScopedName("backup-auth", f.Namespace)},
			Spec: openbaov1alpha1.OpenBaoBackupAuthProfileSpec{
				Mode: openbaov1alpha1.OpenBaoBackupAuthModeStaticCredentials,
				StaticCredentials: &openbaov1alpha1.OpenBaoBackupStaticCredentialsSpec{
					SecretName: credentialsSecret.Name,
				},
			},
		}
		backupBackend := catalog.backupBackend()
		backupBackend.Spec.ObjectStorage.Endpoint = rustfsEndpoint
		backupBackend.Spec.ObjectStorage.Region = "us-east-1"
		backupBackend.Spec.ObjectStorage.UsePathStyle = true
		backupBackend.Spec.ObjectStorage.RequiredEgressRules = []networkingv1.NetworkPolicyEgressRule{{
			To: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": rustfsNamespace,
					},
				},
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "rustfs",
					},
				},
			}},
			Ports: []networkingv1.NetworkPolicyPort{{
				Protocol: ptr.To(corev1.ProtocolTCP),
				Port:     ptr.To(intstr.FromInt32(9000)),
			}},
		}}
		backupTarget := catalog.backupTarget()
		backupTarget.Spec.AuthProfileRef = &openbaov1alpha1.LocalReference{Name: backupAuthProfile.Name}
		backupTarget.Spec.LocationPolicy.Location.Value = rustfsBucket
		backupProfile := catalog.backupProfile()
		backupProfile.Spec.Schedule = "17 4 * * *"
		backupProfile.Spec.TargetRef = &openbaov1alpha1.LocalReference{Name: backupTarget.Name}
		claim := catalog.sameClusterClaim(operatorNamespace, claimScopedName("claim", f.Namespace), f.TenantName)
		backupRequestName := claimScopedName("backup-request", f.Namespace)

		DeferCleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			_ = deleteObjects(
				cleanupCtx,
				c,
				&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: claim.Namespace, Name: backupRequestName},
				},
			)
			latest := &openbaov1alpha1.OpenBaoClusterClaim{}
			if err := c.Get(cleanupCtx, client.ObjectKeyFromObject(claim), latest); err == nil {
				_ = c.Delete(cleanupCtx, latest)
				_ = waitForClaimDeleted(cleanupCtx, c, latest.Namespace, latest.Name, 2*time.Minute, framework.DefaultPollInterval)
			}

			_ = deleteObjects(
				cleanupCtx,
				c,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rustfsNamespace}},
				credentialsSecret,
				backupAuthProfile,
				catalog.serviceOffering(),
				catalog.serviceProfile(),
				catalog.secretBootstrapProfile(),
				catalog.internalExposureClass(),
				backupProfile,
				backupTarget,
				backupBackend,
			)
			_ = f.Cleanup(cleanupCtx)
		})

		Expect(createObjects(
			ctx,
			c,
			catalog.bootstrapAuthSecret(f.Namespace),
			credentialsSecret,
			backupAuthProfile,
			backupBackend,
			backupTarget,
			backupProfile,
			catalog.internalExposureClass(),
			catalog.secretBootstrapProfile(),
			catalog.serviceProfile(),
			catalog.serviceOffering(),
		)).To(Succeed())
		Expect(c.Create(ctx, claim)).To(Succeed())

		updated, err := waitForClaimPhase(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			openbaov1alpha1.OpenBaoClusterClaimPhaseReady,
			8*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Connection.SecretRef).NotTo(BeNil())

		localRef, err := waitForClaimLocalClusterRef(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			3*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(f.WaitForClusterPhase(
			ctx,
			localRef.Name,
			openbaov1alpha1.ClusterPhaseRunning,
			8*time.Minute,
			framework.DefaultPollInterval,
		)).To(Succeed())

		backupRequest := &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: claim.Namespace,
				Name:      backupRequestName,
			},
			Spec: openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{
				ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
			},
		}
		Expect(c.Create(ctx, backupRequest)).To(Succeed())

		By("waiting for the claim to publish the active backup workflow summary")
		updated, err = waitForClaim(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			5*time.Minute,
			framework.DefaultPollInterval,
			func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
				return claim.Status.Backup != nil &&
					claim.Status.Backup.RequestRef != nil &&
					claim.Status.Backup.RequestRef.Name == backupRequestName &&
					claim.Status.Summary != nil &&
					claim.Status.Summary.SourceRef != nil, nil
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Backup.RequestState).To(Or(
			Equal(openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatePending),
			Equal(openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateRunning),
		))
		Expect(updated.Status.Summary.SourceRef.Kind).To(BeElementOf("OpenBaoClusterClaimBackupRequest", "OpenBaoCluster"))
		if updated.Status.Summary.SourceRef.Kind == "OpenBaoClusterClaimBackupRequest" {
			Expect(updated.Status.Summary.SourceRef.Name).To(Equal(backupRequestName))
		} else {
			Expect(updated.Status.Summary.SourceRef.Name).To(Equal(localRef.Name))
		}

		By("waiting for the backup request to complete successfully")
		request, err := waitForClaimBackupRequestState(
			ctx,
			c,
			backupRequest.Namespace,
			backupRequest.Name,
			openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateSucceeded,
			12*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(request.Status.SnapshotKey).NotTo(BeEmpty())
		Expect(request.Status.ClusterRef).NotTo(BeNil())
		Expect(request.Status.ClusterRef.Name).To(Equal(localRef.Name))

		By("surfacing the completed claim backup in the namespaced backup request inventory")
		backupInventory := &openbaov1alpha1.OpenBaoClusterClaimBackupRequestList{}
		Expect(c.List(ctx, backupInventory, client.InNamespace(claim.Namespace))).To(Succeed())
		Expect(backupInventory.Items).To(ContainElement(SatisfyAll(
			HaveField("Name", backupRequestName),
			HaveField("Status.State", openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateSucceeded),
			HaveField("Status.SnapshotKey", request.Status.SnapshotKey),
		)))

		By("projecting the completed backup onto claim status and clearing the workflow summary")
		updated, err = waitForClaim(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			5*time.Minute,
			framework.DefaultPollInterval,
			func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
				return claim.Status.Phase == openbaov1alpha1.OpenBaoClusterClaimPhaseReady &&
					claim.Status.Summary == nil &&
					claim.Status.Backup != nil &&
					claim.Status.Backup.RequestRef == nil &&
					claim.Status.Backup.LastBackupTime != nil &&
					claim.Status.Backup.LastBackupName == request.Status.SnapshotKey, nil
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Backup.RequestState).To(BeEmpty())
	})

	It("executes a claim restore request from a selected completed backup request", Label(
		"case:claims-functional-restore-request",
		"covers:claim-restore-request",
		"covers:claim-restore-latest-successful-source",
		"covers:claim-restore-backup-request-source",
		"covers:claim-restore-bluegreen",
		"covers:claim-restore-status-projection",
		"claims-bluegreen",
	), func() {
		if !serviceClaimsE2EEnabled() {
			Skip("claim functional suite requires E2E_ENABLE_SERVICE_CLAIMS=true")
		}

		f, err := framework.NewSetup(ctx, "claims-functional-restore", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())

		restCfg, err := ctrlconfig.GetConfig()
		Expect(err).NotTo(HaveOccurred())
		rustfsNamespace := claimScopedName("rustfs", f.Namespace)
		rustfsEndpoint := fmt.Sprintf("http://rustfs-svc.%s.svc.cluster.local:9000", rustfsNamespace)
		if err := ensureClaimRustFS(ctx, f.Client, restCfg, rustfsNamespace); err != nil {
			Skip("claim restore functional case requires RustFS: " + err.Error())
		}
		c := f.Client
		catalog := newSameClusterClaimCatalog(f.Namespace)

		credentialsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      claimScopedName("rustfs-secret", f.Namespace),
				Namespace: f.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"accessKeyId":     []byte(rustfsAccessKey),
				"secretAccessKey": []byte(rustfsSecretKey),
			},
		}
		backupAuthProfile := &openbaov1alpha1.OpenBaoBackupAuthProfile{
			ObjectMeta: metav1.ObjectMeta{Name: claimScopedName("backup-auth", f.Namespace)},
			Spec: openbaov1alpha1.OpenBaoBackupAuthProfileSpec{
				Mode: openbaov1alpha1.OpenBaoBackupAuthModeStaticCredentials,
				StaticCredentials: &openbaov1alpha1.OpenBaoBackupStaticCredentialsSpec{
					SecretName: credentialsSecret.Name,
				},
			},
		}
		backupBackend := catalog.backupBackend()
		backupBackend.Spec.ObjectStorage.Endpoint = rustfsEndpoint
		backupBackend.Spec.ObjectStorage.Region = "us-east-1"
		backupBackend.Spec.ObjectStorage.UsePathStyle = true
		backupBackend.Spec.ObjectStorage.RequiredEgressRules = []networkingv1.NetworkPolicyEgressRule{{
			To: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": rustfsNamespace,
					},
				},
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "rustfs",
					},
				},
			}},
			Ports: []networkingv1.NetworkPolicyPort{{
				Protocol: ptr.To(corev1.ProtocolTCP),
				Port:     ptr.To(intstr.FromInt32(9000)),
			}},
		}}
		backupTarget := catalog.backupTarget()
		backupTarget.Spec.AuthProfileRef = &openbaov1alpha1.LocalReference{Name: backupAuthProfile.Name}
		backupTarget.Spec.LocationPolicy.Location.Value = rustfsBucket
		backupProfile := catalog.backupProfile()
		backupProfile.Spec.Schedule = "17 4 * * *"
		backupProfile.Spec.TargetRef = &openbaov1alpha1.LocalReference{Name: backupTarget.Name}
		upgradePolicyName := claimScopedName("bluegreen-policy", f.Namespace)
		autoPromote := true
		maxJobFailures := int32(2)
		upgradePolicy := &openbaov1alpha1.OpenBaoUpgradePolicy{
			ObjectMeta: metav1.ObjectMeta{Name: upgradePolicyName},
			Spec: openbaov1alpha1.OpenBaoUpgradePolicySpec{
				BlueGreen: &openbaov1alpha1.OpenBaoUpgradePolicyBlueGreenSpec{
					AutoPromote:     &autoPromote,
					MinSyncDuration: "1s",
					MaxJobFailures:  &maxJobFailures,
				},
			},
		}
		serviceProfile := catalog.serviceProfile()
		serviceProfile.Spec.Lifecycle.UpgradeStrategy = openbaov1alpha1.UpdateStrategyBlueGreen
		serviceProfile.Spec.Lifecycle.PolicyRef = &openbaov1alpha1.LocalReference{Name: upgradePolicyName}
		claim := catalog.sameClusterClaim(operatorNamespace, claimScopedName("claim", f.Namespace), f.TenantName)
		backupRequestName := claimScopedName("backup-request", f.Namespace)
		latestRestoreRequestName := claimScopedName("restore-latest", f.Namespace)
		restoreRequestName := claimScopedName("restore-request", f.Namespace)

		var localRef *openbaov1alpha1.NamespacedReference

		DeferCleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			if localRef != nil {
				_ = deleteObjects(
					cleanupCtx,
					c,
					&openbaov1alpha1.OpenBaoRestore{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: localRef.Namespace,
							Name:      latestRestoreRequestName,
						},
					},
					&openbaov1alpha1.OpenBaoRestore{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: localRef.Namespace,
							Name:      restoreRequestName,
						},
					},
				)
			}

			_ = deleteObjects(
				cleanupCtx,
				c,
				&openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: claim.Namespace, Name: latestRestoreRequestName},
				},
				&openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: claim.Namespace, Name: restoreRequestName},
				},
				&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: claim.Namespace, Name: backupRequestName},
				},
			)
			latest := &openbaov1alpha1.OpenBaoClusterClaim{}
			if err := c.Get(cleanupCtx, client.ObjectKeyFromObject(claim), latest); err == nil {
				_ = c.Delete(cleanupCtx, latest)
				_ = waitForClaimDeleted(cleanupCtx, c, latest.Namespace, latest.Name, 2*time.Minute, framework.DefaultPollInterval)
			}

			_ = deleteObjects(
				cleanupCtx,
				c,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rustfsNamespace}},
				credentialsSecret,
				backupAuthProfile,
				catalog.serviceOffering(),
				serviceProfile,
				upgradePolicy,
				catalog.secretBootstrapProfile(),
				catalog.internalExposureClass(),
				backupProfile,
				backupTarget,
				backupBackend,
			)
			_ = f.Cleanup(cleanupCtx)
		})

		Expect(createObjects(
			ctx,
			c,
			catalog.bootstrapAuthSecret(f.Namespace),
			credentialsSecret,
			backupAuthProfile,
			backupBackend,
			backupTarget,
			backupProfile,
			catalog.internalExposureClass(),
			catalog.secretBootstrapProfile(),
			upgradePolicy,
			serviceProfile,
			catalog.serviceOffering(),
		)).To(Succeed())
		Expect(c.Create(ctx, claim)).To(Succeed())

		updated, err := waitForClaimPhase(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			openbaov1alpha1.OpenBaoClusterClaimPhaseReady,
			8*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Connection.SecretRef).NotTo(BeNil())

		localRef, err = waitForClaimLocalClusterRef(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			3*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(f.WaitForClusterPhase(
			ctx,
			localRef.Name,
			openbaov1alpha1.ClusterPhaseRunning,
			8*time.Minute,
			framework.DefaultPollInterval,
		)).To(Succeed())
		Eventually(func(g Gomega) {
			localCluster := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Namespace: localRef.Namespace, Name: localRef.Name}, localCluster)).To(Succeed())
			g.Expect(localCluster.Spec.Upgrade).NotTo(BeNil())
			g.Expect(localCluster.Spec.Upgrade.Strategy).To(Equal(openbaov1alpha1.UpdateStrategyBlueGreen))
			g.Expect(localCluster.Status.BlueGreen).NotTo(BeNil())
			g.Expect(localCluster.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle))
		}, 8*time.Minute, framework.DefaultPollInterval).Should(Succeed())

		By("creating a fresh successful backup for the restore request to consume")
		backupRequest := &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: claim.Namespace,
				Name:      backupRequestName,
			},
			Spec: openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{
				ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
			},
		}
		Expect(c.Create(ctx, backupRequest)).To(Succeed())

		backupResult, err := waitForClaimBackupRequestState(
			ctx,
			c,
			backupRequest.Namespace,
			backupRequest.Name,
			openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateSucceeded,
			12*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(backupResult.Status.SnapshotKey).NotTo(BeEmpty())

		By("surfacing the completed claim backup in the namespaced backup request inventory")
		backupInventory := &openbaov1alpha1.OpenBaoClusterClaimBackupRequestList{}
		Expect(c.List(ctx, backupInventory, client.InNamespace(claim.Namespace))).To(Succeed())
		Expect(backupInventory.Items).To(ContainElement(SatisfyAll(
			HaveField("Name", backupRequestName),
			HaveField("Status.State", openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateSucceeded),
			HaveField("Status.SnapshotKey", backupResult.Status.SnapshotKey),
		)))

		_, err = waitForClaim(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			5*time.Minute,
			framework.DefaultPollInterval,
			func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
				return claim.Status.Phase == openbaov1alpha1.OpenBaoClusterClaimPhaseReady &&
					claim.Status.Summary == nil &&
					claim.Status.Backup != nil &&
					claim.Status.Backup.LastBackupName == backupResult.Status.SnapshotKey, nil
			},
		)
		Expect(err).NotTo(HaveOccurred())

		latestRestoreRequest := &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: claim.Namespace,
				Name:      latestRestoreRequestName,
			},
			Spec: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSpec{
				ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
			},
		}
		Expect(c.Create(ctx, latestRestoreRequest)).To(Succeed())

		By("restoring the latest successful claim backup when no explicit source is selected")
		latestRestoreResult, err := waitForClaimRestoreRequestState(
			ctx,
			c,
			latestRestoreRequest.Namespace,
			latestRestoreRequest.Name,
			openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateSucceeded,
			15*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(latestRestoreResult.Status.SnapshotKey).To(Equal(backupResult.Status.SnapshotKey))
		Expect(latestRestoreResult.Status.RestoreRef).NotTo(BeNil())
		rawLatestRestore := &openbaov1alpha1.OpenBaoRestore{}
		Expect(c.Get(ctx, types.NamespacedName{
			Namespace: latestRestoreResult.Status.RestoreRef.Namespace,
			Name:      latestRestoreResult.Status.RestoreRef.Name,
		}, rawLatestRestore)).To(Succeed())
		Expect(rawLatestRestore.Spec.Cluster).To(Equal(localRef.Name))
		Expect(rawLatestRestore.Spec.Source.Key).To(Equal(backupResult.Status.SnapshotKey))

		_, err = waitForClaim(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			5*time.Minute,
			framework.DefaultPollInterval,
			func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
				return claim.Status.Phase == openbaov1alpha1.OpenBaoClusterClaimPhaseReady &&
					claim.Status.Summary == nil &&
					claim.Status.Restore == nil, nil
			},
		)
		Expect(err).NotTo(HaveOccurred())

		restoreRequest := &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: claim.Namespace,
				Name:      restoreRequestName,
			},
			Spec: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSpec{
				ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
				Source: &openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceSpec{
					Mode: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeBackupRequest,
					BackupRequestRef: &openbaov1alpha1.LocalReference{
						Name: backupRequestName,
					},
				},
			},
		}
		Expect(c.Create(ctx, restoreRequest)).To(Succeed())

		By("waiting for the restore request to start or complete")
		restoreResult, err := waitForClaimRestoreRequest(
			ctx,
			c,
			restoreRequest.Namespace,
			restoreRequest.Name,
			8*time.Minute,
			framework.DefaultPollInterval,
			func(request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest) (bool, error) {
				return request.Status.State == openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning ||
					request.Status.State == openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateSucceeded, nil
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(restoreResult.Status.ClusterRef).NotTo(BeNil())
		Expect(restoreResult.Status.ClusterRef.Namespace).To(Equal(localRef.Namespace))
		Expect(restoreResult.Status.ClusterRef.Name).To(Equal(localRef.Name))
		Expect(restoreResult.Status.RestoreRef).NotTo(BeNil())
		Expect(restoreResult.Status.RestoreRef.Namespace).To(Equal(localRef.Namespace))
		Expect(restoreResult.Status.SnapshotKey).To(Equal(backupResult.Status.SnapshotKey))

		rawRestore := &openbaov1alpha1.OpenBaoRestore{}
		Expect(c.Get(ctx, types.NamespacedName{
			Namespace: restoreResult.Status.RestoreRef.Namespace,
			Name:      restoreResult.Status.RestoreRef.Name,
		}, rawRestore)).To(Succeed())
		Expect(rawRestore.Spec.Cluster).To(Equal(localRef.Name))
		Expect(rawRestore.Spec.Source.Key).To(Equal(backupResult.Status.SnapshotKey))

		if restoreResult.Status.State == openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning {
			By("publishing the active restore workflow on claim status")
			updated, err = waitForClaim(
				ctx,
				c,
				claim.Namespace,
				claim.Name,
				5*time.Minute,
				framework.DefaultPollInterval,
				func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
					return claim.Status.Phase == openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded &&
						claim.Status.Restore != nil &&
						claim.Status.Restore.RequestRef != nil &&
						claim.Status.Restore.RequestRef.Name == restoreRequestName &&
						claim.Status.Restore.ExecutionRef != nil &&
						claim.Status.Restore.ExecutionRef.Namespace == localRef.Namespace &&
						claim.Status.Restore.ExecutionRef.Name == rawRestore.Name &&
						claim.Status.Summary != nil &&
						claim.Status.Summary.SourceRef != nil &&
						claim.Status.Summary.SourceRef.Kind == "OpenBaoClusterClaimRestoreRequest" &&
						claim.Status.Summary.SourceRef.Name == restoreRequestName, nil
				},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.Status.Summary.Severity).To(Equal(openbaov1alpha1.OpenBaoClusterClaimStatusSeverityWarning))
			Expect(updated.Status.Restore.SnapshotKey).To(Equal(backupResult.Status.SnapshotKey))
		}

		By("waiting for the restore request to complete successfully")
		restoreResult, err = waitForClaimRestoreRequestState(
			ctx,
			c,
			restoreRequest.Namespace,
			restoreRequest.Name,
			openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateSucceeded,
			15*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(restoreResult.Status.CompletionTime).NotTo(BeNil())
		Expect(restoreResult.Status.SnapshotKey).To(Equal(backupResult.Status.SnapshotKey))

		Eventually(func(g Gomega) {
			current := &openbaov1alpha1.OpenBaoRestore{}
			g.Expect(c.Get(ctx, types.NamespacedName{
				Namespace: restoreResult.Status.RestoreRef.Namespace,
				Name:      restoreResult.Status.RestoreRef.Name,
			}, current)).To(Succeed())
			g.Expect(current.Status.Phase).To(Equal(openbaov1alpha1.RestorePhaseCompleted))
			g.Expect(current.Status.SnapshotKey).To(Equal(backupResult.Status.SnapshotKey))
		}, 5*time.Minute, framework.DefaultPollInterval).Should(Succeed())

		By("clearing the active restore workflow from claim status after completion")
		updated, err = waitForClaim(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			5*time.Minute,
			framework.DefaultPollInterval,
			func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
				return claim.Status.Phase == openbaov1alpha1.OpenBaoClusterClaimPhaseReady &&
					claim.Status.Summary == nil &&
					claim.Status.Restore == nil, nil
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Connection.SecretRef).NotTo(BeNil())
		Expect(f.WaitForClusterPhase(
			ctx,
			localRef.Name,
			openbaov1alpha1.ClusterPhaseRunning,
			8*time.Minute,
			framework.DefaultPollInterval,
		)).To(Succeed())
	})

	It("keeps the claim pending when a secret-backed bootstrap source is missing", Label(
		"case:claims-functional-missing-bootstrap-source",
		"covers:claim-missing-bootstrap-source",
		"negative",
	), func() {
		if !serviceClaimsE2EEnabled() {
			Skip("claim functional suite requires E2E_ENABLE_SERVICE_CLAIMS=true")
		}

		f, err := framework.NewSetup(ctx, "claims-functional-missing-bootstrap", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		c := f.Client
		catalog := newSameClusterClaimCatalog(f.Namespace)

		var claim *openbaov1alpha1.OpenBaoClusterClaim

		DeferCleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			if claim != nil {
				latest := &openbaov1alpha1.OpenBaoClusterClaim{}
				if err := c.Get(cleanupCtx, client.ObjectKeyFromObject(claim), latest); err == nil {
					_ = c.Delete(cleanupCtx, latest)
					_ = waitForClaimDeleted(cleanupCtx, c, latest.Namespace, latest.Name, 2*time.Minute, framework.DefaultPollInterval)
				}
			}

			_ = deleteObjects(
				cleanupCtx,
				c,
				catalog.serviceOffering(),
				catalog.serviceProfile(),
				catalog.secretBootstrapProfile(),
				catalog.internalExposureClass(),
				catalog.backupProfile(),
			)
			_ = f.Cleanup(cleanupCtx)
		})

		Expect(createObjects(
			ctx,
			c,
			catalog.backupProfile(),
			catalog.internalExposureClass(),
			catalog.secretBootstrapProfile(),
			catalog.serviceProfile(),
			catalog.serviceOffering(),
		)).To(Succeed())

		claim = catalog.sameClusterClaim(operatorNamespace, claimScopedName("claim", f.Namespace), f.TenantName)
		Expect(c.Create(ctx, claim)).To(Succeed())

		updated, err := waitForClaim(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			5*time.Minute,
			framework.DefaultPollInterval,
			func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
				if claim.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhasePending {
					return false, nil
				}
				if claim.Status.Materialization.LocalRef != nil {
					return false, nil
				}
				for _, condition := range claim.Status.Conditions {
					if condition.Type == "MaterializationResolved" &&
						condition.Status == metav1.ConditionFalse &&
						strings.Contains(condition.Message, "Secret does not exist yet") {
						return true, nil
					}
				}
				return false, nil
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Materialization.LocalRef).To(BeNil())
	})
})
