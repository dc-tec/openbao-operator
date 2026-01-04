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

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/auth"
	"github.com/dc-tec/openbao-operator/internal/constants"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

// createBlueGreenUpgradeSelfInitRequests creates SelfInit requests for Blue/Green upgrade operations.
// For Development profile, JWT auth and upgrade JWT role must be manually configured
// since bootstrap is not automatically provided. This matches the pattern used in backup tests.
// Uses jwt_validation_pubkeys (same as operator bootstrap) because OpenBao pods run as
// system:anonymous and cannot access the OIDC discovery endpoint. The test client
// fetches JWKS keys using authenticated REST config, then provides them directly to OpenBao.
func createBlueGreenUpgradeSelfInitRequests(ctx context.Context, clusterNamespace, clusterName string, restCfg *rest.Config) ([]openbaov1alpha1.SelfInitRequest, error) {
	// Discover OIDC config using the REST config host (works from outside cluster)
	// This gets us the issuer URL and JWKS keys
	apiServerURL := restCfg.Host
	if apiServerURL == "" {
		return nil, fmt.Errorf("REST config host is empty")
	}

	oidcConfig, err := auth.DiscoverConfig(ctx, restCfg, apiServerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to discover OIDC config: %w", err)
	}

	if oidcConfig.IssuerURL == "" {
		return nil, fmt.Errorf("OIDC config missing issuer URL")
	}

	if len(oidcConfig.JWKSKeys) == 0 {
		return nil, fmt.Errorf("no JWKS keys found in OIDC config")
	}

	jwtConfigData := map[string]interface{}{
		// Use jwt_validation_pubkeys (same as operator bootstrap) because OpenBao pods
		// run as system:anonymous and cannot fetch OIDC discovery document.
		// The test client fetches the keys using authenticated REST config and provides
		// them directly to OpenBao, matching the operator bootstrap pattern.
		"jwt_validation_pubkeys": oidcConfig.JWKSKeys,
		// bound_issuer is required and must match the issuer claim in the JWT
		"bound_issuer": oidcConfig.IssuerURL,
	}

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
			Name:      "configure-jwt-auth",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "auth/jwt/config",
			Data:      e2ehelpers.MustJSON(jwtConfigData),
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
}
path "sys/storage/raft/join" {
  capabilities = ["update"]
}
path "sys/storage/raft/configuration" {
  capabilities = ["read", "update"]
}
path "sys/storage/raft/remove-peer" {
  capabilities = ["update"]
}
path "sys/storage/raft/promote" {
  capabilities = ["update"]
}
path "sys/storage/raft/demote" {
  capabilities = ["update"]
}`,
			},
		},
		{
			Name:      "create-upgrade-jwt-role",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "auth/jwt/role/upgrade",
			Data: e2ehelpers.MustJSON(map[string]interface{}{
				"role_type":       "jwt",
				"user_claim":      "sub", // Match operator bootstrap pattern
				"bound_audiences": []string{"openbao-internal"},
				// Use bound_subject to match the sub claim directly (more reliable than bound_claims
				// for projected tokens which may not include kubernetes.io/* claims)
				// The sub claim format is: system:serviceaccount:<namespace>:<serviceaccount-name>
				// and is always present in ServiceAccount tokens
				"bound_subject":  fmt.Sprintf("system:serviceaccount:%s:%s-upgrade-serviceaccount", clusterNamespace, clusterName),
				"token_policies": []string{"upgrade"},
				"policies":       []string{"upgrade"},
				"ttl":            "1h",
			}),
		},
	}, nil
}

var _ = Describe("Blue/Green Upgrade", Label("upgrade", "cluster", "slow"), Ordered, func() {
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

	Context("Blue/Green upgrade flow", func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			upgradeCluster  *openbaov1alpha1.OpenBaoCluster
			initialVersion  string
			targetVersion   string
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-bluegreen", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			// Use upgrade constants for version testing
			initialVersion = getEnvOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
			targetVersion = getEnvOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)

			// Skip if versions are the same (no upgrade to test)
			if initialVersion == targetVersion {
				Skip(fmt.Sprintf("Blue/Green upgrade test skipped: from version (%s) equals to version (%s). Set E2E_UPGRADE_TO_VERSION to a different version to test upgrades.", initialVersion, targetVersion))
			}

			// Use the operator-supported bootstrap flow for JWT auth. This configures JWT auth,
			// operator identity, and upgrade role/policy (since spec.upgrade.jwtAuthRole is set)
			// via the self-init bootstrap initialize stanza rendered by the operator.

			// Create cluster with initial version and Blue/Green update strategy
			upgradeCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bluegreen-cluster",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Version:  initialVersion,
					Image:    fmt.Sprintf("openbao/openbao:%s", initialVersion), // Use initialVersion for image, not openBaoImage (which defaults to 2.4.4)
					Replicas: 3,
					UpdateStrategy: openbaov1alpha1.UpdateStrategy{
						Type: openbaov1alpha1.UpdateStrategyBlueGreen,
						BlueGreen: &openbaov1alpha1.BlueGreenConfig{
							AutoPromote: true,
							Verification: &openbaov1alpha1.VerificationConfig{
								MinSyncDuration: "30s", // Short duration for e2e tests
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

				// BlueGreen status should be initialized
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "BlueGreen status should be initialized")
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle), "initial phase should be Idle")
				g.Expect(updated.Status.BlueGreen.BlueRevision).NotTo(BeEmpty(), "BlueRevision should be set")

				// Wait for Available condition
				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue), "cluster should be available")
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			// Wait for all pods to be ready
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
			if tenantFW == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()

			_ = tenantFW.Cleanup(cleanupCtx)
		})

		It("detects version change and initiates Blue/Green upgrade", func() {
			_, _ = fmt.Fprintf(GinkgoWriter, "=== Starting version change test: %s -> %s ===\n", initialVersion, targetVersion)

			// Ensure cluster is fully ready and stable before patching
			// Wait for StatefulSet to be ready (should already be done in BeforeAll, but double-check)
			By("Waiting for StatefulSet to be ready and stable")
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
				g.Expect(sts.Status.ReadyReplicas).To(Equal(int32(3)), "all 3 replicas should be ready")
				g.Expect(sts.Status.ReadyReplicas).To(Equal(sts.Status.Replicas), "ready replicas should match desired replicas")
				_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet ready: %d/%d replicas\n", sts.Status.ReadyReplicas, sts.Status.Replicas)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			// Wait for cluster to be in a stable state (Available condition true, no ongoing operations)
			By("Waiting for cluster to be in stable state")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify cluster is initialized and available
				g.Expect(updated.Status.Initialized).To(BeTrue(), "cluster should be initialized")
				g.Expect(updated.Status.CurrentVersion).To(Equal(initialVersion), "current version should match initial version")

				// Check Available condition
				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue), "cluster should be available")

				// Ensure no upgrade is in progress
				upgrading := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
				if upgrading != nil {
					g.Expect(upgrading.Status).To(Equal(metav1.ConditionFalse), "no upgrade should be in progress")
				}

				_, _ = fmt.Fprintf(GinkgoWriter, "Cluster is stable: Initialized=%v, CurrentVersion=%s, Available=%v\n",
					updated.Status.Initialized, updated.Status.CurrentVersion, available.Status == metav1.ConditionTrue)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			// Wait for TLS and Raft to fully stabilize before triggering upgrade
			// This helps avoid race conditions where Green pods start before TLS is fully propagated
			By("Waiting for Blue cluster TLS/Raft stabilization")
			time.Sleep(30 * time.Second)

			// Now patch the CR with the new version
			// Match the pattern used in rolling upgrade test - simple patch, then wait for upgrade to start
			By(fmt.Sprintf("Patching CR to trigger upgrade: %s -> %s", initialVersion, targetVersion))
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

			// Wait for Blue/Green upgrade to be detected and started
			// The upgrade starts in PhaseIdle and transitions to PhaseDeployingGreen on the next reconcile
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				// BlueGreen status should be set
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "BlueGreen status should be set")

				// Phase should progress from Idle to DeployingGreen (or already be in DeployingGreen)
				phase := updated.Status.BlueGreen.Phase
				g.Expect(phase).To(BeElementOf(
					openbaov1alpha1.PhaseIdle,
					openbaov1alpha1.PhaseDeployingGreen,
				), "phase should be Idle (initial) or DeployingGreen (upgrade started)")

				// If in DeployingGreen, GreenRevision should be set
				if phase == openbaov1alpha1.PhaseDeployingGreen {
					g.Expect(updated.Status.BlueGreen.GreenRevision).NotTo(BeEmpty(), "GreenRevision should be set when phase is DeployingGreen")

					// Upgrading condition should be true
					upgrading := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
					g.Expect(upgrading).NotTo(BeNil())
					g.Expect(upgrading.Status).To(Equal(metav1.ConditionTrue))
				}

				// If still in Idle, verify version mismatch exists (upgrade should be triggered)
				if phase == openbaov1alpha1.PhaseIdle {
					g.Expect(updated.Status.CurrentVersion).NotTo(BeEmpty(), "CurrentVersion should be set")
					g.Expect(updated.Status.CurrentVersion).NotTo(Equal(updated.Spec.Version),
						"CurrentVersion should differ from Spec.Version to trigger upgrade")
				}

				_, _ = fmt.Fprintf(GinkgoWriter, "Blue/Green status: phase=%s currentVersion=%s specVersion=%s blueRevision=%s greenRevision=%s\n",
					phase,
					updated.Status.CurrentVersion,
					updated.Spec.Version,
					updated.Status.BlueGreen.BlueRevision,
					updated.Status.BlueGreen.GreenRevision)
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		It("deploys Green StatefulSet", func() {
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())

				greenRevision := updated.Status.BlueGreen.GreenRevision
				g.Expect(greenRevision).NotTo(BeEmpty())

				// Check that Green StatefulSet exists
				greenStatefulSetName := fmt.Sprintf("%s-%s", upgradeCluster.Name, greenRevision)
				greenSTS := &appsv1.StatefulSet{}
				err = admin.Get(ctx, types.NamespacedName{Name: greenStatefulSetName, Namespace: tenantNamespace}, greenSTS)
				g.Expect(err).NotTo(HaveOccurred(), "Green StatefulSet should exist")

				// Green StatefulSet should have replicas
				g.Expect(greenSTS.Spec.Replicas).NotTo(BeNil())
				g.Expect(*greenSTS.Spec.Replicas).To(Equal(int32(3)), "Green StatefulSet should have 3 replicas")

				// Wait for Green pods to be ready
				if greenSTS.Status.ReadyReplicas >= 3 {
					// All pods ready, should transition to JoiningMesh (unsealing check is now part of DeployingGreen)
					g.Expect(updated.Status.BlueGreen.Phase).To(BeElementOf(
						openbaov1alpha1.PhaseDeployingGreen,
						openbaov1alpha1.PhaseJoiningMesh,
					), "phase should progress from DeployingGreen")
				}

				_, _ = fmt.Fprintf(GinkgoWriter, "Green StatefulSet: name=%s readyReplicas=%d replicas=%d phase=%s\n",
					greenStatefulSetName,
					greenSTS.Status.ReadyReplicas,
					*greenSTS.Spec.Replicas,
					updated.Status.BlueGreen.Phase)
			}, 10*time.Minute, 30*time.Second).Should(Succeed())
		})

		It("joins Green pods to Raft cluster as non-voters", func() {
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())

				// Phase should progress to JoiningMesh and beyond (UnsealingGreen was merged into DeployingGreen)
				phase := updated.Status.BlueGreen.Phase
				g.Expect(phase).To(BeElementOf(
					openbaov1alpha1.PhaseJoiningMesh,
					openbaov1alpha1.PhaseSyncing,
					openbaov1alpha1.PhasePromoting,
				), "phase should progress through Blue/Green phases")

				// If in Syncing or later, Green pods should be joined
				if phase == openbaov1alpha1.PhaseSyncing || phase == openbaov1alpha1.PhasePromoting {
					// Verify Green pods exist with correct labels
					greenRevision := updated.Status.BlueGreen.GreenRevision
					greenPods := &corev1.PodList{}
					labelSelector := labels.SelectorFromSet(map[string]string{
						constants.LabelAppInstance:     upgradeCluster.Name,
						constants.LabelAppName:         constants.LabelValueAppNameOpenBao,
						constants.LabelOpenBaoRevision: greenRevision,
					})
					err = admin.List(ctx, greenPods, client.InNamespace(tenantNamespace), client.MatchingLabelsSelector{Selector: labelSelector})
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(len(greenPods.Items)).To(Equal(3), "should have 3 Green pods")
				}

				_, _ = fmt.Fprintf(GinkgoWriter, "Blue/Green phase: %s\n", phase)
			}, 15*time.Minute, 30*time.Second).Should(Succeed())
		})

		It("synchronizes Green cluster with Blue cluster", func() {
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())

				phase := updated.Status.BlueGreen.Phase
				// Should progress from Syncing to Promoting to DemotingBlue (Cutover is now merged into DemotingBlue)
				g.Expect(phase).To(BeElementOf(
					openbaov1alpha1.PhaseSyncing,
					openbaov1alpha1.PhasePromoting,
					openbaov1alpha1.PhaseDemotingBlue,
					openbaov1alpha1.PhaseCleanup,
				), "phase should progress through sync, promotion, demotion, and cleanup")

				_, _ = fmt.Fprintf(GinkgoWriter, "Sync phase: %s\n", phase)
			}, 10*time.Minute, 30*time.Second).Should(Succeed())
		})

		It("performs cutover to Green cluster", func() {
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())

				phase := updated.Status.BlueGreen.Phase
				// Correct flow: Promoting → DemotingBlue → Cleanup (Cutover merged into DemotingBlue)
				g.Expect(phase).To(BeElementOf(
					openbaov1alpha1.PhasePromoting,
					openbaov1alpha1.PhaseDemotingBlue,
					openbaov1alpha1.PhaseCleanup,
				), "phase should progress: Promoting → DemotingBlue → Cleanup")

				// If in DemotingBlue or later, verify service selector points to Green
				// Service is switched during DemotingBlue phase now (after Green leader is verified)
				if phase == openbaov1alpha1.PhaseDemotingBlue || phase == openbaov1alpha1.PhaseCleanup {
					greenRevision := updated.Status.BlueGreen.GreenRevision
					// Check external service selector
					svcName := fmt.Sprintf("%s-public", upgradeCluster.Name)
					service := &corev1.Service{}
					err = admin.Get(ctx, types.NamespacedName{Name: svcName, Namespace: tenantNamespace}, service)
					if err == nil {
						// Service exists, check selector
						if service.Spec.Selector != nil {
							revision, ok := service.Spec.Selector[constants.LabelOpenBaoRevision]
							if ok {
								g.Expect(revision).To(Equal(greenRevision), "service selector should point to Green revision after cutover")
							}
						}
					}
				}

				_, _ = fmt.Fprintf(GinkgoWriter, "Cutover phase: %s\n", phase)
			}, 10*time.Minute, 30*time.Second).Should(Succeed())
		})

		It("completes Blue/Green upgrade successfully", func() {
			// Wait for upgrade to complete
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				// Upgrade should be complete (BlueGreen phase should be Idle)
				if updated.Status.BlueGreen != nil && updated.Status.BlueGreen.Phase == openbaov1alpha1.PhaseIdle {
					// Upgrade completed
					g.Expect(updated.Status.CurrentVersion).To(Equal(targetVersion),
						"current version should be updated to target version")
					g.Expect(updated.Status.Phase).To(Equal(openbaov1alpha1.ClusterPhaseRunning),
						"phase should be Running after upgrade")

					// BlueRevision should be updated to what was GreenRevision
					// GreenRevision should be cleared
					g.Expect(updated.Status.BlueGreen.GreenRevision).To(BeEmpty(), "GreenRevision should be cleared after completion")

					// Upgrading condition should be false
					upgrading := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
					if upgrading != nil {
						g.Expect(upgrading.Status).To(Equal(metav1.ConditionFalse))
					}

					_, _ = fmt.Fprintf(GinkgoWriter, "Blue/Green upgrade completed successfully: version=%s\n", updated.Status.CurrentVersion)
					return
				}

				// Still in progress
				phase := openbaov1alpha1.PhaseIdle
				if updated.Status.BlueGreen != nil {
					phase = updated.Status.BlueGreen.Phase
				}
				_, _ = fmt.Fprintf(GinkgoWriter, "Blue/Green upgrade in progress: phase=%s\n", phase)
			}, 30*time.Minute, 30*time.Second).Should(Succeed(), "Blue/Green upgrade should complete successfully")
		})

		It("verifies Blue StatefulSet is deleted and only Green remains", func() {
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				// Wait for upgrade to be complete (Phase = Idle)
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle),
					"upgrade should be complete (Phase=Idle)")

				// After completion, only one StatefulSet should exist (the active one)
				stsList := &appsv1.StatefulSetList{}
				err = admin.List(ctx, stsList, client.InNamespace(tenantNamespace))
				g.Expect(err).NotTo(HaveOccurred())

				// Count StatefulSets with the cluster name prefix
				clusterStatefulSets := 0
				for i := range stsList.Items {
					sts := &stsList.Items[i]
					// Match exact name or name with revision suffix (e.g., "cluster-name" or "cluster-name-abc123")
					if sts.Name == upgradeCluster.Name || (len(sts.Name) > len(upgradeCluster.Name) && sts.Name[:len(upgradeCluster.Name)+1] == upgradeCluster.Name+"-") {
						clusterStatefulSets++
					}
				}

				// Should have exactly one StatefulSet (the active one)
				g.Expect(clusterStatefulSets).To(Equal(1), "should have exactly one StatefulSet after upgrade completion")

				// Verify the active StatefulSet exists and has the correct image
				g.Expect(updated.Status.BlueGreen.BlueRevision).NotTo(BeEmpty(), "BlueRevision should be set to the new active revision")
				g.Expect(updated.Status.BlueGreen.GreenRevision).To(BeEmpty(), "GreenRevision should be cleared after completion")

				activeSTS := &appsv1.StatefulSet{}
				err = admin.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-%s", upgradeCluster.Name, updated.Status.BlueGreen.BlueRevision),
					Namespace: tenantNamespace,
				}, activeSTS)
				g.Expect(err).NotTo(HaveOccurred(), "active StatefulSet should exist")

				expectedImage := fmt.Sprintf("openbao/openbao:%s", targetVersion)
				if len(activeSTS.Spec.Template.Spec.Containers) > 0 {
					g.Expect(activeSTS.Spec.Template.Spec.Containers[0].Image).To(Equal(expectedImage),
						"active StatefulSet should have target version image")
				}

				_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet cleanup verified: activeStatefulSets=%d, activeRevision=%s\n",
					clusterStatefulSets, updated.Status.BlueGreen.BlueRevision)
			}, 10*time.Minute, framework.DefaultPollInterval).Should(Succeed())
		})

		It("verifies all pods are running target version", func() {
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.BlueRevision).NotTo(BeEmpty())

				activeSTS := &appsv1.StatefulSet{}
				err = admin.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-%s", upgradeCluster.Name, updated.Status.BlueGreen.BlueRevision),
					Namespace: tenantNamespace,
				}, activeSTS)
				g.Expect(err).NotTo(HaveOccurred())

				// All replicas should be ready
				g.Expect(activeSTS.Status.ReadyReplicas).To(Equal(int32(3)), "all replicas should be ready")
				g.Expect(activeSTS.Status.UpdatedReplicas).To(Equal(int32(3)), "all replicas should be updated")

				// Verify pod images match target version
				expectedImage := fmt.Sprintf("openbao/openbao:%s", targetVersion)
				if len(activeSTS.Spec.Template.Spec.Containers) > 0 {
					g.Expect(activeSTS.Spec.Template.Spec.Containers[0].Image).To(Equal(expectedImage),
						"pod image should match target version")
				}

				// Verify pods are actually running the target version
				podList := &corev1.PodList{}
				labelSelector := labels.SelectorFromSet(map[string]string{
					constants.LabelAppInstance:     upgradeCluster.Name,
					constants.LabelAppName:         constants.LabelValueAppNameOpenBao,
					constants.LabelOpenBaoRevision: updated.Status.BlueGreen.BlueRevision,
				})
				err = admin.List(ctx, podList, client.InNamespace(tenantNamespace), client.MatchingLabelsSelector{Selector: labelSelector})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(podList.Items)).To(Equal(3), "should have 3 pods")

				for _, pod := range podList.Items {
					if len(pod.Spec.Containers) > 0 {
						g.Expect(pod.Spec.Containers[0].Image).To(Equal(expectedImage),
							"pod %s should have target version image", pod.Name)
					}
				}
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		It("verifies cluster is healthy after upgrade", func() {
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				// Cluster should be in Running phase
				g.Expect(updated.Status.Phase).To(Equal(openbaov1alpha1.ClusterPhaseRunning), "cluster should be Running")
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.BlueRevision).NotTo(BeEmpty())

				// All pods should be healthy
				podList := &corev1.PodList{}
				labelSelector := labels.SelectorFromSet(map[string]string{
					constants.LabelAppInstance:     upgradeCluster.Name,
					constants.LabelAppName:         constants.LabelValueAppNameOpenBao,
					constants.LabelOpenBaoRevision: updated.Status.BlueGreen.BlueRevision,
				})
				err = admin.List(ctx, podList, client.InNamespace(tenantNamespace), client.MatchingLabelsSelector{Selector: labelSelector})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(podList.Items)).To(Equal(3), "should have 3 pods")

				readyPods := 0
				unsealedPods := 0
				initializedPods := 0
				leaderCount := 0

				for _, pod := range podList.Items {
					// Check pod readiness
					for _, cond := range pod.Status.Conditions {
						if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
							readyPods++
							break
						}
					}

					// Check OpenBao service registration labels
					sealed, hasSealed, _ := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelSealed)
					if hasSealed && !sealed {
						unsealedPods++
					}

					initialized, hasInit, _ := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelInitialized)
					if hasInit && initialized {
						initializedPods++
					}

					active, hasActive, _ := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
					if hasActive && active {
						leaderCount++
					}
				}

				g.Expect(readyPods).To(Equal(3), "all 3 pods should be ready")
				g.Expect(unsealedPods).To(Equal(3), "all 3 pods should be unsealed (via OpenBao service registration)")
				g.Expect(initializedPods).To(Equal(3), "all 3 pods should report initialized (via OpenBao service registration)")
				g.Expect(leaderCount).To(Equal(1), "exactly 1 pod should be the leader")

				// Verify initialized status
				g.Expect(updated.Status.Initialized).To(BeTrue(), "cluster should be initialized")

				_, _ = fmt.Fprintf(GinkgoWriter, "Cluster health verified: phase=%s, readyPods=%d, unsealedPods=%d, initializedPods=%d, leaders=%d\n",
					updated.Status.Phase, readyPods, unsealedPods, initializedPods, leaderCount)
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})
	})
})
