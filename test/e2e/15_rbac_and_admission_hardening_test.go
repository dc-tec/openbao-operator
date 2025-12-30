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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/admission"
	"github.com/openbao/operator/internal/provisioner"
	"github.com/openbao/operator/test/e2e/framework"
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

	It("does not grant Secret watch/list in the tenant Role", func() {
		roleKey := types.NamespacedName{
			Name:      provisioner.TenantRoleName,
			Namespace: f.Namespace,
		}

		Eventually(func(g Gomega) {
			role := &rbacv1.Role{}
			g.Expect(c.Get(ctx, roleKey, role)).To(Succeed())

			var secretRule *rbacv1.PolicyRule
			for i := range role.Rules {
				rule := &role.Rules[i]
				if containsString(rule.APIGroups, "") && containsString(rule.Resources, "secrets") {
					secretRule = rule
					break
				}
			}
			g.Expect(secretRule).NotTo(BeNil(), "expected tenant Role to have a Secrets rule")

			g.Expect(containsString(secretRule.Verbs, "watch")).To(BeFalse(), "tenant Role must not grant watch on Secrets")
			g.Expect(containsString(secretRule.Verbs, "list")).To(BeFalse(), "tenant Role must not grant list on Secrets")

			for _, verb := range []string{"get", "create", "patch", "update", "delete"} {
				g.Expect(containsString(secretRule.Verbs, verb)).To(BeTrue(), fmt.Sprintf("tenant Role must include %q on Secrets", verb))
			}
		}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
	})

	It("has required ValidatingAdmissionPolicy dependencies installed and correctly bound", func() {
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		status, err := admission.CheckDependencies(checkCtx, c, admission.DefaultDependencies(), []string{"openbao-operator-", ""})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.OverallReady).To(BeTrue(), status.SummaryMessage())
		Expect(status.SentinelReady).To(BeTrue(), status.SummaryMessage())
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
