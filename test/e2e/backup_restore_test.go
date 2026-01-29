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
	nativev1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

const (
	// RustFS (S3) constants
	rustfsName      = "rustfs"
	rustfsEndpoint  = "http://rustfs-svc.rustfs.svc.cluster.local:9000"
	rustfsBucket    = "openbao-backups"
	rustfsAccessKey = "rustfsadmin"
	rustfsSecretKey = "rustfsadmin"

	// fake-gcs-server constants
	fakeGCSName     = "fake-gcs-server"
	fakeGCSEndpoint = "http://fake-gcs-server.gcs.svc.cluster.local:4443"
	fakeGCSBucket   = "openbao-backups"
	fakeGCSProject  = "test-project"

	// Azurite constants
	azuriteName      = "azurite"
	azuriteEndpoint  = "http://azurite.azure.svc.cluster.local:10000"
	azuriteAccount   = "devstoreaccount1"
	azuriteKey       = "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=="
	azuriteContainer = "openbao-backups"
)

// ensureFakeGCS deploys fake-gcs-server in the cluster for GCS emulator testing.
func ensureFakeGCS(ctx context.Context, c client.Client, namespace string) error {
	cfg := e2ehelpers.DefaultGCSConfig()
	cfg.Namespace = namespace
	cfg.Name = fakeGCSName
	cfg.Project = fakeGCSProject

	_, _ = fmt.Fprintf(GinkgoWriter, "Deploying fake-gcs-server in namespace %q...\n", namespace)
	if err := e2ehelpers.EnsureFakeGCS(ctx, c, cfg); err != nil {
		return fmt.Errorf("failed to deploy fake-gcs-server: %w", err)
	}

	_, _ = fmt.Fprintf(GinkgoWriter, "fake-gcs-server deployed successfully\n")
	return nil
}

// ensureAzurite deploys Azurite in the cluster for Azure emulator testing.
func ensureAzurite(ctx context.Context, c client.Client, namespace string) error {
	cfg := e2ehelpers.DefaultAzuriteConfig()
	cfg.Namespace = namespace
	cfg.Name = azuriteName

	_, _ = fmt.Fprintf(GinkgoWriter, "Deploying Azurite in namespace %q...\n", namespace)
	if err := e2ehelpers.EnsureAzurite(ctx, c, cfg); err != nil {
		return fmt.Errorf("failed to deploy Azurite: %w", err)
	}

	_, _ = fmt.Fprintf(GinkgoWriter, "Azurite deployed successfully\n")
	return nil
}

// createAzureCredentialsSecret creates an Azure credentials Secret with account key.
func createAzureCredentialsSecret(ctx context.Context, c client.Client, namespace, name string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"accountKey": []byte(azuriteKey),
		},
	}

	if err := c.Create(ctx, secret); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create Azure credentials secret: %w", err)
	}
	return nil
}

// ensureRustFS ensures RustFS is deployed in the cluster.
// It creates the namespace if needed and deploys RustFS using the helper.
func ensureRustFS(ctx context.Context, c client.Client, restCfg *rest.Config, namespace string) error {
	// Ensure namespace exists
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	if err := c.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create RustFS namespace: %w", err)
	}

	// Deploy RustFS using the helper
	cfg := e2ehelpers.DefaultRustFSConfig()
	cfg.Namespace = namespace
	cfg.Name = rustfsName
	cfg.AccessKey = rustfsAccessKey
	cfg.SecretKey = rustfsSecretKey
	cfg.Buckets = []string{rustfsBucket} // Create the backup bucket

	_, _ = fmt.Fprintf(GinkgoWriter, "Deploying RustFS in namespace %q...\n", namespace)
	if err := e2ehelpers.EnsureRustFS(ctx, c, restCfg, cfg); err != nil {
		return fmt.Errorf("failed to deploy RustFS: %w", err)
	}

	_, _ = fmt.Fprintf(GinkgoWriter, "RustFS deployed successfully\n")
	return nil
}

// createBackupSelfInitRequests creates SelfInit requests for backup operations.
func createBackupSelfInitRequests(ctx context.Context, clusterNamespace, clusterName string, restCfg *rest.Config) ([]openbaov1alpha1.SelfInitRequest, error) {
	return createSelfInitRequests(ctx, clusterNamespace, clusterName, restCfg, true, false)
}

// createRestoreSelfInitRequests creates SelfInit requests for restore operations (includes both backup and restore roles).
func createRestoreSelfInitRequests(ctx context.Context, clusterNamespace, clusterName string, restCfg *rest.Config) ([]openbaov1alpha1.SelfInitRequest, error) {
	return createSelfInitRequests(ctx, clusterNamespace, clusterName, restCfg, true, true)
}

// createSelfInitRequests creates SelfInit requests for backup and optionally restore operations.
func createSelfInitRequests(ctx context.Context, clusterNamespace, clusterName string, restCfg *rest.Config, includeBackup, includeRestore bool) ([]openbaov1alpha1.SelfInitRequest, error) {
	// BootstrapJWTAuth handles this
	return []openbaov1alpha1.SelfInitRequest{}, nil
}

// createE2ERequests helper removed in favor of e2ehelpers.CreateE2ERequests

var _ = Describe("DR: Storage Providers Backup & Restore", Label("dr", "backup", "restore", "storage-providers", "nightly", "slow"), Ordered, func() {
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
			Skip(fmt.Sprintf("RustFS deployment failed: %v. Skipping S3 tests.", err))
		}

		gcsNamespace := "gcs"
		err = ensureFakeGCS(ctx, admin, gcsNamespace)
		if err != nil {
			Skip(fmt.Sprintf("fake-gcs-server deployment failed: %v. Skipping GCS tests.", err))
		}

		azureNamespace := "azure"
		err = ensureAzurite(ctx, admin, azureNamespace)
		if err != nil {
			Skip(fmt.Sprintf("Azurite deployment failed: %v. Skipping Azure tests.", err))
		}
	})

	Context("S3 Backup & Restore with RustFS", func() {
		var (
			tenantNamespace   string
			tenantFW          *framework.Framework
			drCluster         *openbaov1alpha1.OpenBaoCluster
			credentialsSecret *corev1.Secret
			backupKey         string
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-s3-dr", operatorNamespace)
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

			// Create cluster with S3 backup/restore configuration
			// Using BootstrapJWTAuth to auto-create backup and restore roles
			drCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-dr-cluster",
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
						OIDC:     &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true}, // Operator will auto-create backup and restore roles
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
					Backup: &openbaov1alpha1.BackupSchedule{
						Schedule: "*/5 * * * *",
						Image:    backupExecutorImage,
						// JWTAuthRole not set - operator will auto-create backup role when OIDC is enabled
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
						JWTAuthRole: "restore", // Triggers auto-creation of restore policy/role via bootstrap
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, drCluster)).To(Succeed())

			// Create NetworkPolicy for backup/restore pods
			backupNetworkPolicy := &nativev1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-dr-network-policy", drCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: nativev1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/cluster": drCluster.Name,
						},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "openbao.org/component",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"backup", "restore"},
							},
						},
					},
					PolicyTypes: []nativev1.PolicyType{
						nativev1.PolicyTypeEgress,
					},
					Egress: []nativev1.NetworkPolicyEgressRule{
						{
							To: []nativev1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "kube-system",
										},
									},
								},
							},
							Ports: []nativev1.NetworkPolicyPort{
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
							To: []nativev1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "rustfs",
										},
									},
								},
							},
							Ports: []nativev1.NetworkPolicyPort{
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
							To: []nativev1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"openbao.org/cluster": drCluster.Name,
										},
									},
								},
							},
							Ports: []nativev1.NetworkPolicyPort{
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

			// Wait for cluster to be ready
			Eventually(func(g Gomega) {
				_ = tenantFW.TriggerReconcile(ctx, drCluster.Name)

				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: drCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.SelfInitialized).To(BeTrue())

				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, drCluster.Name)).To(Succeed())
		})

		AfterAll(func() {
			if tenantFW == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			_ = tenantFW.Cleanup(cleanupCtx)
		})

		It("triggers manual backup to S3", func() {
			By("Writing a secret before backup")
			secretPath := "secret/backup-test"
			secretData := map[string]string{"foo": "bar", "version": "v1"}
			bypassLabels := map[string]string{
				constants.LabelOpenBaoCluster:   drCluster.Name,
				constants.LabelOpenBaoComponent: "backup",
			}

			// Enable KV engine (idempotent)
			Eventually(func(g Gomega) {
				baoAddr, err := e2ehelpers.ResolveActiveOpenBaoAddress(ctx, admin, tenantNamespace, drCluster.Name)
				g.Expect(err).NotTo(HaveOccurred())
				err = e2ehelpers.WriteSecretViaJWT(ctx, cfg, admin, tenantNamespace, openBaoImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, secretData)
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultLongWaitTimeout, 10*time.Second).Should(Succeed(), "Failed to write pre-backup secret")

			triggerTimestamp := time.Now().Format(time.RFC3339Nano)
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: drCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				if updated.Annotations == nil {
					updated.Annotations = make(map[string]string)
				}
				updated.Annotations[constants.AnnotationTriggerBackup] = triggerTimestamp
				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, drCluster.Name)).To(Succeed())

			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
					constants.LabelOpenBaoCluster:  drCluster.Name,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(jobs.Items)).To(BeNumerically(">", 0))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		It("executes backup job successfully to S3", func() {
			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
					constants.LabelOpenBaoCluster:  drCluster.Name,
				})
				g.Expect(err).NotTo(HaveOccurred())

				foundSuccess := false
				for i := range jobs.Items {
					if jobs.Items[i].Status.Succeeded > 0 {
						foundSuccess = true
						break
					}
				}
				g.Expect(foundSuccess).To(BeTrue())
			}, 15*time.Minute, 30*time.Second).Should(Succeed())

			// Capture backup key
			Eventually(func(g Gomega) {
				_ = tenantFW.TriggerReconcile(ctx, drCluster.Name)
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: drCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Backup).NotTo(BeNil())
				g.Expect(updated.Status.Backup.LastBackupName).NotTo(BeEmpty())
				backupKey = updated.Status.Backup.LastBackupName
			}, framework.DefaultLongWaitTimeout, 5*time.Second).Should(Succeed())
		})

		It("restores from S3 backup using OpenBaoRestore CR", func() {
			Expect(backupKey).NotTo(BeEmpty(), "backup key should have been set by previous test")

			restore := &openbaov1alpha1.OpenBaoRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-restore",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoRestoreSpec{
					Cluster: drCluster.Name,
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
					JWTAuthRole: "restore",
					Image:       backupExecutorImage,
					Force:       true,
				},
			}

			_, _ = fmt.Fprintf(GinkgoWriter, "Creating OpenBaoRestore CR: %s\n", restore.Name)
			Expect(admin.Create(ctx, restore)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoRestore{}
				err := admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Phase).To(Equal(openbaov1alpha1.RestorePhaseCompleted))
			}, 15*time.Minute, 30*time.Second).Should(Succeed())

			By("Verifying secret persists after restore")
			secretPath := "secret/backup-test"
			bypassLabels := map[string]string{
				constants.LabelOpenBaoCluster:   drCluster.Name,
				constants.LabelOpenBaoComponent: "backup",
			}
			// Note: Restore is destructive, it replaces the data.
			// So after restore, the secret we wrote before backup should be there.
			// Reuse the same JWT logic as before (roles are auto-created by bootstrap)
			Eventually(func(g Gomega) {
				baoAddr, err := e2ehelpers.ResolveActiveOpenBaoAddress(ctx, admin, tenantNamespace, drCluster.Name)
				g.Expect(err).NotTo(HaveOccurred())
				val, err := e2ehelpers.ReadSecretViaJWT(ctx, cfg, admin, tenantNamespace, openBaoImage, baoAddr, "default", "e2e-test", secretPath, bypassLabels, "foo")
				g.Expect(err).NotTo(HaveOccurred(), "Failed to read post-restore secret")
				g.Expect(val).To(Equal("bar"))
			}, framework.DefaultLongWaitTimeout, 10*time.Second).Should(Succeed())
		})
	})

	Context("GCS Backup with fake-gcs-server", func() {
		var (
			tenantNamespace   string
			tenantFW          *framework.Framework
			drCluster         *openbaov1alpha1.OpenBaoCluster
			credentialsSecret *corev1.Secret
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-gcs-dr", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			// Create GCS credentials Secret
			err = e2ehelpers.CreateGCSCredentialsSecret(ctx, admin, tenantNamespace, "gcs-credentials", fakeGCSProject)
			Expect(err).NotTo(HaveOccurred())
			credentialsSecret = &corev1.Secret{}
			Expect(admin.Get(ctx, types.NamespacedName{Name: "gcs-credentials", Namespace: tenantNamespace}, credentialsSecret)).To(Succeed())

			// Initialize the bucket in fake-gcs-server (required because emulator starts empty)
			err = e2ehelpers.CreateFakeGCSBucket(ctx, cfg, admin, "gcs", fakeGCSEndpoint, fakeGCSBucket)
			Expect(err).NotTo(HaveOccurred(), "Failed to create bucket in fake-gcs-server")

			// Create cluster with GCS backup configuration
			// Using BootstrapJWTAuth to auto-create backup role (restore skipped due to fake-gcs-server limitations)
			drCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcs-dr-cluster",
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
						Enabled: true,
						OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
							Enabled: true,
						}, // Operator will auto-create backup role
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
					Backup: &openbaov1alpha1.BackupSchedule{
						Schedule: "*/5 * * * *",
						Image:    backupExecutorImage,
						Target: openbaov1alpha1.BackupTarget{
							Provider:   constants.StorageProviderGCS,
							Endpoint:   fakeGCSEndpoint,
							Bucket:     fakeGCSBucket,
							PathPrefix: "clusters",
							GCS: &openbaov1alpha1.GCSTargetConfig{
								Project: fakeGCSProject,
							},
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: credentialsSecret.Name,
							},
						},
						Retention: &openbaov1alpha1.BackupRetention{
							MaxCount: 7,
							MaxAge:   "168h",
						},
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, drCluster)).To(Succeed())

			// Create NetworkPolicy for backup pods
			backupNetworkPolicy := &nativev1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-dr-network-policy", drCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: nativev1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/cluster": drCluster.Name,
						},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "openbao.org/component",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"backup"},
							},
						},
					},
					PolicyTypes: []nativev1.PolicyType{
						nativev1.PolicyTypeEgress,
					},
					Egress: []nativev1.NetworkPolicyEgressRule{
						{
							To: []nativev1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "kube-system",
										},
									},
								},
							},
							Ports: []nativev1.NetworkPolicyPort{
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
							To: []nativev1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "gcs",
										},
									},
								},
							},
							Ports: []nativev1.NetworkPolicyPort{
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
							To: []nativev1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"openbao.org/cluster": drCluster.Name,
										},
									},
								},
							},
							Ports: []nativev1.NetworkPolicyPort{
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

			// Wait for cluster to be ready
			Eventually(func(g Gomega) {
				_ = tenantFW.TriggerReconcile(ctx, drCluster.Name)

				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: drCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.SelfInitialized).To(BeTrue())

				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, drCluster.Name)).To(Succeed())
		})

		AfterAll(func() {
			if tenantFW == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			_ = tenantFW.Cleanup(cleanupCtx)
		})

		It("triggers manual backup to GCS", func() {
			triggerTimestamp := time.Now().Format(time.RFC3339Nano)
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: drCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				if updated.Annotations == nil {
					updated.Annotations = make(map[string]string)
				}
				updated.Annotations[constants.AnnotationTriggerBackup] = triggerTimestamp
				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, drCluster.Name)).To(Succeed())

			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
					constants.LabelOpenBaoCluster:  drCluster.Name,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(jobs.Items)).To(BeNumerically(">", 0))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		It("executes backup job successfully to GCS", func() {
			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
					constants.LabelOpenBaoCluster:  drCluster.Name,
				})
				g.Expect(err).NotTo(HaveOccurred())

				foundSuccess := false
				for i := range jobs.Items {
					if jobs.Items[i].Status.Succeeded > 0 {
						foundSuccess = true
						break
					}
				}
				g.Expect(foundSuccess).To(BeTrue())
			}, 15*time.Minute, 30*time.Second).Should(Succeed())
		})

		It("restores from GCS backup using OpenBaoRestore CR", func() {
			Skip("GCS restore test skipped due to limitations with fake-gcs-server")
		})
	})

	Context("Azure Backup & Restore with Azurite", func() {
		var (
			tenantNamespace   string
			tenantFW          *framework.Framework
			drCluster         *openbaov1alpha1.OpenBaoCluster
			credentialsSecret *corev1.Secret
			backupKey         string
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-azure-dr", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			// Create Azure credentials Secret
			err = createAzureCredentialsSecret(ctx, admin, tenantNamespace, "azure-credentials")
			Expect(err).NotTo(HaveOccurred())
			credentialsSecret = &corev1.Secret{}
			Expect(admin.Get(ctx, types.NamespacedName{Name: "azure-credentials", Namespace: tenantNamespace}, credentialsSecret)).To(Succeed())

			// Initialize the container in Azurite (required because emulator starts empty)
			err = e2ehelpers.CreateAzuriteContainer(ctx, cfg, admin, "azure", azuriteEndpoint, azuriteContainer, azuriteKey)
			Expect(err).NotTo(HaveOccurred(), "Failed to create container in Azurite")

			// Create cluster with Azure backup/restore configuration
			// Using BootstrapJWTAuth to auto-create backup and restore roles
			drCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-dr-cluster",
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
						OIDC:     &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true}, // Operator will auto-create backup and restore roles
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
					Backup: &openbaov1alpha1.BackupSchedule{
						Schedule: "*/5 * * * *",
						Image:    backupExecutorImage,
						// JWTAuthRole not set - operator will auto-create backup role when OIDC is enabled
						Target: openbaov1alpha1.BackupTarget{
							Provider:   constants.StorageProviderAzure,
							Endpoint:   azuriteEndpoint,
							Bucket:     azuriteContainer,
							PathPrefix: "clusters",
							Azure: &openbaov1alpha1.AzureTargetConfig{
								StorageAccount: azuriteAccount,
								Container:      azuriteContainer,
							},
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
						JWTAuthRole: "restore", // Triggers auto-creation of restore policy/role via bootstrap
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, drCluster)).To(Succeed())

			// Create NetworkPolicy for backup/restore pods
			backupNetworkPolicy := &nativev1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-dr-network-policy", drCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: nativev1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/cluster": drCluster.Name,
						},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "openbao.org/component",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"backup", "restore"},
							},
						},
					},
					PolicyTypes: []nativev1.PolicyType{
						nativev1.PolicyTypeEgress,
					},
					Egress: []nativev1.NetworkPolicyEgressRule{
						{
							To: []nativev1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "kube-system",
										},
									},
								},
							},
							Ports: []nativev1.NetworkPolicyPort{
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
							To: []nativev1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "azure",
										},
									},
								},
							},
							Ports: []nativev1.NetworkPolicyPort{
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
							To: []nativev1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"openbao.org/cluster": drCluster.Name,
										},
									},
								},
							},
							Ports: []nativev1.NetworkPolicyPort{
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

			// Wait for cluster to be ready
			Eventually(func(g Gomega) {
				_ = tenantFW.TriggerReconcile(ctx, drCluster.Name)

				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: drCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.SelfInitialized).To(BeTrue())

				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, drCluster.Name)).To(Succeed())
		})

		AfterAll(func() {
			if tenantFW == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			_ = tenantFW.Cleanup(cleanupCtx)
		})

		It("triggers manual backup to Azure", func() {
			triggerTimestamp := time.Now().Format(time.RFC3339Nano)
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: drCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				if updated.Annotations == nil {
					updated.Annotations = make(map[string]string)
				}
				updated.Annotations[constants.AnnotationTriggerBackup] = triggerTimestamp
				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, drCluster.Name)).To(Succeed())

			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
					constants.LabelOpenBaoCluster:  drCluster.Name,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(jobs.Items)).To(BeNumerically(">", 0))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		It("executes backup job successfully to Azure", func() {
			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
					constants.LabelOpenBaoCluster:  drCluster.Name,
				})
				g.Expect(err).NotTo(HaveOccurred())

				foundSuccess := false
				for i := range jobs.Items {
					if jobs.Items[i].Status.Succeeded > 0 {
						foundSuccess = true
						break
					}
				}
				g.Expect(foundSuccess).To(BeTrue())
			}, 15*time.Minute, 30*time.Second).Should(Succeed())

			// Capture backup key
			Eventually(func(g Gomega) {
				_ = tenantFW.TriggerReconcile(ctx, drCluster.Name)
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: drCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Backup).NotTo(BeNil())
				g.Expect(updated.Status.Backup.LastBackupName).NotTo(BeEmpty())
				backupKey = updated.Status.Backup.LastBackupName
			}, framework.DefaultLongWaitTimeout, 5*time.Second).Should(Succeed())
		})

		It("restores from Azure backup using OpenBaoRestore CR", func() {
			Expect(backupKey).NotTo(BeEmpty(), "backup key should have been set by previous test")

			restore := &openbaov1alpha1.OpenBaoRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-restore",
					Namespace: tenantNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoRestoreSpec{
					Cluster: drCluster.Name,
					Source: openbaov1alpha1.RestoreSource{
						Target: openbaov1alpha1.BackupTarget{
							Provider:   constants.StorageProviderAzure,
							Endpoint:   azuriteEndpoint,
							Bucket:     azuriteContainer,
							PathPrefix: "clusters",
							Azure: &openbaov1alpha1.AzureTargetConfig{
								StorageAccount: azuriteAccount,
								Container:      azuriteContainer,
							},
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: credentialsSecret.Name,
							},
						},
						Key: backupKey,
					},
					JWTAuthRole: "restore",
					Image:       backupExecutorImage,
					Force:       true,
				},
			}

			_, _ = fmt.Fprintf(GinkgoWriter, "Creating OpenBaoRestore CR: %s\n", restore.Name)
			Expect(admin.Create(ctx, restore)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoRestore{}
				err := admin.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Phase).To(Equal(openbaov1alpha1.RestorePhaseCompleted))
			}, 15*time.Minute, 30*time.Second).Should(Succeed())
		})
	})
})
