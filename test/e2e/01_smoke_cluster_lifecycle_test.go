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

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
)

var _ = Describe("Smoke: Tenant + Cluster lifecycle", Label("smoke", "critical", "tenant", "cluster"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		c      client.Client
		f      *framework.Framework
	)

	const (
		clusterName = "smoke-cluster"
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

		f, err = framework.New(ctx, c, "smoke", operatorNamespace)
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

	It("creates an OpenBaoCluster and converges to Available", func() {
		By(fmt.Sprintf("creating OpenBaoCluster %q in namespace %q", clusterName, f.Namespace))
		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: f.Namespace,
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
					Requests: framework.DefaultAdminSelfInitRequests(),
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

		err := c.Create(ctx, cluster)
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "Created OpenBaoCluster %q\n", clusterName)

		DeferCleanup(func() {
			_ = c.Delete(ctx, cluster)
		})

		By("waiting for OpenBaoCluster to be observed by the API server")
		Eventually(func() error {
			return c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, &openbaov1alpha1.OpenBaoCluster{})
		}, 30*time.Second, 1*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "OpenBaoCluster %q observed by API server\n", clusterName)

		By("checking for prerequisite resources (ConfigMap and TLS Secret)")
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
			g.Expect(err).NotTo(HaveOccurred(), "ConfigMap should exist")

			// Check for TLS Secret
			tlsSecret := &corev1.Secret{}
			tlsSecretName := types.NamespacedName{Name: clusterName + "-tls-server", Namespace: f.Namespace}
			err = c.Get(ctx, tlsSecretName, tlsSecret)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "TLS Secret %q not found yet: %v\n", tlsSecretName.Name, err)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "TLS Secret %q exists\n", tlsSecretName.Name)
			}
			g.Expect(err).NotTo(HaveOccurred(), "TLS Secret should exist")

			// Check cluster status for errors
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated)).To(Succeed())

			// Log all conditions
			for _, cond := range updated.Status.Conditions {
				_, _ = fmt.Fprintf(GinkgoWriter, "Cluster condition: %s=%s reason=%s message=%q\n",
					cond.Type, cond.Status, cond.Reason, cond.Message)
			}

			// Check for degraded condition - if PrerequisitesMissing, wait for it to clear
			degraded := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
			if degraded != nil && degraded.Status == metav1.ConditionTrue {
				if degraded.Reason == "PrerequisitesMissing" {
					_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Cluster is Degraded (PrerequisitesMissing): %s. Waiting for prerequisites to be ready...\n", degraded.Message)
					// Don't fail yet - prerequisites might still be creating
					return
				}
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Cluster is Degraded: %s\n", degraded.Message)
			}
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("waiting for StatefulSet to be created")
		Eventually(func(g Gomega) {
			// Check cluster status first to see if there are any blocking conditions
			updated := &openbaov1alpha1.OpenBaoCluster{}
			err := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated)
			if err == nil {
				// Log any Degraded conditions that might be blocking StatefulSet creation
				degraded := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
				if degraded != nil && degraded.Status == metav1.ConditionTrue {
					_, _ = fmt.Fprintf(GinkgoWriter, "Cluster is Degraded: reason=%s message=%q\n", degraded.Reason, degraded.Message)
					if degraded.Reason == "PrerequisitesMissing" {
						_, _ = fmt.Fprintf(GinkgoWriter, "Prerequisites still missing, waiting for reconciliation...\n")
					}
				}
			}

			sts := &appsv1.StatefulSet{}
			err = c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q not found yet: %v\n", clusterName, err)
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(sts.Spec.Replicas).NotTo(BeNil())
			_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q exists with replicas=%d (ready=%d)\n",
				clusterName, *sts.Spec.Replicas, sts.Status.ReadyReplicas)
			g.Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q created successfully\n", clusterName)

		By("waiting for StatefulSet pods to become Ready")
		Eventually(func(g Gomega) {
			sts := &appsv1.StatefulSet{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet status: replicas=%d ready=%d updated=%d\n",
				sts.Status.Replicas, sts.Status.ReadyReplicas, sts.Status.UpdatedReplicas)
			g.Expect(sts.Status.ReadyReplicas).To(Equal(int32(1)))
		}, framework.DefaultLongWaitTimeout, 3*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q pods are Ready\n", clusterName)

		By("triggering a reconcile and waiting for Available condition")
		Expect(f.TriggerReconcile(ctx, clusterName)).To(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Triggered reconcile for cluster %q\n", clusterName)

		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated)).To(Succeed())

			available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			if available == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Available condition not set yet\n")
				g.Expect(available).NotTo(BeNil())
			}
			_, _ = fmt.Fprintf(GinkgoWriter, "Available condition: status=%s reason=%s message=%q\n",
				available.Status, available.Reason, available.Message)
			g.Expect(available.Status).To(Equal(metav1.ConditionTrue),
				fmt.Sprintf("Available=%s reason=%s message=%s", available.Status, available.Reason, available.Message))
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "OpenBaoCluster %q is Available\n", clusterName)
	})
})
