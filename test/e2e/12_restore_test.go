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
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

// createRestoreSelfInitRequests creates SelfInit requests for restore operations.
// This configures JWT auth and a restore policy with permission to call snapshot-force.
func createRestoreSelfInitRequests(ctx context.Context, clusterNamespace, clusterName string, restCfg *rest.Config) ([]openbaov1alpha1.SelfInitRequest, error) {
	// Discover OIDC config using the REST config host
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
		// Backup policy for taking snapshots
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
		// Restore policy for force-restoring snapshots
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
		// Enable KV secrets engine for test data
		{
			Name:      "enable-kv-secrets",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "sys/mounts/secret",
			SecretEngine: &openbaov1alpha1.SelfInitSecretEngine{
				Type: "kv",
				Options: map[string]string{
					"version": "2",
				},
			},
		},
		// Backup JWT role
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
		// Restore JWT role
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

var _ = Describe("Restore", Label("backup", "restore", "cluster", "requires-rustfs", "slow"), Ordered, func() {
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

		admin, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())

		// Ensure RustFS is available in the rustfs namespace
		rustfsNamespace := "rustfs"
		err = ensureRustFS(ctx, admin, cfg, rustfsNamespace)
		if err != nil {
			Skip(fmt.Sprintf("RustFS deployment failed: %v. Skipping restore tests.", err))
		}
	})

	Context("Restore from backup", func() {
		var (
			tenantNamespace   string
			tenantFW          *framework.Framework
			restoreCluster    *openbaov1alpha1.OpenBaoCluster
			credentialsSecret *corev1.Secret
			backupKey         string
		)

		BeforeAll(func() {
			var err error

			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] Creating tenant framework for restore tests...\n")
			// Create separate tenant context for restore tests (enables parallel execution)
			tenantFW, err = framework.New(ctx, admin, "tenant-restore", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace
			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] Tenant namespace created: %s\n", tenantNamespace)

			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] Creating S3 credentials secret for RustFS...\n")
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
			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] S3 credentials secret created\n")

			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] Creating SelfInit requests for JWT auth...\n")
			// Create SelfInit requests with JWT validation pubkeys
			selfInitRequests, err := createRestoreSelfInitRequests(ctx, tenantNamespace, "restore-cluster", cfg)
			Expect(err).NotTo(HaveOccurred(), "failed to create SelfInit requests")
			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] Created %d SelfInit requests\n", len(selfInitRequests))

			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] Creating OpenBaoCluster with backup configuration...\n")
			// Create cluster with backup configuration
			restoreCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "restore-cluster",
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
						APIServerCIDR: kindDefaultServiceCIDR,
					},
					Backup: &openbaov1alpha1.BackupSchedule{
						Schedule:      "*/5 * * * *",
						ExecutorImage: backupExecutorImage,
						JWTAuthRole:   "backup",
						Target: openbaov1alpha1.BackupTarget{
							Endpoint:     fmt.Sprintf("http://%s-svc.%s.svc.cluster.local:9000", rustfsName, "rustfs"),
							Bucket:       rustfsBucket,
							PathPrefix:   "clusters",
							UsePathStyle: true,
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: credentialsSecret.Name,
							},
						},
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, restoreCluster)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] OpenBaoCluster %s created\n", restoreCluster.Name)

			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] Creating NetworkPolicy for restore/backup pods...\n")
			// Create NetworkPolicy for restore/backup pods
			restoreNetworkPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-restore-network-policy", restoreCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/cluster": restoreCluster.Name,
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
						// Allow access to OpenBao pods in same namespace
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
						},
					},
				},
			}
			Expect(admin.Create(ctx, restoreNetworkPolicy)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] NetworkPolicy created\n")

			// Wait for cluster to be ready
			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] Waiting for cluster to become ready (may take up to 5 minutes)...\n")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: restoreCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST]   Cluster status: Initialized=%v, Phase=%s\n",
					updated.Status.Initialized, updated.Status.Phase)

				g.Expect(updated.Status.Initialized).To(BeTrue(), "cluster should be initialized")
			}, 5*time.Minute, 10*time.Second).Should(Succeed())

			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] Cluster is ready!\n")
		})

		AfterAll(func() {
			if tenantFW != nil {
				_ = tenantFW.Cleanup(ctx)
			}
		})

		It("creates a backup via manual trigger", func() {
			// Trigger manual backup
			triggerTime := time.Now().Format(time.RFC3339)
			updated := restoreCluster.DeepCopy()
			if updated.Annotations == nil {
				updated.Annotations = make(map[string]string)
			}
			updated.Annotations["openbao.org/trigger-backup"] = triggerTime
			Expect(admin.Patch(ctx, updated, client.MergeFrom(restoreCluster))).To(Succeed())

			// Wait for backup job to complete
			_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for backup job to complete...\n")
			Eventually(func(g Gomega) {
				jobs := &batchv1.JobList{}
				err := admin.List(ctx, jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"openbao.org/component": "backup",
					"openbao.org/cluster":   restoreCluster.Name,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(jobs.Items)).To(BeNumerically(">", 0), "backup job should be created")

				// Find a successful job
				for _, job := range jobs.Items {
					if jobSucceeded(&job) {
						_, _ = fmt.Fprintf(GinkgoWriter, "Backup job %s completed successfully\n", job.Name)
						return
					}
				}
				g.Expect(false).To(BeTrue(), "no successful backup job found")
			}, 5*time.Minute, 5*time.Second).Should(Succeed())

			// Get the backup key from cluster status
			// The controller needs to reconcile and update status after job completion
			_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] Waiting for cluster backup status to be updated...\n")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: restoreCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Backup).NotTo(BeNil(), "backup status should be populated")

				_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST]   Backup status: LastBackupName=%q, LastBackupTime=%v\n",
					updated.Status.Backup.LastBackupName, updated.Status.Backup.LastBackupTime)

				g.Expect(updated.Status.Backup.LastBackupName).NotTo(BeEmpty(), "last backup name should be set")
				backupKey = updated.Status.Backup.LastBackupName
				_, _ = fmt.Fprintf(GinkgoWriter, "[RESTORE TEST] Backup complete! Key: %s\n", backupKey)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("restores from the backup using OpenBaoRestore CR", func() {
			// Ensure we have a backup key from the previous test
			Expect(backupKey).NotTo(BeEmpty(), "backup key should have been set by previous test")

			// Create OpenBaoRestore CR
			restore := &openbaov1alpha1.OpenBaoRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-restore",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoRestoreSpec{
					Cluster: restoreCluster.Name,
					Source: openbaov1alpha1.RestoreSource{
						Target: openbaov1alpha1.BackupTarget{
							Endpoint:     fmt.Sprintf("http://%s-svc.%s.svc.cluster.local:9000", rustfsName, "rustfs"),
							Bucket:       rustfsBucket,
							UsePathStyle: true,
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: credentialsSecret.Name,
							},
						},
						Key: backupKey,
					},
					JWTAuthRole:   "restore",
					ExecutorImage: backupExecutorImage, // Same image, uses EXECUTOR_MODE=restore
					Force:         true,                // Allow restore even if cluster is healthy
				},
			}

			_, _ = fmt.Fprintf(GinkgoWriter, "Creating OpenBaoRestore CR: %s\n", restore.Name)
			Expect(admin.Create(ctx, restore)).To(Succeed())

			// Wait for restore to transition from Pending to Running (or complete/fail)
			_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for restore to start...\n")
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoRestore{}
				err := admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				_, _ = fmt.Fprintf(GinkgoWriter, "Restore phase: %s, message: %s\n", updated.Status.Phase, updated.Status.Message)

				// Should have moved past Pending
				g.Expect(updated.Status.Phase).NotTo(Equal(openbaov1alpha1.RestorePhasePending),
					"restore should have started processing")
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			// Wait for restore to reach terminal state (Completed or Failed)
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

				// Must reach a terminal state
				g.Expect(finalPhase).To(Or(
					Equal(openbaov1alpha1.RestorePhaseCompleted),
					Equal(openbaov1alpha1.RestorePhaseFailed),
				), "restore should reach terminal state")
			}, 5*time.Minute, 5*time.Second).Should(Succeed())

			// Verify success
			Expect(finalPhase).To(Equal(openbaov1alpha1.RestorePhaseCompleted),
				"restore should complete successfully, but got phase: %s, message: %s", finalPhase, finalMessage)

			_, _ = fmt.Fprintf(GinkgoWriter, "Restore completed successfully!\n")

			// Verify the restore resource status
			finalRestore := &openbaov1alpha1.OpenBaoRestore{}
			Expect(admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, finalRestore)).To(Succeed())
			Expect(finalRestore.Status.SnapshotKey).To(Equal(backupKey), "snapshot key should match")
			Expect(finalRestore.Status.StartTime).NotTo(BeNil(), "start time should be set")
			Expect(finalRestore.Status.CompletionTime).NotTo(BeNil(), "completion time should be set")
		})

		It("cleans up restore resources", func() {
			// Delete any restore CRs created during the test
			restores := &openbaov1alpha1.OpenBaoRestoreList{}
			err := admin.List(ctx, restores, client.InNamespace(tenantNamespace))
			if err == nil {
				for i := range restores.Items {
					_, _ = fmt.Fprintf(GinkgoWriter, "Cleaning up restore: %s\n", restores.Items[i].Name)
					_ = admin.Delete(ctx, &restores.Items[i])
				}
			}
		})
	})
})
