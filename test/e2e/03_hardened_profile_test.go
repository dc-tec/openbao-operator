//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

var _ = Describe("Hardened profile (External TLS + Transit auto-unseal + SelfInit)", Label("profile-hardened", "security", "cluster"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		c      client.Client
		f      *framework.Framework
	)

	const (
		clusterName = "hardened-cluster"

		infraBaoName            = "infra-bao"
		infraBaoKeyName         = "openbao-unseal"
		infraBaoTokenSecretName = "infra-bao-token"
	)

	var infraBaoRootToken string

	BeforeAll(func() {
		var err error

		By("setting up test client and scheme")
		cfg, err = ctrlconfig.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(openbaov1alpha1.AddToScheme(scheme)).To(Succeed())

		c, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())

		f, err = framework.New(ctx, c, "hardened", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "Created namespace %q\n", f.Namespace)

		By(fmt.Sprintf("setting up infra-bao instance %q in namespace %q", infraBaoName, f.Namespace))
		infraCfg := e2ehelpers.InfraBaoConfig{
			Namespace: f.Namespace,
			Name:      infraBaoName,
			Image:     openBaoImage,
			// Placeholder; actual root token is captured from init secret below.
			RootToken: "placeholder",
		}
		Expect(e2ehelpers.EnsureInfraBao(ctx, c, infraCfg)).To(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Infra-bao instance %q is running\n", infraBaoName)

		// Fetch the actual infra-bao root token captured during initialization.
		infraBaoRootSecret := &corev1.Secret{}
		Expect(c.Get(ctx, types.NamespacedName{
			Name:      infraBaoName + "-root-token",
			Namespace: f.Namespace,
		}, infraBaoRootSecret)).To(Succeed())
		tokenBytes := infraBaoRootSecret.Data["token"]
		Expect(tokenBytes).NotTo(BeEmpty(), "infra-bao root token should be present")
		infraBaoRootToken = strings.TrimSpace(string(tokenBytes))

		By("configuring transit secrets engine on infra-bao")
		// Infra-bao always runs with TLS in production mode
		infraAddr := fmt.Sprintf("https://%s.%s.svc:8200", infraBaoName, f.Namespace)
		result, err := e2ehelpers.ConfigureInfraBaoTransit(ctx, cfg, c, f.Namespace, openBaoImage, infraAddr, infraBaoRootToken, infraBaoKeyName)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Phase).To(Equal(corev1.PodSucceeded), "infra-bao transit setup failed, logs:\n%s", result.Logs)
		_, _ = fmt.Fprintf(GinkgoWriter, "Transit secrets engine configured with key %q\n", infraBaoKeyName)

		By("verifying root token can access transit key (test encryption)")
		// Pull the actual root token from infra-bao init secret
		verifyToken := strings.TrimSpace(infraBaoRootToken)
		verifySecret := &corev1.Secret{}
		if err := c.Get(ctx, types.NamespacedName{
			Name:      "infra-bao-root-token",
			Namespace: f.Namespace,
		}, verifySecret); err == nil {
			if data, ok := verifySecret.Data["token"]; ok && len(data) > 0 {
				verifyToken = string(data)
			}
		}

		verifyPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "verify-transit-token",
				Namespace: f.Namespace,
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: ptr.To(true),
					RunAsUser:    ptr.To(int64(100)),
					RunAsGroup:   ptr.To(int64(1000)),
					FSGroup:      ptr.To(int64(1000)),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "bao",
						Image: openBaoImage,
						Env: []corev1.EnvVar{
							{Name: "BAO_ADDR", Value: infraAddr},
							{Name: "BAO_TOKEN", Value: verifyToken},
							// Skip TLS verification for self-signed certificates in test environment
							{Name: "BAO_SKIP_VERIFY", Value: "true"},
						},
						Command: []string{"/bin/sh", "-ec"},
						Args: []string{
							fmt.Sprintf("bao write -format=json transit/encrypt/%s plaintext=$(echo -n 'test' | base64) >/dev/null && echo 'ok'", infraBaoKeyName),
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							RunAsNonRoot: ptr.To(true),
						},
					},
				},
			},
		}
		verifyResult, err := e2ehelpers.RunPodUntilCompletion(ctx, cfg, c, verifyPod, 30*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(verifyResult.Phase).To(Equal(corev1.PodSucceeded), "Root token verification failed, logs:\n%s", verifyResult.Logs)
		_, _ = fmt.Fprintf(GinkgoWriter, "Verified root token can encrypt with transit key %q\n", infraBaoKeyName)
		_ = e2ehelpers.DeletePodBestEffort(ctx, c, f.Namespace, verifyPod.Name)
	})

	AfterAll(func() {
		if f == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		_ = f.Cleanup(cleanupCtx)
	})

	It("provisions tenant RBAC via OpenBaoTenant", func() {
		By("verifying OpenBaoTenant is provisioned")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoTenant{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: f.TenantName, Namespace: operatorNamespace}, updated)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "OpenBaoTenant status: Provisioned=%v, LastError=%q\n", updated.Status.Provisioned, updated.Status.LastError)
			g.Expect(updated.Status.Provisioned).To(BeTrue())
			g.Expect(updated.Status.LastError).To(BeEmpty())
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Tenant %q successfully provisioned\n", f.TenantName)
	})

	It("creates a Hardened cluster that self-initializes and stays unsealed across restarts", func() {
		By("creating external TLS secrets required for TLS mode External")
		Expect(e2ehelpers.EnsureExternalTLSSecrets(ctx, c, f.Namespace, clusterName, 1)).To(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Created external TLS secrets for cluster %q\n", clusterName)

		By("creating transit token secret with CA certificate for TLS verification")
		// Get infra-bao CA certificate for TLS verification
		infraBaoCASecret := &corev1.Secret{}
		Expect(c.Get(ctx, types.NamespacedName{Name: infraBaoName + "-tls-ca", Namespace: f.Namespace}, infraBaoCASecret)).To(Succeed())
		infraBaoCACert := infraBaoCASecret.Data["ca.crt"]
		Expect(infraBaoCACert).NotTo(BeEmpty(), "Infra-bao CA certificate should exist")

		// Ensure token has no trailing whitespace/newlines that could cause issues
		// when OpenBao reads it from the token_file
		tokenValue := strings.TrimSpace(infraBaoRootToken)
		tokenSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      infraBaoTokenSecretName,
				Namespace: f.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"token":  []byte(tokenValue),
				"ca.crt": infraBaoCACert, // Include CA cert for TLS verification
			},
		}
		err := c.Create(ctx, tokenSecret)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		_, _ = fmt.Fprintf(GinkgoWriter, "Created transit token secret %q with root token %q\n", infraBaoTokenSecretName, infraBaoRootToken)

		// Verify the token secret was created correctly
		Eventually(func(g Gomega) {
			created := &corev1.Secret{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: infraBaoTokenSecretName, Namespace: f.Namespace}, created)).To(Succeed())
			tokenValue := string(created.Data["token"])
			g.Expect(tokenValue).To(Equal(infraBaoRootToken), "Token secret should contain the root token")
			_, _ = fmt.Fprintf(GinkgoWriter, "Verified token secret contains root token\n")
		}, 10*time.Second, 1*time.Second).Should(Succeed())

		By("verifying transit token secret can be read from file and access infra-bao transit key")
		// Infra-bao always runs with TLS in production mode
		infraAddr := fmt.Sprintf("https://%s.%s.svc:8200", infraBaoName, f.Namespace)
		verifyTokenPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "verify-transit-token-hardened",
				Namespace: f.Namespace,
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: ptr.To(true),
					RunAsUser:    ptr.To(int64(100)),
					RunAsGroup:   ptr.To(int64(1000)),
					FSGroup:      ptr.To(int64(1000)),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "bao",
						Image: openBaoImage,
						Env: []corev1.EnvVar{
							{Name: "BAO_ADDR", Value: infraAddr},
							// Read token from file (same way hardened cluster does)
							{Name: "BAO_TOKEN", Value: ""},
							// Skip TLS verification for self-signed certificates in test environment
							{Name: "BAO_SKIP_VERIFY", Value: "true"},
						},
						Command: []string{"/bin/sh", "-ec"},
						Args: []string{
							// Read token from file and trim whitespace, then test encryption
							// Also debug: show the raw file contents and length
							`echo "DEBUG: Token file contents (hex): $(cat /etc/bao/seal-creds/token | od -An -tx1 | tr -d ' \n')"
							echo "DEBUG: Token file length: $(cat /etc/bao/seal-creds/token | wc -c)"
							TOKEN=$(cat /etc/bao/seal-creds/token | tr -d '\n\r' | xargs)
							if [ -z "$TOKEN" ]; then
								echo "ERROR: Token file is empty"
								exit 1
							fi
							echo "DEBUG: Trimmed token: $TOKEN"
							export BAO_TOKEN="$TOKEN"
							bao write -format=json transit/encrypt/` + infraBaoKeyName + ` plaintext=$(echo -n 'test' | base64) >/dev/null && echo 'ok'`,
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "seal-creds",
								MountPath: "/etc/bao/seal-creds",
								ReadOnly:  true,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							RunAsNonRoot: ptr.To(true),
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "seal-creds",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: infraBaoTokenSecretName,
							},
						},
					},
				},
			},
		}
		verifyResult, err := e2ehelpers.RunPodUntilCompletion(ctx, cfg, c, verifyTokenPod, 30*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(verifyResult.Phase).To(Equal(corev1.PodSucceeded), "Token file verification failed, logs:\n%s", verifyResult.Logs)
		_, _ = fmt.Fprintf(GinkgoWriter, "Verified transit token file can be read and used to encrypt with transit key %q\n", infraBaoKeyName)
		_ = e2ehelpers.DeletePodBestEffort(ctx, c, f.Namespace, verifyTokenPod.Name)

		By(fmt.Sprintf("creating Hardened OpenBaoCluster %q with External TLS and Transit auto-unseal", clusterName))
		// Infra-bao always runs with TLS in production mode
		infraAddr = fmt.Sprintf("https://%s.%s.svc:8200", infraBaoName, f.Namespace)

		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: f.Namespace,
			},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Profile:  openbaov1alpha1.ProfileHardened,
				Version:  openBaoVersion,
				Image:    openBaoImage,
				Replicas: 3,
				InitContainer: &openbaov1alpha1.InitContainerConfig{
					Enabled: true,
					Image:   configInitImage,
				},
				SelfInit: &openbaov1alpha1.SelfInitConfig{
					Enabled: true,
					Requests: []openbaov1alpha1.SelfInitRequest{
						{
							Name:      "enable-userpass-auth",
							Operation: openbaov1alpha1.SelfInitOperationUpdate,
							Path:      "sys/auth/userpass",
							AuthMethod: &openbaov1alpha1.SelfInitAuthMethod{
								Type: "userpass",
								Config: map[string]string{
									"default_lease_ttl":  "0",
									"max_lease_ttl":      "0",
									"listing_visibility": "unauthenticated",
								},
							},
						},
						{
							Name:      "create-admin-policy",
							Operation: openbaov1alpha1.SelfInitOperationUpdate,
							Path:      "sys/policies/acl/admin",
							Policy: &openbaov1alpha1.SelfInitPolicy{
								Policy: `path "*" {
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}`,
							},
						},
					},
				},
				TLS: openbaov1alpha1.TLSConfig{
					Enabled: true,
					Mode:    openbaov1alpha1.TLSModeExternal,
				},
				Unseal: &openbaov1alpha1.UnsealConfig{
					Type: "transit",
					Transit: &openbaov1alpha1.TransitSealConfig{
						Address:   infraAddr,
						MountPath: "transit",
						KeyName:   infraBaoKeyName,
						Token:     "", // Token is provided via VAULT_TOKEN environment variable
						TLSCACert: "/etc/bao/seal-creds/ca.crt",
						// Note: token is provided via VAULT_TOKEN environment variable
						// (set by the operator from CredentialsSecretRef) to avoid issues
						// with trailing newlines in mounted Secret files.
					},
					CredentialsSecretRef: &corev1.LocalObjectReference{
						Name: infraBaoTokenSecretName,
					},
				},
				Storage: openbaov1alpha1.StorageConfig{
					Size: "1Gi",
				},
				Network: &openbaov1alpha1.NetworkConfig{
					APIServerCIDR: kindDefaultServiceCIDR,
					EgressRules: []networkingv1.NetworkPolicyEgressRule{
						{
							// Allow egress to infra-bao in the same namespace for transit seal backend
							To: []networkingv1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"app": infraBaoName,
										},
									},
								},
							},
							Ports: []networkingv1.NetworkPolicyPort{
								{
									Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
									Port:     &[]intstr.IntOrString{intstr.FromInt(8200)}[0],
								},
							},
						},
					},
				},
				DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
			},
		}

		Expect(c.Create(ctx, cluster)).To(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Created OpenBaoCluster %q\n", clusterName)
		DeferCleanup(func() {
			_ = c.Delete(ctx, cluster)
		})

		By("waiting for OpenBaoCluster to be observed by the API server")
		Eventually(func() error {
			return c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, &openbaov1alpha1.OpenBaoCluster{})
		}, 30*time.Second, 1*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "OpenBaoCluster %q observed by API server\n", clusterName)

		By("verifying NetworkPolicy was created")
		Eventually(func(g Gomega) {
			np := &networkingv1.NetworkPolicy{}
			npName := types.NamespacedName{Name: clusterName + "-network-policy", Namespace: f.Namespace}
			g.Expect(c.Get(ctx, npName, np)).To(Succeed())
		}, 30*time.Second, 2*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "NetworkPolicy created successfully\n")

		By("checking for prerequisite resources (ConfigMap and TLS Secrets)")
		Eventually(func(g Gomega) {
			// Check for ConfigMap
			cm := &corev1.ConfigMap{}
			cmName := types.NamespacedName{Name: clusterName + "-config", Namespace: f.Namespace}
			err := c.Get(ctx, cmName, cm)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "ConfigMap %q not found yet: %v\n", cmName.Name, err)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "ConfigMap %q exists\n", cmName.Name)
			}

			// Check for TLS Secrets (External mode requires both CA and server secrets)
			tlsCASecret := &corev1.Secret{}
			tlsCASecretName := types.NamespacedName{Name: clusterName + "-tls-ca", Namespace: f.Namespace}
			err = c.Get(ctx, tlsCASecretName, tlsCASecret)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "TLS CA Secret %q not found yet: %v\n", tlsCASecretName.Name, err)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "TLS CA Secret %q exists\n", tlsCASecretName.Name)
			}

			tlsServerSecret := &corev1.Secret{}
			tlsServerSecretName := types.NamespacedName{Name: clusterName + "-tls-server", Namespace: f.Namespace}
			err = c.Get(ctx, tlsServerSecretName, tlsServerSecret)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "TLS server Secret %q not found yet: %v\n", tlsServerSecretName.Name, err)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "TLS server Secret %q exists\n", tlsServerSecretName.Name)
			}

			// Check cluster status for errors
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated)).To(Succeed())

			// Log all conditions
			for _, cond := range updated.Status.Conditions {
				_, _ = fmt.Fprintf(GinkgoWriter, "Cluster condition: %s=%s reason=%s message=%q\n",
					cond.Type, cond.Status, cond.Reason, cond.Message)
			}

			// Check for degraded condition
			degraded := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
			if degraded != nil && degraded.Status == metav1.ConditionTrue {
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Cluster is Degraded: %s\n", degraded.Message)
			}

			// Check TLSReady condition
			tlsReady := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionTLSReady))
			if tlsReady != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "TLSReady condition: status=%s reason=%s message=%q\n",
					tlsReady.Status, tlsReady.Reason, tlsReady.Message)
			}
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("waiting for StatefulSet to be created")
		Eventually(func(g Gomega) {
			sts := &appsv1.StatefulSet{}
			err := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q not found yet: %v\n", clusterName, err)
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(sts.Spec.Replicas).NotTo(BeNil())
			_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q exists with replicas=%d (ready=%d)\n",
				clusterName, *sts.Spec.Replicas, sts.Status.ReadyReplicas)
		}, 2*time.Minute, 2*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q created successfully\n", clusterName)

		By("waiting for the StatefulSet pod to become Ready (proves auto-unseal worked)")
		Eventually(func(g Gomega) {
			sts := &appsv1.StatefulSet{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet status: replicas=%d ready=%d updated=%d\n",
				sts.Status.Replicas, sts.Status.ReadyReplicas, sts.Status.UpdatedReplicas)
			g.Expect(sts.Status.ReadyReplicas).To(Equal(int32(1)))
		}, 8*time.Minute, 5*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q pod is Ready (auto-unseal successful)\n", clusterName)

		By("waiting for status.initialized=true (self-init, no operator init)")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "Cluster status: Initialized=%v SelfInitialized=%v ReadyReplicas=%d\n",
				updated.Status.Initialized, updated.Status.SelfInitialized, updated.Status.ReadyReplicas)

			// Check StatefulSet status for debugging
			sts := &appsv1.StatefulSet{}
			if err := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts); err == nil {
				var specReplicas int32
				if sts.Spec.Replicas != nil {
					specReplicas = *sts.Spec.Replicas
				}
				_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet status: spec.replicas=%d ready=%d current=%d updated=%d\n",
					specReplicas, sts.Status.ReadyReplicas, sts.Status.CurrentReplicas, sts.Status.UpdatedReplicas)
			}

			available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			if available != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Available condition: status=%s reason=%s message=%q\n",
					available.Status, available.Reason, available.Message)
			}
			g.Expect(updated.Status.Initialized).To(BeTrue())
			g.Expect(updated.Status.SelfInitialized).To(BeTrue())

			available = meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			g.Expect(available).NotTo(BeNil())
			g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
		}, 8*time.Minute, 5*time.Second).Should(Succeed())

		// Trigger a reconcile to ensure status is updated promptly after all replicas are ready.
		// The controller now has requeue logic, but this ensures immediate status update in tests.
		By("triggering reconcile to ensure status is updated")
		Expect(f.TriggerReconcile(ctx, clusterName)).To(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Triggered reconcile for cluster %q\n", clusterName)
		_, _ = fmt.Fprintf(GinkgoWriter, "Cluster %q is initialized via self-init\n", clusterName)

		By("asserting root token and static unseal secrets do NOT exist")
		Consistently(func() bool {
			err := c.Get(ctx, types.NamespacedName{Name: clusterName + "-root-token", Namespace: f.Namespace}, &corev1.Secret{})
			return apierrors.IsNotFound(err)
		}, 15*time.Second, 1*time.Second).Should(BeTrue())
		_, _ = fmt.Fprintf(GinkgoWriter, "Verified root-token Secret does not exist (as expected for self-init)\n")

		Consistently(func() bool {
			err := c.Get(ctx, types.NamespacedName{Name: clusterName + "-unseal-key", Namespace: f.Namespace}, &corev1.Secret{})
			return apierrors.IsNotFound(err)
		}, 15*time.Second, 1*time.Second).Should(BeTrue())
		_, _ = fmt.Fprintf(GinkgoWriter, "Verified unseal-key Secret does not exist (using Transit auto-unseal)\n")

		By("restarting via OpenBaoCluster scale-down/up to respect admission policy")
		clusterObj := &openbaov1alpha1.OpenBaoCluster{}
		Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, clusterObj)).To(Succeed())
		originalReplicas := clusterObj.Spec.Replicas

		// Scale down to 1 (min allowed) to recycle pods without violating validation (replicas>=1)
		clusterObj.Spec.Replicas = 1
		Expect(c.Update(ctx, clusterObj)).To(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Scaled OpenBaoCluster %q down to 1 replica for restart\n", clusterName)

		// Wait for old pods to go away (statefulset will converge to 1)
		Eventually(func(g Gomega) {
			podList := &corev1.PodList{}
			err := c.List(ctx, podList, client.InNamespace(f.Namespace), client.MatchingLabels{
				"app.kubernetes.io/instance":   clusterName,
				"app.kubernetes.io/name":       "openbao",
				"app.kubernetes.io/managed-by": "openbao-operator",
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(len(podList.Items)).To(BeNumerically("<=", 1), "Pods should be scaled down to 1")
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		// Scale back up via the CR
		clusterObj = &openbaov1alpha1.OpenBaoCluster{}
		Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, clusterObj)).To(Succeed())
		clusterObj.Spec.Replicas = originalReplicas
		Expect(c.Update(ctx, clusterObj)).To(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Scaled OpenBaoCluster %q back up to %d replicas\n", clusterName, originalReplicas)

		// Wait for pods to become Ready
		Eventually(func(g Gomega) {
			sts := &appsv1.StatefulSet{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet status after restart: replicas=%d ready=%d\n",
				sts.Status.Replicas, sts.Status.ReadyReplicas)
			g.Expect(sts.Status.ReadyReplicas).To(Equal(originalReplicas))
		}, 8*time.Minute, 5*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Pods restarted via scale-down/up and became Ready (Transit auto-unseal working)\n")
	})
})
