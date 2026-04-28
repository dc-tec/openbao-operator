//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
)

var _ = Describe("Cluster Runtime Controls", Label("lifecycle", "cluster", "runtime"), Ordered, func() {
	ctx := context.Background()

	var (
		f *framework.Framework
		c client.Client
	)

	newDevelopmentCluster := func(name string) *openbaov1alpha1.OpenBaoCluster {
		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
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
				DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
			},
		}

		return cluster
	}

	waitForClusterAvailable := func(clusterName string) {
		Eventually(func(g Gomega) {
			cluster := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, cluster)).To(Succeed())
			available := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			g.Expect(available).NotTo(BeNil())
			g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
		}, 10*time.Minute, framework.DefaultPollInterval).Should(Succeed())
	}

	waitForClusterPod := func(clusterName string) *corev1.Pod {
		var readyPod *corev1.Pod
		Eventually(func(g Gomega) {
			pods := &corev1.PodList{}
			g.Expect(c.List(ctx, pods,
				client.InNamespace(f.Namespace),
				client.MatchingLabels{
					constants.LabelOpenBaoCluster: clusterName,
				},
			)).To(Succeed())

			found := false
			for i := range pods.Items {
				pod := &pods.Items[i]
				if pod.DeletionTimestamp != nil || !isPodReady(pod) {
					continue
				}
				copy := pod.DeepCopy()
				readyPod = copy
				found = true
				break
			}
			g.Expect(found).To(BeTrue())
		}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		return readyPod
	}

	parseServerCertificate := func(secret *corev1.Secret) *x509.Certificate {
		certPEM := secret.Data["tls.crt"]
		Expect(certPEM).NotTo(BeEmpty())

		block, _ := pem.Decode(certPEM)
		Expect(block).NotTo(BeNil())

		cert, err := x509.ParseCertificate(block.Bytes)
		Expect(err).NotTo(HaveOccurred())
		return cert
	}

	BeforeAll(func() {
		var err error
		f, err = framework.NewSetup(ctx, "cluster-runtime", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		c = f.Client
	})

	AfterAll(func() {
		if f == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		_ = f.Cleanup(cleanupCtx)
	})

	It("pauses reconciliation until spec.paused is cleared", Label(
		"case:cluster-paused-resume",
		"covers:paused-reconcile",
		"covers:paused-status",
		"lower-layer-covered",
	), func() {
		const clusterName = "paused-cluster"

		cluster := newDevelopmentCluster(clusterName)
		cluster.Spec.Paused = true
		Expect(c.Create(ctx, cluster)).To(Succeed())
		DeferCleanup(func() { _ = c.Delete(ctx, cluster) })

		By("waiting for paused status conditions to be reported")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated)).To(Succeed())

			available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			g.Expect(available).NotTo(BeNil())
			g.Expect(available.Reason).To(Equal("Paused"))

			degraded := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
			g.Expect(degraded).NotTo(BeNil())
			g.Expect(degraded.Reason).To(Equal("Paused"))

			tlsReady := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionTLSReady))
			g.Expect(tlsReady).NotTo(BeNil())
			g.Expect(tlsReady.Reason).To(Equal("Paused"))
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("verifying workload resources are not created while reconciliation is paused")
		Consistently(func(g Gomega) {
			sts := &appsv1.StatefulSet{}
			err := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

			serverSecret := &corev1.Secret{}
			err = c.Get(ctx, types.NamespacedName{Name: clusterName + "-tls-server", Namespace: f.Namespace}, serverSecret)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}, 20*time.Second, framework.DefaultPollInterval).Should(Succeed())

		By("clearing spec.paused so normal reconciliation resumes")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated)).To(Succeed())
			original := updated.DeepCopy()
			updated.Spec.Paused = false
			g.Expect(c.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		_, err := f.WaitForStatefulSetReady(ctx, clusterName, 1, 10*time.Minute, framework.DefaultPollInterval)
		Expect(err).NotTo(HaveOccurred())
		waitForClusterAvailable(clusterName)
	})

	It("rolls the OpenBao pod when runtime.restartAt changes", Label(
		"case:cluster-restart-at-rolls-pod",
		"covers:restart-at",
		"covers:pod-rollout",
	), func() {
		const clusterName = "restart-at-cluster"

		cluster := newDevelopmentCluster(clusterName)
		Expect(c.Create(ctx, cluster)).To(Succeed())
		DeferCleanup(func() { _ = c.Delete(ctx, cluster) })

		_, err := f.WaitForStatefulSetReady(ctx, clusterName, 1, 10*time.Minute, framework.DefaultPollInterval)
		Expect(err).NotTo(HaveOccurred())
		waitForClusterAvailable(clusterName)
		initialPod := waitForClusterPod(clusterName)

		restartAt := time.Now().UTC().Format(time.RFC3339Nano)

		By("setting spec.runtime.restartAt to trigger a rolling restart")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated)).To(Succeed())
			original := updated.DeepCopy()
			updated.Spec.Runtime = &openbaov1alpha1.RuntimeConfig{RestartAt: restartAt}
			g.Expect(c.Patch(ctx, updated, client.MergeFrom(original))).To(Succeed())
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("waiting for the StatefulSet pod template to carry the restart annotation")
		Eventually(func(g Gomega) {
			sts := &appsv1.StatefulSet{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)).To(Succeed())
			g.Expect(sts.Spec.Template.Annotations).To(HaveKeyWithValue(constants.AnnotationRestartAt, restartAt))
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("waiting for the original pod to be replaced")
		Eventually(func(g Gomega) {
			currentPod := waitForClusterPod(clusterName)
			g.Expect(currentPod.UID).NotTo(Equal(initialPod.UID))
		}, 10*time.Minute, framework.DefaultPollInterval).Should(Succeed())

		waitForClusterAvailable(clusterName)
	})

	It("creates ingress resources and includes the ingress host in the server certificate", Label(
		"case:cluster-ingress-host-san",
		"covers:ingress",
		"covers:external-service",
		"covers:tls-san",
	), func() {
		const clusterName = "ingress-cluster"

		host := fmt.Sprintf("%s.%s.e2e.example.invalid", clusterName, f.Namespace)
		cluster := newDevelopmentCluster(clusterName)
		cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
			Enabled: true,
			Host:    host,
			Path:    "/",
		}
		Expect(c.Create(ctx, cluster)).To(Succeed())
		DeferCleanup(func() { _ = c.Delete(ctx, cluster) })

		_, err := f.WaitForStatefulSetReady(ctx, clusterName, 1, 10*time.Minute, framework.DefaultPollInterval)
		Expect(err).NotTo(HaveOccurred())
		waitForClusterAvailable(clusterName)

		By("verifying the ingress and public service are created for external access")
		Eventually(func(g Gomega) {
			service := &corev1.Service{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName + "-public", Namespace: f.Namespace}, service)).To(Succeed())

			ingress := &networkingv1.Ingress{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, ingress)).To(Succeed())
			g.Expect(ingress.Spec.Rules).To(HaveLen(1))
			g.Expect(ingress.Spec.Rules[0].Host).To(Equal(host))
			g.Expect(ingress.Spec.TLS).To(HaveLen(1))
			g.Expect(ingress.Spec.TLS[0].SecretName).To(Equal(clusterName + "-tls-server"))
			g.Expect(ingress.Spec.Rules[0].HTTP).NotTo(BeNil())
			g.Expect(ingress.Spec.Rules[0].HTTP.Paths).NotTo(BeEmpty())
			g.Expect(ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service).NotTo(BeNil())
			g.Expect(ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name).To(Equal(clusterName + "-public"))
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("verifying the operator-managed server certificate includes the ingress host")
		Eventually(func(g Gomega) {
			secret := &corev1.Secret{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName + "-tls-server", Namespace: f.Namespace}, secret)).To(Succeed())
			cert := parseServerCertificate(secret)
			g.Expect(cert.DNSNames).To(ContainElement(host))
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
	})

	It("renders telemetry configuration when observability metrics are enabled", Label(
		"case:cluster-telemetry-rendering",
		"covers:telemetry",
		"covers:observability-metrics",
		"lower-layer-covered",
	), func() {
		const clusterName = "telemetry-cluster"

		cluster := newDevelopmentCluster(clusterName)
		cluster.Spec.Observability = &openbaov1alpha1.ObservabilityConfig{
			Metrics: &openbaov1alpha1.MetricsConfig{
				Enabled: true,
			},
		}
		cluster.Spec.Telemetry = &openbaov1alpha1.TelemetryConfig{
			MetricsPrefix:           "openbao.e2e",
			PrometheusRetentionTime: "45s",
			EnableHostnameLabel:     true,
		}
		Expect(c.Create(ctx, cluster)).To(Succeed())
		DeferCleanup(func() { _ = c.Delete(ctx, cluster) })

		_, err := f.WaitForStatefulSetReady(ctx, clusterName, 1, 10*time.Minute, framework.DefaultPollInterval)
		Expect(err).NotTo(HaveOccurred())
		waitForClusterAvailable(clusterName)

		By("verifying the rendered config includes the telemetry stanza")
		Eventually(func(g Gomega) {
			configMap := &corev1.ConfigMap{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName + "-config", Namespace: f.Namespace}, configMap)).To(Succeed())
			cfgText := configMap.Data["config.hcl"]
			g.Expect(cfgText).To(ContainSubstring("telemetry {"))
			g.Expect(cfgText).To(ContainSubstring("metrics_prefix"))
			g.Expect(cfgText).To(ContainSubstring("openbao.e2e"))
			g.Expect(cfgText).To(ContainSubstring("prometheus_retention_time"))
			g.Expect(cfgText).To(ContainSubstring("45s"))
			g.Expect(cfgText).To(ContainSubstring("disable_hostname"))
			g.Expect(cfgText).To(ContainSubstring("enable_hostname_label"))
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
	})
})
