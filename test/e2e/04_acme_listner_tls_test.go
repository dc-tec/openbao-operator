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

var _ = Describe("ACME TLS (OpenBao native ACME client)", Label("tls", "security"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		c      client.Client
		f      *framework.Framework
	)

	const (
		clusterName = "acme-cluster"

		infraBaoName            = "infra-bao"
		infraBaoKeyName         = "openbao-unseal"
		infraBaoTokenSecretName = "infra-bao-token-acme"
	)

	var (
		infraBaoRootToken string
		infraBaoCASecret  *corev1.Secret
		infraBaoPKICA     []byte
	)

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

		f, err = framework.New(ctx, c, "acme", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "Created namespace %q\n", f.Namespace)

		By(fmt.Sprintf("setting up infra-bao instance %q in namespace %q (production mode with TLS)", infraBaoName, f.Namespace))
		infraCfg := e2ehelpers.InfraBaoConfig{
			Namespace: f.Namespace,
			Name:      infraBaoName,
			Image:     openBaoImage,
			// Placeholder; actual root token is captured from init secret below.
			RootToken: "placeholder",
		}
		Expect(e2ehelpers.EnsureInfraBao(ctx, c, infraCfg)).To(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Infra-bao instance %q is running in production mode with TLS\n", infraBaoName)

		// Fetch the actual root token captured during infra-bao initialization.
		rootTokenSecret := &corev1.Secret{}
		Expect(c.Get(ctx, types.NamespacedName{
			Name:      infraBaoName + "-root-token",
			Namespace: f.Namespace,
		}, rootTokenSecret)).To(Succeed())
		tokenBytes := rootTokenSecret.Data["token"]
		Expect(tokenBytes).NotTo(BeEmpty(), "infra-bao root token should be present")
		infraBaoRootToken = strings.TrimSpace(string(tokenBytes))

		// Fetch infra-bao CA certificate for TLS verification when using transit seal.
		infraBaoCASecret = &corev1.Secret{}
		Expect(c.Get(ctx, types.NamespacedName{
			Name:      infraBaoName + "-tls-ca",
			Namespace: f.Namespace,
		}, infraBaoCASecret)).To(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Fetched infra-bao CA secret %q\n", infraBaoName+"-tls-ca")
		Expect(infraBaoCASecret.Data["ca.crt"]).NotTo(BeEmpty(), "infra-bao CA certificate should exist")

		By("configuring PKI secrets engine with ACME support on infra-bao")
		// Use HTTPS since infra-bao is configured with TLS for ACME tests
		infraAddr := fmt.Sprintf("https://%s.%s.svc:8200", infraBaoName, f.Namespace)
		clusterPath := infraAddr + "/v1/pki"

		result, err := e2ehelpers.ConfigureInfraBaoPKIACME(ctx, cfg, c, f.Namespace, openBaoImage, infraAddr, infraBaoRootToken, clusterPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Phase).To(Equal(corev1.PodSucceeded), "infra-bao pki/acme setup failed, logs:\n%s", result.Logs)
		_, _ = fmt.Fprintf(GinkgoWriter, "PKI secrets engine configured with ACME support at path %q\n", clusterPath)

		By("configuring transit secrets engine on infra-bao")
		result, err = e2ehelpers.ConfigureInfraBaoTransit(ctx, cfg, c, f.Namespace, openBaoImage, infraAddr, infraBaoRootToken, infraBaoKeyName)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Phase).To(Equal(corev1.PodSucceeded), "infra-bao transit setup failed, logs:\n%s", result.Logs)
		_, _ = fmt.Fprintf(GinkgoWriter, "Transit secrets engine configured with key %q\n", infraBaoKeyName)

		By("fetching PKI CA certificate from infra-bao for ACME certificate verification")
		infraBaoPKICA, err = e2ehelpers.FetchInfraBaoPKICA(ctx, cfg, c, f.Namespace, openBaoImage, infraAddr)
		Expect(err).NotTo(HaveOccurred())
		Expect(infraBaoPKICA).NotTo(BeEmpty(), "PKI CA certificate should not be empty")
		_, _ = fmt.Fprintf(GinkgoWriter, "Fetched PKI CA certificate from infra-bao (length: %d bytes)\n", len(infraBaoPKICA))
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
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Tenant %q successfully provisioned\n", f.TenantName)
	})

	It("creates an ACME TLS cluster and becomes Ready (no TLS secrets mounted)", func() {
		By("setting up ACME service and domain")
		// Use HTTPS for ACME directory URL as OpenBao requires HTTPS for non-internal CAs
		// Note: infra-bao is running in dev mode (HTTP), but for ACME tests we need HTTPS.
		// The directory URL must use HTTPS even if infra-bao itself uses HTTP internally.
		// In a real scenario, the ACME CA would be external (e.g., Let's Encrypt) and use HTTPS.
		infraAddr := fmt.Sprintf("https://%s.%s.svc:8200", infraBaoName, f.Namespace)
		acmeDirectoryURL := infraAddr + "/v1/pki/acme/directory"

		// Create a dedicated service at port 443 to support standard ACME TLS-ALPN-01 validation.
		// The service forwards 443 -> 8200 (OpenBao listener port).
		acmeServiceName := clusterName + "-acme"
		acmeDomain := fmt.Sprintf("%s.%s.svc", acmeServiceName, f.Namespace)
		_, _ = fmt.Fprintf(GinkgoWriter, "ACME directory URL: %q, domain: %q\n", acmeDirectoryURL, acmeDomain)

		By(fmt.Sprintf("creating ACME service %q on ports 80 and 443", acmeServiceName))
		acmeSvc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      acmeServiceName,
				Namespace: f.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name": "openbao-acme",
				},
			},
			Spec: corev1.ServiceSpec{
				// Allow traffic to reach the pod while it is initializing/waiting for certs.
				// This is required for ACME validation: the ACME server must be able to reach
				// the OpenBao pod to complete the ACME challenges (HTTP-01 on port 80, TLS-ALPN-01 on port 443),
				// but the pod cannot become Ready until it has a valid certificate, creating a circular dependency
				// without this setting.
				PublishNotReadyAddresses: true,
				Selector: map[string]string{
					"app.kubernetes.io/name":       "openbao",
					"app.kubernetes.io/instance":   clusterName,
					"openbao.org/cluster":          clusterName,
					"app.kubernetes.io/managed-by": "openbao-operator",
				},
				Ports: []corev1.ServicePort{
					{
						Name:       "http-80",
						Protocol:   corev1.ProtocolTCP,
						Port:       80,
						TargetPort: intstr.FromInt(8200),
					},
					{
						Name:       "https-443",
						Protocol:   corev1.ProtocolTCP,
						Port:       443,
						TargetPort: intstr.FromInt(8200),
					},
				},
			},
		}
		err := c.Create(ctx, acmeSvc)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		_, _ = fmt.Fprintf(GinkgoWriter, "Created ACME service %q\n", acmeServiceName)
		DeferCleanup(func() { _ = c.Delete(ctx, acmeSvc) })

		By("creating transit token secret for auto-unseal (include TLS CA for transit and PKI CA for ACME)")
		tokenSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      infraBaoTokenSecretName,
				Namespace: f.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"token":      []byte(infraBaoRootToken),
				"ca.crt":     infraBaoCASecret.Data["ca.crt"], // TLS CA for transit seal
				"pki-ca.crt": infraBaoPKICA,                   // PKI CA for ACME certificate verification
			},
		}
		err = c.Create(ctx, tokenSecret)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		_, _ = fmt.Fprintf(GinkgoWriter, "Created transit token secret %q\n", infraBaoTokenSecretName)

		// Verify the token in the secret works with infra-bao before the cluster tries to use it
		By("verifying transit token secret can access infra-bao transit key")
		// Use HTTPS and skip TLS verification for test environment
		infraAddr = fmt.Sprintf("https://%s.%s.svc:8200", infraBaoName, f.Namespace)
		verifyTokenPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "verify-transit-token-acme",
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
							{Name: "BAO_TOKEN", Value: infraBaoRootToken},
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
		verifyResult, err := e2ehelpers.RunPodUntilCompletion(ctx, cfg, c, verifyTokenPod, 30*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(verifyResult.Phase).To(Equal(corev1.PodSucceeded), "Token verification failed, logs:\n%s", verifyResult.Logs)
		_, _ = fmt.Fprintf(GinkgoWriter, "Verified transit token can encrypt with transit key %q\n", infraBaoKeyName)
		_ = e2ehelpers.DeletePodBestEffort(ctx, c, f.Namespace, verifyTokenPod.Name)

		By(fmt.Sprintf("creating OpenBaoCluster %q with ACME TLS mode", clusterName))
		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: f.Namespace,
			},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Profile:  openbaov1alpha1.ProfileHardened,
				Version:  openBaoVersion,
				Image:    openBaoImage,
				Replicas: 1,
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
					Mode:    openbaov1alpha1.TLSModeACME,
					ACME: &openbaov1alpha1.ACMEConfig{
						DirectoryURL: acmeDirectoryURL,
						Domain:       acmeDomain,
						Email:        "e2e@example.invalid",
					},
				},
				Configuration: &openbaov1alpha1.OpenBaoConfiguration{
					ACMECARoot: "/etc/bao/seal-creds/ca.crt", // TLS CA for verifying ACME directory server (infra-bao)
				},
				Unseal: &openbaov1alpha1.UnsealConfig{
					Type: "transit",
					Transit: &openbaov1alpha1.TransitSealConfig{
						Address:   infraAddr,
						MountPath: "transit",
						KeyName:   infraBaoKeyName,
						Token:     "", // Token is provided via token_file in Secret
						TLSCACert: "/etc/bao/seal-creds/ca.crt",
						// Note: tls_skip_verify is not set because:
						// 1. Hardened profile validation policy disallows tls_skip_verify=true
						// 2. When using HTTP (not HTTPS), TLS verification is not applicable
					},
					CredentialsSecretRef: &corev1.SecretReference{
						Name: infraBaoTokenSecretName,
					},
				},
				Storage: openbaov1alpha1.StorageConfig{
					Size: "1Gi",
				},
				Network: &openbaov1alpha1.NetworkConfig{
					APIServerCIDR: kindDefaultServiceCIDR,
					IngressRules: []networkingv1.NetworkPolicyIngressRule{
						{
							// Allow ingress from same namespace for ACME validation.
							// The ACME server (infra-bao) needs to connect for both:
							// - HTTP-01 challenge: service port 80 -> pod port 8200
							// - TLS-ALPN-01 challenge: service port 443 -> pod port 8200
							// NetworkPolicy is evaluated at the pod level, so we need to allow port 8200.
							// Since infra-bao is in the same namespace, we can use a pod selector or allow all in namespace.
							From: []networkingv1.NetworkPolicyPeer{
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
					EgressRules: []networkingv1.NetworkPolicyEgressRule{
						{
							// Allow egress to infra-bao in the same namespace for ACME directory access
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
				// Ensure the headless service exists (used for raft), and our additional
				// ACME validation service routes to the same pods.
				DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
			},
		}
		Expect(c.Create(ctx, cluster)).To(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Created OpenBaoCluster %q with ACME TLS\n", clusterName)
		DeferCleanup(func() { _ = c.Delete(ctx, cluster) })

		By("waiting for OpenBaoCluster to be observed by the API server")
		Eventually(func() error {
			return c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, &openbaov1alpha1.OpenBaoCluster{})
		}, 30*time.Second, 1*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "OpenBaoCluster %q observed by API server\n", clusterName)

		By("verifying TLS secrets are NOT created (ACME mode)")
		Consistently(func() bool {
			err := c.Get(ctx, types.NamespacedName{Name: clusterName + "-tls-server", Namespace: f.Namespace}, &corev1.Secret{})
			return apierrors.IsNotFound(err)
		}, 30*time.Second, 2*time.Second).Should(BeTrue(), "tls-server Secret should not exist in ACME mode")
		_, _ = fmt.Fprintf(GinkgoWriter, "Verified tls-server Secret does not exist (as expected for ACME mode)\n")

		Consistently(func() bool {
			err := c.Get(ctx, types.NamespacedName{Name: clusterName + "-tls-ca", Namespace: f.Namespace}, &corev1.Secret{})
			return apierrors.IsNotFound(err)
		}, 30*time.Second, 2*time.Second).Should(BeTrue(), "tls-ca Secret should not exist in ACME mode")
		_, _ = fmt.Fprintf(GinkgoWriter, "Verified tls-ca Secret does not exist (as expected for ACME mode)\n")

		By("checking for prerequisite resources (ConfigMap)")
		Eventually(func(g Gomega) {
			// Check for ConfigMap (required before StatefulSet creation)
			cm := &corev1.ConfigMap{}
			cmName := types.NamespacedName{Name: clusterName + "-config", Namespace: f.Namespace}
			err := c.Get(ctx, cmName, cm)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "ConfigMap %q not found yet: %v\n", cmName.Name, err)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "ConfigMap %q exists\n", cmName.Name)
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

			// Check TLSReady condition (should be True or not set for ACME mode)
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

		By("waiting for StatefulSet pods to become Ready")
		Eventually(func(g Gomega) {
			sts := &appsv1.StatefulSet{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet status: replicas=%d ready=%d updated=%d\n",
				sts.Status.Replicas, sts.Status.ReadyReplicas, sts.Status.UpdatedReplicas)
			g.Expect(sts.Status.ReadyReplicas).To(Equal(int32(1)))
		}, 10*time.Minute, 5*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q pods are Ready\n", clusterName)

		By("validating that the config contains ACME parameters")
		// Validate that the config rendered by the operator contains the ACME parameters.
		// We run a short-lived pod that queries the OpenBao pod's config via the API server
		// using kubectl is not available here, so we inspect the config ConfigMap instead,
		// which must contain the tls_acme settings when TLS mode is ACME.
		Eventually(func(g Gomega) {
			cm := &corev1.ConfigMap{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName + "-config", Namespace: f.Namespace}, cm)).To(Succeed())
			cfgText := cm.Data["config.hcl"]
			_, _ = fmt.Fprintf(GinkgoWriter, "Checking config.hcl for ACME parameters...\n")
			g.Expect(cfgText).To(ContainSubstring("tls_acme_ca_directory"))
			g.Expect(cfgText).To(ContainSubstring(acmeDirectoryURL))
			g.Expect(cfgText).To(ContainSubstring("tls_acme_domains"))
			g.Expect(cfgText).To(ContainSubstring(acmeDomain))
		}, 2*time.Minute, 2*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "ConfigMap contains ACME parameters (tls_acme_ca_directory, tls_acme_domains)\n")
	})
})
