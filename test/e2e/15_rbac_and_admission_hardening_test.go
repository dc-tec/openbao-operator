//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/admission"
	"github.com/dc-tec/openbao-operator/internal/provisioner"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
)

var _ = Describe("Workstream B: RBAC & Admission Hardening", Label("security", "rbac", "admission"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		c      client.Client
		f      *framework.Framework
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

		f, err = framework.New(ctx, c, "workstream-b", operatorNamespace)
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

	It("scopes Secret access via allowlist Roles", func() {
		clusterName := "rbac-cluster"
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
					APIServerCIDR: apiServerCIDR,
				},
				DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
			},
		}
		Expect(c.Create(ctx, cluster)).To(Succeed())
		DeferCleanup(func() {
			_ = c.Delete(ctx, cluster)
		})

		roleKey := types.NamespacedName{
			Name:      provisioner.TenantRoleName,
			Namespace: f.Namespace,
		}

		Eventually(func(g Gomega) {
			role := &rbacv1.Role{}
			g.Expect(c.Get(ctx, roleKey, role)).To(Succeed())

			for i := range role.Rules {
				rule := &role.Rules[i]
				g.Expect(containsString(rule.APIGroups, "") && containsString(rule.Resources, "secrets")).To(BeFalse(),
					"tenant Role must not grant Secrets access; Secrets are handled via dedicated allowlist Roles")
			}
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

		writerRoleKey := types.NamespacedName{
			Name:      provisioner.TenantSecretsWriterRoleName,
			Namespace: f.Namespace,
		}
		writerRBKey := types.NamespacedName{
			Name:      provisioner.TenantSecretsWriterRoleBindingName,
			Namespace: f.Namespace,
		}

		Eventually(func(g Gomega) {
			role := &rbacv1.Role{}
			g.Expect(c.Get(ctx, writerRoleKey, role)).To(Succeed())

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
			g.Expect(c.Get(ctx, writerRBKey, rb)).To(Succeed())
			g.Expect(rb.RoleRef.Name).To(Equal(provisioner.TenantSecretsWriterRoleName))
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
	})

	It("has required ValidatingAdmissionPolicy dependencies installed and correctly bound", func() {
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		status, err := admission.CheckDependencies(checkCtx, c, admission.DefaultDependencies(), []string{"openbao-operator-", ""})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.OverallReady).To(BeTrue(), status.SummaryMessage())
	})
})

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
