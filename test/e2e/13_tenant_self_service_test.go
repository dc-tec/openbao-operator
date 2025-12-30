//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

var _ = Describe("Tenant Self-Service", Label("security", "tenant", "critical"), Ordered, func() {
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

	Context("Confused Deputy Prevention", func() {
		var (
			attackerNS string
			victimNS   string
		)

		BeforeAll(func() {
			// Setup Namespaces
			attackerNS = "e2e-attacker-" + randString(5)
			victimNS = "e2e-victim-" + randString(5)

			ns1 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: attackerNS}}
			Expect(admin.Create(ctx, ns1)).To(Succeed())

			ns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: victimNS}}
			Expect(admin.Create(ctx, ns2)).To(Succeed())

			// Create ServiceAccount for Attacker
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "attacker",
					Namespace: attackerNS,
				},
			}
			Expect(admin.Create(ctx, sa)).To(Succeed())

			// Grant 'edit' role to Attacker in AttackerNS
			// The 'openbaotenant-editor-role' is aggregated to 'edit', so this grants OpenBaoTenant permissions
			rb := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "attacker-edit",
					Namespace: attackerNS,
				},
				Subjects: []rbacv1.Subject{
					{Kind: "ServiceAccount", Name: "attacker", Namespace: attackerNS},
				},
				RoleRef: rbacv1.RoleRef{
					Kind: "ClusterRole",
					Name: "edit",
				},
			}
			Expect(admin.Create(ctx, rb)).To(Succeed())
		})

		AfterAll(func() {
			_ = admin.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: attackerNS}})
			_ = admin.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: victimNS}})
		})

		It("blocks cross-namespace provisioning attempts", func() {
			// Create client impersonating the attacker
			userCfg := *cfg
			userCfg.Impersonate = rest.ImpersonationConfig{
				UserName: fmt.Sprintf("system:serviceaccount:%s:attacker", attackerNS),
			}
			attackerClient, err := client.New(&userCfg, client.Options{Scheme: scheme})
			Expect(err).NotTo(HaveOccurred())

			// Attempt to create Tenant targeting VictimNS
			tenant := &openbaov1alpha1.OpenBaoTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "malicious-tenant",
					Namespace: attackerNS,
				},
				Spec: openbaov1alpha1.OpenBaoTenantSpec{
					TargetNamespace: victimNS,
				},
			}

			// Creation should succeed (RBAC allows it)
			Expect(attackerClient.Create(ctx, tenant)).To(Succeed())

			// But Controller should mark it as Failed/SecurityViolation
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoTenant{}
				err := admin.Get(ctx, types.NamespacedName{Name: tenant.Name, Namespace: attackerNS}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updated.Status.Provisioned).To(BeFalse(), "Status.Provisioned should be false")
				g.Expect(updated.Status.LastError).To(ContainSubstring("security violation"), "LastError should contain security violation")
			}, 30*time.Second, 1*time.Second).Should(Succeed())
		})

		It("allows self-service provisioning within own namespace", func() {
			// Create client impersonating the attacker
			userCfg := *cfg
			userCfg.Impersonate = rest.ImpersonationConfig{
				UserName: fmt.Sprintf("system:serviceaccount:%s:attacker", attackerNS),
			}
			attackerClient, err := client.New(&userCfg, client.Options{Scheme: scheme})
			Expect(err).NotTo(HaveOccurred())

			// Attempt to create Tenant targeting Own Namespace
			tenant := &openbaov1alpha1.OpenBaoTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "legit-tenant",
					Namespace: attackerNS,
				},
				Spec: openbaov1alpha1.OpenBaoTenantSpec{
					TargetNamespace: attackerNS, // Match!
				},
			}

			Expect(attackerClient.Create(ctx, tenant)).To(Succeed())

			// Controller should Provision it
			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoTenant{}
				err := admin.Get(ctx, types.NamespacedName{Name: tenant.Name, Namespace: attackerNS}, updated)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updated.Status.Provisioned).To(BeTrue(), "Status.Provisioned should be true")
			}, 30*time.Second, 1*time.Second).Should(Succeed())
		})
	})
})

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

func randString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
