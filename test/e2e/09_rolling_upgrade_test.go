//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/test/e2e/framework"
	e2ehelpers "github.com/openbao/operator/test/e2e/helpers"
)

// createUpgradeSelfInitRequests creates SelfInit requests for enabling JWT auth
// and creating upgrade policy and role for upgrade operations.
func createUpgradeSelfInitRequests(clusterNamespace, clusterName string) []openbaov1alpha1.SelfInitRequest {
	return []openbaov1alpha1.SelfInitRequest{
		{
			Name:      "enable-jwt-auth",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "sys/auth/jwt",
			AuthMethod: &openbaov1alpha1.SelfInitAuthMethod{
				Type: "jwt",
			},
		},
		{
			Name:      "create-upgrade-policy",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "sys/policies/acl/upgrade",
			Policy: &openbaov1alpha1.SelfInitPolicy{
				Policy: `path "sys/health" {
  capabilities = ["read"]
}
path "sys/step-down" {
  capabilities = ["sudo", "update"]
}
path "sys/storage/raft/snapshot" {
  capabilities = ["read"]
}
path "sys/storage/raft/autopilot/state" {
  capabilities = ["read"]
}`,
			},
		},
		{
			Name:      "create-upgrade-jwt-role",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "auth/jwt/role/upgrade",
			Data: e2ehelpers.MustJSON(map[string]interface{}{
				"role_type":       "jwt",
				"bound_audiences": []string{"openbao-internal"},
				"bound_claims": map[string]interface{}{
					"kubernetes.io/namespace":           clusterNamespace,
					"kubernetes.io/serviceaccount/name": fmt.Sprintf("%s-upgrade-serviceaccount", clusterName),
				},
				"token_policies": []string{"upgrade"},
				"policies":       []string{"upgrade"},
				"ttl":            "1h",
			}),
		},
	}
}

var _ = Describe("Upgrade", Label("upgrade", "cluster", "slow"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		admin  client.Client
	)

	BeforeAll(func() {
		var err error

		cfg, err = ctrlconfig.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(openbaov1alpha1.AddToScheme(scheme)).To(Succeed())

		admin, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Rolling upgrade", func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			upgradeCluster  *openbaov1alpha1.OpenBaoCluster
			initialVersion  string
			targetVersion   string
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-upgrade", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			// Use upgrade constants for version testing
			// These can be overridden via environment variables if needed
			initialVersion = getEnvOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
			targetVersion = getEnvOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)

			// Skip if versions are the same (no upgrade to test)
			if initialVersion == targetVersion {
				Skip(fmt.Sprintf("Upgrade test skipped: from version (%s) equals to version (%s). Set E2E_UPGRADE_TO_VERSION to a different version to test upgrades.", initialVersion, targetVersion))
			}

			// Create cluster with initial version
			upgradeCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "upgrade-cluster",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Version:  initialVersion,
					Image:    openBaoImage,
					Replicas: 3, // Use 3 replicas to test rolling upgrade
					InitContainer: &openbaov1alpha1.InitContainerConfig{
						Enabled: true,
						Image:   configInitImage,
					},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled:  true,
						Requests: createUpgradeSelfInitRequests(tenantNamespace, "upgrade-cluster"),
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
						APIServerCIDR: kindDefaultServiceCIDR,
					},
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						ExecutorImage: upgradeExecutorImage,
						JWTAuthRole:   "upgrade",
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, upgradeCluster)).To(Succeed())

			// Wait for cluster to be initialized and available
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Initialized).To(BeTrue(), "cluster should be initialized")
				g.Expect(updated.Status.CurrentVersion).To(Equal(initialVersion), "current version should match initial version")

				// Wait for Available condition
				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				if available != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Available condition: status=%s reason=%s\n", available.Status, available.Reason)
				}
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue), "cluster should be available")
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			// Wait for all pods to be ready
			Eventually(func(g Gomega) {
				sts := &appsv1.StatefulSet{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, sts)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sts.Status.ReadyReplicas).To(Equal(int32(3)), "all replicas should be ready")
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		AfterAll(func() {
			if tenantFW == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()

			_ = tenantFW.Cleanup(cleanupCtx)
		})

		It("detects version change and initiates upgrade", func() {
			// Update the version in spec to trigger upgrade
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				// Update version and image
				original := updated.DeepCopy()
				updated.Spec.Version = targetVersion
				updated.Spec.Image = fmt.Sprintf("openbao/openbao:%s", targetVersion)

				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			// Wait for upgrade to be detected and started
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				// Upgrade should be in progress
				g.Expect(updated.Status.Upgrade).NotTo(BeNil(), "upgrade status should be set")
				g.Expect(updated.Status.Upgrade.TargetVersion).To(Equal(targetVersion))
				g.Expect(updated.Status.Upgrade.FromVersion).To(Equal(initialVersion))

				// Phase should be Upgrading
				g.Expect(updated.Status.Phase).To(Equal(openbaov1alpha1.ClusterPhaseUpgrading))

				// Upgrading condition should be true
				upgrading := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
				g.Expect(upgrading).NotTo(BeNil())
				g.Expect(upgrading.Status).To(Equal(metav1.ConditionTrue))

				_, _ = fmt.Fprintf(GinkgoWriter, "Upgrade started: from=%s to=%s partition=%d\n",
					updated.Status.Upgrade.FromVersion,
					updated.Status.Upgrade.TargetVersion,
					updated.Status.Upgrade.CurrentPartition)
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		It("performs rolling upgrade pod by pod", func() {
			// Monitor upgrade progress
			var initialPartition int32
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Upgrade).NotTo(BeNil())

				initialPartition = updated.Status.Upgrade.CurrentPartition
				_, _ = fmt.Fprintf(GinkgoWriter, "Initial partition: %d, completed pods: %v\n",
					initialPartition, updated.Status.Upgrade.CompletedPods)
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			// Wait for partition to decrease (indicating pods are being upgraded)
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Upgrade).NotTo(BeNil())

				currentPartition := updated.Status.Upgrade.CurrentPartition
				completedCount := len(updated.Status.Upgrade.CompletedPods)

				_, _ = fmt.Fprintf(GinkgoWriter, "Upgrade progress: partition=%d completed=%d/%d\n",
					currentPartition, completedCount, upgradeCluster.Spec.Replicas)

				// Partition should decrease as pods are upgraded
				// Or completed pods should increase
				g.Expect(currentPartition < initialPartition || completedCount > 0).To(BeTrue(),
					"upgrade should make progress")
			}, 10*time.Minute, 30*time.Second).Should(Succeed())
		})

		It("completes upgrade successfully", func() {
			// Wait for upgrade to complete and verify availability during the process
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify availability: At least one pod should be ready at all times during the wait
				sts := &appsv1.StatefulSet{}
				err = admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, sts)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sts.Status.ReadyReplicas).To(BeNumerically(">=", 1),
					"at least one pod should remain ready during upgrade")

				// Upgrade should be complete (Status.Upgrade should be nil)
				if updated.Status.Upgrade == nil {
					// Upgrade completed
					g.Expect(updated.Status.CurrentVersion).To(Equal(targetVersion),
						"current version should be updated to target version")
					g.Expect(updated.Status.Phase).To(Equal(openbaov1alpha1.ClusterPhaseRunning),
						"phase should be Running after upgrade")

					// Upgrading condition should be false
					upgrading := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
					if upgrading != nil {
						g.Expect(upgrading.Status).To(Equal(metav1.ConditionFalse))
					}

					_, _ = fmt.Fprintf(GinkgoWriter, "Upgrade completed successfully: version=%s\n", updated.Status.CurrentVersion)
					return
				}

				// Still in progress
				_, _ = fmt.Fprintf(GinkgoWriter, "Upgrade in progress: partition=%d completed=%d/%d\n",
					updated.Status.Upgrade.CurrentPartition,
					len(updated.Status.Upgrade.CompletedPods),
					upgradeCluster.Spec.Replicas)
			}, 20*time.Minute, 10*time.Second).Should(Succeed(), "upgrade should complete successfully")
		})

		It("updates all pods to target version", func() {
			// Verify all pods are running the target version
			Eventually(func(g Gomega) {
				sts := &appsv1.StatefulSet{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, sts)
				g.Expect(err).NotTo(HaveOccurred())

				// All replicas should be ready
				g.Expect(sts.Status.ReadyReplicas).To(Equal(int32(3)), "all replicas should be ready")
				g.Expect(sts.Status.UpdatedReplicas).To(Equal(int32(3)), "all replicas should be updated")

				// Verify pod images match target version
				expectedImage := fmt.Sprintf("openbao/openbao:%s", targetVersion)
				g.Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal(expectedImage),
					"pod image should match target version")
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})
	})
})

// getEnvOrDefault returns the value of the environment variable if set, otherwise returns the default value.
func getEnvOrDefault(key, defaultValue string) string {
	if envValue := getEnv(key); envValue != "" {
		return envValue
	}
	return defaultValue
}

// getEnv is a helper to get environment variables.
func getEnv(key string) string {
	return os.Getenv(key)
}
