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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

var _ = Describe("GitOps contract (Argo-like apply)", Label("gitops", "contract"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		c      client.Client
		f      *framework.Framework
	)

	const (
		clusterName = "gitops-contract"

		infraBaoName            = "infra-bao"
		infraBaoKeyName         = "openbao-unseal"
		infraBaoTokenSecretName = "infra-bao-token"
	)

	var infraBaoRootToken string

	BeforeAll(func() {
		var err error

		cfg, err = ctrlconfig.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(openbaov1alpha1.AddToScheme(scheme)).To(Succeed())

		c, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())

		f, err = framework.New(ctx, c, "gitops-contract", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = f.Cleanup(ctx)
		})

		By(fmt.Sprintf("setting up infra-bao instance %q in namespace %q", infraBaoName, f.Namespace))
		infraCfg := e2ehelpers.InfraBaoConfig{
			Namespace: f.Namespace,
			Name:      infraBaoName,
			Image:     openBaoImage,
			RootToken: "placeholder",
		}
		Expect(e2ehelpers.EnsureInfraBao(ctx, c, infraCfg)).To(Succeed())

		infraBaoRootSecret := &corev1.Secret{}
		Expect(c.Get(ctx, types.NamespacedName{
			Name:      infraBaoName + "-root-token",
			Namespace: f.Namespace,
		}, infraBaoRootSecret)).To(Succeed())
		tokenBytes := infraBaoRootSecret.Data["token"]
		Expect(tokenBytes).NotTo(BeEmpty(), "infra-bao root token should be present")
		infraBaoRootToken = strings.TrimSpace(string(tokenBytes))

		By("configuring transit secrets engine on infra-bao")
		infraAddr := fmt.Sprintf("https://%s.%s.svc:8200", infraBaoName, f.Namespace)
		result, err := e2ehelpers.ConfigureInfraBaoTransit(ctx, cfg, c, f.Namespace, openBaoImage, infraAddr, infraBaoRootToken, infraBaoKeyName)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Phase).To(Equal(corev1.PodSucceeded), "infra-bao transit setup failed, logs:\n%s", result.Logs)

		By("creating external TLS secrets required for TLS mode External")
		Expect(e2ehelpers.EnsureExternalTLSSecrets(ctx, c, f.Namespace, clusterName, 1)).To(Succeed())

		By("creating transit credentials secret for the cluster (token + CA cert)")
		infraBaoCASecret := &corev1.Secret{}
		Expect(c.Get(ctx, types.NamespacedName{Name: infraBaoName + "-tls-ca", Namespace: f.Namespace}, infraBaoCASecret)).To(Succeed())
		infraBaoCACert := infraBaoCASecret.Data["ca.crt"]
		Expect(infraBaoCACert).NotTo(BeEmpty(), "infra-bao CA certificate should exist")

		tokenValue := strings.TrimSpace(infraBaoRootToken)
		tokenSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      infraBaoTokenSecretName,
				Namespace: f.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"token":  []byte(tokenValue),
				"ca.crt": infraBaoCACert,
			},
		}
		err = c.Create(ctx, tokenSecret)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("repeatedly applies the same manifest without spec/metadata drift", func() {
		desired := func() *openbaov1alpha1.OpenBaoCluster {
			return &openbaov1alpha1.OpenBaoCluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: openbaov1alpha1.GroupVersion.String(),
					Kind:       "OpenBaoCluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: f.Namespace,
					Labels:    map[string]string{},
					// Intentionally empty for GitOps: no runtime triggers in annotations.
					Annotations: map[string]string{},
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
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeExternal,
					},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled:  true,
						Requests: framework.DefaultAdminSelfInitRequests(),
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "transit",
						Transit: &openbaov1alpha1.TransitSealConfig{
							Address:   fmt.Sprintf("https://%s.%s.svc:8200", infraBaoName, f.Namespace),
							MountPath: "transit",
							KeyName:   infraBaoKeyName,
							Token:     "",
							TLSCACert: "/etc/bao/seal-creds/ca.crt",
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
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
		}

		normalizeStringMap := func(in map[string]string) map[string]string {
			if in == nil {
				return map[string]string{}
			}
			out := make(map[string]string, len(in))
			for k, v := range in {
				out[k] = v
			}
			return out
		}

		assertContract := func(g Gomega, want *openbaov1alpha1.OpenBaoCluster) {
			live := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, live)).To(Succeed())

			g.Expect(live.Spec).To(Equal(want.Spec), "operator must not mutate spec")
			g.Expect(normalizeStringMap(live.Labels)).To(Equal(normalizeStringMap(want.Labels)), "operator must not add labels")
			g.Expect(normalizeStringMap(live.Annotations)).To(Equal(normalizeStringMap(want.Annotations)), "operator must not add annotations")

			g.Expect(live.Finalizers).To(ContainElement(openbaov1alpha1.OpenBaoClusterFinalizer), "finalizer should be present")
		}

		const applyIterations = 3

		for i := 0; i < applyIterations; i++ {
			want := desired()

			By(fmt.Sprintf("server-side applying desired manifest (iteration %d/%d)", i+1, applyIterations))
			Expect(c.Patch(ctx, want, client.Apply, client.FieldOwner("argocd"))).To(Succeed())

			Eventually(func(g Gomega) {
				assertContract(g, want)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())

			// Give the operator a brief window to reconcile between applies; the assertions above
			// must continue to hold even after controller activity.
			time.Sleep(2 * time.Second)
			Consistently(func(g Gomega) {
				assertContract(g, want)
			}, 10*time.Second, 2*time.Second).Should(Succeed())
		}
	})
})
