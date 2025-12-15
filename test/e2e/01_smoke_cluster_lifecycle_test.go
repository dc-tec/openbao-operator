//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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

var _ = Describe("Smoke: Tenant + Cluster lifecycle", Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		c      client.Client
	)

	const (
		tenantNamespace = "e2e-smoke-tenant"
		tenantName      = "smoke-tenant"
		clusterName     = "smoke-cluster"
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

	triggerReconcile := func(cluster *openbaov1alpha1.OpenBaoCluster) {
		original := cluster.DeepCopy()
		annotations := cluster.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations["e2e.openbao.org/reconcile-trigger"] = time.Now().UTC().Format(time.RFC3339Nano)
		cluster.SetAnnotations(annotations)
		Expect(c.Patch(ctx, cluster, client.MergeFrom(original))).To(Succeed())
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

	It("provisions tenant RBAC via OpenBaoTenant", func() {
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
			g.Expect(updated.Status.LastError).To(BeEmpty())
		}, 2*time.Minute, 2*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Tenant %q successfully provisioned\n", tenantName)
	})

	It("creates an OpenBaoCluster and converges to Available", func() {
		By(fmt.Sprintf("creating OpenBaoCluster %q in namespace %q", clusterName, tenantNamespace))
		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
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
					Requests: []openbaov1alpha1.SelfInitRequest{
						{
							Name:      "enable-stdout-audit",
							Operation: openbaov1alpha1.SelfInitOperationUpdate,
							Path:      "sys/audit/stdout",
							Data: mustJSON(map[string]interface{}{
								"type": "file",
								"options": map[string]interface{}{
									"file_path": "/dev/stdout",
									"log_raw":   true,
								},
							}),
						},
					},
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
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		_, _ = fmt.Fprintf(GinkgoWriter, "Created OpenBaoCluster %q\n", clusterName)

		DeferCleanup(func() {
			_ = c.Delete(ctx, cluster)
		})

		By("waiting for OpenBaoCluster to be observed by the API server")
		Eventually(func() error {
			return c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, &openbaov1alpha1.OpenBaoCluster{})
		}, 30*time.Second, 1*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "OpenBaoCluster %q observed by API server\n", clusterName)

		By("checking for prerequisite resources (ConfigMap and TLS Secret)")
		Eventually(func(g Gomega) {
			// Check for ConfigMap
			cm := &corev1.ConfigMap{}
			cmName := types.NamespacedName{Name: clusterName + "-config", Namespace: tenantNamespace}
			err := c.Get(ctx, cmName, cm)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "ConfigMap %q not found yet: %v\n", cmName.Name, err)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "ConfigMap %q exists\n", cmName.Name)
			}
			g.Expect(err).NotTo(HaveOccurred(), "ConfigMap should exist")

			// Check for TLS Secret
			tlsSecret := &corev1.Secret{}
			tlsSecretName := types.NamespacedName{Name: clusterName + "-tls-server", Namespace: tenantNamespace}
			err = c.Get(ctx, tlsSecretName, tlsSecret)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "TLS Secret %q not found yet: %v\n", tlsSecretName.Name, err)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "TLS Secret %q exists\n", tlsSecretName.Name)
			}
			g.Expect(err).NotTo(HaveOccurred(), "TLS Secret should exist")

			// Check cluster status for errors
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, updated)).To(Succeed())

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
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("waiting for StatefulSet to be created")
		Eventually(func(g Gomega) {
			// Check cluster status first to see if there are any blocking conditions
			updated := &openbaov1alpha1.OpenBaoCluster{}
			err := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, updated)
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
			err = c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, sts)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q not found yet: %v\n", clusterName, err)
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(sts.Spec.Replicas).NotTo(BeNil())
			_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q exists with replicas=%d (ready=%d)\n",
				clusterName, *sts.Spec.Replicas, sts.Status.ReadyReplicas)
			g.Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
		}, 2*time.Minute, 2*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q created successfully\n", clusterName)

		By("waiting for StatefulSet pods to become Ready")
		Eventually(func(g Gomega) {
			sts := &appsv1.StatefulSet{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, sts)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet status: replicas=%d ready=%d updated=%d\n",
				sts.Status.Replicas, sts.Status.ReadyReplicas, sts.Status.UpdatedReplicas)
			g.Expect(sts.Status.ReadyReplicas).To(Equal(int32(1)))
		}, 5*time.Minute, 3*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q pods are Ready\n", clusterName)

		By("triggering a reconcile and waiting for Available condition")
		Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, cluster)).To(Succeed())
		triggerReconcile(cluster)
		_, _ = fmt.Fprintf(GinkgoWriter, "Triggered reconcile for cluster %q\n", clusterName)

		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, updated)).To(Succeed())

			available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			if available == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Available condition not set yet\n")
				g.Expect(available).NotTo(BeNil())
			}
			_, _ = fmt.Fprintf(GinkgoWriter, "Available condition: status=%s reason=%s message=%q\n",
				available.Status, available.Reason, available.Message)
			g.Expect(available.Status).To(Equal(metav1.ConditionTrue),
				fmt.Sprintf("Available=%s reason=%s message=%s", available.Status, available.Reason, available.Message))
		}, 2*time.Minute, 2*time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "OpenBaoCluster %q is Available\n", clusterName)
	})
})

// mustJSON converts a Go value to apiextensionsv1.JSON for use in SelfInitRequest.Data
func mustJSON(v interface{}) *apiextensionsv1.JSON {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal JSON: %v", err))
	}
	return &apiextensionsv1.JSON{Raw: raw}
}
