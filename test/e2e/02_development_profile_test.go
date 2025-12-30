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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/test/e2e/framework"
)

var _ = Describe("Basic: Development profile (operator init + operator-managed TLS)", Label("profile-development", "cluster"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		c      client.Client
		f      *framework.Framework
	)

	const (
		clusterName = "basic-cluster"
	)

	BeforeAll(func() {
		var err error

		cfg, err = ctrlconfig.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(openbaov1alpha1.AddToScheme(scheme)).To(Succeed())

		c, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())

		f, err = framework.New(ctx, c, "basic", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		if f == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		_ = f.Cleanup(cleanupCtx)
	})

	It("creates a cluster with self-init disabled and produces expected Secrets", func() {
		By(fmt.Sprintf("creating OpenBaoCluster %q with Development profile (self-init disabled)", clusterName))
		cluster, err := f.CreateDevelopmentCluster(ctx, framework.DevelopmentClusterConfig{
			Name:          clusterName,
			Replicas:      3,
			Version:       openBaoVersion,
			Image:         openBaoImage,
			ConfigInitImg: configInitImage,
			APIServerCIDR: kindDefaultServiceCIDR,
		})
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "Created OpenBaoCluster %q in namespace %q\n", clusterName, f.Namespace)
		DeferCleanup(func() {
			_ = c.Delete(ctx, cluster)
		})

		By("waiting for TLS CA Secret to be created")
		Eventually(func() error {
			return c.Get(ctx, types.NamespacedName{Name: clusterName + "-tls-ca", Namespace: f.Namespace}, &corev1.Secret{})
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed(), "expected CA Secret to exist")
		_, _ = fmt.Fprintf(GinkgoWriter, "TLS CA Secret %q exists\n", clusterName+"-tls-ca")

		By("waiting for TLS server Secret to be created")
		Eventually(func() error {
			return c.Get(ctx, types.NamespacedName{Name: clusterName + "-tls-server", Namespace: f.Namespace}, &corev1.Secret{})
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed(), "expected server TLS Secret to exist")
		_, _ = fmt.Fprintf(GinkgoWriter, "TLS server Secret %q exists\n", clusterName+"-tls-server")

		By("waiting for unseal key Secret to be created")
		Eventually(func() error {
			return c.Get(ctx, types.NamespacedName{Name: clusterName + "-unseal-key", Namespace: f.Namespace}, &corev1.Secret{})
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed(), "expected unseal key Secret to exist")
		_, _ = fmt.Fprintf(GinkgoWriter, "Unseal key Secret %q exists\n", clusterName+"-unseal-key")

		By("checking cluster status and conditions before waiting for root token")
		Eventually(func(g Gomega) {
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

			// Log initialization status
			_, _ = fmt.Fprintf(GinkgoWriter, "Cluster status: Initialized=%v SelfInitialized=%v\n",
				updated.Status.Initialized, updated.Status.SelfInitialized)

			// Check if StatefulSet exists
			sts := &appsv1.StatefulSet{}
			err := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q not found yet: %v\n", clusterName, err)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q exists with replicas=%d ready=%d\n",
					clusterName, *sts.Spec.Replicas, sts.Status.ReadyReplicas)
			}

			// Check pod status for debugging
			podList := &corev1.PodList{}
			err = c.List(ctx, podList, client.InNamespace(f.Namespace), client.MatchingLabels{
				"app.kubernetes.io/instance":   clusterName,
				"app.kubernetes.io/name":       "openbao",
				"app.kubernetes.io/managed-by": "openbao-operator",
			})
			if err == nil {
				for _, pod := range podList.Items {
					ready := false
					for _, cond := range pod.Status.Conditions {
						if cond.Type == corev1.PodReady {
							ready = cond.Status == corev1.ConditionTrue
							break
						}
					}
					_, _ = fmt.Fprintf(GinkgoWriter, "Pod %q: phase=%s ready=%v containerStatuses=%d\n",
						pod.Name, pod.Status.Phase, ready, len(pod.Status.ContainerStatuses))
					for _, cs := range pod.Status.ContainerStatuses {
						_, _ = fmt.Fprintf(GinkgoWriter, "  Container %q: ready=%v state=%+v\n",
							cs.Name, cs.Ready, cs.State)
					}
				}
			}
		}, 1*time.Minute, framework.DefaultPollInterval).Should(Succeed())

		By("waiting for root token Secret to be created (self-init disabled)")
		Eventually(func(g Gomega) {
			// Check if root token exists
			err := c.Get(ctx, types.NamespacedName{Name: clusterName + "-root-token", Namespace: f.Namespace}, &corev1.Secret{})
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Root token Secret %q not found yet: %v\n", clusterName+"-root-token", err)

				// Also check cluster status to see why it might not be created
				updated := &openbaov1alpha1.OpenBaoCluster{}
				if getErr := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated); getErr == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Cluster Initialized=%v SelfInitialized=%v, checking conditions...\n",
						updated.Status.Initialized, updated.Status.SelfInitialized)
					for _, cond := range updated.Status.Conditions {
						if cond.Status == metav1.ConditionFalse {
							_, _ = fmt.Fprintf(GinkgoWriter, "  Condition %s is False: reason=%s message=%q\n",
								cond.Type, cond.Reason, cond.Message)
						}
					}

					// Check pod status for more details
					podList := &corev1.PodList{}
					if listErr := c.List(ctx, podList, client.InNamespace(f.Namespace), client.MatchingLabels{
						"app.kubernetes.io/instance":   clusterName,
						"app.kubernetes.io/name":       "openbao",
						"app.kubernetes.io/managed-by": "openbao-operator",
					}); listErr == nil {
						for _, pod := range podList.Items {
							initialized := pod.Labels["openbao-initialized"]
							sealed := pod.Labels["openbao-sealed"]
							_, _ = fmt.Fprintf(GinkgoWriter, "  Pod %q: initialized=%s sealed=%s phase=%s\n",
								pod.Name, initialized, sealed, pod.Status.Phase)
						}
					}
				}
			}
			g.Expect(err).NotTo(HaveOccurred())
		}, framework.DefaultLongWaitTimeout, 3*time.Second).Should(Succeed(), "expected root token Secret to exist when self-init is disabled")
		_, _ = fmt.Fprintf(GinkgoWriter, "Root token Secret %q exists (self-init disabled)\n", clusterName+"-root-token")
	})

})
