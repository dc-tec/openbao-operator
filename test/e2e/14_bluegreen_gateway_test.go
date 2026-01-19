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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
)

// This test drives a Blue/Green upgrade flow with Gateway API enabled and verifies that:
// - the HTTPRoute always targets the main external Service (no weighted traffic splitting)
// - the external Service selector switches to the Green revision at cutover (DemotingBlue)
var _ = Describe("Blue/Green Upgrade with Gateway API", Label("upgrade", "bluegreen", "gateway-api", "requires-gateway-api", "slow"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		admin  client.Client

		tenantNamespace string
		tenantFW        *framework.Framework
		upgradeCluster  *openbaov1alpha1.OpenBaoCluster
		initialVersion  string
		targetVersion   string
	)

	BeforeAll(func() {
		var err error

		By("getting cluster REST config")
		cfg, err = ctrlconfig.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(openbaov1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(gatewayv1.AddToScheme(scheme)).To(Succeed())

		admin, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())

		By("installing Gateway API CRDs for Gateway tests")
		Expect(InstallGatewayAPI()).To(Succeed())

		By("creating tenant framework for Blue/Green Gateway test")
		tenantFW, err = framework.New(ctx, admin, "tenant-bluegreen-gateway", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		tenantNamespace = tenantFW.Namespace

		initialVersion = getEnvOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
		targetVersion = getEnvOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)

		if initialVersion == targetVersion {
			Skip(fmt.Sprintf("Gateway upgrade test skipped: from version (%s) equals to version (%s). Set E2E_UPGRADE_TO_VERSION to a different version to test upgrades.", initialVersion, targetVersion))
		}

		By("creating Gateway resource in tenant namespace")
		gw := &gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tenant-gateway",
				Namespace: tenantNamespace,
			},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "traefik", // Assumes a compatible GatewayClass; tests focus on HTTPRoute shape.
				Listeners: []gatewayv1.Listener{
					{
						Name:     "https",
						Port:     443,
						Protocol: gatewayv1.HTTPSProtocolType,
						Hostname: ptrTo(gatewayv1.Hostname("bao.example.local")),
					},
				},
			},
		}
		Expect(admin.Create(ctx, gw)).To(Succeed())

		By("creating SelfInit requests for Blue/Green upgrade")
		// Use the operator-supported bootstrap flow for JWT auth (OIDC discovery + JWKS keys
		// are done by the operator at startup; OpenBao is configured via self-init bootstrap).

		By("creating OpenBaoCluster with Blue/Green strategy and Gateway enabled")
		upgradeCluster = &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bluegreen-gateway-cluster",
				Namespace: tenantNamespace,
			},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Profile:  openbaov1alpha1.ProfileDevelopment,
				Version:  initialVersion,
				Image:    fmt.Sprintf("openbao/openbao:%s", initialVersion),
				Replicas: 3,
				UpdateStrategy: openbaov1alpha1.UpdateStrategy{
					Type: openbaov1alpha1.UpdateStrategyBlueGreen,
					BlueGreen: &openbaov1alpha1.BlueGreenConfig{
						AutoPromote: true,
						Verification: &openbaov1alpha1.VerificationConfig{
							MinSyncDuration: "10s",
						},
						AutoRollback: &openbaov1alpha1.AutoRollbackConfig{
							Enabled:             true,
							OnJobFailure:        true,
							OnValidationFailure: true,
						},
					},
				},
				InitContainer: &openbaov1alpha1.InitContainerConfig{
					Enabled: true,
					Image:   configInitImage,
				},
				SelfInit: &openbaov1alpha1.SelfInitConfig{
					Enabled:          true,
					BootstrapJWTAuth: true,
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
				Upgrade: &openbaov1alpha1.UpgradeConfig{
					ExecutorImage: upgradeExecutorImage,
					JWTAuthRole:   "upgrade",
				},
				Gateway: &openbaov1alpha1.GatewayConfig{
					Enabled: true,
					GatewayRef: openbaov1alpha1.GatewayReference{
						Name: "tenant-gateway",
					},
					Hostname: "bao.example.local",
				},
				DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
			},
		}
		Expect(admin.Create(ctx, upgradeCluster)).To(Succeed())

		By("waiting for Blue/Green cluster to be initialized and available")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Status.Initialized).To(BeTrue(), "cluster should be initialized")
			g.Expect(updated.Status.CurrentVersion).To(Equal(initialVersion), "current version should match initial version")
			g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "BlueGreen status should be initialized")

			available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			g.Expect(available).NotTo(BeNil())
			g.Expect(available.Status).To(Equal(metav1.ConditionTrue), "cluster should be available")
		}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("waiting for all pods to be ready")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "BlueGreen status should be initialized")
			g.Expect(updated.Status.BlueGreen.BlueRevision).NotTo(BeEmpty(), "BlueRevision should be set")

			sts := &appsv1.StatefulSet{}
			err = admin.Get(ctx, types.NamespacedName{
				Name:      fmt.Sprintf("%s-%s", upgradeCluster.Name, updated.Status.BlueGreen.BlueRevision),
				Namespace: tenantNamespace,
			}, sts)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(sts.Status.ReadyReplicas).To(Equal(int32(3)), "all replicas should be ready")
		}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
	})

	AfterAll(func() {
		if tenantFW != nil {
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			_ = tenantFW.Cleanup(cleanupCtx)
		}

		// Best-effort cleanup of Gateway API CRDs after tests.
		_ = UninstallGatewayAPI()
	})

	It("keeps HTTPRoute stable and switches external Service selector at cutover", func() {
		By("triggering a Blue/Green upgrade by updating the spec version")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
			g.Expect(err).NotTo(HaveOccurred())

			original := updated.DeepCopy()
			updated.Spec.Version = targetVersion
			updated.Spec.Image = fmt.Sprintf("openbao/openbao:%s", targetVersion)

			err = admin.Patch(ctx, updated, client.MergeFrom(original))
			g.Expect(err).NotTo(HaveOccurred())
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("waiting for the HTTPRoute to exist and target only the main Service")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "BlueGreen status should be initialized")

			route := &gatewayv1.HTTPRoute{}
			err = admin.Get(ctx, types.NamespacedName{
				Namespace: tenantNamespace,
				Name:      fmt.Sprintf("%s-httproute", upgradeCluster.Name),
			}, route)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(route.Spec.Rules).NotTo(BeEmpty())

			backends := route.Spec.Rules[0].BackendRefs
			g.Expect(backends).To(HaveLen(1))
			g.Expect(string(backends[0].Name)).To(Equal(fmt.Sprintf("%s-public", upgradeCluster.Name)))
			// Some Gateway API implementations default Weight to 1 when unset.
			if backends[0].Weight != nil {
				g.Expect(*backends[0].Weight).To(Equal(int32(1)))
			}
		}, 10*time.Minute, framework.DefaultPollInterval).Should(Succeed())

		By("waiting for cutover (DemotingBlue) and verifying the external Service selector switches to Green")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "BlueGreen status should be initialized")

			if updated.Status.BlueGreen.Phase != openbaov1alpha1.PhaseDemotingBlue {
				_, _ = fmt.Fprintf(GinkgoWriter, "Current phase: %s\n", updated.Status.BlueGreen.Phase)
				g.Expect(updated.Status.BlueGreen.Phase).ToNot(BeEmpty())
				return
			}

			g.Expect(updated.Status.BlueGreen.GreenRevision).NotTo(BeEmpty(), "GreenRevision should be set at cutover")

			svc := &corev1.Service{}
			err = admin.Get(ctx, types.NamespacedName{
				Namespace: tenantNamespace,
				Name:      fmt.Sprintf("%s-public", upgradeCluster.Name),
			}, svc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(svc.Spec.Selector).To(HaveKeyWithValue(constants.LabelOpenBaoRevision, updated.Status.BlueGreen.GreenRevision))
		}, 20*time.Minute, framework.DefaultPollInterval).Should(Succeed())

		By("waiting for Blue/Green upgrade to complete")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "BlueGreen status should be initialized")

			// Explicitly require PhaseIdle - this will retry until phase is Idle
			g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle), "upgrade should complete and return to Idle")
			g.Expect(updated.Status.CurrentVersion).To(Equal(targetVersion))
			g.Expect(updated.Status.BlueGreen.GreenRevision).To(BeEmpty())

			// Verify pods with the final Blue revision are healthy.
			labelSelector := labels.SelectorFromSet(map[string]string{
				constants.LabelAppInstance:     upgradeCluster.Name,
				constants.LabelAppName:         constants.LabelValueAppNameOpenBao,
				constants.LabelOpenBaoRevision: updated.Status.BlueGreen.BlueRevision,
			})
			podList := &corev1.PodList{}
			err = admin.List(ctx, podList, client.InNamespace(tenantNamespace), client.MatchingLabelsSelector{Selector: labelSelector})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(len(podList.Items)).To(Equal(3))

			for _, pod := range podList.Items {
				g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning))
			}
		}, 30*time.Minute, 30*time.Second).Should(Succeed())

		By("verifying legacy blue/green Services do not exist")
		Eventually(func(g Gomega) {
			// Blue service should not exist
			blueSvc := &corev1.Service{}
			err := admin.Get(ctx, types.NamespacedName{
				Namespace: tenantNamespace,
				Name:      fmt.Sprintf("%s-public-blue", upgradeCluster.Name),
			}, blueSvc)
			g.Expect(err).To(HaveOccurred())
			g.Expect(client.IgnoreNotFound(err)).To(Succeed(), "blue service should be deleted")

			// Green service should not exist
			greenSvc := &corev1.Service{}
			err = admin.Get(ctx, types.NamespacedName{
				Namespace: tenantNamespace,
				Name:      fmt.Sprintf("%s-public-green", upgradeCluster.Name),
			}, greenSvc)
			g.Expect(err).To(HaveOccurred())
			g.Expect(client.IgnoreNotFound(err)).To(Succeed(), "green service should be deleted")

			// Main public service should still exist
			mainSvc := &corev1.Service{}
			err = admin.Get(ctx, types.NamespacedName{
				Namespace: tenantNamespace,
				Name:      fmt.Sprintf("%s-public", upgradeCluster.Name),
			}, mainSvc)
			g.Expect(err).NotTo(HaveOccurred(), "main public service should still exist")
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying BackendTLSPolicy only targets main service after cleanup")
		Eventually(func(g Gomega) {
			policy := &gatewayv1.BackendTLSPolicy{}
			err := admin.Get(ctx, types.NamespacedName{
				Namespace: tenantNamespace,
				Name:      fmt.Sprintf("%s-backend-tls-policy", upgradeCluster.Name),
			}, policy)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(policy.Spec.TargetRefs).To(HaveLen(1), "BackendTLSPolicy should only target main service after upgrade")
			g.Expect(string(policy.Spec.TargetRefs[0].Name)).To(Equal(fmt.Sprintf("%s-public", upgradeCluster.Name)))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
	})
})

func ptrTo[T any](v T) *T {
	return &v
}
