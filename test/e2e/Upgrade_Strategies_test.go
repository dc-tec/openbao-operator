//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
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
	"github.com/dc-tec/openbao-operator/test/utils"
)

// === Shared Helpers ===

const (
	upgradeActionAnnotationKey = "openbao.org/upgrade-action"
	upgradeRunIDAnnotationKey  = "openbao.org/upgrade-run-id"
	rollbackRunID              = "rollback"
	invalidUpgradeJWTAuthRole  = "invalid-upgrade-role"
)

type serviceAvailabilityStats struct {
	Samples                int
	Failures               int
	ConsecutiveFailures    int
	MaxConsecutiveFailures int
	LastFailure            string
}

func probeOpenBaoServiceHealthViaAPIProxy(ctx context.Context, cfg *rest.Config, namespace, serviceName string) (int, error) {
	if cfg == nil {
		return 0, fmt.Errorf("rest config is required")
	}
	if namespace == "" {
		return 0, fmt.Errorf("namespace is required")
	}
	if serviceName == "" {
		return 0, fmt.Errorf("service name is required")
	}

	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return 0, fmt.Errorf("failed to create API transport: %w", err)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	url := fmt.Sprintf(
		"%s/api/v1/namespaces/%s/services/https:%s:%d/proxy/v1/sys/health?standbyok=true&perfstandbyok=true",
		cfg.Host,
		namespace,
		serviceName,
		constants.PortAPI,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to build health probe request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("health probe request failed: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

func isOpenBaoServiceAvailableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusOK, http.StatusTooManyRequests, 472, 473:
		return true
	default:
		return false
	}
}

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

func jobSucceeded(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	if job.Status.Succeeded > 0 {
		return true
	}
	for _, cond := range job.Status.Conditions {
		if cond.Status != "True" {
			continue
		}
		if cond.Type == batchv1.JobComplete {
			return true
		}
	}
	return false
}

func hasSucceededUpgradeAction(jobs []batchv1.Job, action bluegreen.ExecutorAction) bool {
	for i := range jobs {
		job := &jobs[i]
		if job.Annotations == nil {
			continue
		}
		if job.Annotations[upgradeActionAnnotationKey] != string(action) {
			continue
		}
		if jobSucceeded(job) {
			return true
		}
	}
	return false
}

func ptrTo[T any](v T) *T {
	return &v
}

func dumpKubectlOutput(args ...string) {
	cmd := exec.Command("kubectl", args...) // #nosec G204 -- E2E diagnostics command with fixed binary and controlled args.
	output, err := utils.Run(cmd)
	_, _ = fmt.Fprintf(GinkgoWriter, "\n$ kubectl %v\n", args)
	if err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "kubectl command failed: %v\n", err)
	}
	if output == "" {
		_, _ = fmt.Fprintln(GinkgoWriter, "(no output)")
		return
	}
	_, _ = fmt.Fprintln(GinkgoWriter, output)
}

func dumpRollingUpgradeDiagnostics(ctx context.Context, admin client.Client, namespace, clusterName string) {
	if namespace == "" || clusterName == "" {
		return
	}

	_, _ = fmt.Fprintf(GinkgoWriter, "\n========== Rolling Upgrade Diagnostics (%s/%s) ==========\n", namespace, clusterName)
	dumpKubectlOutput("get", "openbaocluster", clusterName, "-n", namespace, "-o", "yaml")
	dumpKubectlOutput("get", "statefulset", clusterName, "-n", namespace, "-o", "yaml")
	dumpKubectlOutput("get", "pods", "-n", namespace, "-o", "wide")
	dumpKubectlOutput("get", "jobs", "-n", namespace, "-o", "wide")
	dumpKubectlOutput("get", "events", "-n", namespace, "--sort-by=.lastTimestamp")

	dumpKubectlOutput("logs", "deployment/openbao-operator-controller", "-n", operatorNamespace, "--tail=400")

	if admin == nil || ctx == nil {
		_, _ = fmt.Fprintln(GinkgoWriter, "Skipping step-down job diagnostics: client/context unavailable")
		return
	}

	jobList := &batchv1.JobList{}
	if err := admin.List(ctx, jobList,
		client.InNamespace(namespace),
		client.MatchingLabels{
			constants.LabelOpenBaoCluster:   clusterName,
			constants.LabelOpenBaoComponent: upgrade.ComponentUpgrade,
		},
	); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Failed to list upgrade jobs for diagnostics: %v\n", err)
		return
	}

	for i := range jobList.Items {
		job := &jobList.Items[i]
		if job.Annotations == nil {
			continue
		}
		if job.Annotations[upgradeActionAnnotationKey] != string(upgrade.ExecutorActionRollingStepDownLeader) {
			continue
		}

		_, _ = fmt.Fprintf(GinkgoWriter, "\n------ Step-down job diagnostics: %s ------\n", job.Name)
		dumpKubectlOutput("describe", "job", job.Name, "-n", namespace)
		dumpKubectlOutput("get", "pods", "-n", namespace, "-l", fmt.Sprintf("job-name=%s", job.Name), "-o", "wide")
		dumpKubectlOutput("logs", fmt.Sprintf("job/%s", job.Name), "-n", namespace, "--all-containers=true")
	}
}

// createE2ERequests helper removed in favor of e2ehelpers.CreateE2ERequests

// === Tests ===

var _ = Describe("Upgrade Strategies", Label("upgrade", "upgrades", "cluster", "slow"), Ordered, func() {
	ctx := context.Background()

	// --- Rolling Upgrade ---
	Context("Rolling Upgrade", Label("rolling"), func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			upgradeCluster  *openbaov1alpha1.OpenBaoCluster
			initialVersion  string
			initialImage    string
			targetVersion   string
			targetImage     string
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
			initialImage = fmt.Sprintf("openbao/openbao:%s", initialVersion)
			targetImage = fmt.Sprintf("openbao/openbao:%s", targetVersion)

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
					Image:    initialImage,
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
						Image: upgradeExecutorImage,
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

		AfterEach(func() {
			if !CurrentSpecReport().Failed() {
				return
			}
			if upgradeCluster == nil {
				return
			}

			By("Collecting rolling upgrade diagnostics")
			dumpRollingUpgradeDiagnostics(ctx, admin, tenantNamespace, upgradeCluster.Name)
		})

		It("performs rolling upgrade", func() {
			cfg, err := ctrlconfig.GetConfig()
			Expect(err).NotTo(HaveOccurred())

			By("Writing a secret before upgrade")
			// Note: bao kv put/get automatically adds /data/ for KV v2, so use path without /data/
			secretPath := "secret/rolling-upgrade-test"
			secretData := map[string]string{"foo": "bar", "version": "v1"}
			bypassLabels := map[string]string{
				constants.LabelOpenBaoCluster:   upgradeCluster.Name,
				constants.LabelOpenBaoComponent: "backup",
			}

			// Enable KV engine (idempotent) - dev mode usually has it enabled at secret/
			// but we'll try to write directly first.
			Eventually(func(g Gomega) {
				baoAddr, err := e2ehelpers.ResolveActiveOpenBaoAddress(ctx, admin, tenantNamespace, upgradeCluster.Name)
				g.Expect(err).NotTo(HaveOccurred())
				err = e2ehelpers.WriteSecretViaJWT(ctx, cfg, admin, tenantNamespace, initialImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, secretData)
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultLongWaitTimeout, 10*time.Second).Should(Succeed(), "Failed to write pre-upgrade secret")

			By("Triggering upgrade")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				original := updated.DeepCopy()
				updated.Spec.Version = targetVersion
				updated.Spec.Image = targetImage
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

			By("Monitoring rolling invariants during upgrade")
			var (
				seenUpgradeInProgress  bool
				lastPartition          = upgradeCluster.Spec.Replicas
				lastCompletedPodCount  = 0
				lastUpgradeStartedAt   int64
				lastResourceVersionInt int64
				consecutiveReadFailure = 0
			)

			monitorDeadline := time.Now().Add(20 * time.Minute)
			for time.Now().Before(monitorDeadline) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())

				// Cache-backed e2e reads can occasionally surface older snapshots while watches catch up.
				// Ignore non-forward resourceVersions so monotonic checks evaluate ordered observations.
				if currentRV, err := strconv.ParseInt(updated.ResourceVersion, 10, 64); err == nil {
					if currentRV <= lastResourceVersionInt {
						time.Sleep(5 * time.Second)
						continue
					}
					lastResourceVersionInt = currentRV
				}

				if updated.Status.Upgrade == nil {
					if seenUpgradeInProgress {
						break
					}
					time.Sleep(5 * time.Second)
					continue
				}

				seenUpgradeInProgress = true
				progress := updated.Status.Upgrade
				if progress.StartedAt != nil {
					startedAtUnix := progress.StartedAt.Time.UnixNano()
					if lastUpgradeStartedAt == 0 || startedAtUnix != lastUpgradeStartedAt {
						lastUpgradeStartedAt = startedAtUnix
						lastPartition = updated.Spec.Replicas
						lastCompletedPodCount = 0
					}
				}

				Expect(progress.TargetVersion).To(Equal(targetVersion))
				Expect(progress.FromVersion).To(Equal(initialVersion))
				Expect(progress.LastErrorReason).To(BeEmpty(), "rolling upgrade entered failed state: %s", progress.LastErrorMessage)

				Expect(progress.CurrentPartition).To(BeNumerically(">=", 0))
				Expect(progress.CurrentPartition).To(BeNumerically("<=", lastPartition), "partition should move monotonically downward")
				lastPartition = progress.CurrentPartition

				Expect(len(progress.CompletedPods)).To(BeNumerically(">=", lastCompletedPodCount), "completed pod list should not shrink")
				lastCompletedPodCount = len(progress.CompletedPods)

				seenOrdinals := make(map[int32]struct{}, len(progress.CompletedPods))
				for _, ordinal := range progress.CompletedPods {
					Expect(ordinal).To(BeNumerically(">=", 0))
					Expect(ordinal).To(BeNumerically("<", updated.Spec.Replicas))
					_, duplicate := seenOrdinals[ordinal]
					Expect(duplicate).To(BeFalse(), "completed pods should not contain duplicate ordinals")
					seenOrdinals[ordinal] = struct{}{}
				}

				sts := &appsv1.StatefulSet{}
				Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, sts)).To(Succeed())
				Expect(sts.Status.ReadyReplicas).To(BeNumerically(">=", updated.Spec.Replicas-1), "rolling upgrade should keep at least quorum available")

				baoAddr, err := e2ehelpers.ResolveActiveOpenBaoAddress(ctx, admin, tenantNamespace, upgradeCluster.Name)
				if err == nil {
					_, err = e2ehelpers.ReadSecretViaJWT(ctx, cfg, admin, tenantNamespace, initialImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, "foo")
				}
				if err != nil {
					consecutiveReadFailure++
				} else {
					consecutiveReadFailure = 0
				}
				Expect(consecutiveReadFailure).To(BeNumerically("<=", 2), "data plane was unavailable for too long during rolling upgrade")

				time.Sleep(10 * time.Second)
			}

			Expect(seenUpgradeInProgress).To(BeTrue(), "rolling upgrade never entered in-progress state")
			Expect(time.Now()).To(BeTemporally("<", monitorDeadline), "timed out waiting for rolling upgrade to complete")

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Upgrade).To(BeNil(), "Status.Upgrade should be cleared when upgrade completes")
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
			}, 20*time.Minute, 10*time.Second).Should(Succeed())

			By("Verifying rolling step-down jobs are deterministic and successful")
			jobs := &batchv1.JobList{}
			Expect(admin.List(ctx, jobs,
				client.InNamespace(tenantNamespace),
				client.MatchingLabels{
					constants.LabelOpenBaoCluster:   upgradeCluster.Name,
					constants.LabelOpenBaoComponent: upgrade.ComponentUpgrade,
				},
			)).To(Succeed())

			stepDownRunIDs := map[string]struct{}{}
			for i := range jobs.Items {
				job := &jobs.Items[i]
				if job.Annotations == nil {
					continue
				}
				if job.Annotations[upgradeActionAnnotationKey] != string(upgrade.ExecutorActionRollingStepDownLeader) {
					continue
				}

				Expect(job.Status.Failed).To(Equal(int32(0)), "step-down job should not fail: %s", job.Name)

				runID := job.Annotations[upgradeRunIDAnnotationKey]
				Expect(runID).NotTo(BeEmpty(), "step-down job should carry run ID annotation")
				_, duplicate := stepDownRunIDs[runID]
				Expect(duplicate).To(BeFalse(), "step-down jobs should not duplicate run IDs")
				stepDownRunIDs[runID] = struct{}{}
			}
			Expect(len(stepDownRunIDs)).To(BeNumerically("<=", int(upgradeCluster.Spec.Replicas)))

			By("Verifying secret persists after upgrade")
			Eventually(func(g Gomega) {
				baoAddr, err := e2ehelpers.ResolveActiveOpenBaoAddress(ctx, admin, tenantNamespace, upgradeCluster.Name)
				g.Expect(err).NotTo(HaveOccurred())
				val, err := e2ehelpers.ReadSecretViaJWT(ctx, cfg, admin, tenantNamespace, targetImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, "foo")
				g.Expect(err).NotTo(HaveOccurred(), "Failed to read post-upgrade secret")
				g.Expect(val).To(Equal("bar"))
			}, framework.DefaultLongWaitTimeout, 10*time.Second).Should(Succeed())

			By("Verifying upgrade metrics reflect idle state")
			metricsOutput, metricErr := framework.WaitForControllerMetricSubstrings(
				operatorNamespace,
				3*time.Minute,
				"openbao_upgrade_in_progress{",
				fmt.Sprintf(`namespace="%s"`, tenantNamespace),
				fmt.Sprintf(`name="%s"`, upgradeCluster.Name),
				"} 0",
			)
			Expect(metricErr).NotTo(HaveOccurred(), "Last metrics output:\n%s", metricsOutput)
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
					Service: &openbaov1alpha1.ServiceConfig{
						Type: corev1.ServiceTypeClusterIP,
					},
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
						Image:    upgradeExecutorImage,
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
						Schedule: "0 0 * * *",
						Image:    backupExecutorImage,
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
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
			Eventually(func(g Gomega) {
				stsList := &appsv1.StatefulSetList{}
				g.Expect(admin.List(ctx, stsList,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{
						constants.LabelOpenBaoCluster: upgradeCluster.Name,
					},
				)).To(Succeed())
				g.Expect(stsList.Items).NotTo(BeEmpty(), "expected at least one StatefulSet for blue/green cluster")

				var totalReady int32
				for _, sts := range stsList.Items {
					totalReady += sts.Status.ReadyReplicas
				}
				g.Expect(totalReady).To(Equal(upgradeCluster.Spec.Replicas),
					"expected total ready replicas across StatefulSets to match desired cluster replicas")
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
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
			serviceName := fmt.Sprintf("%s-public", upgradeCluster.Name)
			bypassLabels := map[string]string{
				constants.LabelOpenBaoCluster:   upgradeCluster.Name,
				constants.LabelOpenBaoComponent: "backup",
			}

			// Enable KV engine (idempotent)
			Eventually(func(g Gomega) {
				baoAddr, err := e2ehelpers.ResolveActiveOpenBaoAddress(ctx, admin, tenantNamespace, upgradeCluster.Name)
				g.Expect(err).NotTo(HaveOccurred())
				err = e2ehelpers.WriteSecretViaJWT(ctx, cfg, admin, tenantNamespace, openBaoImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, secretData)
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultLongWaitTimeout, 10*time.Second).Should(Succeed(), "Failed to write pre-upgrade secret")

			By("Ensuring the external Service exists for zero-downtime probing")
			Eventually(func(g Gomega) {
				svc := &corev1.Service{}
				g.Expect(admin.Get(ctx, types.NamespacedName{
					Name:      serviceName,
					Namespace: tenantNamespace,
				}, svc)).To(Succeed())
				g.Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Continuously probing Service availability during the full Blue/Green upgrade")
			probeCtx, stopProbe := context.WithCancel(ctx)
			defer stopProbe()

			probeDone := make(chan struct{})
			var probeMu sync.Mutex
			probeStats := serviceAvailabilityStats{}

			probeOnce := func() {
				probeAttemptCtx, cancel := context.WithTimeout(probeCtx, 20*time.Second)
				defer cancel()

				statusCode, err := probeOpenBaoServiceHealthViaAPIProxy(probeAttemptCtx, cfg, tenantNamespace, serviceName)

				probeMu.Lock()
				defer probeMu.Unlock()

				probeStats.Samples++
				if err != nil {
					probeStats.Failures++
					probeStats.ConsecutiveFailures++
					if probeStats.ConsecutiveFailures > probeStats.MaxConsecutiveFailures {
						probeStats.MaxConsecutiveFailures = probeStats.ConsecutiveFailures
					}
					probeStats.LastFailure = err.Error()
					_, _ = fmt.Fprintf(GinkgoWriter, "service availability probe failed: %v\n", err)
					return
				}

				if !isOpenBaoServiceAvailableStatus(statusCode) {
					probeStats.Failures++
					probeStats.ConsecutiveFailures++
					if probeStats.ConsecutiveFailures > probeStats.MaxConsecutiveFailures {
						probeStats.MaxConsecutiveFailures = probeStats.ConsecutiveFailures
					}
					probeStats.LastFailure = fmt.Sprintf("unexpected status code: %d", statusCode)
					_, _ = fmt.Fprintf(GinkgoWriter, "service availability probe returned unexpected status: %d\n", statusCode)
					return
				}

				probeStats.ConsecutiveFailures = 0
			}

			go func() {
				defer close(probeDone)
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()

				probeOnce()
				for {
					select {
					case <-probeCtx.Done():
						return
					case <-ticker.C:
						probeOnce()
					}
				}
			}()
			defer func() {
				stopProbe()
				<-probeDone
			}()

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
			phaseRank := map[openbaov1alpha1.BlueGreenPhase]int{
				openbaov1alpha1.PhaseIdle:           0,
				openbaov1alpha1.PhaseDeployingGreen: 1,
				openbaov1alpha1.PhaseJoiningMesh:    2,
				openbaov1alpha1.PhaseSyncing:        3,
				openbaov1alpha1.PhasePromoting:      4,
				openbaov1alpha1.PhaseDemotingBlue:   5,
				openbaov1alpha1.PhaseCleanup:        6,
			}
			highestObservedPhase := openbaov1alpha1.PhaseIdle
			highestObservedRank := phaseRank[highestObservedPhase]

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())

				currentPhase := updated.Status.BlueGreen.Phase
				if currentPhase == openbaov1alpha1.PhaseIdle && updated.Status.CurrentVersion == targetVersion {
					return
				}
				currentRank, knownPhase := phaseRank[currentPhase]
				g.Expect(knownPhase).To(BeTrue(), "phase should be known for monotonic ordering")
				g.Expect(currentRank).To(BeNumerically(">=", highestObservedRank),
					"phase regressed from %s to %s", highestObservedPhase, currentPhase)
				if currentRank > highestObservedRank {
					highestObservedRank = currentRank
					highestObservedPhase = currentPhase
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

				g.Expect(currentPhase).To(BeElementOf(
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

			stopProbe()
			<-probeDone
			probeMu.Lock()
			finalProbeStats := probeStats
			probeMu.Unlock()

			By("Verifying Service availability remained continuous during the upgrade")
			Expect(finalProbeStats.Samples).To(BeNumerically(">", 0), "expected at least one service availability sample")
			Expect(finalProbeStats.MaxConsecutiveFailures).To(BeNumerically("<=", 1),
				"service had prolonged unavailability during Blue/Green upgrade (samples=%d failures=%d maxConsecutive=%d lastFailure=%q)",
				finalProbeStats.Samples, finalProbeStats.Failures, finalProbeStats.MaxConsecutiveFailures, finalProbeStats.LastFailure)

			By("Verifying critical blue/green executor actions completed successfully")
			Eventually(func(g Gomega) {
				jobs := &batchv1.JobList{}
				g.Expect(admin.List(ctx, jobs,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{
						constants.LabelAppInstance:      upgradeCluster.Name,
						constants.LabelOpenBaoCluster:   upgradeCluster.Name,
						constants.LabelOpenBaoComponent: upgrade.ComponentUpgrade,
					},
				)).To(Succeed())

				g.Expect(hasSucceededUpgradeAction(jobs.Items, bluegreen.ActionJoinGreenNonVoters)).To(BeTrue(), "join-green-non-voters action should succeed")
				g.Expect(hasSucceededUpgradeAction(jobs.Items, bluegreen.ActionWaitGreenSynced)).To(BeTrue(), "wait-green-synced action should succeed")
				g.Expect(hasSucceededUpgradeAction(jobs.Items, bluegreen.ActionPromoteGreenVoters)).To(BeTrue(), "promote-green-voters action should succeed")
				g.Expect(hasSucceededUpgradeAction(jobs.Items, bluegreen.ActionDemoteBlueNonVotersStepDown)).To(BeTrue(), "demote-blue-non-voters-stepdown action should succeed")
				g.Expect(hasSucceededUpgradeAction(jobs.Items, bluegreen.ActionRemoveBluePeers)).To(BeTrue(), "remove-blue-peers action should succeed")
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Verifying secret persists after upgrade")
			Eventually(func(g Gomega) {
				baoAddr, err := e2ehelpers.ResolveActiveOpenBaoAddress(ctx, admin, tenantNamespace, upgradeCluster.Name)
				g.Expect(err).NotTo(HaveOccurred())
				val, err := e2ehelpers.ReadSecretViaJWT(ctx, cfg, admin, tenantNamespace, openBaoImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, "foo")
				g.Expect(err).NotTo(HaveOccurred(), "Failed to read post-upgrade secret")
				g.Expect(val).To(Equal("bar"))
			}, framework.DefaultLongWaitTimeout, 10*time.Second).Should(Succeed())
		})
	})

	// --- Failure Scenarios ---
	Context("Failure Scenarios", Label("failure", "bluegreen"), func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			failureCluster  *openbaov1alpha1.OpenBaoCluster
			initialVersion  string
			targetVersion   string
			admin           client.Client
		)

		BeforeAll(func() {
			var err error
			tenantFW, err = framework.NewSetup(ctx, "tenant-failure", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace
			admin = tenantFW.Client

			initialVersion = envOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
			targetVersion = envOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)
			if initialVersion == targetVersion {
				Skip("Failure test skipped")
			}

			// Deliberately remove upgrade capabilities so executor jobs fail and retry/abort logic is exercised.
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

			maxFailures := int32(2)
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
						Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
						Image:    upgradeExecutorImage,
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
						Enabled: true,
						OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
							Enabled: true,
						},
						Requests: append([]openbaov1alpha1.SelfInitRequest{brokenPolicyRequest}, e2ehelpers.CreateE2ERequests(tenantNamespace)...),
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

		It("induces executor failure and validates retry plus auto-abort behavior", func() {
			var failedEarlyAction bluegreen.ExecutorAction

			By("Triggering a Blue/Green upgrade")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: failureCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())

				original := updated.DeepCopy()
				updated.Spec.Version = targetVersion
				updated.Spec.Image = fmt.Sprintf("openbao/openbao:%s", targetVersion)
				g.Expect(admin.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for upgrade to enter an early execution phase")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: failureCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).To(BeElementOf(
					openbaov1alpha1.PhaseDeployingGreen,
					openbaov1alpha1.PhaseJoiningMesh,
					openbaov1alpha1.PhaseSyncing,
					openbaov1alpha1.PhaseIdle,
				))
			}, 15*time.Minute, framework.DefaultPollInterval).Should(Succeed())

			By("Verifying an initial early-phase executor job fails due to induced policy restriction")
			Eventually(func(g Gomega) {
				jobs := &batchv1.JobList{}
				g.Expect(admin.List(ctx, jobs,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{
						constants.LabelAppInstance:      failureCluster.Name,
						constants.LabelOpenBaoCluster:   failureCluster.Name,
						constants.LabelOpenBaoComponent: upgrade.ComponentUpgrade,
					},
				)).To(Succeed())

				candidates := []bluegreen.ExecutorAction{
					bluegreen.ActionJoinGreenNonVoters,
					bluegreen.ActionWaitGreenSynced,
				}

				var observed *batchv1.Job
				for _, action := range candidates {
					job := findUpgradeExecutorJob(jobs.Items, action, "")
					if job == nil || !jobFailed(job) {
						continue
					}
					failedEarlyAction = action
					observed = job
					break
				}

				g.Expect(observed).NotTo(BeNil(), "expected initial failed executor job for one of actions: %v", candidates)
			}, 20*time.Minute, framework.DefaultPollInterval).Should(Succeed())

			By("Verifying retry job for that action is created and also fails")
			Eventually(func(g Gomega) {
				g.Expect(failedEarlyAction).NotTo(BeEmpty(), "failed early-phase action should be discovered before retry assertion")

				jobs := &batchv1.JobList{}
				g.Expect(admin.List(ctx, jobs,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{
						constants.LabelAppInstance:      failureCluster.Name,
						constants.LabelOpenBaoCluster:   failureCluster.Name,
						constants.LabelOpenBaoComponent: upgrade.ComponentUpgrade,
					},
				)).To(Succeed())

				retryJob := findUpgradeExecutorJob(jobs.Items, failedEarlyAction, "retry-1")
				g.Expect(retryJob).NotTo(BeNil(), "retry executor job should exist for action %s", failedEarlyAction)
				g.Expect(jobFailed(retryJob)).To(BeTrue(), "retry executor job should fail for action %s", failedEarlyAction)
			}, 20*time.Minute, framework.DefaultPollInterval).Should(Succeed())

			By("Verifying threshold triggers early-phase abort and cluster returns to idle on original version")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: failureCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle), "upgrade should return to idle after abort")
				g.Expect(updated.Status.BlueGreen.GreenRevision).To(BeEmpty(), "green revision should be cleaned up on abort")
				g.Expect(updated.Status.BlueGreen.RollbackStartTime).To(BeNil(), "early-phase failure should abort, not trigger rollback")
				g.Expect(updated.Status.BlueGreen.JobFailureCount).To(Equal(int32(0)))
				g.Expect(updated.Status.CurrentVersion).To(Equal(initialVersion), "current version should remain at initial version after abort")
			}, 30*time.Minute, framework.DefaultPollInterval).Should(Succeed())
		})
	})

	Context("Late-Phase Rollback Scenarios", Label("failure", "bluegreen", "rollback"), func() {
		var (
			tenantNamespace     string
			tenantFW            *framework.Framework
			rollbackFailCluster *openbaov1alpha1.OpenBaoCluster
			initialVersion      string
			targetVersion       string
			admin               client.Client
		)

		BeforeAll(func() {
			var err error
			tenantFW, err = framework.NewSetup(ctx, "tenant-failure-rollback", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace
			admin = tenantFW.Client

			initialVersion = envOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
			targetVersion = envOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)
			if initialVersion == targetVersion {
				Skip("Late-phase rollback test skipped")
			}

			maxFailures := int32(2)
			autoPromote := false
			rollbackFailCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rollback-failure-cluster",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Version:  initialVersion,
					Image:    fmt.Sprintf("openbao/openbao:%s", initialVersion),
					Replicas: 3,
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						Strategy:    openbaov1alpha1.UpdateStrategyBlueGreen,
						Image:       upgradeExecutorImage,
						JWTAuthRole: constants.RoleNameUpgrade,
						BlueGreen: &openbaov1alpha1.BlueGreenConfig{
							AutoPromote:    autoPromote,
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
			Expect(admin.Create(ctx, rollbackFailCluster)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: rollbackFailCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.CurrentVersion).To(Equal(initialVersion))
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		AfterAll(func() {
			if tenantFW != nil {
				_ = tenantFW.Cleanup(ctx)
			}
		})

		It("triggers late-phase rollback after promotion failures and recovers when auth is restored", func() {
			By("Triggering a Blue/Green upgrade")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: rollbackFailCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())

				original := updated.DeepCopy()
				updated.Spec.Version = targetVersion
				updated.Spec.Image = fmt.Sprintf("openbao/openbao:%s", targetVersion)
				g.Expect(admin.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for Syncing hold (autoPromote=false)")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: rollbackFailCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseSyncing))
				g.Expect(updated.Status.BlueGreen.GreenRevision).NotTo(BeEmpty())
			}, 20*time.Minute, framework.DefaultPollInterval).Should(Succeed())

			By("Verifying wait-green-synced completes before forcing promotion")
			Eventually(func(g Gomega) {
				jobs := &batchv1.JobList{}
				g.Expect(admin.List(ctx, jobs,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{
						constants.LabelAppInstance:      rollbackFailCluster.Name,
						constants.LabelOpenBaoCluster:   rollbackFailCluster.Name,
						constants.LabelOpenBaoComponent: upgrade.ComponentUpgrade,
					},
				)).To(Succeed())
				waitSyncedJob := findUpgradeExecutorJob(jobs.Items, bluegreen.ActionWaitGreenSynced, "")
				g.Expect(waitSyncedJob).NotTo(BeNil(), "wait-green-synced job should exist before promoting")
				g.Expect(jobSucceeded(waitSyncedJob)).To(BeTrue(), "wait-green-synced job should succeed before promoting")
			}, 20*time.Minute, framework.DefaultPollInterval).Should(Succeed())

			By("Introducing a realistic temporary auth misconfiguration right before promotion")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: rollbackFailCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				original := updated.DeepCopy()
				updated.Spec.Upgrade.JWTAuthRole = invalidUpgradeJWTAuthRole
				updated.Spec.Upgrade.BlueGreen.AutoPromote = true
				g.Expect(admin.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Verifying promotion executor job fails")
			Eventually(func(g Gomega) {
				jobs := &batchv1.JobList{}
				g.Expect(admin.List(ctx, jobs,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{
						constants.LabelAppInstance:      rollbackFailCluster.Name,
						constants.LabelOpenBaoCluster:   rollbackFailCluster.Name,
						constants.LabelOpenBaoComponent: upgrade.ComponentUpgrade,
					},
				)).To(Succeed())
				initialPromoteJob := findUpgradeExecutorJob(jobs.Items, bluegreen.ActionPromoteGreenVoters, "")
				g.Expect(initialPromoteJob).NotTo(BeNil(), "initial promote job should exist")
				g.Expect(jobFailed(initialPromoteJob)).To(BeTrue(), "initial promote job should fail")
			}, 20*time.Minute, framework.DefaultPollInterval).Should(Succeed())

			By("Verifying retry promotion job fails and triggers rollback threshold")
			Eventually(func(g Gomega) {
				jobs := &batchv1.JobList{}
				g.Expect(admin.List(ctx, jobs,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{
						constants.LabelAppInstance:      rollbackFailCluster.Name,
						constants.LabelOpenBaoCluster:   rollbackFailCluster.Name,
						constants.LabelOpenBaoComponent: upgrade.ComponentUpgrade,
					},
				)).To(Succeed())
				retryPromoteJob := findUpgradeExecutorJob(jobs.Items, bluegreen.ActionPromoteGreenVoters, "retry-1")
				g.Expect(retryPromoteJob).NotTo(BeNil(), "retry promote job should exist")
				g.Expect(jobFailed(retryPromoteJob)).To(BeTrue(), "retry promote job should fail")
			}, 20*time.Minute, framework.DefaultPollInterval).Should(Succeed())

			By("Restoring JWT auth role so rollback automation can proceed")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: rollbackFailCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				original := updated.DeepCopy()
				updated.Spec.Upgrade.JWTAuthRole = constants.RoleNameUpgrade
				g.Expect(admin.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("Verifying rollback was initiated")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: rollbackFailCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.RollbackStartTime).NotTo(BeNil())
			}, 15*time.Minute, framework.DefaultPollInterval).Should(Succeed())

			By("Verifying rollback completes and cluster returns to stable initial version")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: rollbackFailCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil())
				g.Expect(updated.Status.BlueGreen.Phase).To(Equal(openbaov1alpha1.PhaseIdle))
				g.Expect(updated.Status.BlueGreen.GreenRevision).To(BeEmpty())
				g.Expect(updated.Status.CurrentVersion).To(Equal(initialVersion))
				g.Expect(updated.Status.BreakGlass).To(BeNil(), "rollback should recover without entering break glass")
			}, 30*time.Minute, framework.DefaultPollInterval).Should(Succeed())
		})
	})

	// --- Safe Mode (Chaos) ---
	Context("Safe Mode (chaos)", Label("chaos", "bluegreen"), func() {
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
						Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
						Image:    upgradeExecutorImage,
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
			bypassLabels := map[string]string{
				constants.LabelOpenBaoCluster:   chaosCluster.Name,
				constants.LabelOpenBaoComponent: "backup",
			}

			// Enable KV engine (idempotent)
			Eventually(func(g Gomega) {
				baoAddr, err := e2ehelpers.ResolveActiveOpenBaoAddress(ctx, admin, tenantNamespace, chaosCluster.Name)
				g.Expect(err).NotTo(HaveOccurred())
				err = e2ehelpers.WriteSecretViaJWT(ctx, cfg, admin, tenantNamespace, openBaoImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, secretData)
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultLongWaitTimeout, 10*time.Second).Should(Succeed(), "Failed to write pre-upgrade secret")

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
			secretPath = "secret/safemode-test"
			// bypassLabels is in scope from the beginning of the It block
			// bypassLabels is in scope from the beginning of the It block
			Eventually(func(g Gomega) {
				baoAddr, err := e2ehelpers.ResolveActiveOpenBaoAddress(ctx, admin, tenantNamespace, chaosCluster.Name)
				g.Expect(err).NotTo(HaveOccurred())
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
	Context("Gateway Integration", Label("gateway", "requires-gateway-api", "bluegreen"), func() {
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
						Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
						Image:    upgradeExecutorImage,
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

			By("Verifying external Service remains on Blue while DemotingBlue is in progress")
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
				g.Expect(svc.Spec.Selector).To(HaveKeyWithValue(constants.LabelOpenBaoRevision, updated.Status.BlueGreen.BlueRevision))
			}, 20*time.Minute, framework.DefaultPollInterval).Should(Succeed())

			By("Waiting for Cleanup phase and verifying the external Service selector switches to Green")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: upgradeCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.BlueGreen).NotTo(BeNil(), "BlueGreen status should be initialized")

				if updated.Status.BlueGreen.Phase != openbaov1alpha1.PhaseCleanup {
					_, _ = fmt.Fprintf(GinkgoWriter, "Current phase before cutover: %s\n", updated.Status.BlueGreen.Phase)
					g.Expect(updated.Status.BlueGreen.Phase).ToNot(BeEmpty())
					return
				}

				g.Expect(updated.Status.BlueGreen.GreenRevision).NotTo(BeEmpty(), "GreenRevision should be set during cleanup")

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
