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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

var _ = Describe("Basic: Development profile (operator init + operator-managed TLS)", Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		c      client.Client
	)

	const (
		tenantNamespace = "e2e-basic-tenant"
		tenantName      = "basic-tenant"
		clusterName     = "basic-cluster"
	)

	createNamespace := func(name string) {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					"pod-security.kubernetes.io/enforce": "restricted",
				},
			},
		}

		err := c.Create(ctx, ns)
		if apierrors.IsAlreadyExists(err) {
			return
		}
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %q", name)
	}

	BeforeAll(func() {
		var err error

		cfg, err = ctrlconfig.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(openbaov1alpha1.AddToScheme(scheme)).To(Succeed())

		c, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		_ = c.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: tenantNamespace}})
		_ = c.Delete(ctx, &openbaov1alpha1.OpenBaoTenant{
			ObjectMeta: metav1.ObjectMeta{Name: tenantName, Namespace: operatorNamespace},
		})
	})

	It("creates a cluster with self-init disabled and produces expected Secrets", func() {
		By(fmt.Sprintf("creating tenant namespace %q", tenantNamespace))
		createNamespace(tenantNamespace)
		_, _ = fmt.Fprintf(GinkgoWriter, "Created namespace %q\n", tenantNamespace)

		By(fmt.Sprintf("creating OpenBaoTenant %q in namespace %q", tenantName, operatorNamespace))
		tenant := &openbaov1alpha1.OpenBaoTenant{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tenantName,
				Namespace: operatorNamespace,
			},
			Spec: openbaov1alpha1.OpenBaoTenantSpec{
				TargetNamespace: tenantNamespace,
			},
		}
		err := c.Create(ctx, tenant)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		_, _ = fmt.Fprintf(GinkgoWriter, "Created OpenBaoTenant %q\n", tenantName)

		By("waiting for tenant to be provisioned")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoTenant{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: tenantName, Namespace: operatorNamespace}, updated)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "OpenBaoTenant status: Provisioned=%v, LastError=%q\n", updated.Status.Provisioned, updated.Status.LastError)
			g.Expect(updated.Status.Provisioned).To(BeTrue())
		}, 2*time.Minute, 2*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Tenant %q successfully provisioned\n", tenantName)

		By(fmt.Sprintf("creating OpenBaoCluster %q with Development profile (self-init disabled)", clusterName))
		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: tenantNamespace,
			},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Profile:  openbaov1alpha1.ProfileDevelopment,
				Version:  openBaoVersion,
				Image:    openBaoImage,
				Replicas: 3,
				InitContainer: &openbaov1alpha1.InitContainerConfig{
					Enabled: true,
					Image:   configInitImage,
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
				DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
			},
		}
		Expect(c.Create(ctx, cluster)).To(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Created OpenBaoCluster %q\n", clusterName)
		DeferCleanup(func() {
			_ = c.Delete(ctx, cluster)
		})

		By("waiting for TLS CA Secret to be created")
		Eventually(func() error {
			return c.Get(ctx, types.NamespacedName{Name: clusterName + "-tls-ca", Namespace: tenantNamespace}, &corev1.Secret{})
		}, 2*time.Minute, 2*time.Second).Should(Succeed(), "expected CA Secret to exist")
		_, _ = fmt.Fprintf(GinkgoWriter, "TLS CA Secret %q exists\n", clusterName+"-tls-ca")

		By("waiting for TLS server Secret to be created")
		Eventually(func() error {
			return c.Get(ctx, types.NamespacedName{Name: clusterName + "-tls-server", Namespace: tenantNamespace}, &corev1.Secret{})
		}, 2*time.Minute, 2*time.Second).Should(Succeed(), "expected server TLS Secret to exist")
		_, _ = fmt.Fprintf(GinkgoWriter, "TLS server Secret %q exists\n", clusterName+"-tls-server")

		By("waiting for unseal key Secret to be created")
		Eventually(func() error {
			return c.Get(ctx, types.NamespacedName{Name: clusterName + "-unseal-key", Namespace: tenantNamespace}, &corev1.Secret{})
		}, 2*time.Minute, 2*time.Second).Should(Succeed(), "expected unseal key Secret to exist")
		_, _ = fmt.Fprintf(GinkgoWriter, "Unseal key Secret %q exists\n", clusterName+"-unseal-key")

		By("checking cluster status and conditions before waiting for root token")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, updated)).To(Succeed())

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
			err := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, sts)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q not found yet: %v\n", clusterName, err)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q exists with replicas=%d ready=%d\n",
					clusterName, *sts.Spec.Replicas, sts.Status.ReadyReplicas)
			}

			// Check pod status for debugging
			podList := &corev1.PodList{}
			err = c.List(ctx, podList, client.InNamespace(tenantNamespace), client.MatchingLabels{
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
		}, 1*time.Minute, 2*time.Second).Should(Succeed())

		By("waiting for root token Secret to be created (self-init disabled)")
		Eventually(func(g Gomega) {
			// Check if root token exists
			err := c.Get(ctx, types.NamespacedName{Name: clusterName + "-root-token", Namespace: tenantNamespace}, &corev1.Secret{})
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Root token Secret %q not found yet: %v\n", clusterName+"-root-token", err)

				// Also check cluster status to see why it might not be created
				updated := &openbaov1alpha1.OpenBaoCluster{}
				if getErr := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, updated); getErr == nil {
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
					if listErr := c.List(ctx, podList, client.InNamespace(tenantNamespace), client.MatchingLabels{
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
		}, 5*time.Minute, 3*time.Second).Should(Succeed(), "expected root token Secret to exist when self-init is disabled")
		_, _ = fmt.Fprintf(GinkgoWriter, "Root token Secret %q exists (self-init disabled)\n", clusterName+"-root-token")
	})

})
