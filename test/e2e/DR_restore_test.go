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
	"github.com/dc-tec/openbao-operator/internal/auth"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

// Constants and helper functions are defined in 20_storage_providers_backup_test.go to avoid duplication
// Note: createBackupSelfInitRequests in the backup file only creates backup roles.
// For restore tests, we need restore roles too, so we define a restore-specific version here.
func createRestoreSelfInitRequests(ctx context.Context, clusterNamespace, clusterName string, restCfg *rest.Config) ([]openbaov1alpha1.SelfInitRequest, error) {
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
		"jwt_validation_pubkeys": oidcConfig.JWKSKeys,
		"bound_issuer":           oidcConfig.IssuerURL,
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
			Name:      "create-backup-policy",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "sys/policies/acl/backup",
			Policy: &openbaov1alpha1.SelfInitPolicy{
				Policy: `path "sys/storage/raft/snapshot" {
  capabilities = ["read"]
}`,
			},
		},
		{
			Name:      "create-backup-jwt-role",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "auth/jwt/role/backup",
			Data: e2ehelpers.MustJSON(map[string]interface{}{
				"role_type":       "jwt",
				"user_claim":      "sub",
				"bound_audiences": []string{"openbao-internal"},
				"bound_subject":   fmt.Sprintf("system:serviceaccount:%s:%s-backup-serviceaccount", clusterNamespace, clusterName),
				"token_policies":  []string{"backup"},
				"policies":        []string{"backup"},
				"ttl":             "1h",
			}),
		},
		{
			Name:      "create-restore-policy",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "sys/policies/acl/restore",
			Policy: &openbaov1alpha1.SelfInitPolicy{
				Policy: `path "sys/storage/raft/snapshot-force" {
  capabilities = ["update"]
}`,
			},
		},
		{
			Name:      "create-restore-jwt-role",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "auth/jwt/role/restore",
			Data: e2ehelpers.MustJSON(map[string]interface{}{
				"role_type":       "jwt",
				"user_claim":      "sub",
				"bound_audiences": []string{"openbao-internal"},
				"bound_subject":   fmt.Sprintf("system:serviceaccount:%s:%s-restore-serviceaccount", clusterNamespace, clusterName),
				"token_policies":  []string{"restore"},
				"policies":        []string{"restore"},
				"ttl":             "1h",
			}),
		},
	}, nil
}

var _ = Describe("Storage Providers Restore", Label("backup", "restore", "storage-providers", "nightly", "slow"), Ordered, func() {
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
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())

		admin, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())

		// Deploy storage emulators
		rustfsNamespace := "rustfs"
		err = ensureRustFS(ctx, admin, cfg, rustfsNamespace)
		if err != nil {
			Skip(fmt.Sprintf("RustFS deployment failed: %v. Skipping S3 restore tests.", err))
		}

		gcsNamespace := "gcs"
		err = ensureFakeGCS(ctx, admin, gcsNamespace)
		if err != nil {
			Skip(fmt.Sprintf("fake-gcs-server deployment failed: %v. Skipping GCS restore tests.", err))
		}

		azureNamespace := "azure"
		err = ensureAzurite(ctx, admin, azureNamespace)
		if err != nil {
			Skip(fmt.Sprintf("Azurite deployment failed: %v. Skipping Azure restore tests.", err))
		}
	})

	Context("S3 Restore", func() {
		var (
			tenantNamespace   string
			tenantFW          *framework.Framework
			restoreCluster    *openbaov1alpha1.OpenBaoCluster
			credentialsSecret *corev1.Secret
			backupKey         string
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-s3-restore", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

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

			// Create SelfInit requests (includes both backup and restore roles)
			selfInitRequests, err := createRestoreSelfInitRequests(ctx, tenantNamespace, "s3-restore-cluster", cfg)
			Expect(err).NotTo(HaveOccurred())

			// Create cluster with S3 backup configuration
			restoreCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-restore-cluster",
					Namespace: tenantNamespace,
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
						APIServerCIDR: apiServerCIDR,
					},
					Backup: &openbaov1alpha1.BackupSchedule{
						Schedule:      "*/5 * * * *",
						ExecutorImage: backupExecutorImage,
						JWTAuthRole:   "backup",
						Target: openbaov1alpha1.BackupTarget{
							Provider:     constants.StorageProviderS3,
							Endpoint:     rustfsEndpoint,
							Bucket:       rustfsBucket,
							PathPrefix:   "clusters",
							UsePathStyle: true,
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: credentialsSecret.Name,
							},
						},
						Retention: &openbaov1alpha1.BackupRetention{
							MaxCount: 7,
							MaxAge:   "168h",
						},
					},
					Restore: &openbaov1alpha1.RestoreConfig{
						JWTAuthRole: "restore",
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, restoreCluster)).To(Succeed())

			// Create NetworkPolicy for backup/restore pods
			backupNetworkPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-backup-network-policy", restoreCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/component": "backup",
							"openbao.org/cluster":   restoreCluster.Name,
						},
					},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeEgress,
					},
					Egress: []networkingv1.NetworkPolicyEgressRule{
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
							},
						},
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
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"openbao.org/cluster": restoreCluster.Name,
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
			Expect(admin.Create(ctx, backupNetworkPolicy)).To(Succeed())

			// Create NetworkPolicy for restore pods
			restoreNetworkPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-restore-network-policy", restoreCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/component": "restore",
							"openbao.org/cluster":   restoreCluster.Name,
						},
					},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeEgress,
					},
					Egress: []networkingv1.NetworkPolicyEgressRule{
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
							},
						},
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
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"openbao.org/cluster": restoreCluster.Name,
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
			Expect(admin.Create(ctx, restoreNetworkPolicy)).To(Succeed())

			// Wait for cluster to be ready
			Eventually(func(g Gomega) {
				_ = tenantFW.TriggerReconcile(ctx, restoreCluster.Name)

				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: restoreCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.SelfInitialized).To(BeTrue())

				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, restoreCluster.Name)).To(Succeed())
		})

		AfterAll(func() {
			if tenantFW == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			_ = tenantFW.Cleanup(cleanupCtx)
		})

		It("creates a backup via manual trigger to S3", func() {
			triggerTimestamp := time.Now().Format(time.RFC3339Nano)
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: restoreCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				if updated.Annotations == nil {
					updated.Annotations = make(map[string]string)
				}
				updated.Annotations[constants.AnnotationTriggerBackup] = triggerTimestamp
				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, restoreCluster.Name)).To(Succeed())

			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(jobs.Items)).To(BeNumerically(">", 0))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			// Wait for backup job to complete
			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
				})
				g.Expect(err).NotTo(HaveOccurred())

				foundSuccess := false
				for i := range jobs.Items {
					if jobSucceeded(&jobs.Items[i]) {
						foundSuccess = true
						break
					}
				}
				g.Expect(foundSuccess).To(BeTrue())
			}, 15*time.Minute, 30*time.Second).Should(Succeed())

			// Get the backup key from cluster status
			// Trigger reconciles to ensure backup manager processes the completed job
			Eventually(func(g Gomega) {
				// Trigger reconcile to ensure backup manager processes completed job
				_ = tenantFW.TriggerReconcile(ctx, restoreCluster.Name)

				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: restoreCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Backup).NotTo(BeNil(), "backup status should be populated")

				g.Expect(updated.Status.Backup.LastBackupName).NotTo(BeEmpty(), "last backup name should be set")
				backupKey = updated.Status.Backup.LastBackupName
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("restores from S3 backup using OpenBaoRestore CR", func() {
			// Ensure we have a backup key from the previous test
			Expect(backupKey).NotTo(BeEmpty(), "backup key should have been set by previous test")

			// Create OpenBaoRestore CR with S3 provider
			restore := &openbaov1alpha1.OpenBaoRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-restore",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoRestoreSpec{
					Cluster: restoreCluster.Name,
					Source: openbaov1alpha1.RestoreSource{
						Target: openbaov1alpha1.BackupTarget{
							Provider:     constants.StorageProviderS3,
							Endpoint:     rustfsEndpoint,
							Bucket:       rustfsBucket,
							UsePathStyle: true,
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: credentialsSecret.Name,
							},
						},
						Key: backupKey,
					},
					JWTAuthRole:   "restore",
					ExecutorImage: backupExecutorImage,
					Force:         true,
				},
			}

			_, _ = fmt.Fprintf(GinkgoWriter, "Creating OpenBaoRestore CR: %s\n", restore.Name)
			Expect(admin.Create(ctx, restore)).To(Succeed())

			// Wait for restore to transition from Pending to Running
			_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for restore to start...\n")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoRestore{}
				err := admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				_, _ = fmt.Fprintf(GinkgoWriter, "Restore phase: %s, message: %s\n", updated.Status.Phase, updated.Status.Message)

				g.Expect(updated.Status.Phase).NotTo(Equal(openbaov1alpha1.RestorePhasePending),
					"restore should have started processing")
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			// Wait for restore to complete
			_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for restore to complete...\n")
			var finalPhase openbaov1alpha1.RestorePhase
			var finalMessage string

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoRestore{}
				err := admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				finalPhase = updated.Status.Phase
				finalMessage = updated.Status.Message

				_, _ = fmt.Fprintf(GinkgoWriter, "Restore phase: %s, message: %s\n", finalPhase, finalMessage)

				g.Expect(finalPhase).To(Or(
					Equal(openbaov1alpha1.RestorePhaseCompleted),
					Equal(openbaov1alpha1.RestorePhaseFailed),
				), "restore should reach terminal state")
			}, 15*time.Minute, 30*time.Second).Should(Succeed())

			// Verify success
			Expect(finalPhase).To(Equal(openbaov1alpha1.RestorePhaseCompleted),
				"restore should complete successfully, but got phase: %s, message: %s", finalPhase, finalMessage)

			_, _ = fmt.Fprintf(GinkgoWriter, "S3 restore completed successfully!\n")

			// Verify the restore resource status
			finalRestore := &openbaov1alpha1.OpenBaoRestore{}
			Expect(admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, finalRestore)).To(Succeed())
			Expect(finalRestore.Status.SnapshotKey).To(Equal(backupKey), "snapshot key should match")
			Expect(finalRestore.Status.StartTime).NotTo(BeNil(), "start time should be set")
			Expect(finalRestore.Status.CompletionTime).NotTo(BeNil(), "completion time should be set")
		})
	})

	Context("GCS Restore", func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			restoreCluster  *openbaov1alpha1.OpenBaoCluster
			backupKey       string
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-gcs-restore", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			// Create GCS credentials Secret
			err = e2ehelpers.CreateGCSCredentialsSecret(ctx, admin, tenantNamespace, "gcs-credentials", fakeGCSProject)
			Expect(err).NotTo(HaveOccurred())

			// Initialize the bucket in fake-gcs-server
			err = e2ehelpers.CreateFakeGCSBucket(ctx, cfg, admin, "gcs", fakeGCSEndpoint, fakeGCSBucket)
			Expect(err).NotTo(HaveOccurred(), "Failed to create bucket in fake-gcs-server")

			// Create SelfInit requests (includes both backup and restore roles)
			selfInitRequests, err := createRestoreSelfInitRequests(ctx, tenantNamespace, "gcs-restore-cluster", cfg)
			Expect(err).NotTo(HaveOccurred())

			// Create cluster with GCS backup configuration
			restoreCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcs-restore-cluster",
					Namespace: tenantNamespace,
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
						APIServerCIDR: apiServerCIDR,
					},
					Backup: &openbaov1alpha1.BackupSchedule{
						Schedule:      "*/5 * * * *",
						ExecutorImage: backupExecutorImage,
						JWTAuthRole:   "backup",
						Target: openbaov1alpha1.BackupTarget{
							Provider:   "gcs",
							Endpoint:   fakeGCSEndpoint,
							Bucket:     fakeGCSBucket,
							PathPrefix: "clusters",
							GCS: &openbaov1alpha1.GCSTargetConfig{
								Project: fakeGCSProject,
							},
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: "gcs-credentials",
							},
						},
						Retention: &openbaov1alpha1.BackupRetention{
							MaxCount: 7,
							MaxAge:   "168h",
						},
					},
					Restore: &openbaov1alpha1.RestoreConfig{
						JWTAuthRole: "restore",
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, restoreCluster)).To(Succeed())

			// Create NetworkPolicy for backup/restore pods
			backupNetworkPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-backup-network-policy", restoreCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/component": "backup",
							"openbao.org/cluster":   restoreCluster.Name,
						},
					},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeEgress,
					},
					Egress: []networkingv1.NetworkPolicyEgressRule{
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
							},
						},
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "gcs",
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
										p := intstr.FromInt(4443)
										return &p
									}(),
								},
							},
						},
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"openbao.org/cluster": restoreCluster.Name,
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
			Expect(admin.Create(ctx, backupNetworkPolicy)).To(Succeed())

			// Create NetworkPolicy for restore pods
			restoreNetworkPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-restore-network-policy", restoreCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/component": "restore",
							"openbao.org/cluster":   restoreCluster.Name,
						},
					},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeEgress,
					},
					Egress: []networkingv1.NetworkPolicyEgressRule{
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
							},
						},
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "gcs",
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
										p := intstr.FromInt(4443)
										return &p
									}(),
								},
							},
						},
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"openbao.org/cluster": restoreCluster.Name,
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
			Expect(admin.Create(ctx, restoreNetworkPolicy)).To(Succeed())

			// Wait for cluster to be ready
			Eventually(func(g Gomega) {
				_ = tenantFW.TriggerReconcile(ctx, restoreCluster.Name)

				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: restoreCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.SelfInitialized).To(BeTrue())

				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, restoreCluster.Name)).To(Succeed())
		})

		AfterAll(func() {
			if tenantFW == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			_ = tenantFW.Cleanup(cleanupCtx)
		})

		It("creates a backup via manual trigger to GCS", func() {
			triggerTimestamp := time.Now().Format(time.RFC3339Nano)
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: restoreCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				if updated.Annotations == nil {
					updated.Annotations = make(map[string]string)
				}
				updated.Annotations[constants.AnnotationTriggerBackup] = triggerTimestamp
				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, restoreCluster.Name)).To(Succeed())

			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(jobs.Items)).To(BeNumerically(">", 0))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			// Wait for backup job to complete
			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
				})
				g.Expect(err).NotTo(HaveOccurred())

				foundSuccess := false
				for i := range jobs.Items {
					if jobSucceeded(&jobs.Items[i]) {
						foundSuccess = true
						break
					}
				}
				g.Expect(foundSuccess).To(BeTrue())
			}, 15*time.Minute, 30*time.Second).Should(Succeed())

			// Get the backup key from cluster status
			// Trigger reconciles to ensure backup manager processes the completed job
			Eventually(func(g Gomega) {
				// Trigger reconcile to ensure backup manager processes completed job
				_ = tenantFW.TriggerReconcile(ctx, restoreCluster.Name)

				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: restoreCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Backup).NotTo(BeNil(), "backup status should be populated")

				g.Expect(updated.Status.Backup.LastBackupName).NotTo(BeEmpty(), "last backup name should be set")
				backupKey = updated.Status.Backup.LastBackupName
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("restores from GCS backup using OpenBaoRestore CR", func() {
			Skip("Skipping GCS restore due to fake-gcs-server limitations with Go CDK download path (HEAD works, GET fails on root path)")
			// Ensure we have a backup key from the previous test
			Expect(backupKey).NotTo(BeEmpty(), "backup key should have been set by previous test")

			// Create OpenBaoRestore CR with GCS provider
			restore := &openbaov1alpha1.OpenBaoRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcs-restore",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoRestoreSpec{
					Cluster: restoreCluster.Name,
					Source: openbaov1alpha1.RestoreSource{
						Target: openbaov1alpha1.BackupTarget{
							Provider: "gcs",
							Endpoint: fakeGCSEndpoint,
							Bucket:   fakeGCSBucket,
							GCS: &openbaov1alpha1.GCSTargetConfig{
								Project: fakeGCSProject,
							},
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: "gcs-credentials",
							},
						},
						Key: backupKey,
					},
					JWTAuthRole:   "restore",
					ExecutorImage: backupExecutorImage,
					Force:         true,
				},
			}

			_, _ = fmt.Fprintf(GinkgoWriter, "Creating OpenBaoRestore CR: %s\n", restore.Name)
			Expect(admin.Create(ctx, restore)).To(Succeed())

			// Wait for restore to transition from Pending to Running
			_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for restore to start...\n")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoRestore{}
				err := admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				_, _ = fmt.Fprintf(GinkgoWriter, "Restore phase: %s, message: %s\n", updated.Status.Phase, updated.Status.Message)

				g.Expect(updated.Status.Phase).NotTo(Equal(openbaov1alpha1.RestorePhasePending),
					"restore should have started processing")
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			// Wait for restore to complete
			_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for restore to complete...\n")
			var finalPhase openbaov1alpha1.RestorePhase
			var finalMessage string

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoRestore{}
				err := admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				finalPhase = updated.Status.Phase
				finalMessage = updated.Status.Message

				_, _ = fmt.Fprintf(GinkgoWriter, "Restore phase: %s, message: %s\n", finalPhase, finalMessage)

				g.Expect(finalPhase).To(Or(
					Equal(openbaov1alpha1.RestorePhaseCompleted),
					Equal(openbaov1alpha1.RestorePhaseFailed),
				), "restore should reach terminal state")
			}, 15*time.Minute, 30*time.Second).Should(Succeed())

			// Verify success
			Expect(finalPhase).To(Equal(openbaov1alpha1.RestorePhaseCompleted),
				"restore should complete successfully, but got phase: %s, message: %s", finalPhase, finalMessage)

			_, _ = fmt.Fprintf(GinkgoWriter, "GCS restore completed successfully!\n")

			// Verify the restore resource status
			finalRestore := &openbaov1alpha1.OpenBaoRestore{}
			Expect(admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, finalRestore)).To(Succeed())
			Expect(finalRestore.Status.SnapshotKey).To(Equal(backupKey), "snapshot key should match")
			Expect(finalRestore.Status.StartTime).NotTo(BeNil(), "start time should be set")
			Expect(finalRestore.Status.CompletionTime).NotTo(BeNil(), "completion time should be set")
		})
	})

	Context("Azure Restore", func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			restoreCluster  *openbaov1alpha1.OpenBaoCluster
			backupKey       string
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-azure-restore", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			// Create Azure credentials Secret
			err = createAzureCredentialsSecret(ctx, admin, tenantNamespace, "azure-credentials")
			Expect(err).NotTo(HaveOccurred())

			// Initialize the container in Azurite
			err = e2ehelpers.CreateAzuriteContainer(ctx, cfg, admin, "azure", azuriteEndpoint, azuriteContainer, azuriteKey)
			Expect(err).NotTo(HaveOccurred(), "Failed to create container in Azurite")

			// Create SelfInit requests (includes both backup and restore roles)
			selfInitRequests, err := createRestoreSelfInitRequests(ctx, tenantNamespace, "azure-restore-cluster", cfg)
			Expect(err).NotTo(HaveOccurred())

			// Create cluster with Azure backup configuration
			restoreCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-restore-cluster",
					Namespace: tenantNamespace,
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
						APIServerCIDR: apiServerCIDR,
					},
					Backup: &openbaov1alpha1.BackupSchedule{
						Schedule:      "*/5 * * * *",
						ExecutorImage: backupExecutorImage,
						JWTAuthRole:   "backup",
						Target: openbaov1alpha1.BackupTarget{
							Provider:   "azure",
							Endpoint:   azuriteEndpoint,
							Bucket:     azuriteContainer,
							PathPrefix: "clusters",
							Azure: &openbaov1alpha1.AzureTargetConfig{
								StorageAccount: azuriteAccount,
								Container:      azuriteContainer,
							},
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: "azure-credentials",
							},
						},
						Retention: &openbaov1alpha1.BackupRetention{
							MaxCount: 7,
							MaxAge:   "168h",
						},
					},
					Restore: &openbaov1alpha1.RestoreConfig{
						JWTAuthRole: "restore",
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, restoreCluster)).To(Succeed())

			// Create NetworkPolicy for backup/restore pods
			backupNetworkPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-backup-network-policy", restoreCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/component": "backup",
							"openbao.org/cluster":   restoreCluster.Name,
						},
					},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeEgress,
					},
					Egress: []networkingv1.NetworkPolicyEgressRule{
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
							},
						},
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "azure",
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
										p := intstr.FromInt(10000)
										return &p
									}(),
								},
							},
						},
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"openbao.org/cluster": restoreCluster.Name,
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
			Expect(admin.Create(ctx, backupNetworkPolicy)).To(Succeed())

			// Create NetworkPolicy for restore pods
			restoreNetworkPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-restore-network-policy", restoreCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/component": "restore",
							"openbao.org/cluster":   restoreCluster.Name,
						},
					},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeEgress,
					},
					Egress: []networkingv1.NetworkPolicyEgressRule{
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
							},
						},
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "azure",
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
										p := intstr.FromInt(10000)
										return &p
									}(),
								},
							},
						},
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"openbao.org/cluster": restoreCluster.Name,
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
			Expect(admin.Create(ctx, restoreNetworkPolicy)).To(Succeed())

			// Wait for cluster to be ready
			Eventually(func(g Gomega) {
				_ = tenantFW.TriggerReconcile(ctx, restoreCluster.Name)

				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: restoreCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.SelfInitialized).To(BeTrue())

				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, restoreCluster.Name)).To(Succeed())
		})

		AfterAll(func() {
			if tenantFW == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			_ = tenantFW.Cleanup(cleanupCtx)
		})

		It("creates a backup via manual trigger to Azure", func() {
			triggerTimestamp := time.Now().Format(time.RFC3339Nano)
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: restoreCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				if updated.Annotations == nil {
					updated.Annotations = make(map[string]string)
				}
				updated.Annotations[constants.AnnotationTriggerBackup] = triggerTimestamp
				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, restoreCluster.Name)).To(Succeed())

			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(jobs.Items)).To(BeNumerically(">", 0))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			// Wait for backup job to complete
			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
				})
				g.Expect(err).NotTo(HaveOccurred())

				foundSuccess := false
				for i := range jobs.Items {
					if jobSucceeded(&jobs.Items[i]) {
						foundSuccess = true
						break
					}
				}
				g.Expect(foundSuccess).To(BeTrue())
			}, 15*time.Minute, 30*time.Second).Should(Succeed())

			// Get the backup key from cluster status
			// Trigger reconciles to ensure backup manager processes the completed job
			Eventually(func(g Gomega) {
				// Trigger reconcile to ensure backup manager processes completed job
				_ = tenantFW.TriggerReconcile(ctx, restoreCluster.Name)

				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: restoreCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Backup).NotTo(BeNil(), "backup status should be populated")

				g.Expect(updated.Status.Backup.LastBackupName).NotTo(BeEmpty(), "last backup name should be set")
				backupKey = updated.Status.Backup.LastBackupName
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("restores from Azure backup using OpenBaoRestore CR", func() {
			// Ensure we have a backup key from the previous test
			Expect(backupKey).NotTo(BeEmpty(), "backup key should have been set by previous test")

			// Create OpenBaoRestore CR with Azure provider
			restore := &openbaov1alpha1.OpenBaoRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-restore",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoRestoreSpec{
					Cluster: restoreCluster.Name,
					Source: openbaov1alpha1.RestoreSource{
						Target: openbaov1alpha1.BackupTarget{
							Provider: "azure",
							Endpoint: azuriteEndpoint,
							Bucket:   azuriteContainer,
							Azure: &openbaov1alpha1.AzureTargetConfig{
								StorageAccount: azuriteAccount,
								Container:      azuriteContainer,
							},
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: "azure-credentials",
							},
						},
						Key: backupKey,
					},
					JWTAuthRole:   "restore",
					ExecutorImage: backupExecutorImage,
					Force:         true,
				},
			}

			_, _ = fmt.Fprintf(GinkgoWriter, "Creating OpenBaoRestore CR: %s\n", restore.Name)
			Expect(admin.Create(ctx, restore)).To(Succeed())

			// Wait for restore to transition from Pending to Running
			_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for restore to start...\n")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoRestore{}
				err := admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				_, _ = fmt.Fprintf(GinkgoWriter, "Restore phase: %s, message: %s\n", updated.Status.Phase, updated.Status.Message)

				g.Expect(updated.Status.Phase).NotTo(Equal(openbaov1alpha1.RestorePhasePending),
					"restore should have started processing")
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			// Wait for restore to complete
			_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for restore to complete...\n")
			var finalPhase openbaov1alpha1.RestorePhase
			var finalMessage string

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoRestore{}
				err := admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				finalPhase = updated.Status.Phase
				finalMessage = updated.Status.Message

				_, _ = fmt.Fprintf(GinkgoWriter, "Restore phase: %s, message: %s\n", finalPhase, finalMessage)

				g.Expect(finalPhase).To(Or(
					Equal(openbaov1alpha1.RestorePhaseCompleted),
					Equal(openbaov1alpha1.RestorePhaseFailed),
				), "restore should reach terminal state")
			}, 15*time.Minute, 30*time.Second).Should(Succeed())

			// Verify success
			Expect(finalPhase).To(Equal(openbaov1alpha1.RestorePhaseCompleted),
				"restore should complete successfully, but got phase: %s, message: %s", finalPhase, finalMessage)

			_, _ = fmt.Fprintf(GinkgoWriter, "Azure restore completed successfully!\n")

			// Verify the restore resource status
			finalRestore := &openbaov1alpha1.OpenBaoRestore{}
			Expect(admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, finalRestore)).To(Succeed())
			Expect(finalRestore.Status.SnapshotKey).To(Equal(backupKey), "snapshot key should match")
			Expect(finalRestore.Status.StartTime).NotTo(BeNil(), "start time should be set")
			Expect(finalRestore.Status.CompletionTime).NotTo(BeNil(), "completion time should be set")
		})
	})
})
