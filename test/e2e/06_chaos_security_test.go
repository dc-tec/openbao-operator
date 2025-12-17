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
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/test/e2e/framework"
	e2ehelpers "github.com/openbao/operator/test/e2e/helpers"
)

const (
	impersonatedUser  = "jane-developer"
	impersonatedGroup = "e2e-developers"
)

// createAdminPolicyAndJWTAuthRequests creates SelfInit requests for enabling JWT auth
// and creating an admin policy.
func createAdminPolicyAndJWTAuthRequests() []openbaov1alpha1.SelfInitRequest {
	return []openbaov1alpha1.SelfInitRequest{
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
	}
}

var _ = Describe("Chaos and Security", Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		admin  client.Client
	)

	createRoleBindingForGroup := func(namespace string, role *rbacv1.Role) {
		err := admin.Create(ctx, role)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred(), "Failed to create Role %q in namespace %q", role.Name, namespace)
		}

		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      role.Name + "-binding",
				Namespace: namespace,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     role.Name,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind: "Group",
					Name: impersonatedGroup,
				},
			},
		}

		err = admin.Create(ctx, rb)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred(), "Failed to create RoleBinding %q in namespace %q", rb.Name, namespace)
		}
	}

	BeforeAll(func() {
		var err error

		cfg, err = ctrlconfig.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(openbaov1alpha1.AddToScheme(scheme)).To(Succeed())

		admin, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Admission Policy Enforcement", func() {
		const guardrailsNamespace = "e2e-guardrails"

		BeforeAll(func() {
			Expect(framework.EnsureRestrictedNamespace(ctx, admin, guardrailsNamespace)).To(Succeed())

			role := &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-openbaocluster-writer",
					Namespace: guardrailsNamespace,
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"openbao.org"},
						Resources: []string{"openbaoclusters"},
						Verbs:     []string{"create"},
					},
				},
			}

			createRoleBindingForGroup(guardrailsNamespace, role)
		})

		AfterAll(func() {
			_ = admin.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: guardrailsNamespace}})
		})

		It("accepts structured configuration (protected stanzas cannot be overridden)", func() {
			uiEnabled := true
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-structured-config",
					Namespace: guardrailsNamespace,
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
						RotationPeriod: "720h",
					},
					Storage: openbaov1alpha1.StorageConfig{
						Size: "1Gi",
					},
					Network: &openbaov1alpha1.NetworkConfig{
						APIServerCIDR: kindDefaultServiceCIDR,
					},
					Configuration: &openbaov1alpha1.OpenBaoConfiguration{
						UI:       &uiEnabled,
						LogLevel: "debug",
					},
				},
			}

			err := e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, impersonatedUser, []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
				return c.Create(ctx, cluster)
			})
			Expect(err).NotTo(HaveOccurred())
			// Note: Protected stanzas (listener, storage, seal) cannot be overridden
			// because they are not part of the structured Configuration API.
			// The operator enforces these values directly.
		})

		It("enforces Hardened profile invariants", func() {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-hardened",
					Namespace: guardrailsNamespace,
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
						Enabled:  true,
						Requests: createAdminPolicyAndJWTAuthRequests(),
					},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled:        true,
						Mode:           openbaov1alpha1.TLSModeExternal,
						RotationPeriod: "720h",
					},
					Storage: openbaov1alpha1.StorageConfig{
						Size: "1Gi",
					},
					Network: &openbaov1alpha1.NetworkConfig{
						APIServerCIDR: kindDefaultServiceCIDR,
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "static",
					},
				},
			}

			err := e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, impersonatedUser, []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
				return c.Create(ctx, cluster)
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Hardened profile requires"))
		})
	})

	Context("Resource Locking (anti-tamper)", func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			victim          *openbaov1alpha1.OpenBaoCluster
			unsealName      string
			statefulSet     string
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-locks", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			role := &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-rogue-user",
					Namespace: tenantNamespace,
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"secrets"},
						Verbs:     []string{"get", "list", "delete"},
					},
					{
						APIGroups: []string{"apps"},
						Resources: []string{"statefulsets"},
						Verbs:     []string{"get", "list", "update"},
					},
				},
			}
			createRoleBindingForGroup(tenantNamespace, role)

			victim = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "victim-cluster",
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
						Enabled:  true,
						Requests: createAdminPolicyAndJWTAuthRequests(),
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
			Expect(admin.Create(ctx, victim)).To(Succeed())

			unsealName = victim.Name + "-unseal-key"
			statefulSet = victim.Name

			Eventually(func() error {
				secret := &corev1.Secret{}
				return admin.Get(ctx, types.NamespacedName{Name: unsealName, Namespace: tenantNamespace}, secret)
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			// Check cluster status for any blocking conditions
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: victim.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				// Log all conditions for debugging
				for _, cond := range updated.Status.Conditions {
					_, _ = fmt.Fprintf(GinkgoWriter, "Cluster condition: %s=%s reason=%s message=%q\n",
						cond.Type, cond.Status, cond.Reason, cond.Message)
				}
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				sts := &appsv1.StatefulSet{}
				err := admin.Get(ctx, types.NamespacedName{Name: statefulSet, Namespace: tenantNamespace}, sts)
				if err != nil {
					// If StatefulSet not found, check cluster status again
					updated := &openbaov1alpha1.OpenBaoCluster{}
					if getErr := admin.Get(ctx, types.NamespacedName{Name: victim.Name, Namespace: tenantNamespace}, updated); getErr == nil {
						for _, cond := range updated.Status.Conditions {
							_, _ = fmt.Fprintf(GinkgoWriter, "Cluster condition while waiting for StatefulSet: %s=%s reason=%s message=%q\n",
								cond.Type, cond.Status, cond.Reason, cond.Message)
						}
					}
					_, _ = fmt.Fprintf(GinkgoWriter, "StatefulSet %q not found yet: %v\n", statefulSet, err)
				}
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sts.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "openbao-operator"))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())
		})

		AfterAll(func() {
			if tenantFW == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()

			_ = tenantFW.Cleanup(cleanupCtx)
		})

		It("prevents unauthorized deletion of the unseal Secret", func() {
			secret := &corev1.Secret{}
			err := admin.Get(ctx, types.NamespacedName{Name: unsealName, Namespace: tenantNamespace}, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "openbao-operator"))

			err = e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, impersonatedUser, []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
				return c.Delete(ctx, secret)
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Direct modification of OpenBao-managed resources is prohibited"))

			Consistently(func() error {
				return admin.Get(ctx, types.NamespacedName{Name: unsealName, Namespace: tenantNamespace}, &corev1.Secret{})
			}, 5*time.Second, 1*time.Second).Should(Succeed())
		})

		It("prevents sidecar injection via StatefulSet updates", func() {
			err := e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, "hacker", []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
				sts := &appsv1.StatefulSet{}
				if err := c.Get(ctx, types.NamespacedName{Name: statefulSet, Namespace: tenantNamespace}, sts); err != nil {
					return fmt.Errorf("failed to get StatefulSet: %w", err)
				}

				if len(sts.Spec.Template.Spec.Containers) == 0 {
					return fmt.Errorf("StatefulSet has no containers")
				}

				sts.Spec.Template.Spec.Containers[0].Image = "malicious.invalid/sidecar:latest"
				if err := c.Update(ctx, sts); err != nil {
					return err
				}
				return nil
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Direct modification of OpenBao-managed resources is prohibited"))
		})
	})

	Context("Controller resilience (chaos)", func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			chaos           *openbaov1alpha1.OpenBaoCluster
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-chaos", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			chaos = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "chaos-cluster",
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
						Enabled:  true,
						Requests: createAdminPolicyAndJWTAuthRequests(),
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
			Expect(admin.Create(ctx, chaos)).To(Succeed())

			Eventually(func() error {
				return admin.Get(ctx, types.NamespacedName{Name: chaos.Name + "-config", Namespace: tenantNamespace}, &corev1.ConfigMap{})
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		AfterAll(func() {
			if tenantFW == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()

			_ = tenantFW.Cleanup(cleanupCtx)
		})

		It("prevents deletion of managed ConfigMap (policy enforcement)", func() {
			cm := &corev1.ConfigMap{}
			err := admin.Get(ctx, types.NamespacedName{Name: chaos.Name + "-config", Namespace: tenantNamespace}, cm)
			Expect(err).NotTo(HaveOccurred())

			// Attempt to delete the ConfigMap - this should be blocked by the ValidatingAdmissionPolicy
			err = admin.Delete(ctx, cm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Direct modification of OpenBao-managed resources is prohibited"))
			Expect(err.Error()).To(ContainSubstring("ValidatingAdmissionPolicy"))

			// Verify the ConfigMap still exists
			Eventually(func() error {
				return admin.Get(ctx, types.NamespacedName{Name: chaos.Name + "-config", Namespace: tenantNamespace}, &corev1.ConfigMap{})
			}, 5*time.Second, 1*time.Second).Should(Succeed())
		})

		It("prevents deletion of managed StatefulSet (policy enforcement)", func() {
			sts := &appsv1.StatefulSet{}
			err := admin.Get(ctx, types.NamespacedName{Name: chaos.Name, Namespace: tenantNamespace}, sts)
			Expect(err).NotTo(HaveOccurred())

			// Attempt to delete the StatefulSet - this should be blocked by the ValidatingAdmissionPolicy
			err = admin.Delete(ctx, sts)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Direct modification of OpenBao-managed resources is prohibited"))
			Expect(err.Error()).To(ContainSubstring("ValidatingAdmissionPolicy"))

			// Verify the StatefulSet still exists
			Eventually(func() error {
				return admin.Get(ctx, types.NamespacedName{Name: chaos.Name, Namespace: tenantNamespace}, &appsv1.StatefulSet{})
			}, 5*time.Second, 1*time.Second).Should(Succeed())
		})
	})

	Context("Bad configuration handling", func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			bad             *openbaov1alpha1.OpenBaoCluster
		)

		BeforeAll(func() {
			var err error

			tenantFW, err = framework.New(ctx, admin, "tenant-bad-config", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

			bad = &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-missing",
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
						Enabled:  true,
						Requests: createAdminPolicyAndJWTAuthRequests(),
					},
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled:  true,
						Hostname: "example.invalid",
						GatewayRef: openbaov1alpha1.GatewayReference{
							Name: "does-not-exist",
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
			Expect(admin.Create(ctx, bad)).To(Succeed())
		})

		AfterAll(func() {
			if tenantFW == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()

			_ = tenantFW.Cleanup(cleanupCtx)
		})

		It("reports Degraded when Gateway API CRDs are missing", func() {
			// This test verifies that the operator correctly reports a Degraded condition
			// when Gateway API CRDs are not installed but spec.gateway.enabled is true.
			// Gateway API CRDs are NOT installed by default in the e2e suite, so this test
			// should work without any setup.
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				err := admin.Get(ctx, types.NamespacedName{Name: bad.Name, Namespace: tenantNamespace}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				found := false
				for _, cond := range updated.Status.Conditions {
					if cond.Type == string(openbaov1alpha1.ConditionDegraded) {
						found = true
						g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
						g.Expect(cond.Reason).To(Equal("GatewayAPIMissing"))
					}
				}
				g.Expect(found).To(BeTrue(), "expected Degraded condition to be present")
			}, 2*time.Minute, 2*time.Second).Should(Succeed())
		})
	})
})
