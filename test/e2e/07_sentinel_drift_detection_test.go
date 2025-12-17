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
	"github.com/openbao/operator/internal/constants"
	"github.com/openbao/operator/test/e2e/framework"
	e2ehelpers "github.com/openbao/operator/test/e2e/helpers"
)

var _ = Describe("Sentinel: Drift Detection and Fast-Path Reconciliation", Ordered, func() {
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

	It("detects drift and triggers fast-path reconciliation", func() {
		By("simulating infrastructure drift by modifying a Service")
		serviceName := clusterName
		service := &corev1.Service{}
		Eventually(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: f.Namespace}, service)).To(Succeed())
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		// Use impersonation to modify the Service as a break-glass admin with maintenance mode
		// This simulates an administrator making changes that should trigger drift detection
		err := e2ehelpers.RunWithImpersonation(
			ctx,
			cfg,
			scheme,
			"e2e-test-admin",
			[]string{"system:masters"},
			func(impersonatedClient client.Client) error {
				// Get the service with the impersonated client
				serviceToModify := &corev1.Service{}
				if err := impersonatedClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: f.Namespace}, serviceToModify); err != nil {
					return err
				}

				// Enable maintenance mode and add drift annotation
				if serviceToModify.Annotations == nil {
					serviceToModify.Annotations = make(map[string]string)
				}
				serviceToModify.Annotations["openbao.org/maintenance"] = "true"
				serviceToModify.Annotations["e2e.openbao.org/drift-test"] = "simulated-drift"

				return impersonatedClient.Update(ctx, serviceToModify)
			},
		)
		Expect(err).NotTo(HaveOccurred(), "Failed to modify Service to simulate drift")
		_, _ = fmt.Fprintf(GinkgoWriter, "Modified Service %q to simulate drift\n", serviceName)

		// Verify the Service was actually updated
		Eventually(func(g Gomega) {
			updatedService := &corev1.Service{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: f.Namespace}, updatedService)).To(Succeed())
			g.Expect(updatedService.Annotations).NotTo(BeNil())
			g.Expect(updatedService.Annotations["e2e.openbao.org/drift-test"]).To(Equal("simulated-drift"), "Service should have drift test annotation")
			// Log managedFields to help diagnose filtering issues
			if len(updatedService.ManagedFields) > 0 {
				_, _ = fmt.Fprintf(GinkgoWriter, "Service %q managedFields: manager=%q operation=%q time=%v\n",
					serviceName, updatedService.ManagedFields[0].Manager, updatedService.ManagedFields[0].Operation, updatedService.ManagedFields[0].Time)
			}
			_, _ = fmt.Fprintf(GinkgoWriter, "Verified Service %q was updated with drift annotation\n", serviceName)
		}, 10*time.Second, 1*time.Second).Should(Succeed())

		// Wait for Sentinel to detect the drift (debounce window is 2s, so wait at least 3s to be safe)
		// Also account for potential operator reconciliation that might fix the drift
		time.Sleep(3 * time.Second)

		By("verifying Sentinel trigger annotation is set on OpenBaoCluster")
		Eventually(func(g Gomega) {
			cluster := &openbaov1alpha1.OpenBaoCluster{}
			err := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, cluster)
			if apierrors.IsNotFound(err) {
				g.Expect(err).NotTo(HaveOccurred(), "OpenBaoCluster was deleted before Sentinel could detect drift - this may indicate a test isolation issue")
			}
			g.Expect(err).NotTo(HaveOccurred(), "Failed to get OpenBaoCluster")
			_, hasTrigger := cluster.Annotations[constants.AnnotationSentinelTrigger]
			g.Expect(hasTrigger).To(BeTrue(), "expected Sentinel trigger annotation to be set. Cluster annotations: %v", cluster.Annotations)
			_, _ = fmt.Fprintf(GinkgoWriter, "Sentinel trigger annotation detected on OpenBaoCluster\n")
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		By("verifying trigger annotation is cleared after reconciliation")
		Eventually(func(g Gomega) {
			cluster := &openbaov1alpha1.OpenBaoCluster{}
			g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, cluster)).To(Succeed())
			_, hasTrigger := cluster.Annotations[constants.AnnotationSentinelTrigger]
			g.Expect(hasTrigger).To(BeFalse(), "expected Sentinel trigger annotation to be cleared after reconciliation")
			_, _ = fmt.Fprintf(GinkgoWriter, "Sentinel trigger annotation cleared after reconciliation\n")
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
