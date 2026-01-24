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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/admission"
	"github.com/dc-tec/openbao-operator/internal/provisioner"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

const (
	impersonatedUser  = "jane-developer"
	impersonatedGroup = "e2e-developers"
)

// === Shared Helpers ===

func createRoleBindingForGroup(ctx context.Context, c client.Client, namespace string, role *rbacv1.Role) {
	err := c.Create(ctx, role)
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

	err = c.Create(ctx, rb)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred(), "Failed to create RoleBinding %q in namespace %q", rb.Name, namespace)
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

var _ = Describe("Security Guardrails", Label("security", "critical"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		admin  client.Client
	)

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

	// --- Admission Policy Enforcement ---
	Context("Admission Policy Enforcement", Label("admission"), func() {
		const guardrailsNamespace = "e2e-guardrails"

		BeforeAll(func() {
			Expect(framework.EnsureRestrictedNamespace(ctx, admin, guardrailsNamespace)).To(Succeed())

			By("onboarding the guardrails namespace as a tenant (so the controller has namespace-scoped permissions)")
			tenant := &openbaov1alpha1.OpenBaoTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      guardrailsNamespace,
					Namespace: operatorNamespace,
				},
				Spec: openbaov1alpha1.OpenBaoTenantSpec{
					TargetNamespace: guardrailsNamespace,
				},
			}
			err := admin.Create(ctx, tenant)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoTenant{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: guardrailsNamespace, Namespace: operatorNamespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Provisioned).To(BeTrue(), "expected tenant to be provisioned before creating OpenBaoClusters")
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

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

			createRoleBindingForGroup(ctx, admin, guardrailsNamespace, role)
		})

		AfterAll(func() {
			if admin != nil {
				_ = admin.Delete(ctx, &openbaov1alpha1.OpenBaoTenant{
					ObjectMeta: metav1.ObjectMeta{
						Name:      guardrailsNamespace,
						Namespace: operatorNamespace,
					},
				})
				_ = admin.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: guardrailsNamespace}})
			}
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
						APIServerCIDR: apiServerCIDR,
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
					Replicas: 3, // Minimum for Hardened profile (VAP rule)
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
						Mode:           openbaov1alpha1.TLSModeExternal,
						RotationPeriod: "720h",
					},
					Storage: openbaov1alpha1.StorageConfig{
						Size: "1Gi",
					},
					Network: &openbaov1alpha1.NetworkConfig{
						APIServerCIDR: apiServerCIDR,
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

		It("blocks decimal IP encoding in backup endpoint (SSRF protection)", func() {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ssrf-decimal-ip",
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
						APIServerCIDR: apiServerCIDR,
					},
					Backup: &openbaov1alpha1.BackupSchedule{
						Schedule:      "0 0 * * *",
						ExecutorImage: "ghcr.io/dc-tec/openbao-backup:v1.0.0",
						JWTAuthRole:   "backup-role",
						Target: openbaov1alpha1.BackupTarget{
							Endpoint: "http://2130706433:9000", // decimal for 127.0.0.1
							Bucket:   "test-bucket",
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: "backup-creds",
							},
						},
					},
				},
			}

			err := e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, impersonatedUser, []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
				return c.Create(ctx, cluster)
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("numeric IP encoding"))
		})

		It("blocks cross-namespace tenant targeting (self-service mode)", func() {
			tenant := &openbaov1alpha1.OpenBaoTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cross-ns-tenant",
					Namespace: guardrailsNamespace, // Not the operator namespace
				},
				Spec: openbaov1alpha1.OpenBaoTenantSpec{
					TargetNamespace: "kube-system", // Trying to target a different namespace
				},
			}

			err := admin.Create(ctx, tenant)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("self-service mode"))

			_ = admin.Delete(ctx, tenant)
		})
	})

	// --- Resource Locking ---
	Context("Resource Locking (anti-tamper)", Label("tamper"), func() {
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
			createRoleBindingForGroup(ctx, admin, tenantNamespace, role)

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
						APIServerCIDR: apiServerCIDR,
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

			Eventually(func(g Gomega) {
				sts := &appsv1.StatefulSet{}
				err := admin.Get(ctx, types.NamespacedName{Name: statefulSet, Namespace: tenantNamespace}, sts)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sts.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "openbao-operator"))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())
		})

		AfterAll(func() {
			if tenantFW != nil {
				_ = tenantFW.Cleanup(ctx)
			}
		})

		It("prevents unauthorized deletion of the unseal Secret", func() {
			secret := &corev1.Secret{}
			err := admin.Get(ctx, types.NamespacedName{Name: unsealName, Namespace: tenantNamespace}, secret)
			Expect(err).NotTo(HaveOccurred())

			err = e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, impersonatedUser, []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
				return c.Delete(ctx, secret)
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Direct modification of OpenBao-managed resources is prohibited"))
		})

		It("prevents unauthorized deletion of the TLS CA secret", func() {
			tlsCAName := victim.Name + "-tls-ca"
			secret := &corev1.Secret{}
			err := admin.Get(ctx, types.NamespacedName{Name: tlsCAName, Namespace: tenantNamespace}, secret)
			Expect(err).NotTo(HaveOccurred())

			err = e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, impersonatedUser, []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
				return c.Delete(ctx, secret)
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Direct modification of OpenBao-managed resources is prohibited"))
		})

		It("prevents sidecar injection via StatefulSet updates", func() {
			err := e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, "hacker", []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
				sts := &appsv1.StatefulSet{}
				if err := c.Get(ctx, types.NamespacedName{Name: statefulSet, Namespace: tenantNamespace}, sts); err != nil {
					return fmt.Errorf("failed to get StatefulSet: %w", err)
				}
				sts.Spec.Template.Spec.Containers[0].Image = "malicious.invalid/sidecar:latest"
				return c.Update(ctx, sts)
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Direct modification of OpenBao-managed resources is prohibited"))
		})
	})

	// --- Configuration Handling ---
	Context("Configuration Handling", Label("config"), func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
			bad             *openbaov1alpha1.OpenBaoCluster
		)

		BeforeAll(func() {
			var err error
			tenantFW, err = framework.NewSetup(ctx, "tenant-bad-config", operatorNamespace)
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
						Requests: framework.DefaultAdminSelfInitRequests(),
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
						APIServerCIDR: apiServerCIDR,
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, bad)).To(Succeed())
		})

		AfterAll(func() {
			if tenantFW != nil {
				_ = tenantFW.Cleanup(ctx)
			}
		})

		It("reports Degraded when Gateway API CRDs are missing", func() {
			var httpRouteList unstructured.UnstructuredList
			httpRouteList.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "gateway.networking.k8s.io",
				Version: "v1",
				Kind:    "HTTPRouteList",
			})
			if err := admin.List(ctx, &httpRouteList); err == nil {
				Skip("Gateway API CRDs are installed (likely by another test), skipping missing CRDs test")
			}
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

	// --- RBAC & Dependencies ---
	Context("RBAC & Dependencies", Label("rbac"), func() {
		var (
			tenantNamespace string
			tenantFW        *framework.Framework
		)

		BeforeAll(func() {
			var err error
			tenantFW, err = framework.NewSetup(ctx, "tenant-rbac", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace
		})

		AfterAll(func() {
			if tenantFW != nil {
				_ = tenantFW.Cleanup(ctx)
			}
		})

		It("scopes Secret access via allowlist Roles", func() {
			clusterName := "rbac-cluster"
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
						APIServerCIDR: apiServerCIDR,
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, cluster)).To(Succeed())
			DeferCleanup(func() {
				_ = admin.Delete(ctx, cluster)
			})

			roleKey := types.NamespacedName{
				Name:      provisioner.TenantRoleName,
				Namespace: tenantNamespace,
			}

			Eventually(func(g Gomega) {
				role := &rbacv1.Role{}
				g.Expect(admin.Get(ctx, roleKey, role)).To(Succeed())

				for i := range role.Rules {
					rule := &role.Rules[i]
					g.Expect(containsString(rule.APIGroups, "") && containsString(rule.Resources, "secrets")).To(BeFalse(),
						"tenant Role must not grant Secrets access; Secrets are handled via dedicated allowlist Roles")
				}
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			writerRoleKey := types.NamespacedName{
				Name:      provisioner.TenantSecretsWriterRoleName,
				Namespace: tenantNamespace,
			}
			writerRBKey := types.NamespacedName{
				Name:      provisioner.TenantSecretsWriterRoleBindingName,
				Namespace: tenantNamespace,
			}

			Eventually(func(g Gomega) {
				role := &rbacv1.Role{}
				g.Expect(admin.Get(ctx, writerRoleKey, role)).To(Succeed())

				var createRule *rbacv1.PolicyRule
				var namedRule *rbacv1.PolicyRule
				for i := range role.Rules {
					rule := &role.Rules[i]
					if !containsString(rule.APIGroups, "") || !containsString(rule.Resources, "secrets") {
						continue
					}
					if containsString(rule.Verbs, "create") && len(rule.ResourceNames) == 0 {
						createRule = rule
					}
					if len(rule.ResourceNames) > 0 && containsString(rule.Verbs, "get") {
						namedRule = rule
					}
				}

				g.Expect(createRule).NotTo(BeNil(), "expected a Secrets create rule without resourceNames")
				g.Expect(containsString(createRule.Verbs, "list")).To(BeFalse())
				g.Expect(containsString(createRule.Verbs, "watch")).To(BeFalse())

				g.Expect(namedRule).NotTo(BeNil(), "expected a Secrets rule scoped by resourceNames")
				g.Expect(containsString(namedRule.Verbs, "list")).To(BeFalse())
				g.Expect(containsString(namedRule.Verbs, "watch")).To(BeFalse())
				for _, verb := range []string{"get", "patch", "update", "delete"} {
					g.Expect(containsString(namedRule.Verbs, verb)).To(BeTrue(), fmt.Sprintf("expected %q on Secrets", verb))
				}

				expected := []string{
					clusterName + "-tls-ca",
					clusterName + "-tls-server",
				}
				for _, name := range expected {
					g.Expect(containsString(namedRule.ResourceNames, name)).To(BeTrue(), fmt.Sprintf("expected Secrets allowlist to include %q", name))
				}

				rb := &rbacv1.RoleBinding{}
				g.Expect(admin.Get(ctx, writerRBKey, rb)).To(Succeed())
				g.Expect(rb.RoleRef.Name).To(Equal(provisioner.TenantSecretsWriterRoleName))
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		It("has required ValidatingAdmissionPolicy dependencies installed and correctly bound", func() {
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			status, err := admission.CheckDependencies(checkCtx, admin, admission.DefaultDependencies(), []string{"openbao-operator-", ""})
			Expect(err).NotTo(HaveOccurred())
			Expect(status.OverallReady).To(BeTrue(), status.SummaryMessage())
		})
	})
})
