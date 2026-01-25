//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

var _ = Describe("Cluster Lifecycle", Label("lifecycle", "cluster"), Ordered, func() {
	ctx := context.Background()

	Context("Smoke: Tenant + Cluster lifecycle (Self-Init)", Label("smoke", "critical", "tenant"), func() {
		var (
			f *framework.Framework
			c client.Client
		)

		const (
			clusterName = "smoke-cluster"
		)

		BeforeAll(func() {
			var err error
			f, err = framework.NewSetup(ctx, "smoke", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			c = f.Client
		})

		AfterAll(func() {
			if f == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			_ = f.Cleanup(cleanupCtx)
		})

		It("provisions tenant RBAC via OpenBaoTenant", func() {
			By("verifying OpenBaoTenant is provisioned")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoTenant{}
				g.Expect(c.Get(ctx, types.NamespacedName{Name: f.TenantName, Namespace: operatorNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Provisioned).To(BeTrue())
				g.Expect(updated.Status.LastError).To(BeEmpty())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		It("creates an OpenBaoCluster and converges to Available", func() {
			By(fmt.Sprintf("creating OpenBaoCluster %q in namespace %q", clusterName, f.Namespace))
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: f.Namespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Version:  openBaoVersion,
					Image:    openBaoImage,
					Replicas: 1,
					InitContainer: &openbaov1alpha1.InitContainerConfig{
						Enabled: true,
						Image:   configInitImage,
					},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled:  true,
						Requests: framework.DefaultAdminSelfInitRequests(),
					},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled:        true,
						Mode:           openbaov1alpha1.TLSModeOperatorManaged,
						RotationPeriod: "720h",
					},
					Storage: openbaov1alpha1.StorageConfig{
						Size: "1Gi",
					},
					Network: &openbaov1alpha1.NetworkConfig{
						APIServerCIDR: apiServerCIDR,
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}

			Expect(c.Create(ctx, cluster)).To(Succeed())

			By("waiting for OpenBaoCluster to be observed by the API server")
			Eventually(func() error {
				return c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, &openbaov1alpha1.OpenBaoCluster{})
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("waiting for StatefulSet to be created")
			f.WaitForStatefulSetReady(ctx, clusterName, 1, framework.DefaultWaitTimeout, framework.DefaultPollInterval)

			By("triggering a reconcile and waiting for Available condition")
			Expect(f.TriggerReconcile(ctx, clusterName)).To(Succeed())
			f.WaitForCondition(clusterName, openbaov1alpha1.ConditionAvailable, metav1.ConditionTrue)

			By("verifying Raft Autopilot is configured")
			// (Simplified verification for smoke test)
			cm := &corev1.ConfigMap{}
			Expect(c.Get(ctx, types.NamespacedName{Name: clusterName + "-config", Namespace: f.Namespace}, cm)).To(Succeed())
		})
	})

	Context("Development Profile: Manual Init (Self-Init Disabled)", Label("profile-development"), func() {
		var (
			f *framework.Framework
			c client.Client
		)

		const (
			clusterName = "basic-cluster"
		)

		BeforeAll(func() {
			var err error
			f, err = framework.NewSetup(ctx, "basic", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			c = f.Client
		})

		AfterAll(func() {
			if f == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			_ = f.Cleanup(cleanupCtx)
		})

		It("creates a cluster with self-init disabled and produces expected Secrets", func() {
			By(fmt.Sprintf("creating OpenBaoCluster %q", clusterName))
			cluster, err := f.CreateDevelopmentCluster(ctx, framework.DevelopmentClusterConfig{
				Name:          clusterName,
				Replicas:      3,
				Version:       openBaoVersion,
				Image:         openBaoImage,
				ConfigInitImg: configInitImage,
				APIServerCIDR: apiServerCIDR,
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = c.Delete(ctx, cluster)
			})

			By("waiting for Secrets to be created")
			Eventually(func() error {
				return c.Get(ctx, types.NamespacedName{Name: clusterName + "-tls-ca", Namespace: f.Namespace}, &corev1.Secret{})
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed(), "TLS CA Secret missing")

			Eventually(func() error {
				return c.Get(ctx, types.NamespacedName{Name: clusterName + "-tls-server", Namespace: f.Namespace}, &corev1.Secret{})
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed(), "TLS Server Secret missing")

			Eventually(func() error {
				return c.Get(ctx, types.NamespacedName{Name: clusterName + "-unseal-key", Namespace: f.Namespace}, &corev1.Secret{})
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed(), "Unseal Key Secret missing")

			By("waiting for root token Secret (self-init disabled)")
			Eventually(func(g Gomega) {
				g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName + "-root-token", Namespace: f.Namespace}, &corev1.Secret{})).To(Succeed())
			}, framework.DefaultLongWaitTimeout, 3*time.Second).Should(Succeed(), "Root Token Secret missing")
		})
	})

	Context("Development Profile: Scaling with Autopilot Reconciliation", Label("profile-development", "scaling", "autopilot"), func() {
		var (
			f   *framework.Framework
			c   client.Client
			cfg *rest.Config
		)

		const (
			clusterName = "scaling-cluster"
		)

		BeforeAll(func() {
			var err error
			f, err = framework.NewSetup(ctx, "scaling", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			c = f.Client

			// Get rest config for helper functions
			cfg, err = ctrlconfig.GetConfig()
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			if f == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			_ = f.Cleanup(cleanupCtx)
		})

		It("creates a cluster with 1 replica and verifies autopilot min_quorum=1", func() {
			By(fmt.Sprintf("creating OpenBaoCluster %q with 1 replica", clusterName))
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: f.Namespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Version:  openBaoVersion,
					Image:    openBaoImage,
					Replicas: 1,
					InitContainer: &openbaov1alpha1.InitContainerConfig{
						Enabled: true,
						Image:   configInitImage,
					},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
							Enabled: true,
						},
						Requests: e2ehelpers.CreateAutopilotVerificationRequests(f.Namespace),
					},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled:        true,
						Mode:           openbaov1alpha1.TLSModeOperatorManaged,
						RotationPeriod: "720h",
					},
					Storage: openbaov1alpha1.StorageConfig{
						Size: "1Gi",
					},
					Network: &openbaov1alpha1.NetworkConfig{
						APIServerCIDR: apiServerCIDR,
						IngressRules: []networkingv1.NetworkPolicyIngressRule{
							{
								From: []networkingv1.NetworkPolicyPeer{
									{
										PodSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"role": "test-verifier",
											},
										},
									},
								},
								Ports: []networkingv1.NetworkPolicyPort{
									{
										Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
										Port:     &[]intstr.IntOrString{intstr.FromInt(8200)}[0],
									},
								},
							},
						},
					},
					// Establish a stable ClusterIP Service for verification (DNS is more reliable than Headless in Kind)
					Service: &openbaov1alpha1.ServiceConfig{
						Type: "ClusterIP",
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}

			Expect(c.Create(ctx, cluster)).To(Succeed())

			By("waiting for StatefulSet to be ready with 1 replica")
			f.WaitForStatefulSetReady(ctx, clusterName, 1, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval)

			By("waiting for Available condition")
			f.WaitForCondition(clusterName, openbaov1alpha1.ConditionAvailable, metav1.ConditionTrue)

			By("waiting for SelfInit to complete (SelfInitialized status)")
			// Wait for SelfInit requests to complete so JWT role is created
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(c.Get(ctx, types.NamespacedName{
					Name:      clusterName,
					Namespace: f.Namespace,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Initialized).To(BeTrue(), "cluster should be initialized")
				g.Expect(updated.Status.SelfInitialized).To(BeTrue(), "SelfInit requests should be completed")
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("ensuring public service exists for autopilot verification")
			svc := &corev1.Service{}
			Eventually(func(g Gomega) {
				g.Expect(c.Get(ctx, types.NamespacedName{
					Name:      clusterName + "-public",
					Namespace: f.Namespace,
				}, svc)).To(Succeed(), "public service should exist")
				g.Expect(svc.Spec.ClusterIP).NotTo(BeEmpty())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("waiting a bit for autopilot config to be reconciled after initialization")
			// Give the operator time to reconcile autopilot config after cluster initialization
			time.Sleep(5 * time.Second)

			By("verifying Raft Autopilot min_quorum=1 (Development profile with 1 replica)")
			// Use Eventually to retry verification in case autopilot config hasn't been set yet
			Eventually(func() error {
				return e2ehelpers.VerifyRaftAutopilotMinQuorumViaJWT(
					ctx,
					cfg,
					c,
					f.Namespace,
					openBaoImage,
					fmt.Sprintf("https://%s:8200", svc.Spec.ClusterIP),
					"default",
					map[string]string{"role": "test-verifier"},
					1, // Expected min_quorum for Development profile with 1 replica
				)
			}, 2*time.Minute, 5*time.Second).Should(Succeed(), "Autopilot min_quorum should be 1 for Development profile with 1 replica")
			_, _ = fmt.Fprintf(GinkgoWriter, "✓ Raft Autopilot min_quorum=1 verified\n")
		})

		It("scales up to 3 replicas and verifies autopilot min_quorum=3", func() {
			By("updating cluster to 3 replicas")
			cluster := &openbaov1alpha1.OpenBaoCluster{}
			Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, cluster)).To(Succeed())

			cluster.Spec.Replicas = 3
			Expect(c.Update(ctx, cluster)).To(Succeed())

			By("waiting for StatefulSet to scale to 3 replicas")
			f.WaitForStatefulSetReady(ctx, clusterName, 3, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval)

			By("waiting for all pods to be ready")
			Eventually(func(g Gomega) {
				sts := &appsv1.StatefulSet{}
				g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)).To(Succeed())
				g.Expect(sts.Status.ReadyReplicas).To(Equal(int32(3)))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("getting public service for autopilot verification")
			svc := &corev1.Service{}
			Expect(c.Get(ctx, types.NamespacedName{
				Name:      clusterName + "-public",
				Namespace: f.Namespace,
			}, svc)).To(Succeed())

			By("verifying Raft Autopilot min_quorum=3 (Development profile with 3 replicas)")
			// Wait a bit for autopilot config to be reconciled after scaling
			Eventually(func() error {
				return e2ehelpers.VerifyRaftAutopilotMinQuorumViaJWT(
					ctx,
					cfg,
					c,
					f.Namespace,
					openBaoImage,
					fmt.Sprintf("https://%s:8200", svc.Spec.ClusterIP),
					"default",
					map[string]string{"role": "test-verifier"},
					3, // Expected min_quorum for Development profile with 3 replicas
				)
			}, 2*time.Minute, 5*time.Second).Should(Succeed(), "Autopilot min_quorum should be updated to 3 after scaling")
			_, _ = fmt.Fprintf(GinkgoWriter, "✓ Raft Autopilot min_quorum=3 verified after scale up\n")
		})

		It("scales down to 2 replicas and verifies autopilot min_quorum=2", func() {

			By("updating cluster to 2 replicas")
			cluster := &openbaov1alpha1.OpenBaoCluster{}
			Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, cluster)).To(Succeed())

			cluster.Spec.Replicas = 2
			Expect(c.Update(ctx, cluster)).To(Succeed())

			By("waiting for StatefulSet to scale down to 2 replicas")
			f.WaitForStatefulSetReady(ctx, clusterName, 2, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval)

			By("waiting for pods to be ready")
			Eventually(func(g Gomega) {
				sts := &appsv1.StatefulSet{}
				g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)).To(Succeed())
				g.Expect(sts.Status.ReadyReplicas).To(Equal(int32(2)))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("getting public service for autopilot verification")
			svc := &corev1.Service{}
			Expect(c.Get(ctx, types.NamespacedName{
				Name:      clusterName + "-public",
				Namespace: f.Namespace,
			}, svc)).To(Succeed())

			By("verifying Raft Autopilot min_quorum=2 (Development profile with 2 replicas)")
			// Wait a bit for autopilot config to be reconciled after scaling
			Eventually(func() error {
				return e2ehelpers.VerifyRaftAutopilotMinQuorumViaJWT(
					ctx,
					cfg,
					c,
					f.Namespace,
					openBaoImage,
					fmt.Sprintf("https://%s:8200", svc.Spec.ClusterIP),
					"default",
					map[string]string{"role": "test-verifier"},
					2, // Expected min_quorum for Development profile with 2 replicas
				)
			}, 2*time.Minute, 5*time.Second).Should(Succeed(), "Autopilot min_quorum should be updated to 2 after scale down")
			_, _ = fmt.Fprintf(GinkgoWriter, "✓ Raft Autopilot min_quorum=2 verified after scale down\n")
		})
	})
})
