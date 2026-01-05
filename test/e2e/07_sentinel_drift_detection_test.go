//go:build e2e
// +build e2e

/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	"k8s.io/apimachinery/pkg/util/uuid"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
)

var _ = Describe("Sentinel: Drift Detection and Fast-Path Reconciliation", Label("sentinel", "security", "cluster", "slow"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		c      client.Client
		f      *framework.Framework
	)

	const (
		clusterName = "sentinel-cluster"
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

		f, err = framework.New(ctx, c, "sentinel", operatorNamespace)
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

	It("creates Sentinel Deployment when Sentinel is enabled", func() {

		By(fmt.Sprintf("creating OpenBaoCluster %q with Sentinel enabled", clusterName))
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
				Sentinel: &openbaov1alpha1.SentinelConfig{
					Enabled: true,
				},
			},
		}

		err := c.Create(ctx, cluster)
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "Created OpenBaoCluster %q with Sentinel enabled\n", clusterName)

		// Note: No DeferCleanup here - the cluster is shared across multiple tests in this suite.
		// Cleanup is handled by AfterAll via f.Cleanup() which deletes all clusters in the namespace.

		By("waiting for OpenBaoCluster to be observed by the API server")
		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "OpenBaoCluster %q observed by API server\n", clusterName)
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("checking for prerequisite resources (ConfigMap, config-init ConfigMap, and TLS Secret)")
		Eventually(func(g Gomega) {
			configMapName := clusterName + "-config"
			configMap := &corev1.ConfigMap{}
			err := c.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: f.Namespace}, configMap)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "ConfigMap %q not found yet: %v\n", configMapName, err)
			}
			g.Expect(err).NotTo(HaveOccurred(), "expected ConfigMap to exist")
			_, _ = fmt.Fprintf(GinkgoWriter, "ConfigMap %q exists\n", configMapName)
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		Eventually(func(g Gomega) {
			configInitMapName := clusterName + "-config-init"
			configMap := &corev1.ConfigMap{}
			err := c.Get(ctx, types.NamespacedName{Name: configInitMapName, Namespace: f.Namespace}, configMap)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "ConfigMap %q not found yet: %v\n", configInitMapName, err)
			}
			g.Expect(err).NotTo(HaveOccurred(), "expected config-init ConfigMap to exist (created even when empty)")
			_, _ = fmt.Fprintf(GinkgoWriter, "ConfigMap %q exists\n", configInitMapName)
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		Eventually(func(g Gomega) {
			tlsSecretName := clusterName + "-tls-server"
			tlsSecret := &corev1.Secret{}
			err := c.Get(ctx, types.NamespacedName{Name: tlsSecretName, Namespace: f.Namespace}, tlsSecret)
			g.Expect(err).NotTo(HaveOccurred(), "expected TLS Secret to exist")
			_, _ = fmt.Fprintf(GinkgoWriter, "TLS Secret %q exists\n", tlsSecretName)
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("waiting for StatefulSet to be created")
		Eventually(func(g Gomega) {
			sts := &appsv1.StatefulSet{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)).To(Succeed())
			g.Expect(sts.Spec.Replicas).NotTo(BeNil())
			_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q exists with replicas=%d (ready=%d)\n", clusterName, *sts.Spec.Replicas, sts.Status.ReadyReplicas)
			_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q created successfully\n", clusterName)
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("waiting for StatefulSet pods to become Ready")
		Eventually(func(g Gomega) {
			sts := &appsv1.StatefulSet{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, sts)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet status: replicas=%d ready=%d updated=%d\n", sts.Status.Replicas, sts.Status.ReadyReplicas, sts.Status.UpdatedReplicas)
			g.Expect(sts.Status.ReadyReplicas).To(Equal(*sts.Spec.Replicas))
			_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q pods are Ready\n", clusterName)
		}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("triggering a reconcile and waiting for Available condition")
		Expect(f.TriggerReconcile(ctx, clusterName)).To(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Triggered reconcile for cluster %q\n", clusterName)

		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated)).To(Succeed())
			available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			if available == nil || available.Status != metav1.ConditionTrue {
				_, _ = fmt.Fprintf(GinkgoWriter, "Available condition not set yet\n")
			}
			g.Expect(available).NotTo(BeNil())
			g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
			_, _ = fmt.Fprintf(GinkgoWriter, "OpenBaoCluster %q is Available\n", clusterName)
		}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("verifying Sentinel Deployment is created")
		deploymentName := clusterName + constants.SentinelDeploymentNameSuffix
		Eventually(func(g Gomega) {
			deployment := &appsv1.Deployment{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: f.Namespace}, deployment)).To(Succeed())
			g.Expect(deployment.Spec.Replicas).NotTo(BeNil())
			g.Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
			_, _ = fmt.Fprintf(GinkgoWriter, "Sentinel Deployment %q exists with %d replicas\n", deploymentName, *deployment.Spec.Replicas)
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("verifying Sentinel ServiceAccount is created")
		Eventually(func(g Gomega) {
			sa := &corev1.ServiceAccount{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: constants.SentinelServiceAccountName, Namespace: f.Namespace}, sa)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "Sentinel ServiceAccount %q exists\n", constants.SentinelServiceAccountName)
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("verifying Sentinel Deployment becomes ready")
		Eventually(func(g Gomega) {
			deployment := &appsv1.Deployment{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: f.Namespace}, deployment)).To(Succeed())
			g.Expect(deployment.Status.ReadyReplicas).To(Equal(int32(1)))
			_, _ = fmt.Fprintf(GinkgoWriter, "Sentinel Deployment %q is ready\n", deploymentName)
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
	})

	It("verifies Sentinel drift detection infrastructure is operational", func() {
		// This test verifies that Sentinel drift detection infrastructure is working,
		// rather than trying to catch a specific timing-dependent drift event.
		// The challenge with testing drift detection is that:
		// 1. The operator may reconcile and fix drift before Sentinel detects it
		// 2. Sentinel's predicate filters may exclude certain updates
		// 3. Timing between modification, detection, and reconciliation is non-deterministic
		//
		// Instead of trying to force a specific drift event, we verify that:
		// 1. Sentinel is running and watching resources
		// 2. Drift detection infrastructure is operational (status exists and is populated)
		// 3. The drift detection mechanism has worked at some point (proven by existing drift status)

		By("verifying Sentinel pod is running and ready")
		Eventually(func(g Gomega) {
			pods := &corev1.PodList{}
			labels := client.MatchingLabels{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
				constants.LabelAppComponent: "sentinel",
				constants.LabelAppInstance:  clusterName,
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			}
			g.Expect(c.List(ctx, pods, client.InNamespace(f.Namespace), labels)).To(Succeed())
			g.Expect(len(pods.Items)).To(BeNumerically(">=", 1), "expected at least one Sentinel pod")
			for _, pod := range pods.Items {
				g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning), "expected Sentinel pod to be Running")
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady {
						g.Expect(condition.Status).To(Equal(corev1.ConditionTrue), "expected Sentinel pod to be Ready")
					}
				}
			}
			_, _ = fmt.Fprintf(GinkgoWriter, "Sentinel pod is running and ready\n")
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("verifying drift detection infrastructure is operational")
		// Verify that drift status exists and is properly populated, which proves
		// that drift detection has worked at some point. This is more reliable than
		// trying to catch a specific drift event which is subject to timing/race conditions.
		Eventually(func(g Gomega) {
			cluster := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, cluster)).To(Succeed())

			// Verify drift status exists and is populated
			// The presence of drift status with populated fields proves that:
			// 1. Sentinel has detected drift at some point
			// 2. The operator has processed drift detection
			// 3. The drift detection mechanism is operational
			g.Expect(cluster.Status.Drift).NotTo(BeNil(), "expected drift status to be set (proves drift detection has worked)")
			g.Expect(cluster.Status.Drift.LastDriftDetected).NotTo(BeNil(), "expected LastDriftDetected to be set")
			g.Expect(cluster.Status.Drift.DriftCorrectionCount).To(BeNumerically(">=", 1), "expected DriftCorrectionCount to be at least 1")
			g.Expect(cluster.Status.Drift.LastCorrectionTime).NotTo(BeNil(), "expected LastCorrectionTime to be set")
			g.Expect(cluster.Status.Drift.LastDriftResource).NotTo(BeEmpty(), "expected LastDriftResource to be set")

			// Verify LastCorrectionTime is after LastDriftDetected (data consistency check)
			g.Expect(cluster.Status.Drift.LastCorrectionTime.Time).To(BeTemporally(">=", cluster.Status.Drift.LastDriftDetected.Time),
				"LastCorrectionTime should be after LastDriftDetected")

			_, _ = fmt.Fprintf(GinkgoWriter, "Drift detection infrastructure verified: LastDriftDetected=%v, DriftCorrectionCount=%d, LastDriftResource=%q\n",
				cluster.Status.Drift.LastDriftDetected.Time,
				cluster.Status.Drift.DriftCorrectionCount,
				cluster.Status.Drift.LastDriftResource)
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("verifying the operator responds to a Sentinel trigger via status")
		// Simulate Sentinel emitting a drift trigger by patching status.sentinel.*.
		// This avoids relying on non-deterministic timing of real drift detection.
		// Use UUID instead of timestamp to avoid flakiness from timing differences.
		triggerID := string(uuid.NewUUID())
		cluster := &openbaov1alpha1.OpenBaoCluster{}
		Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, cluster)).To(Succeed())
		original := cluster.DeepCopy()
		if cluster.Status.Sentinel == nil {
			cluster.Status.Sentinel = &openbaov1alpha1.SentinelStatus{}
		}
		cluster.Status.Sentinel.TriggerID = triggerID
		now := metav1.Now()
		cluster.Status.Sentinel.TriggeredAt = &now
		cluster.Status.Sentinel.TriggerResource = fmt.Sprintf("StatefulSet/%s", clusterName)
		Expect(c.Status().Patch(ctx, cluster, client.MergeFrom(original))).To(Succeed())

		Eventually(func(g Gomega) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, updated)).To(Succeed())

			g.Expect(updated.Status.Sentinel).NotTo(BeNil())
			g.Expect(updated.Status.Sentinel.LastHandledTriggerID).To(Equal(triggerID), "expected operator to mark trigger as handled")
			g.Expect(updated.Status.Sentinel.LastHandledAt).NotTo(BeNil())

			g.Expect(updated.Status.Drift).NotTo(BeNil(), "expected drift status to exist after trigger handling")
			g.Expect(updated.Status.Drift.LastDriftDetected).NotTo(BeNil())
			g.Expect(updated.Status.Drift.LastCorrectionTime).NotTo(BeNil())
			g.Expect(updated.Status.Drift.LastDriftResource).NotTo(BeEmpty())

			// GitOps contract: legacy trigger annotations must not be used.
			if updated.Annotations != nil {
				_, hasTrigger := updated.Annotations["openbao.org/sentinel-trigger"]
				g.Expect(hasTrigger).To(BeFalse())
				_, hasResource := updated.Annotations["openbao.org/sentinel-trigger-resource"]
				g.Expect(hasResource).To(BeFalse())
			}
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
	})

	It("removes Sentinel resources when Sentinel is disabled", func() {
		By("disabling Sentinel in the cluster spec")
		cluster := &openbaov1alpha1.OpenBaoCluster{}
		Eventually(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, cluster)).To(Succeed())
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		cluster.Spec.Sentinel = &openbaov1alpha1.SentinelConfig{
			Enabled: false,
		}
		err := c.Update(ctx, cluster)
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "Disabled Sentinel in OpenBaoCluster spec\n")

		By("verifying Sentinel Deployment is deleted")
		deploymentName := clusterName + constants.SentinelDeploymentNameSuffix
		Eventually(func(g Gomega) {
			deployment := &appsv1.Deployment{}
			err := c.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: f.Namespace}, deployment)
			g.Expect(err).To(HaveOccurred())
			_, _ = fmt.Fprintf(GinkgoWriter, "Sentinel Deployment %q deleted\n", deploymentName)
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
	})

	It("creates Sentinel with custom debounce window configuration", func() {

		By("creating a new cluster with custom Sentinel configuration")
		customClusterName := "sentinel-custom-cluster"
		customDebounceWindow := int32(5)
		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      customClusterName,
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
					Enabled: true,
					Requests: []openbaov1alpha1.SelfInitRequest{
						{
							Name:      "enable-jwt-auth",
							Operation: openbaov1alpha1.SelfInitOperationUpdate,
							Path:      "sys/auth/jwt",
							AuthMethod: &openbaov1alpha1.SelfInitAuthMethod{
								Type: "jwt",
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
				Sentinel: &openbaov1alpha1.SentinelConfig{
					Enabled:               true,
					DebounceWindowSeconds: &customDebounceWindow,
				},
			},
		}

		err := c.Create(ctx, cluster)
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "Created OpenBaoCluster %q with custom Sentinel debounce window\n", customClusterName)

		DeferCleanup(func() {
			_ = c.Delete(ctx, cluster)
		})

		By("verifying Sentinel Deployment is created with custom configuration")
		deploymentName := customClusterName + constants.SentinelDeploymentNameSuffix
		Eventually(func(g Gomega) {
			deployment := &appsv1.Deployment{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: f.Namespace}, deployment)).To(Succeed())
			// Verify the environment variable is set correctly
			container := deployment.Spec.Template.Spec.Containers[0]
			found := false
			for _, env := range container.Env {
				if env.Name == constants.EnvSentinelDebounceWindowSeconds {
					g.Expect(env.Value).To(Equal("5"))
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(), "expected SENTINEL_DEBOUNCE_WINDOW_SECONDS environment variable to be set")
			_, _ = fmt.Fprintf(GinkgoWriter, "Sentinel Deployment %q created with custom debounce window\n", deploymentName)
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
	})
})
