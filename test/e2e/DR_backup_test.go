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
	"github.com/dc-tec/openbao-operator/internal/auth"
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

// createBackupSelfInitRequests creates SelfInit requests for backup operations.
// For Development profile, JWT auth and backup JWT role must be manually configured
// since bootstrap is not automatically provided. This matches the pattern used in upgrade tests.
// Uses jwt_validation_pubkeys (same as operator bootstrap) because OpenBao pods run as
// system:anonymous and cannot access the OIDC discovery endpoint. The test client
// fetches JWKS keys using authenticated REST config, then provides them directly to OpenBao.
func createBackupSelfInitRequests(ctx context.Context, clusterNamespace, clusterName string, restCfg *rest.Config) ([]openbaov1alpha1.SelfInitRequest, error) {
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
				"user_claim":      "sub", // Match operator bootstrap pattern
				"bound_audiences": []string{"openbao-internal"},
				// Use bound_subject to match the sub claim directly (more reliable than bound_claims
				// for projected tokens which may not include kubernetes.io/* claims)
				// The sub claim format is: system:serviceaccount:<namespace>:<serviceaccount-name>
				// and is always present in ServiceAccount tokens
				"bound_subject":  fmt.Sprintf("system:serviceaccount:%s:%s-backup-serviceaccount", clusterNamespace, clusterName),
				"token_policies": []string{"backup"},
				"policies":       []string{"backup"},
				"ttl":            "1h",
			}),
		},
	}, nil
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

var _ = Describe("Storage Providers Backup", Label("backup", "storage-providers", "nightly", "slow"), Ordered, func() {
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

	Context("S3 Backup with RustFS", func() {
		var (
			tenantNamespace   string
			tenantFW          *framework.Framework
			backupCluster     *openbaov1alpha1.OpenBaoCluster
			credentialsSecret *corev1.Secret
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-s3-backup", operatorNamespace)
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

			// Create SelfInit requests
			selfInitRequests, err := createBackupSelfInitRequests(ctx, tenantNamespace, "s3-backup-cluster", cfg)
			Expect(err).NotTo(HaveOccurred())

			// Create cluster with S3 backup configuration
			backupCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-backup-cluster",
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
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, backupCluster)).To(Succeed())

			// Create NetworkPolicy for backup pods
			backupNetworkPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-backup-network-policy", backupCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/component": "backup",
							"openbao.org/cluster":   backupCluster.Name,
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
											"openbao.org/cluster": backupCluster.Name,
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

			// Wait for cluster to be ready
			Eventually(func(g Gomega) {
				_ = tenantFW.TriggerReconcile(ctx, backupCluster.Name)

				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: backupCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.SelfInitialized).To(BeTrue())

				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, backupCluster.Name)).To(Succeed())
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
			triggerTimestamp := time.Now().Format(time.RFC3339Nano)
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: backupCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				if updated.Annotations == nil {
					updated.Annotations = make(map[string]string)
				}
				updated.Annotations[constants.AnnotationTriggerBackup] = triggerTimestamp
				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, backupCluster.Name)).To(Succeed())

			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
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
		})
	})

	Context("GCS Backup with fake-gcs-server", func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			backupCluster   *openbaov1alpha1.OpenBaoCluster
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-gcs-backup", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			// Create GCS credentials Secret
			err = e2ehelpers.CreateGCSCredentialsSecret(ctx, admin, tenantNamespace, "gcs-credentials", fakeGCSProject)
			Expect(err).NotTo(HaveOccurred())

			// Initialize the bucket in fake-gcs-server (required because emulator starts empty)
			err = e2ehelpers.CreateFakeGCSBucket(ctx, cfg, admin, "gcs", fakeGCSEndpoint, fakeGCSBucket)
			Expect(err).NotTo(HaveOccurred(), "Failed to create bucket in fake-gcs-server")

			// Create SelfInit requests
			selfInitRequests, err := createBackupSelfInitRequests(ctx, tenantNamespace, "gcs-backup-cluster", cfg)
			Expect(err).NotTo(HaveOccurred())

			// Create cluster with GCS backup configuration
			backupCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcs-backup-cluster",
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
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, backupCluster)).To(Succeed())

			// Create NetworkPolicy for backup pods
			backupNetworkPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-backup-network-policy", backupCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/component": "backup",
							"openbao.org/cluster":   backupCluster.Name,
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
											"openbao.org/cluster": backupCluster.Name,
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

			// Wait for cluster to be ready
			Eventually(func(g Gomega) {
				_ = tenantFW.TriggerReconcile(ctx, backupCluster.Name)

				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: backupCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.SelfInitialized).To(BeTrue())

				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, backupCluster.Name)).To(Succeed())
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
				err := admin.Get(ctx, types.NamespacedName{Name: backupCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				if updated.Annotations == nil {
					updated.Annotations = make(map[string]string)
				}
				updated.Annotations[constants.AnnotationTriggerBackup] = triggerTimestamp
				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, backupCluster.Name)).To(Succeed())

			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
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
		})
	})

	Context("Azure Backup with Azurite", func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			backupCluster   *openbaov1alpha1.OpenBaoCluster
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-azure-backup", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			// Create Azure credentials Secret
			err = createAzureCredentialsSecret(ctx, admin, tenantNamespace, "azure-credentials")
			Expect(err).NotTo(HaveOccurred())

			// Initialize the container in Azurite (required because emulator starts empty)
			err = e2ehelpers.CreateAzuriteContainer(ctx, cfg, admin, "azure", azuriteEndpoint, azuriteContainer, azuriteKey)
			Expect(err).NotTo(HaveOccurred(), "Failed to create container in Azurite")

			// Create SelfInit requests
			selfInitRequests, err := createBackupSelfInitRequests(ctx, tenantNamespace, "azure-backup-cluster", cfg)
			Expect(err).NotTo(HaveOccurred())

			// Create cluster with Azure backup configuration
			backupCluster = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-backup-cluster",
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
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, backupCluster)).To(Succeed())

			// Create NetworkPolicy for backup pods
			backupNetworkPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-backup-network-policy", backupCluster.Name),
					Namespace: tenantNamespace,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"openbao.org/component": "backup",
							"openbao.org/cluster":   backupCluster.Name,
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
											"openbao.org/cluster": backupCluster.Name,
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

			// Wait for cluster to be ready
			Eventually(func(g Gomega) {
				_ = tenantFW.TriggerReconcile(ctx, backupCluster.Name)

				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: backupCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updated.Status.Initialized).To(BeTrue())
				g.Expect(updated.Status.SelfInitialized).To(BeTrue())

				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, backupCluster.Name)).To(Succeed())
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
				err := admin.Get(ctx, types.NamespacedName{Name: backupCluster.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				original := updated.DeepCopy()
				if updated.Annotations == nil {
					updated.Annotations = make(map[string]string)
				}
				updated.Annotations[constants.AnnotationTriggerBackup] = triggerTimestamp
				err = admin.Patch(ctx, updated, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, backupCluster.Name)).To(Succeed())

			Eventually(func(g Gomega) {
				var jobs batchv1.JobList
				err := admin.List(ctx, &jobs, client.InNamespace(tenantNamespace), client.MatchingLabels{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/component":        "backup",
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
		})
	})
})
