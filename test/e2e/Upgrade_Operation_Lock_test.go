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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/backup"
	upgradecore "github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

var _ = Describe("Upgrade Strategies: Operation Lock Contention", Label("upgrade", "backup", "operation-lock", "slow"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg               *rest.Config
		tenantFW          *framework.Framework
		tenantNamespace   string
		admin             client.Client
		lockCluster       *openbaov1alpha1.OpenBaoCluster
		initialVersion    string
		initialImage      string
		targetVersion     string
		targetImage       string
		credentialsSecret *corev1.Secret
	)

	BeforeAll(func() {
		var err error

		cfg, err = ctrlconfig.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		tenantFW, err = framework.NewSetup(ctx, "tenant-upgrade-lock", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		tenantNamespace = tenantFW.Namespace
		admin = tenantFW.Client

		initialVersion = envOrDefault("E2E_UPGRADE_FROM_VERSION", defaultUpgradeFromVersion)
		targetVersion = envOrDefault("E2E_UPGRADE_TO_VERSION", defaultUpgradeToVersion)
		initialImage = fmt.Sprintf("openbao/openbao:%s", initialVersion)
		targetImage = fmt.Sprintf("openbao/openbao:%s", targetVersion)

		if initialVersion == targetVersion {
			Skip(fmt.Sprintf("Operation lock contention test skipped: versions identical (%s)", initialVersion))
		}

		err = ensureRustFS(ctx, admin, cfg)
		if err != nil {
			Skip(fmt.Sprintf("RustFS deployment failed: %v. Skipping operation lock contention test.", err))
		}

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

		lockCluster = &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "upgrade-lock-cluster",
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
				Upgrade: &openbaov1alpha1.UpgradeConfig{
					Image: upgradeExecutorImage,
				},
				Backup: &openbaov1alpha1.BackupSchedule{
					Schedule: "0 0 * * *",
					Image:    backupExecutorImage,
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
				DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
			},
		}
		Expect(admin.Create(ctx, lockCluster)).To(Succeed())

		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(admin.Get(ctx, types.NamespacedName{Name: lockCluster.Name, Namespace: tenantNamespace}, updated)).To(Succeed())
			g.Expect(updated.Status.Initialized).To(BeTrue())
			g.Expect(updated.Status.CurrentVersion).To(Equal(initialVersion))

			available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			g.Expect(available).NotTo(BeNil())
			g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
		}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		_, err = tenantFW.WaitForStatefulSetReady(
			ctx,
			lockCluster.Name,
			3,
			framework.DefaultLongWaitTimeout,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		if tenantFW != nil {
			_ = tenantFW.Cleanup(ctx)
		}
	})

	It("holds a manual backup request until the rolling upgrade lock is released", Label(
		"e2e-anchor",
		"case:upgrade-backup-lock-contention",
		"covers:operation-lock",
		"covers:rolling-upgrade",
		"covers:backup-queueing",
	), func() {
		clusterKey := types.NamespacedName{Name: lockCluster.Name, Namespace: tenantNamespace}
		preTriggerJobUIDs := map[types.UID]struct{}{}

		By("starting a rolling upgrade")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(admin.Get(ctx, clusterKey, updated)).To(Succeed())
			original := updated.DeepCopy()
			updated.Spec.Version = targetVersion
			updated.Spec.Image = targetImage
			g.Expect(admin.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		Expect(tenantFW.TriggerReconcile(ctx, lockCluster.Name)).To(Succeed())

		By("waiting for the upgrade controller to hold the cluster operation lock")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(admin.Get(ctx, clusterKey, updated)).To(Succeed())
			g.Expect(updated.Status.Upgrade).NotTo(BeNil())
			g.Expect(updated.Status.OperationLock).NotTo(BeNil())
			g.Expect(updated.Status.OperationLock.Operation).To(Equal(openbaov1alpha1.ClusterOperationUpgrade))
			g.Expect(updated.Status.OperationLock.Holder).To(Equal(upgradecore.UpgradeOperationLockHolder))

			upgrading := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
			g.Expect(upgrading).NotTo(BeNil())
			g.Expect(upgrading.Status).To(Equal(metav1.ConditionTrue))
		}, 10*time.Minute, framework.DefaultPollInterval).Should(Succeed())

		By("requesting a manual backup while the upgrade lock is held")
		{
			jobs := &batchv1.JobList{}
			Expect(admin.List(ctx, jobs,
				client.InNamespace(tenantNamespace),
				client.MatchingLabels{
					constants.LabelOpenBaoCluster:   lockCluster.Name,
					constants.LabelOpenBaoComponent: backup.ComponentBackup,
				},
			)).To(Succeed())
			for i := range jobs.Items {
				preTriggerJobUIDs[jobs.Items[i].UID] = struct{}{}
			}
		}
		Expect(triggerManualBackup(ctx, admin, tenantNamespace, lockCluster.Name)).To(Succeed())

		By("verifying the backup request remains queued behind the active upgrade")
		Consistently(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(admin.Get(ctx, clusterKey, updated)).To(Succeed())
			g.Expect(updated.Status.OperationLock).NotTo(BeNil())
			g.Expect(updated.Status.OperationLock.Operation).To(Equal(openbaov1alpha1.ClusterOperationUpgrade))
			g.Expect(updated.Status.OperationLock.Holder).To(Equal(upgradecore.UpgradeOperationLockHolder))
			g.Expect(updated.Annotations).To(HaveKey(constants.AnnotationTriggerBackup))

			jobs := &batchv1.JobList{}
			g.Expect(admin.List(ctx, jobs,
				client.InNamespace(tenantNamespace),
				client.MatchingLabels{
					constants.LabelOpenBaoCluster:   lockCluster.Name,
					constants.LabelOpenBaoComponent: backup.ComponentBackup,
				},
			)).To(Succeed())
			g.Expect(jobs.Items).To(BeEmpty(), "backup job should not start while the upgrade lock is held")
		}, 20*time.Second, 5*time.Second).Should(Succeed())

		By("waiting for the upgrade to complete")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(admin.Get(ctx, clusterKey, updated)).To(Succeed())
			g.Expect(updated.Status.Upgrade).To(BeNil())
			g.Expect(updated.Status.CurrentVersion).To(Equal(targetVersion))

			available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			g.Expect(available).NotTo(BeNil())
			g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
		}, 20*time.Minute, 10*time.Second).Should(Succeed())

		By("re-triggering reconcile so the queued backup request is picked up immediately")
		Expect(tenantFW.TriggerReconcile(ctx, lockCluster.Name)).To(Succeed())

		By("verifying the queued backup starts once the upgrade lock is released")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(admin.Get(ctx, clusterKey, updated)).To(Succeed())
			if updated.Annotations != nil {
				g.Expect(updated.Annotations).NotTo(HaveKey(constants.AnnotationTriggerBackup))
			}

			jobs := &batchv1.JobList{}
			g.Expect(admin.List(ctx, jobs,
				client.InNamespace(tenantNamespace),
				client.MatchingLabels{
					constants.LabelOpenBaoCluster:   lockCluster.Name,
					constants.LabelOpenBaoComponent: backup.ComponentBackup,
				},
			)).To(Succeed())
			hasNewJob := false
			for i := range jobs.Items {
				if _, found := preTriggerJobUIDs[jobs.Items[i].UID]; found {
					continue
				}
				hasNewJob = true
				break
			}
			g.Expect(hasNewJob).To(BeTrue(), "queued backup should create a backup job after the upgrade lock is released")
			g.Expect(updated.Status.Backup).NotTo(BeNil())
			g.Expect(updated.Status.Backup.LastAttemptScheduledTime).NotTo(BeNil())

			// Backup execution is short in CI, so the lock may already be released by the time we observe the new Job.
			if updated.Status.OperationLock != nil {
				g.Expect(updated.Status.OperationLock.Operation).To(Equal(openbaov1alpha1.ClusterOperationBackup))
			}
		}, 5*time.Minute, 10*time.Second).Should(Succeed())
	})
})
