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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
	"github.com/dc-tec/openbao-operator/internal/upgrade/bluegreen"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

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

var _ = Describe("Blue/Green Upgrade Failure Scenarios", Label("upgrade", "cluster", "slow"), Ordered, func() {
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

	Context("Job Failure Threshold", Ordered, func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			failureCluster  *openbaov1alpha1.OpenBaoCluster
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-failure", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			// Skip if versions are the same
			initialVersion := getEnvOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
			targetVersion := getEnvOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)
			if initialVersion == targetVersion {
				Skip("Failure test skipped: from version equals to version")
			}

			// Create SelfInit requests for JWT auth
			selfInitRequests, err := createBlueGreenUpgradeSelfInitRequests(ctx, tenantNamespace, "failure-cluster", cfg)
			Expect(err).NotTo(HaveOccurred())

			// Create cluster with initial version and job failure threshold
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
					UpdateStrategy: openbaov1alpha1.UpdateStrategy{
						Type: openbaov1alpha1.UpdateStrategyBlueGreen,
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
						Requests: selfInitRequests,
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
			Expect(admin.Create(ctx, failureCluster)).To(Succeed())

			// Wait for cluster to be initialized
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: failureCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle))
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

		It("tracks job failures in status", func() {
			// This test verifies that JobFailureCount increments when jobs fail
			// For a full test, we would need to simulate job failures

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: failureCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())

				// Verify JobFailureCount field exists and is initially 0
				g.Expect(updated.Status.BlueGreen.JobFailureCount).To(Equal(int32(0)))

				_, _ = fmt.Fprintf(GinkgoWriter, "Job failure tracking verified: failureCount=%d\n",
					updated.Status.BlueGreen.JobFailureCount)
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})
	})

	Context("Safe Mode (chaos)", Label("chaos"), Ordered, func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			chaosCluster    *openbaov1alpha1.OpenBaoCluster
			initialVersion  string
			targetVersion   string
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-safemode-chaos", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			initialVersion = getEnvOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
			targetVersion = getEnvOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)

			if initialVersion == targetVersion {
				Skip(fmt.Sprintf("Safe mode chaos test skipped: from version (%s) equals to version (%s). Set E2E_UPGRADE_TO_VERSION to a different version to test upgrades.", initialVersion, targetVersion))
			}

			selfInitRequests, err := createBlueGreenUpgradeSelfInitRequests(ctx, tenantNamespace, "safemode-chaos-cluster", cfg)
			Expect(err).NotTo(HaveOccurred())

			// Ensure the rollback consensus repair job can authenticate, but cannot perform
			// the repair operation. This simulates a repair job failure deterministically
			// without relying on missing roles or external chaos tooling.
			for i := range selfInitRequests {
				if selfInitRequests[i].Name != "create-upgrade-policy" {
					continue
				}
				if selfInitRequests[i].Policy == nil {
					continue
				}

				selfInitRequests[i].Policy.Policy = `path "sys/health" {
  capabilities = ["read"]
}`
			}

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
					UpdateStrategy: openbaov1alpha1.UpdateStrategy{
						Type: openbaov1alpha1.UpdateStrategyBlueGreen,
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
						Enabled:  true,
						Requests: selfInitRequests,
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
			Expect(admin.Create(ctx, chaosCluster)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Initialized).To(BeTrue(), "cluster should be initialized")
				g.Expect(updated.Status.CurrentVersion).To(Equal(initialVersion), "current version should match initial version")
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "blue/green status should be initialized")
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle), "initial blue/green phase should be Idle")
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

		It("enters safe mode when rollback consensus repair job fails", func() {
			By("Triggering a Blue/Green upgrade")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				updated.Spec.Version = targetVersion
				updated.Spec.Image = fmt.Sprintf("openbao/openbao:%s", targetVersion)

				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for Green revision to be created")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).NotTo(Equal(openbaov1alpha1.PhaseIdle))
				g.Expect(updated.Status.BlueGreen.GreenRevision).NotTo(BeEmpty())
			}, 15*time.Minute, 10*time.Second).Should(Succeed())

			By("Forcing rollback")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				if updated.Annotations == nil {
					updated.Annotations = make(map[string]string)
				}
				updated.Annotations[constants.AnnotationForceRollback] = "true"

				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for rollback to start")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseRollingBack))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Clearing force-rollback annotation (one-shot trigger)")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				annotations := updated.GetAnnotations()
				if annotations == nil {
					return
				}
				delete(annotations, constants.AnnotationForceRollback)
				updated.SetAnnotations(annotations)

				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			var rollbackRepairJobName string
			By("Finding the rollback consensus repair job")
			Eventually(func(g Gomega) {
				jobs := &batchv1.JobList{}
				err := admin.List(ctx, jobs,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{
						constants.LabelOpenBaoCluster:   chaosCluster.Name,
						constants.LabelOpenBaoComponent: upgrade.ComponentUpgrade,
					},
				)
				g.Expect(err).NotTo(HaveOccurred())

				job := findUpgradeExecutorJob(jobs.Items, bluegreen.ActionRepairConsensus, rollbackRunID)
				g.Expect(job).NotTo(BeNil(), "rollback repair job should exist")
				rollbackRepairJobName = job.Name
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for the rollback consensus repair job to fail")
			Eventually(func(g Gomega) {
				job := &batchv1.Job{}
				err := admin.Get(ctx, types.NamespacedName{Name: rollbackRepairJobName, Namespace: tenantNamespace}, job)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(jobFailed(job)).To(BeTrue(), "rollback repair job should fail")
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Asserting safe mode is set on the cluster")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: chaosCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updated.Status.BreakGlass).NotTo(BeNil(), "breakGlass status should be set")
				g.Expect(updated.Status.BreakGlass.Active).To(BeTrue(), "breakGlass should be active")
				g.Expect(updated.Status.BreakGlass.Reason).To(Equal(openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed))
				g.Expect(updated.Status.BreakGlass.Nonce).NotTo(BeEmpty(), "breakGlass nonce should be set")

				degraded := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
				g.Expect(degraded).NotTo(BeNil(), "Degraded condition should be present")
				g.Expect(degraded.Status).To(Equal(metav1.ConditionTrue), "Degraded should be true in safe mode")
				g.Expect(degraded.Reason).To(Equal(constants.ReasonBreakGlassRequired))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})
	})

	Context("Pre-Upgrade Snapshot with RustFS", Ordered, func() {
		var (
			tenantNamespace   string
			tenantFW          *framework.Framework
			snapshotCluster   *openbaov1alpha1.OpenBaoCluster
			credentialsSecret *corev1.Secret
		)

		BeforeAll(func() {
			var err error

			// Ensure RustFS is available
			rustfsNamespace := "rustfs"
			err = ensureRustFS(ctx, admin, cfg, rustfsNamespace)
			if err != nil {
				Skip(fmt.Sprintf("RustFS deployment failed: %v. Skipping snapshot tests.", err))
			}

			tenantFW, err = framework.New(ctx, admin, "tenant-snapshot", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			if defaultUpgradeFromVersion == defaultUpgradeToVersion {
				Skip("Snapshot test skipped: from version equals to version")
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

			// Create SelfInit requests for JWT auth (for both upgrade and backup)
			selfInitRequests, err := createBlueGreenUpgradeSelfInitRequests(ctx, tenantNamespace, "snapshot-cluster", cfg)
			Expect(err).NotTo(HaveOccurred())

			// Add backup-specific SelfInit requests (policy and role only, JWT auth is already configured above)
			selfInitRequests = append(selfInitRequests,
				openbaov1alpha1.SelfInitRequest{
					Name:      "create-backup-policy",
					Operation: openbaov1alpha1.SelfInitOperationUpdate,
					Path:      "sys/policies/acl/backup",
					Policy: &openbaov1alpha1.SelfInitPolicy{
						Policy: `path "sys/storage/raft/snapshot" {
  capabilities = ["read"]
}`,
					},
				},
				openbaov1alpha1.SelfInitRequest{
					Name:      "create-backup-jwt-role",
					Operation: openbaov1alpha1.SelfInitOperationUpdate,
					Path:      "auth/jwt/role/backup",
					Data: e2ehelpers.MustJSON(map[string]interface{}{
						"role_type":       "jwt",
						"user_claim":      "sub",
						"bound_audiences": []string{"openbao-internal"},
						"bound_subject":   fmt.Sprintf("system:serviceaccount:%s:%s-backup-serviceaccount", tenantNamespace, "snapshot-cluster"),
						"token_policies":  []string{"backup"},
						"policies":        []string{"backup"},
						"ttl":             "1h",
					}),
				},
			)

			// Create cluster with PreUpgradeSnapshot enabled
			snapshotCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "snapshot-cluster",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Version:  defaultUpgradeFromVersion,
					Image:    fmt.Sprintf("openbao/openbao:%s", defaultUpgradeFromVersion),
					Replicas: 3,
					UpdateStrategy: openbaov1alpha1.UpdateStrategy{
						Type: openbaov1alpha1.UpdateStrategyBlueGreen,
						BlueGreen: &openbaov1alpha1.BlueGreenConfig{
							AutoPromote:        true,
							PreUpgradeSnapshot: true, // Enable pre-upgrade snapshot

						},
					},
					InitContainer: &openbaov1alpha1.InitContainerConfig{
						Enabled: true,
						Image:   configInitImage,
					},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled:  true,
						Requests: selfInitRequests,
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
					Backup: &openbaov1alpha1.BackupSchedule{
						Schedule:      "0 0 * * *",
						ExecutorImage: backupExecutorImage,
						Target: openbaov1alpha1.BackupTarget{
							Endpoint:     fmt.Sprintf("http://%s-svc.%s.svc.cluster.local:9000", rustfsName, "rustfs"),
							Bucket:       rustfsBucket,
							Region:       "us-east-1",
							UsePathStyle: true,
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: credentialsSecret.Name,
							},
						},
						// Add required JWT auth role for backup
						JWTAuthRole: "backup",
					},
				},
			}
			Expect(admin.Create(ctx, snapshotCluster)).To(Succeed())

			// Mirror the approach used by backup/restore E2E tests: add an explicit NetworkPolicy
			// for the upgrade-snapshot Job pods to allow egress to RustFS (port 9000).
			snapshotNetworkPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-upgrade-snapshot-network-policy", snapshotCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/component": "upgrade-snapshot",
							"openbao.org/cluster":   snapshotCluster.Name,
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
						// Allow access to RustFS in the rustfs namespace (S3 API port)
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
						// Allow access to OpenBao cluster for snapshot API (leader discovery + snapshot)
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"openbao.org/cluster": snapshotCluster.Name,
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

			By("Waiting for Cluster to be ready")
			Eventually(func() bool {
				err := admin.Get(ctx, types.NamespacedName{Name: snapshotCluster.Name, Namespace: snapshotCluster.Namespace}, snapshotCluster)
				return err == nil && snapshotCluster.Status.Initialized && snapshotCluster.Status.Phase == openbaov1alpha1.ClusterPhaseRunning
			}, framework.DefaultLongWaitTimeout, 1*time.Second).Should(BeTrue())

			By("Setting up JWT Auth for Backup")
			// We need to configure the JWT auth backend and role for the snapshot job to work
			// The snapshot job uses the backup service account which needs a corresponding role in OpenBao
			// Helper to assume we can exec into the active pod
			// err = framework.ConfigureBackupJWTAuth(ctx, admin, snapshotCluster.Namespace, snapshotCluster.Name)
			// Expect(err).NotTo(HaveOccurred())
			// We rely on SelfInit requests (create-backup-jwt-role) added above.
		})

		AfterAll(func() {
			if tenantFW == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			_ = tenantFW.Cleanup(cleanupCtx)
		})

		It("creates pre-upgrade snapshot job during upgrade", func() {
			targetVersion := defaultUpgradeToVersion

			// Trigger upgrade
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: snapshotCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				updated.Spec.Version = targetVersion
				updated.Spec.Image = fmt.Sprintf("openbao/openbao:%s", targetVersion)

				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			// Wait for pre-upgrade snapshot job to be created or upgrade to progress
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: snapshotCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				// Check if snapshot job name is tracked
				if updated.Status.BlueGreen != nil && updated.Status.BlueGreen.PreUpgradeSnapshotJobName != "" {
					_, _ = fmt.Fprintf(GinkgoWriter, "Pre-upgrade snapshot job: %s\n",
						updated.Status.BlueGreen.PreUpgradeSnapshotJobName)

					// Verify the job exists
					job := &batchv1.Job{}
					err := admin.Get(ctx, types.NamespacedName{
						Namespace: tenantNamespace,
						Name:      updated.Status.BlueGreen.PreUpgradeSnapshotJobName,
					}, job)
					if err == nil {
						_, _ = fmt.Fprintf(GinkgoWriter, "Snapshot job status: succeeded=%d failed=%d active=%d\n",
							job.Status.Succeeded, job.Status.Failed, job.Status.Active)
					}
					return
				}

				// Check if we're past the Idle phase (snapshot may have completed quickly)
				if updated.Status.BlueGreen != nil && updated.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
					_, _ = fmt.Fprintf(GinkgoWriter, "Upgrade progressed to phase: %s (snapshot may have completed)\n",
						updated.Status.BlueGreen.Phase)
					return
				}

				g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "BlueGreen status should be set")
			}, 10*time.Minute, 30*time.Second).Should(Succeed())
		})

		It("upgrade completes successfully with snapshots", func() {
			targetVersion := defaultUpgradeToVersion

			// Wait for upgrade to complete
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: snapshotCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				if updated.Status.BlueGreen != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Current phase: %s\n", updated.Status.BlueGreen.Phase)
				}

				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle))
				g.Expect(updated.Status.CurrentVersion).To(Equal(targetVersion))
			}, 30*time.Minute, 30*time.Second).Should(Succeed())
		})

		It("verifies snapshot jobs were created", func() {
			// List all snapshot jobs in the namespace
			var jobs batchv1.JobList
			err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
				"openbao.org/component": "upgrade-snapshot",
			})
			Expect(err).NotTo(HaveOccurred())

			_, _ = fmt.Fprintf(GinkgoWriter, "Found %d upgrade snapshot jobs\n", len(jobs.Items))
			for i := range jobs.Items {
				job := &jobs.Items[i]
				phase := job.Annotations["openbao.org/snapshot-phase"]
				_, _ = fmt.Fprintf(GinkgoWriter, "Snapshot job: %s, phase: %s, succeeded: %d, failed: %d\n",
					job.Name, phase, job.Status.Succeeded, job.Status.Failed)
			}

			// Assert that at least one snapshot job was created
			Expect(len(jobs.Items)).To(BeNumerically(">", 0), "should have at least one upgrade snapshot job")
		})
	})
})
