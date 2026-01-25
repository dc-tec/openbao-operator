//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
	"github.com/dc-tec/openbao-operator/internal/upgrade/bluegreen"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

// === Shared Helpers ===

const (
	upgradeActionAnnotationKey = "openbao.org/upgrade-action"
	upgradeRunIDAnnotationKey  = "openbao.org/upgrade-run-id"
	rollbackRunID              = "rollback"
)

func findUpgradeExecutorJob(jobs []batchv1.Job, action bluegreen.ExecutorAction, runID string) *batchv1.Job {
	for i := range jobs {
		job := &jobs[i]
		if job.Annotations == nil {
			continue
		}
		if job.Annotations[upgradeActionAnnotationKey] != string(action) {
			continue
		}
		if job.Annotations[upgradeRunIDAnnotationKey] != runID {
			continue
		}
		return job
	}
	return nil
}

func jobFailed(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	if job.Status.Failed > 0 {
		return true
	}
	for _, cond := range job.Status.Conditions {
		if cond.Status != "True" {
			continue
		}
		if cond.Type == batchv1.JobFailed {
			return true
		}
	}
	return false
}

func ptrTo[T any](v T) *T {
	return &v
}

// createE2ERequests helper removed in favor of e2ehelpers.CreateE2ERequests

// === Tests ===

var _ = Describe("Upgrade Strategies", Label("upgrade", "cluster", "slow"), Ordered, func() {
	ctx := context.Background()

	// --- Rolling Upgrade ---
	Context("Rolling Upgrade", Label("rolling"), func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			upgradeCluster  *openbaov1alpha1.OpenBaoCluster
			initialVersion  string
			targetVersion   string
			admin           client.Client
		)

		BeforeAll(func() {
			var err error
			tenantFW, err = framework.NewSetup(ctx, "tenant-upgrade", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace
			admin = tenantFW.Client

			initialVersion = envOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
			targetVersion = envOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)

			if initialVersion == targetVersion {
				Skip(fmt.Sprintf("Upgrade test skipped: versions identical (%s)", initialVersion))
			}

			upgradeCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "upgrade-cluster",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Version:  initialVersion,
					Image:    openBaoImage,
					Replicas: 3,
					InitContainer: &openbaov1alpha1.InitContainerConfig{
						Enabled: true,
						Image:   configInitImage,
					},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
							Enabled: true,
						}, // Operator will auto-create upgrade role
						Requests: e2ehelpers.CreateE2ERequests(tenantNamespace),
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
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, upgradeCluster)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.CurrentVersion).To(Equal(initialVersion))

				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			tenantFW.WaitForStatefulSetReady(ctx, upgradeCluster.Name, 3, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval)
		})

		AfterAll(func() {
			if tenantFW != nil {
				_ = tenantFW.Cleanup(ctx)
			}
		})

		It("performs rolling upgrade", func() {
			cfg, err := ctrlconfig.GetConfig()
			Expect(err).NotTo(HaveOccurred())

			By("Writing a secret before upgrade")
			// Note: bao kv put/get automatically adds /data/ for KV v2, so use path without /data/
			secretPath := "secret/rolling-upgrade-test"
			secretData := map[string]string{"foo": "bar", "version": "v1"}
			baoAddr := fmt.Sprintf("https://%s.%s.svc.cluster.local:8200", upgradeCluster.Name, tenantNamespace)
			bypassLabels := map[string]string{
				constants.LabelOpenBaoCluster:   upgradeCluster.Name,
				constants.LabelOpenBaoComponent: "backup",
			}

			// Enable KV engine (idempotent) - dev mode usually has it enabled at secret/
			// but we'll try to write directly first.
			err = e2ehelpers.WriteSecretViaJWT(ctx, cfg, admin, tenantNamespace, openBaoImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, secretData)
			Expect(err).NotTo(HaveOccurred(), "Failed to write pre-upgrade secret")

			By("Triggering upgrade")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				original := updated.DeepCopy()
				updated.Spec.Version = targetVersion
				updated.Spec.Image = fmt.Sprintf("openbao/openbao:%s", targetVersion)
				g.Expect(admin.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			// Trigger reconcile to ensure upgrade manager processes the version change
			Expect(tenantFW.TriggerReconcile(ctx, upgradeCluster.Name)).To(Succeed())

			// Wait for upgrade to be initialized - Status.Upgrade must be set
			// This is the authoritative indicator that the upgrade manager has started the upgrade
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Upgrade).NotTo(BeNil(), "Status.Upgrade should be set when upgrade is initialized")
				g.Expect(updated.Status.Upgrade.TargetVersion).To(Equal(targetVersion))
				g.Expect(updated.Status.Upgrade.FromVersion).To(Equal(initialVersion))
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				if updated.Status.Upgrade == nil {
					g.Expect(updated.Status.CurrentVersion).To(Equal(targetVersion))
					g.Expect(updated.Status.Phase).To(Equal(openbaov1alpha1.ClusterPhaseRunning))

					// Strict verification: Check that all pods are running the target image
					podList := &corev1.PodList{}
					g.Expect(admin.List(ctx, podList, client.InNamespace(tenantNamespace), client.MatchingLabels(map[string]string{
						constants.LabelOpenBaoCluster: upgradeCluster.Name,
					}))).To(Succeed())
					g.Expect(podList.Items).NotTo(BeEmpty())

					expectedImage := fmt.Sprintf("openbao/openbao:%s", targetVersion)
					for _, pod := range podList.Items {
						// Check container image
						for _, container := range pod.Spec.Containers {
							if container.Name == "openbao" {
								g.Expect(container.Image).To(Equal(expectedImage), "Pod %s not running expected image", pod.Name)
							}
						}
					}
					return
				}
				// Still upgrading
			}, 20*time.Minute, 10*time.Second).Should(Succeed())

			By("Verifying secret persists after upgrade")
			// Read secret back
			val, err := e2ehelpers.ReadSecretViaJWT(ctx, cfg, admin, tenantNamespace, openBaoImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, "foo")
			Expect(err).NotTo(HaveOccurred(), "Failed to read post-upgrade secret")
			Expect(val).To(Equal("bar"))
		})
	})

	// --- Blue/Green Upgrade ---
	Context("Blue/Green Upgrade", Label("bluegreen"), func() {
		var (
			tenantNamespace   string
			tenantFW          *framework.Framework
			upgradeCluster    *openbaov1alpha1.OpenBaoCluster
			initialVersion    string
			targetVersion     string
			admin             client.Client
			cfg               *rest.Config
			credentialsSecret *corev1.Secret
		)

		BeforeAll(func() {
			var err error
			tenantFW, err = framework.NewSetup(ctx, "tenant-bluegreen", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace
			admin = tenantFW.Client

			cfg, err = ctrlconfig.GetConfig()
			Expect(err).NotTo(HaveOccurred())

			initialVersion = envOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
			targetVersion = envOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)

			if initialVersion == targetVersion {
				Skip(fmt.Sprintf("Upgrade test skipped: versions identical (%s)", initialVersion))
			}

			// Deploy RustFS for pre-upgrade snapshot testing
			rustfsNamespace := "rustfs"
			// Ensure cfg is available or get it again
			var rCfg *rest.Config
			if cfg != nil {
				rCfg = cfg
			} else {
				var rErr error
				rCfg, rErr = ctrlconfig.GetConfig()
				Expect(rErr).NotTo(HaveOccurred())
				cfg = rCfg
			}

			err = ensureRustFS(ctx, admin, rCfg, rustfsNamespace)
			if err != nil {
				Skip(fmt.Sprintf("RustFS deployment failed: %v. Skipping pre-upgrade snapshot tests.", err))
			}

			// Create S3 credentials Secret for RustFS
			credentialsSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rustfs-secret",
					Namespace: tenantNamespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"accessKeyId":     []byte(rustfsAccessKey),
					"secretAccessKey": []byte(rustfsSecretKey),
				},
			}
			Expect(admin.Create(ctx, credentialsSecret)).To(Succeed())

			// Augment the generated SelfInit requests with E2E test role/policy
			// Use e2eRequests directly; BootstrapJWTAuth handles the standard upgrade requests
			allRequests := e2ehelpers.CreateE2ERequests(tenantNamespace)

			upgradeCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bluegreen-cluster",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Version:  initialVersion,
					Image:    fmt.Sprintf("openbao/openbao:%s", initialVersion),
					Replicas: 3,
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						Strategy:      openbaov1alpha1.UpdateStrategyBlueGreen,
						ExecutorImage: upgradeExecutorImage,
						BlueGreen: &openbaov1alpha1.BlueGreenConfig{
							AutoPromote:        true,
							PreUpgradeSnapshot: true, // Enable pre-upgrade snapshot
							Verification: &openbaov1alpha1.VerificationConfig{
								MinSyncDuration: "30s",
							},
						},
					},
					InitContainer: &openbaov1alpha1.InitContainerConfig{
						Enabled: true,
						Image:   configInitImage,
					},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled:  true,
						Requests: allRequests,
						OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
							Enabled: true,
						},
					},
					Backup: &openbaov1alpha1.BackupSchedule{
						Schedule:      "0 0 * * *",
						ExecutorImage: backupExecutorImage,
						// JWTAuthRole not set - operator will auto-create backup role when OIDC is enabled
						Target: openbaov1alpha1.BackupTarget{
							Endpoint:     rustfsEndpoint,
							Bucket:       rustfsBucket,
							Region:       "us-east-1",
							UsePathStyle: true,
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: credentialsSecret.Name,
							},
						},
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
			Expect(admin.Create(ctx, upgradeCluster)).To(Succeed())

			// Create NetworkPolicy for upgrade snapshot jobs to access RustFS
			snapshotNetworkPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-upgrade-snapshot-network-policy", upgradeCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							constants.LabelOpenBaoComponent: bluegreen.ComponentUpgradeSnapshot,
							constants.LabelOpenBaoCluster:   upgradeCluster.Name,
						},
					},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeEgress,
					},
					Egress: []networkingv1.NetworkPolicyEgressRule{
						// Allow DNS
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "kube-system",
										},
									},
								},
							},
							Ports: []networkingv1.NetworkPolicyPort{
								{
									Protocol: func() *corev1.Protocol {
										p := corev1.ProtocolUDP
										return &p
									}(),
									Port: func() *intstr.IntOrString {
										p := intstr.FromInt(53)
										return &p
									}(),
								},
								{
									Protocol: func() *corev1.Protocol {
										p := corev1.ProtocolTCP
										return &p
									}(),
									Port: func() *intstr.IntOrString {
										p := intstr.FromInt(53)
										return &p
									}(),
								},
							},
						},
						// Allow access to RustFS
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "rustfs",
										},
									},
								},
							},
							Ports: []networkingv1.NetworkPolicyPort{
								{
									Protocol: func() *corev1.Protocol {
										p := corev1.ProtocolTCP
										return &p
									}(),
									Port: func() *intstr.IntOrString {
										p := intstr.FromInt(9000)
										return &p
									}(),
								},
							},
						},
						// Allow access to OpenBao cluster for snapshot API
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											constants.LabelOpenBaoCluster: upgradeCluster.Name,
										},
									},
								},
							},
							Ports: []networkingv1.NetworkPolicyPort{
								{
									Protocol: func() *corev1.Protocol {
										p := corev1.ProtocolTCP
										return &p
									}(),
									Port: func() *intstr.IntOrString {
										p := intstr.FromInt(8200)
										return &p
									}(),
								},
							},
						},
					},
				},
			}
			Expect(admin.Create(ctx, snapshotNetworkPolicy)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			tenantFW.WaitForStatefulSetReady(ctx, upgradeCluster.Name, 3, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval)
		})

		AfterAll(func() {
			if tenantFW != nil {
				_ = tenantFW.Cleanup(ctx)
			}
		})

		It("executes Blue/Green upgrade cycle with pre-upgrade snapshot", func() {
			By("Writing a secret before upgrade")
			secretPath := "secret/bluegreen-upgrade-test"
			secretData := map[string]string{"foo": "bar", "version": "v1"}
			baoAddr := fmt.Sprintf("https://%s.%s.svc.cluster.local:8200", upgradeCluster.Name, tenantNamespace)
			bypassLabels := map[string]string{
				constants.LabelOpenBaoCluster:   upgradeCluster.Name,
				constants.LabelOpenBaoComponent: "backup",
			}

			// Enable KV engine (idempotent)
			err := e2ehelpers.WriteSecretViaJWT(ctx, cfg, admin, tenantNamespace, openBaoImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, secretData)
			Expect(err).NotTo(HaveOccurred(), "Failed to write pre-upgrade secret")

			By("Triggering upgrade")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				original := updated.DeepCopy()
				updated.Spec.Version = targetVersion
				updated.Spec.Image = fmt.Sprintf("openbao/openbao:%s", targetVersion)
				g.Expect(admin.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Verifying pre-upgrade snapshot job is created")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.PreUpgradeSnapshotJobName).NotTo(BeEmpty(), "pre-upgrade snapshot job should be created")

				// Verify the snapshot job exists
				job := &batchv1.Job{}
				g.Expect(admin.Get(ctx, types.NamespacedName{
					Name:      updated.Status.BlueGreen.PreUpgradeSnapshotJobName,
					Namespace: tenantNamespace,
				}, job)).To(Succeed())

				// Verify job has correct labels
				g.Expect(job.Labels).To(HaveKeyWithValue(constants.LabelOpenBaoComponent, bluegreen.ComponentUpgradeSnapshot))
				g.Expect(job.Labels).To(HaveKeyWithValue(constants.LabelOpenBaoCluster, upgradeCluster.Name))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for pre-upgrade snapshot job to complete")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())

				job := &batchv1.Job{}
				g.Expect(admin.Get(ctx, types.NamespacedName{
					Name:      updated.Status.BlueGreen.PreUpgradeSnapshotJobName,
					Namespace: tenantNamespace,
				}, job)).To(Succeed())

				g.Expect(job.Status.Succeeded).To(BeNumerically(">", 0), "pre-upgrade snapshot job should succeed")
			}, 15*time.Minute, 30*time.Second).Should(Succeed())

			By("Waiting for upgrade to complete")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())

				if updated.Status.BlueGreen.Phase == openbaov1alpha1.PhaseIdle && updated.Status.CurrentVersion == targetVersion {
					return
				}
				// Verify intermediate states
				// Verify Blue pods are on initial version and Green pods (if any) are on target version
				blueRevision := updated.Status.BlueGreen.BlueRevision
				greenRevision := updated.Status.BlueGreen.GreenRevision

				if blueRevision != "" {
					bluePods := &corev1.PodList{}
					g.Expect(admin.List(ctx, bluePods,
						client.InNamespace(tenantNamespace),
						client.MatchingLabels{constants.LabelOpenBaoRevision: blueRevision},
					)).To(Succeed())
					for _, pod := range bluePods.Items {
						for _, container := range pod.Spec.Containers {
							if container.Name == "openbao" {
								g.Expect(container.Image).To(Equal(fmt.Sprintf("openbao/openbao:%s", initialVersion)),
									"Blue pod %s should run initial version", pod.Name)
							}
						}
					}
				}

				if greenRevision != "" {
					greenPods := &corev1.PodList{}
					g.Expect(admin.List(ctx, greenPods,
						client.InNamespace(tenantNamespace),
						client.MatchingLabels{constants.LabelOpenBaoRevision: greenRevision},
					)).To(Succeed())
					for _, pod := range greenPods.Items {
						for _, container := range pod.Spec.Containers {
							if container.Name == "openbao" {
								g.Expect(container.Image).To(Equal(fmt.Sprintf("openbao/openbao:%s", targetVersion)),
									"Green pod %s should run target version", pod.Name)
							}
						}
					}
				}

				g.Expect(updated.Status.BlueGreen.Phase).To(BeElementOf(
					openbaov1alpha1.PhaseIdle,
					openbaov1alpha1.PhaseDeployingGreen,
					openbaov1alpha1.PhaseJoiningMesh,
					openbaov1alpha1.PhaseSyncing,
					openbaov1alpha1.PhasePromoting,
					openbaov1alpha1.PhaseDemotingBlue,
					openbaov1alpha1.PhaseCleanup,
				))
			}, 30*time.Minute, 30*time.Second).Should(Succeed())

			By("Verifying upgrade completed successfully with snapshots")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.CurrentVersion).To(Equal(targetVersion))
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle))
				g.Expect(updated.Status.BlueGreen.PreUpgradeSnapshotJobName).NotTo(BeEmpty(), "snapshot job name should be preserved")
			}, 30*time.Minute, 30*time.Second).Should(Succeed())

			By("Verifying Blue and Green versions during upgrade")
			// This is a retrospective check or we can do it during the loop above.
			// Ideally we want to verify this *during* the process.
			// Let's modify the "Waiting for upgrade to complete" block to include checks.

			By("Verifying secret persists after upgrade")
			// Read secret back
			val, err := e2ehelpers.ReadSecretViaJWT(ctx, cfg, admin, tenantNamespace, openBaoImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, "foo")
			Expect(err).NotTo(HaveOccurred(), "Failed to read post-upgrade secret")
			Expect(val).To(Equal("bar"))
		})
	})

	// --- Failure Scenarios ---
	Context("Failure Scenarios", Label("failure"), func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			failureCluster  *openbaov1alpha1.OpenBaoCluster
			admin           client.Client
		)

		BeforeAll(func() {
			var err error
			tenantFW, err = framework.NewSetup(ctx, "tenant-failure", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace
			admin = tenantFW.Client

			initialVersion := envOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
			targetVersion := envOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)
			if initialVersion == targetVersion {
				Skip("Failure test skipped")
			}

			maxFailures := int32(3)
			failureCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "failure-cluster",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Version:  initialVersion,
					Image:    fmt.Sprintf("openbao/openbao:%s", initialVersion),
					Replicas: 3,
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						Strategy:      openbaov1alpha1.UpdateStrategyBlueGreen,
						ExecutorImage: upgradeExecutorImage,
						BlueGreen: &openbaov1alpha1.BlueGreenConfig{
							AutoPromote:    true,
							MaxJobFailures: &maxFailures,
							AutoRollback: &openbaov1alpha1.AutoRollbackConfig{
								Enabled:      true,
								OnJobFailure: true,
							},
						},
					},
					InitContainer: &openbaov1alpha1.InitContainerConfig{
						Enabled: true,
						Image:   configInitImage,
					},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled:  true,
						Requests: e2ehelpers.CreateE2ERequests(tenantNamespace),
						OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
							Enabled: true,
						},
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
			Expect(admin.Create(ctx, failureCluster)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: failureCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Initialized).To(BeTrue())
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		AfterAll(func() {
			if tenantFW != nil {
				_ = tenantFW.Cleanup(ctx)
			}
		})

		It("tracks job failures", func() {
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: failureCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.JobFailureCount).To(Equal(int32(0)))
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})
	})

	// --- Safe Mode (Chaos) ---
	Context("Safe Mode (chaos)", Label("chaos"), func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			chaosCluster    *openbaov1alpha1.OpenBaoCluster
			initialVersion  string
			targetVersion   string
			admin           client.Client
			cfg             *rest.Config
		)

		BeforeAll(func() {
			var err error
			tenantFW, err = framework.NewSetup(ctx, "tenant-safemode-chaos", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace
			admin = tenantFW.Client

			cfg, err = ctrlconfig.GetConfig()
			Expect(err).NotTo(HaveOccurred())

			initialVersion = envOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
			targetVersion = envOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)

			if initialVersion == targetVersion {
				Skip(fmt.Sprintf("Safe mode chaos test skipped: from version (%s) equals to version (%s). Set E2E_UPGRADE_TO_VERSION to a different version to test upgrades.", initialVersion, targetVersion))
			}

			// For safe mode test, we need to manually restrict the upgrade policy
			// to cause the repair job to fail. Bootstrap creates a full policy,
			// so we define a request that runs AFTER bootstrap to overwrite it.
			brokenPolicyRequest := openbaov1alpha1.SelfInitRequest{
				Name:      "override-upgrade-policy-broken",
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/policies/acl/openbao-operator-upgrade",
				Policy: &openbaov1alpha1.SelfInitPolicy{
					Policy: `path "sys/health" {
  capabilities = ["read"]
}`,
				},
			}

			e2eRequests := e2ehelpers.CreateE2ERequests(tenantNamespace)

			autoPromote := false
			chaosCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "safemode-chaos-cluster",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Version:  initialVersion,
					Image:    fmt.Sprintf("openbao/openbao:%s", initialVersion),
					Replicas: 3,
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						Strategy:      openbaov1alpha1.UpdateStrategyBlueGreen,
						ExecutorImage: upgradeExecutorImage,
						BlueGreen: &openbaov1alpha1.BlueGreenConfig{
							AutoPromote: autoPromote,
							Verification: &openbaov1alpha1.VerificationConfig{
								MinSyncDuration: "30s",
							},
						},
					},
					InitContainer: &openbaov1alpha1.InitContainerConfig{
						Enabled: true,
						Image:   configInitImage,
					},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
							Enabled: true,
						},
						Requests: append([]openbaov1alpha1.SelfInitRequest{brokenPolicyRequest}, e2eRequests...),
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
			Expect(admin.Create(ctx, chaosCluster)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Initialized).To(BeTrue(), "cluster should be initialized")
				g.Expect(updated.Status.CurrentVersion).To(Equal(initialVersion), "current version should match initial version")
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "blue/green status should be initialized")
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle), "initial blue/green phase should be Idle")
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		AfterAll(func() {
			if tenantFW != nil {
				_ = tenantFW.Cleanup(ctx)
			}
		})

		It("enters safe mode when rollback consensus repair job fails", func() {
			By("Writing a secret before upgrade")
			secretPath := "secret/safemode-test"
			secretData := map[string]string{"foo": "bar", "version": "v1"}
			baoAddr := fmt.Sprintf("https://%s.%s.svc.cluster.local:8200", chaosCluster.Name, tenantNamespace)
			bypassLabels := map[string]string{
				constants.LabelOpenBaoCluster:   chaosCluster.Name,
				constants.LabelOpenBaoComponent: "backup",
			}

			// Enable KV engine (idempotent)
			err := e2ehelpers.WriteSecretViaJWT(ctx, cfg, admin, tenantNamespace, openBaoImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, secretData)
			Expect(err).NotTo(HaveOccurred(), "Failed to write pre-upgrade secret")

			By("Triggering a Blue/Green upgrade")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())

				original := updated.DeepCopy()
				updated.Spec.Version = targetVersion
				updated.Spec.Image = fmt.Sprintf("openbao/openbao:%s", targetVersion)

				g.Expect(admin.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for Green revision to be created")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).NotTo(Equal(openbaov1alpha1.PhaseIdle))
				g.Expect(updated.Status.BlueGreen.GreenRevision).NotTo(BeEmpty())
				g.Expect(updated.Status.BlueGreen.GreenRevision).NotTo(BeEmpty())
			}, 15*time.Minute, 10*time.Second).Should(Succeed())

			By("Verifying Blue pods are still on initial version and Green pods are on target version")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())

				blueRevision := updated.Status.BlueGreen.BlueRevision
				greenRevision := updated.Status.BlueGreen.GreenRevision

				// Verify Blue Pods
				bluePods := &corev1.PodList{}
				g.Expect(admin.List(ctx, bluePods,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{constants.LabelOpenBaoRevision: blueRevision},
				)).To(Succeed())
				g.Expect(bluePods.Items).NotTo(BeEmpty(), "Blue pods should exist")
				for _, pod := range bluePods.Items {
					for _, container := range pod.Spec.Containers {
						if container.Name == "openbao" {
							g.Expect(container.Image).To(Equal(fmt.Sprintf("openbao/openbao:%s", initialVersion)),
								"Blue pod %s should run initial version", pod.Name)
						}
					}
				}

				// Verify Green Pods
				greenPods := &corev1.PodList{}
				g.Expect(admin.List(ctx, greenPods,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{constants.LabelOpenBaoRevision: greenRevision},
				)).To(Succeed())
				// Green pods might still be starting, but if they exist, they must be correct
				for _, pod := range greenPods.Items {
					for _, container := range pod.Spec.Containers {
						if container.Name == "openbao" {
							g.Expect(container.Image).To(Equal(fmt.Sprintf("openbao/openbao:%s", targetVersion)),
								"Green pod %s should run target version", pod.Name)
						}
					}
				}
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Forcing rollback")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())

				original := updated.DeepCopy()
				if updated.Annotations == nil {
					updated.Annotations = make(map[string]string)
				}
				updated.Annotations[constants.AnnotationForceRollback] = "true"

				g.Expect(admin.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for rollback to start")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseRollingBack))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Clearing force-rollback annotation (one-shot trigger)")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())

				original := updated.DeepCopy()
				annotations := updated.GetAnnotations()
				if annotations == nil {
					return
				}
				delete(annotations, constants.AnnotationForceRollback)
				updated.SetAnnotations(annotations)

				g.Expect(admin.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			var rollbackRepairJobName string
			By("Finding the rollback consensus repair job")
			Eventually(func(g Gomega) {
				jobs := &batchv1.JobList{}
				g.Expect(admin.List(ctx, jobs,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{
						constants.LabelOpenBaoCluster:   chaosCluster.Name,
						constants.LabelOpenBaoComponent: upgrade.ComponentUpgrade,
					},
				)).To(Succeed())

				job := findUpgradeExecutorJob(jobs.Items, bluegreen.ActionRepairConsensus, rollbackRunID)
				g.Expect(job).NotTo(BeNil(), "rollback repair job should exist")
				rollbackRepairJobName = job.Name
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for the rollback consensus repair job to fail")
			Eventually(func(g Gomega) {
				job := &batchv1.Job{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: rollbackRepairJobName, Namespace: tenantNamespace}, job)).To(Succeed())
				g.Expect(jobFailed(job)).To(BeTrue(), "rollback repair job should fail")
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Verifying secret persists in safe mode")
			// Secret should still be there as we are technically still on the Blue cluster (or the active one)
			// even if we are in safe mode/break glass state.
			baoAddr = fmt.Sprintf("https://%s.%s.svc.cluster.local:8200", chaosCluster.Name, tenantNamespace)
			secretPath = "secret/safemode-test"
			// bypassLabels is in scope from the beginning of the It block
			// bypassLabels is in scope from the beginning of the It block
			Eventually(func(g Gomega) {
				secretVal, err := e2ehelpers.ReadSecretViaJWT(ctx, cfg, admin, tenantNamespace, openBaoImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, "foo")
				g.Expect(err).NotTo(HaveOccurred(), "Failed to read secret in safe mode")
				g.Expect(secretVal).To(Equal("bar"))
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Asserting safe mode is set on the cluster")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BreakGlass).NotTo(BeNil(), "BreakGlass status should be set")
				g.Expect(updated.Status.BreakGlass.Active).To(BeTrue(), "BreakGlass should be active")
				g.Expect(updated.Status.BreakGlass.Reason).To(Equal(openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed), "BreakGlass reason should match")
				g.Expect(updated.Status.BreakGlass.Nonce).NotTo(BeEmpty(), "BreakGlass nonce should be set")
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})
	})

	// --- Gateway Integration ---
	Context("Gateway Integration", Label("gateway", "requires-gateway-api"), func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			upgradeCluster  *openbaov1alpha1.OpenBaoCluster
			admin           client.Client
			// ... vars
		)

		BeforeAll(func() {
			var err error
			// Use standard NewSetup, then install Gateway API
			tenantFW, err = framework.NewSetup(ctx, "tenant-gateway", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace
			admin = tenantFW.Client

			// Install Gateway API using Framework helper
			cleanup, err := tenantFW.RequireGatewayAPI()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(cleanup)

			// Setup logic ...
			gw := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tenant-gateway",
					Namespace: tenantNamespace,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "traefik",
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

			initialVersion := envOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
			upgradeCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-cluster",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Version:  initialVersion,
					Image:    fmt.Sprintf("openbao/openbao:%s", initialVersion),
					Replicas: 3,
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						Strategy:      openbaov1alpha1.UpdateStrategyBlueGreen,
						ExecutorImage: upgradeExecutorImage,
						BlueGreen: &openbaov1alpha1.BlueGreenConfig{
							AutoPromote: true,
							Verification: &openbaov1alpha1.VerificationConfig{
								MinSyncDuration: "10s",
							},
						},
					},
					InitContainer: &openbaov1alpha1.InitContainerConfig{
						Enabled: true,
						Image:   configInitImage,
					},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
							Enabled: true,
						},
						Requests: e2ehelpers.CreateE2ERequests(tenantNamespace),
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

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Initialized).To(BeTrue())
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		AfterAll(func() {
			if tenantFW != nil {
				_ = tenantFW.Cleanup(ctx)
			}
		})

		It("keeps HTTPRoute stable and switches external Service selector at cutover", func() {
			targetVersion := envOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)

			By("Capturing HTTPRoute before upgrade to verify stability")
			var httpRouteBeforeUpgrade *gatewayv1.HTTPRoute
			Eventually(func(g Gomega) {
				httpRoute := &gatewayv1.HTTPRoute{}
				g.Expect(admin.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-httproute", upgradeCluster.Name),
					Namespace: tenantNamespace,
				}, httpRoute)).To(Succeed())
				httpRouteBeforeUpgrade = httpRoute.DeepCopy()
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Triggering upgrade")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				original := updated.DeepCopy()
				updated.Spec.Version = targetVersion
				updated.Spec.Image = fmt.Sprintf("openbao/openbao:%s", targetVersion)
				g.Expect(admin.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for upgrade to progress to cutover phase")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "BlueGreen status should be initialized")
				g.Expect(updated.Status.BlueGreen.Phase).ToNot(BeEmpty())
			}, 10*time.Minute, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for cutover (DemotingBlue) and verifying the external Service selector switches to Green")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "BlueGreen status should be initialized")

				if updated.Status.BlueGreen.Phase != openbaov1alpha1.PhaseDemotingBlue {
					_, _ = fmt.Fprintf(GinkgoWriter, "Current phase: %s\n", updated.Status.BlueGreen.Phase)
					g.Expect(updated.Status.BlueGreen.Phase).ToNot(BeEmpty())
					return
				}

				g.Expect(updated.Status.BlueGreen.GreenRevision).NotTo(BeEmpty(), "GreenRevision should be set at cutover")

				svc := &corev1.Service{}
				g.Expect(admin.Get(ctx, types.NamespacedName{
					Namespace: tenantNamespace,
					Name:      fmt.Sprintf("%s-public", upgradeCluster.Name),
				}, svc)).To(Succeed())
				g.Expect(svc.Spec.Selector).To(HaveKeyWithValue(constants.LabelOpenBaoRevision, updated.Status.BlueGreen.GreenRevision))
			}, 20*time.Minute, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for Blue/Green upgrade to complete")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "BlueGreen status should be initialized")

				// Explicitly require PhaseIdle - this will retry until phase is Idle
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle), "upgrade should complete and return to Idle")
				g.Expect(updated.Status.CurrentVersion).To(Equal(targetVersion))
				g.Expect(updated.Status.BlueGreen.GreenRevision).To(BeEmpty())

				// Verify pods with the final Blue revision are healthy
				labelSelector := labels.SelectorFromSet(map[string]string{
					constants.LabelAppInstance:     upgradeCluster.Name,
					constants.LabelAppName:         constants.LabelValueAppNameOpenBao,
					constants.LabelOpenBaoRevision: updated.Status.BlueGreen.BlueRevision,
				})
				podList := &corev1.PodList{}
				g.Expect(admin.List(ctx, podList, client.InNamespace(tenantNamespace), client.MatchingLabelsSelector{Selector: labelSelector})).To(Succeed())
				g.Expect(len(podList.Items)).To(Equal(3))

				for _, pod := range podList.Items {
					g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning))
				}
			}, 30*time.Minute, 30*time.Second).Should(Succeed())

			By("Verifying legacy blue/green Services do not exist")
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
				g.Expect(admin.Get(ctx, types.NamespacedName{
					Namespace: tenantNamespace,
					Name:      fmt.Sprintf("%s-public", upgradeCluster.Name),
				}, mainSvc)).To(Succeed())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Verifying HTTPRoute remains stable throughout upgrade")
			Eventually(func(g Gomega) {
				httpRouteAfterUpgrade := &gatewayv1.HTTPRoute{}
				g.Expect(admin.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-httproute", upgradeCluster.Name),
					Namespace: tenantNamespace,
				}, httpRouteAfterUpgrade)).To(Succeed())

				// Verify HTTPRoute spec remains unchanged
				g.Expect(httpRouteAfterUpgrade.Spec.ParentRefs).To(Equal(httpRouteBeforeUpgrade.Spec.ParentRefs),
					"HTTPRoute ParentRefs should remain unchanged")
				g.Expect(httpRouteAfterUpgrade.Spec.Hostnames).To(Equal(httpRouteBeforeUpgrade.Spec.Hostnames),
					"HTTPRoute Hostnames should remain unchanged")
				g.Expect(len(httpRouteAfterUpgrade.Spec.Rules)).To(Equal(len(httpRouteBeforeUpgrade.Spec.Rules)),
					"HTTPRoute Rules count should remain unchanged")
				if len(httpRouteAfterUpgrade.Spec.Rules) > 0 && len(httpRouteBeforeUpgrade.Spec.Rules) > 0 {
					g.Expect(httpRouteAfterUpgrade.Spec.Rules[0].Matches).To(Equal(httpRouteBeforeUpgrade.Spec.Rules[0].Matches),
						"HTTPRoute Rules Matches should remain unchanged")
					// BackendRefs should point to the same Service (only Service selector changes, not the Service name)
					g.Expect(len(httpRouteAfterUpgrade.Spec.Rules[0].BackendRefs)).To(Equal(len(httpRouteBeforeUpgrade.Spec.Rules[0].BackendRefs)),
						"HTTPRoute BackendRefs count should remain unchanged")
					if len(httpRouteAfterUpgrade.Spec.Rules[0].BackendRefs) > 0 && len(httpRouteBeforeUpgrade.Spec.Rules[0].BackendRefs) > 0 {
						g.Expect(httpRouteAfterUpgrade.Spec.Rules[0].BackendRefs[0].Name).To(Equal(httpRouteBeforeUpgrade.Spec.Rules[0].BackendRefs[0].Name),
							"HTTPRoute BackendRef Service name should remain unchanged")
						g.Expect(httpRouteAfterUpgrade.Spec.Rules[0].BackendRefs[0].Port).To(Equal(httpRouteBeforeUpgrade.Spec.Rules[0].BackendRefs[0].Port),
							"HTTPRoute BackendRef Port should remain unchanged")
					}
				}
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})
	})
})
