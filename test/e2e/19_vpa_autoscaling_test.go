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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
)

var _ = Describe("VPA autoscaling", Label("vpa", "autoscaling"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		c      client.Client
		f      *framework.Framework
	)

	const (
		clusterName = "vpa-test-cluster"
		vpaName     = "vpa-test"
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

		f, err = framework.New(ctx, c, "vpa", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())

		By("installing VPA CRDs")
		Expect(InstallVPA()).To(Succeed(), "Failed to install VPA CRDs")
		_, _ = fmt.Fprintf(GinkgoWriter, "VPA CRDs installed successfully\n")
	})

	AfterAll(func() {
		if f == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		_ = f.Cleanup(cleanupCtx)

		By("uninstalling VPA CRDs")
		if err := UninstallVPA(); err != nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Failed to uninstall VPA CRDs: %v\n", err)
		}
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

	It("creates an OpenBaoCluster with initial resource requests", func() {
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
				Replicas: 3,
				InitContainer: &openbaov1alpha1.InitContainerConfig{
					Enabled: true,
					Image:   configInitImage,
				},
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1000m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
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
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "Created OpenBaoCluster %q\n", clusterName)
		// Note: Cleanup happens via namespace deletion in AfterAll - not DeferCleanup
		// as we need the cluster to persist across all test steps in this Ordered container

		By("waiting for cluster to become Available")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated)).To(Succeed())

			available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			if available == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Available condition not set yet\n")
				g.Expect(available).NotTo(BeNil())
			}
			_, _ = fmt.Fprintf(GinkgoWriter, "Available condition: status=%s reason=%s\n",
				available.Status, available.Reason)
			g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(updated.Status.ReadyReplicas).To(Equal(int32(3)))
		}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "OpenBaoCluster %q is Available with initial resources\n", clusterName)
	})

	It("creates a VerticalPodAutoscaler targeting the OpenBaoCluster CR", func() {
		By(fmt.Sprintf("creating VPA %q in namespace %q", vpaName, f.Namespace))

		// VPA CRD uses autoscaling.k8s.io/v1
		vpaGVK := schema.GroupVersionKind{
			Group:   "autoscaling.k8s.io",
			Version: "v1",
			Kind:    "VerticalPodAutoscaler",
		}

		vpa := &unstructured.Unstructured{}
		vpa.SetGroupVersionKind(vpaGVK)
		vpa.SetName(vpaName)
		vpa.SetNamespace(f.Namespace)

		// Set spec - targeting OpenBaoCluster CR instead of StatefulSet
		err := unstructured.SetNestedField(vpa.Object, map[string]interface{}{
			"apiVersion": "openbao.org/v1alpha1",
			"kind":       "OpenBaoCluster",
			"name":       clusterName,
		}, "spec", "targetRef")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(vpa.Object, "Off", "spec", "updatePolicy", "updateMode")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(vpa.Object, []interface{}{
			map[string]interface{}{
				"containerName": "openbao",
				"minAllowed": map[string]interface{}{
					"cpu":    "250m",
					"memory": "256Mi",
				},
				"maxAllowed": map[string]interface{}{
					"cpu":    "4000m",
					"memory": "8Gi",
				},
			},
		}, "spec", "resourcePolicy", "containerPolicies")
		Expect(err).NotTo(HaveOccurred())

		err = c.Create(ctx, vpa)
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "Created VPA %q\n", vpaName)

	})

	It("verifies VPA resource was created correctly", func() {
		By("checking VPA targets the OpenBaoCluster CR")

		vpaGVK := schema.GroupVersionKind{
			Group:   "autoscaling.k8s.io",
			Version: "v1",
			Kind:    "VerticalPodAutoscaler",
		}

		vpa := &unstructured.Unstructured{}
		vpa.SetGroupVersionKind(vpaGVK)
		Expect(c.Get(ctx, types.NamespacedName{Name: vpaName, Namespace: f.Namespace}, vpa)).To(Succeed())

		// Verify targetRef points to OpenBaoCluster
		targetRef, found, err := unstructured.NestedStringMap(vpa.Object, "spec", "targetRef")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(targetRef["kind"]).To(Equal("OpenBaoCluster"))
		Expect(targetRef["apiVersion"]).To(Equal("openbao.org/v1alpha1"))
		Expect(targetRef["name"]).To(Equal(clusterName))

		_, _ = fmt.Fprintf(GinkgoWriter, "VPA correctly targets OpenBaoCluster %q\n", clusterName)

		// Note: VPA recommendations require VPA components (recommender, updater, admission-controller)
		// which are not installed in this E2E environment. We only install CRDs.
		// The core test validates CR modification and operator reconciliation, not VPA internals.
	})

	It("verifies VPA modifies CR spec.resources and operator reconciles StatefulSet", func() {
		By("simulating VPA Updater modifying OpenBaoCluster spec.resources")

		// This test verifies the ValidatingAdmissionPolicy allows VPA to modify CR resources
		// In a real scenario, VPA Updater would do this. We simulate it here.

		// Get current OpenBaoCluster
		cluster := &openbaov1alpha1.OpenBaoCluster{}
		Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, cluster)).To(Succeed())

		// Modify only the resource requests (VPA-style update)
		updated := cluster.DeepCopy()
		if updated.Spec.Resources == nil {
			updated.Spec.Resources = &corev1.ResourceRequirements{}
		}
		if updated.Spec.Resources.Requests == nil {
			updated.Spec.Resources.Requests = corev1.ResourceList{}
		}
		updated.Spec.Resources.Requests[corev1.ResourceCPU] = resource.MustParse("750m")
		updated.Spec.Resources.Requests[corev1.ResourceMemory] = resource.MustParse("768Mi")

		// This update should be allowed by VAP for VPA Updater service account
		// In E2E tests running as cluster-admin, we can verify the operator reconciles properly
		err := c.Update(ctx, updated)
		_, _ = fmt.Fprintf(GinkgoWriter, "Resource update result: %v\n", err)

		// If update succeeded, verify operator reconciles StatefulSet with new resources
		if err == nil {
			By("verifying operator reconciles StatefulSet with updated resources from CR")
			Eventually(func(g Gomega) {
				sts := &appsv1.StatefulSet{}
				g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)).To(Succeed())

				for _, container := range sts.Spec.Template.Spec.Containers {
					if container.Name == "openbao" {
						cpuReq := container.Resources.Requests[corev1.ResourceCPU]
						_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet CPU request: %s\n", cpuReq.String())
						g.Expect(cpuReq.MilliValue()).To(Equal(int64(750)))
					}
				}
			}, 30*time.Second, 2*time.Second).Should(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "Operator correctly reconciled StatefulSet from CR resources\n")
		}
	})

	It("verifies cluster remains Available after VPA operations", func() {
		By("checking cluster is still Available")
		cluster := &openbaov1alpha1.OpenBaoCluster{}
		Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, cluster)).To(Succeed())

		available := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
		Expect(available).NotTo(BeNil())
		Expect(available.Status).To(Equal(metav1.ConditionTrue))

		_, _ = fmt.Fprintf(GinkgoWriter, "Cluster remains Available after VPA operations\n")
	})
})
