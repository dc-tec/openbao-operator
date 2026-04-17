//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/provisioner"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

const (
	impersonatedUser  = "jane-developer"
	impersonatedGroup = "e2e-developers"
)

// === Shared Helpers ===

func createRoleBindingForGroup(ctx context.Context, c client.Client, namespace string, role *rbacv1.Role) {
	Expect(e2ehelpers.EnsureRoleBinding(ctx, c, role, []rbacv1.Subject{
		{
			Kind: "Group",
			Name: impersonatedGroup,
		},
	})).To(Succeed(), "Failed to ensure RoleBinding for %q in namespace %q", role.Name, namespace)
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func findAdmissionPolicyBinding(
	ctx context.Context,
	c client.Client,
	suffix string,
	prefixes []string,
) (*admissionregistrationv1.ValidatingAdmissionPolicyBinding, error) {
	for _, prefix := range prefixes {
		binding := &admissionregistrationv1.ValidatingAdmissionPolicyBinding{}
		name := prefix + suffix
		if err := c.Get(ctx, types.NamespacedName{Name: name}, binding); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		return binding, nil
	}
	return nil, fmt.Errorf("no ValidatingAdmissionPolicyBinding found for suffix %q", suffix)
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

	// --- Operator Pod Hardening ---
	Context("Operator Pod Hardening", Label("pentest", "tokens"), func() {
		const (
			controllerDeployment  = "openbao-operator-controller"
			provisionerDeployment = "openbao-operator-provisioner"
		)

		getDeployment := func(name string) (*appsv1.Deployment, error) {
			deploy := &appsv1.Deployment{}
			if err := admin.Get(ctx, types.NamespacedName{Name: name, Namespace: operatorNamespace}, deploy); err != nil {
				return nil, err
			}
			return deploy, nil
		}

		serviceAccountUsername := func(namespace, saName string) string {
			return fmt.Sprintf("system:serviceaccount:%s:%s", namespace, saName)
		}

		findProjectedSAToken := func(vol corev1.Volume) *corev1.ServiceAccountTokenProjection {
			if vol.Projected == nil {
				return nil
			}
			for i := range vol.Projected.Sources {
				src := &vol.Projected.Sources[i]
				if src.ServiceAccountToken != nil {
					return src.ServiceAccountToken
				}
			}
			return nil
		}

		It("disables default ServiceAccount token automount", func() {
			ctrl, err := getDeployment(controllerDeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(ctrl.Spec.Template.Spec.AutomountServiceAccountToken).NotTo(BeNil())
			Expect(*ctrl.Spec.Template.Spec.AutomountServiceAccountToken).To(BeFalse())

			// Provisioner exists only in multi-tenant mode; skip if absent (single-tenant installs).
			prov, err := getDeployment(provisionerDeployment)
			if apierrors.IsNotFound(err) {
				Skip("Provisioner Deployment not found; likely running in single-tenant mode")
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(prov.Spec.Template.Spec.AutomountServiceAccountToken).NotTo(BeNil())
			Expect(*prov.Spec.Template.Spec.AutomountServiceAccountToken).To(BeFalse())
		})

		It("uses projected Kubernetes API token with explicit audience and TTL", func() {
			expectedKubeAudience := os.Getenv("OPENBAO_KUBE_API_AUDIENCE")

			By("inspecting the controller projected service account tokens")
			ctrl, err := getDeployment(controllerDeployment)
			Expect(err).NotTo(HaveOccurred())

			var kubeAPIVol *corev1.Volume
			var openBaoVol *corev1.Volume
			for i := range ctrl.Spec.Template.Spec.Volumes {
				vol := &ctrl.Spec.Template.Spec.Volumes[i]
				switch vol.Name {
				case "kube-api-access":
					kubeAPIVol = vol
				case "openbao-token":
					openBaoVol = vol
				}
			}
			Expect(kubeAPIVol).NotTo(BeNil(), "expected kube-api-access projected volume")
			Expect(openBaoVol).NotTo(BeNil(), "expected openbao-token projected volume")

			kubeToken := findProjectedSAToken(*kubeAPIVol)
			Expect(kubeToken).NotTo(BeNil(), "expected serviceAccountToken projection for kube-api-access")
			Expect(kubeToken.ExpirationSeconds).NotTo(BeNil())
			Expect(*kubeToken.ExpirationSeconds).To(Equal(int64(3600)))
			if expectedKubeAudience == "" {
				Expect(kubeToken.Audience).To(BeEmpty())
			} else {
				Expect(kubeToken.Audience).To(Equal(expectedKubeAudience))
			}

			openBaoToken := findProjectedSAToken(*openBaoVol)
			Expect(openBaoToken).NotTo(BeNil(), "expected serviceAccountToken projection for openbao-token")
			Expect(openBaoToken.ExpirationSeconds).NotTo(BeNil())
			Expect(*openBaoToken.ExpirationSeconds).To(Equal(int64(3600)))
			Expect(openBaoToken.Audience).To(Equal("openbao-internal"))

			By("inspecting the provisioner projected Kubernetes API token when present")
			prov, err := getDeployment(provisionerDeployment)
			if apierrors.IsNotFound(err) {
				Skip("Provisioner Deployment not found; likely running in single-tenant mode")
			}
			Expect(err).NotTo(HaveOccurred())

			var provKubeAPIVol *corev1.Volume
			for i := range prov.Spec.Template.Spec.Volumes {
				vol := &prov.Spec.Template.Spec.Volumes[i]
				if vol.Name == "kube-api-access" {
					provKubeAPIVol = vol
					break
				}
			}
			Expect(provKubeAPIVol).NotTo(BeNil(), "expected kube-api-access projected volume on provisioner")
			provKubeToken := findProjectedSAToken(*provKubeAPIVol)
			Expect(provKubeToken).NotTo(BeNil(), "expected serviceAccountToken projection for provisioner kube-api-access")
			Expect(provKubeToken.ExpirationSeconds).NotTo(BeNil())
			Expect(*provKubeToken.ExpirationSeconds).To(Equal(int64(3600)))
			if expectedKubeAudience == "" {
				Expect(provKubeToken.Audience).To(BeEmpty())
			} else {
				Expect(provKubeToken.Audience).To(Equal(expectedKubeAudience))
			}
		})

		It("applies admission guardrails to provisioner identity", Label("rbac"), func() {
			prov, err := getDeployment(provisionerDeployment)
			if apierrors.IsNotFound(err) {
				Skip("Provisioner Deployment not found; likely running in single-tenant mode")
			}
			Expect(err).NotTo(HaveOccurred())

			tenantFW, err := framework.New(ctx, admin, "tenant-prov-guardrails", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = tenantFW.Cleanup(ctx) })

			provisionerSA := prov.Spec.Template.Spec.ServiceAccountName
			Expect(provisionerSA).NotTo(BeEmpty())
			provisionerUser := serviceAccountUsername(operatorNamespace, provisionerSA)
			provisionerGroups := []string{
				"system:serviceaccounts",
				fmt.Sprintf("system:serviceaccounts:%s", operatorNamespace),
				"system:authenticated",
			}

			By("denying creation of non-allowlisted Roles")
			err = e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, provisionerUser, provisionerGroups, func(c client.Client) error {
				return c.Create(ctx, &rbacv1.Role{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "evil-role",
						Namespace: tenantFW.Namespace,
					},
					// Intentionally empty rules: if we request permissions the Provisioner does not already hold,
					// Kubernetes RBAC escalation checks can deny the request before admission policies run.
					// This test is meant to validate the ValidatingAdmissionPolicy name restriction.
					Rules: []rbacv1.PolicyRule{},
				})
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("The Provisioner can only create Roles"),
				ContainSubstring("Provisioner can only create Roles"),
			))

			By("denying updates that attempt to broaden the tenant Role")
			roleKey := types.NamespacedName{Name: provisioner.TenantRoleName, Namespace: tenantFW.Namespace}
			original := &rbacv1.Role{}
			Expect(admin.Get(ctx, roleKey, original)).To(Succeed(), "expected tenant Role to exist")

			err = e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, provisionerUser, provisionerGroups, func(c client.Client) error {
				current := &rbacv1.Role{}
				if err := c.Get(ctx, roleKey, current); err != nil {
					return err
				}
				current.Rules = append(current.Rules, rbacv1.PolicyRule{
					APIGroups: []string{"*"},
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				})
				return c.Patch(ctx, current, client.MergeFrom(original))
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("wildcard permissions"),
				ContainSubstring("wildcard apiGroups or resources"),
				ContainSubstring("allowlisted set of API groups, resources, and verbs"),
			))

			By("denying updates that attempt to grant pods/exec on the tenant Role")
			err = e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, provisionerUser, provisionerGroups, func(c client.Client) error {
				current := &rbacv1.Role{}
				if err := c.Get(ctx, roleKey, current); err != nil {
					return err
				}
				current.Rules = append(current.Rules, rbacv1.PolicyRule{
					APIGroups: []string{""},
					Resources: []string{"pods/exec"},
					Verbs:     []string{"create"},
				})
				return c.Patch(ctx, current, client.MergeFrom(original))
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("allowlisted set of API groups, resources, and verbs"),
				ContainSubstring("allowlisted set of API groups"),
			))

			By("denying RBAC writes in system namespaces")
			err = e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, provisionerUser, provisionerGroups, func(c client.Client) error {
				return c.Create(ctx, &rbacv1.Role{
					ObjectMeta: metav1.ObjectMeta{
						Name:      provisioner.TenantRoleName,
						Namespace: "kube-system",
					},
				})
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("may not manage tenant RBAC in system namespaces"))
		})

		It("applies admission guardrails to provisioner Namespace mutations", Label("rbac"), func() {
			prov, err := getDeployment(provisionerDeployment)
			if apierrors.IsNotFound(err) {
				Skip("Provisioner Deployment not found; likely running in single-tenant mode")
			}
			Expect(err).NotTo(HaveOccurred())

			tenantFW, err := framework.New(ctx, admin, "tenant-prov-ns-guardrails", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = tenantFW.Cleanup(ctx) })

			provisionerSA := prov.Spec.Template.Spec.ServiceAccountName
			Expect(provisionerSA).NotTo(BeEmpty())
			provisionerUser := serviceAccountUsername(operatorNamespace, provisionerSA)
			provisionerGroups := []string{
				"system:serviceaccounts",
				fmt.Sprintf("system:serviceaccounts:%s", operatorNamespace),
				"system:authenticated",
			}

			By("denying Namespace label mutations outside the PSS enforcement keys")
			nsKey := types.NamespacedName{Name: tenantFW.Namespace}
			original := &corev1.Namespace{}
			Expect(admin.Get(ctx, nsKey, original)).To(Succeed())

			err = e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, provisionerUser, provisionerGroups, func(c client.Client) error {
				current := &corev1.Namespace{}
				if err := c.Get(ctx, nsKey, current); err != nil {
					return err
				}
				if current.Labels == nil {
					current.Labels = map[string]string{}
				}
				current.Labels["e2e.openbao.org/evil"] = "true"
				return c.Patch(ctx, current, client.MergeFrom(original))
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("may only enforce Pod Security Standards labels"),
				ContainSubstring("only enforce Pod Security Standards labels"),
			))

			By("allowing Pod Security Standards enforce=restricted label enforcement")
			original = &corev1.Namespace{}
			Expect(admin.Get(ctx, nsKey, original)).To(Succeed())
			err = e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, provisionerUser, provisionerGroups, func(c client.Client) error {
				current := &corev1.Namespace{}
				if err := c.Get(ctx, nsKey, current); err != nil {
					return err
				}
				if current.Labels == nil {
					current.Labels = map[string]string{}
				}
				current.Labels["pod-security.kubernetes.io/enforce"] = "restricted"
				return c.Patch(ctx, current, client.MergeFrom(original))
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("applies admission guardrails to controller RBAC writes", Label("rbac"), func() {
			ctrl, err := getDeployment(controllerDeployment)
			Expect(err).NotTo(HaveOccurred())

			tenantFW, err := framework.New(ctx, admin, "tenant-ctrl-rbac-guardrails", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = tenantFW.Cleanup(ctx) })

			controllerSA := ctrl.Spec.Template.Spec.ServiceAccountName
			Expect(controllerSA).NotTo(BeEmpty())
			controllerUser := serviceAccountUsername(operatorNamespace, controllerSA)
			controllerGroups := []string{
				"system:serviceaccounts",
				fmt.Sprintf("system:serviceaccounts:%s", operatorNamespace),
				"system:authenticated",
			}

			By("denying controller creation of arbitrary Roles")
			err = e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, controllerUser, controllerGroups, func(c client.Client) error {
				return c.Create(ctx, &rbacv1.Role{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "evil-controller-role",
						Namespace: tenantFW.Namespace,
					},
					// Empty rules avoids RBAC escalation checks and ensures the denial is from the VAP.
					Rules: []rbacv1.PolicyRule{},
				})
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("controller can only create/update Roles"),
				ContainSubstring("Controller can only create/update Roles"),
			))

			By("denying controller creation of RoleBindings that do not match the allowlisted pattern")
			// Create a harmless Role as admin so API servers that validate roleRef existence succeed.
			dummyRole := &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-dummy-role",
					Namespace: tenantFW.Namespace,
				},
				Rules: []rbacv1.PolicyRule{},
			}
			err = admin.Create(ctx, dummyRole)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			err = e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, controllerUser, controllerGroups, func(c client.Client) error {
				return c.Create(ctx, &rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "evil-controller-binding",
						Namespace: tenantFW.Namespace,
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: rbacv1.GroupName,
						Kind:     "Role",
						Name:     dummyRole.Name,
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "some-other-sa",
							Namespace: tenantFW.Namespace,
						},
					},
				})
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("can only create/update RoleBindings"),
				ContainSubstring("only create/update RoleBindings"),
			))
		})

		It("prevents operator identities from cluster-scoped RBAC writes", Label("rbac"), func() {
			ctrl, err := getDeployment(controllerDeployment)
			Expect(err).NotTo(HaveOccurred())

			prov, err := getDeployment(provisionerDeployment)
			if apierrors.IsNotFound(err) {
				Skip("Provisioner Deployment not found; likely running in single-tenant mode")
			}
			Expect(err).NotTo(HaveOccurred())

			controllerSA := ctrl.Spec.Template.Spec.ServiceAccountName
			Expect(controllerSA).NotTo(BeEmpty())
			controllerUser := serviceAccountUsername(operatorNamespace, controllerSA)

			provisionerSA := prov.Spec.Template.Spec.ServiceAccountName
			Expect(provisionerSA).NotTo(BeEmpty())
			provisionerUser := serviceAccountUsername(operatorNamespace, provisionerSA)

			checkCanI := func(user string, ra authorizationv1.ResourceAttributes) bool {
				impCfg := rest.CopyConfig(cfg)
				impCfg.Impersonate = rest.ImpersonationConfig{
					UserName: user,
					Groups: []string{
						"system:serviceaccounts",
						fmt.Sprintf("system:serviceaccounts:%s", operatorNamespace),
						"system:authenticated",
					},
				}
				clientset, err := kubernetes.NewForConfig(impCfg)
				Expect(err).NotTo(HaveOccurred())
				resp, err := clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &authorizationv1.SelfSubjectAccessReview{
					Spec: authorizationv1.SelfSubjectAccessReviewSpec{
						ResourceAttributes: &ra,
					},
				}, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				return resp.Status.Allowed
			}

			By("denying controller clusterrole/clusterrolebinding creation via RBAC")
			Expect(checkCanI(controllerUser, authorizationv1.ResourceAttributes{
				Group:    "rbac.authorization.k8s.io",
				Resource: "clusterroles",
				Verb:     "create",
			})).To(BeFalse())
			Expect(checkCanI(controllerUser, authorizationv1.ResourceAttributes{
				Group:    "rbac.authorization.k8s.io",
				Resource: "clusterrolebindings",
				Verb:     "create",
			})).To(BeFalse())

			By("denying provisioner clusterrole/clusterrolebinding creation via RBAC")
			Expect(checkCanI(provisionerUser, authorizationv1.ResourceAttributes{
				Group:    "rbac.authorization.k8s.io",
				Resource: "clusterroles",
				Verb:     "create",
			})).To(BeFalse())
			Expect(checkCanI(provisionerUser, authorizationv1.ResourceAttributes{
				Group:    "rbac.authorization.k8s.io",
				Resource: "clusterrolebindings",
				Verb:     "create",
			})).To(BeFalse())
		})
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
						Resources: []string{"openbaoclusters", "openbaorestores"},
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

		It("enforces digest-pinned images for managed workloads when digest enforcement is required", func() {
			newManagedJob := func(name, image string) *batchv1.Job {
				return &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: guardrailsNamespace,
						Annotations: map[string]string{
							constants.AnnotationMaintenance: "true",
						},
						Labels: map[string]string{
							constants.LabelAppManagedBy:             constants.LabelValueAppManagedByOpenBaoOperator,
							constants.LabelOpenBaoCluster:           "admission-e2e",
							constants.LabelOpenBaoComponent:         "admission-test",
							constants.LabelOpenBaoDigestEnforcement: constants.LabelValueDigestEnforcementRequired,
						},
					},
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyNever,
								Containers: []corev1.Container{
									{
										Name:    "test",
										Image:   image,
										Command: []string{"sh", "-c", "echo ok"},
									},
								},
							},
						},
					},
				}
			}

			createManagedDryRunAsController := func(job *batchv1.Job) error {
				return e2ehelpers.RunWithImpersonation(
					ctx,
					cfg,
					scheme,
					"system:serviceaccount:openbao-operator-system:openbao-operator-controller",
					[]string{
						"system:serviceaccounts",
						"system:serviceaccounts:openbao-operator-system",
						"system:authenticated",
					},
					func(c client.Client) error {
						return c.Create(ctx, job, client.DryRunAll)
					},
				)
			}

			tagJob := newManagedJob(fmt.Sprintf("digest-deny-%d", time.Now().UnixNano()), "ghcr.io/dc-tec/openbao-backup:dev")
			err := createManagedDryRunAsController(tagJob)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must use digest-pinned images"))

			digestJob := newManagedJob(
				fmt.Sprintf("digest-allow-%d", time.Now().UnixNano()),
				"ghcr.io/dc-tec/openbao-backup@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			)
			Expect(createManagedDryRunAsController(digestJob)).To(Succeed())
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
						Schedule:    "0 0 * * *",
						Image:       "ghcr.io/dc-tec/openbao-backup:1.0.0",
						JWTAuthRole: "backup-role",
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

		It("blocks link-local endpoints in restore source (SSRF protection)", func() {
			Eventually(func() string {
				restore := &openbaov1alpha1.OpenBaoRestore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("ssrf-restore-link-local-%d", time.Now().UnixNano()),
						Namespace: guardrailsNamespace,
					},
					Spec: openbaov1alpha1.OpenBaoRestoreSpec{
						Cluster: "does-not-matter-for-admission",
						Source: openbaov1alpha1.RestoreSource{
							Target: openbaov1alpha1.BackupTarget{
								Provider: "s3",
								Endpoint: "http://169.254.169.254/latest/meta-data",
								Bucket:   "test-bucket",
								CredentialsSecretRef: &corev1.LocalObjectReference{
									Name: "restore-creds",
								},
							},
							Key: "clusters/prod/snapshot.snap",
						},
						JWTAuthRole: "restore",
						Image:       "ghcr.io/dc-tec/openbao-backup:1.0.0",
						Force:       true,
					},
				}

				err := e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, impersonatedUser, []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
					return c.Create(ctx, restore)
				})
				if err == nil {
					_ = admin.Delete(ctx, restore)
					return ""
				}
				return err.Error()
			}, 2*time.Minute, 2*time.Second).Should(ContainSubstring("Restore endpoint cannot point to link-local addresses"))
		})

		It("blocks non-cluster HTTP restore endpoints (require HTTPS except *.svc)", func() {
			Eventually(func() string {
				restore := &openbaov1alpha1.OpenBaoRestore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("restore-require-https-%d", time.Now().UnixNano()),
						Namespace: guardrailsNamespace,
					},
					Spec: openbaov1alpha1.OpenBaoRestoreSpec{
						Cluster: "does-not-matter-for-admission",
						Source: openbaov1alpha1.RestoreSource{
							Target: openbaov1alpha1.BackupTarget{
								Provider: "s3",
								Endpoint: "http://example.com",
								Bucket:   "test-bucket",
								CredentialsSecretRef: &corev1.LocalObjectReference{
									Name: "restore-creds",
								},
							},
							Key: "clusters/prod/snapshot.snap",
						},
						JWTAuthRole: "restore",
						Image:       "ghcr.io/dc-tec/openbao-backup:1.0.0",
						Force:       true,
					},
				}

				err := e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, impersonatedUser, []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
					return c.Create(ctx, restore)
				})
				if err == nil {
					_ = admin.Delete(ctx, restore)
					return ""
				}
				return err.Error()
			}, 2*time.Minute, 2*time.Second).Should(ContainSubstring("Restore endpoint must use HTTPS or S3 scheme"))
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
						APIGroups: []string{""},
						Resources: []string{"pods"},
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
			Expect(secret.Labels).To(HaveKeyWithValue("openbao.org/cluster", victim.Name))

			err = e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, impersonatedUser, []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
				return c.Delete(ctx, secret)
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Direct modification of OpenBao-managed resources is prohibited"))
		})

		It("prevents sidecar injection via StatefulSet updates", func() {
			err := e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, "hacker", []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
				return retry.RetryOnConflict(retry.DefaultRetry, func() error {
					sts := &appsv1.StatefulSet{}
					if err := c.Get(ctx, types.NamespacedName{Name: statefulSet, Namespace: tenantNamespace}, sts); err != nil {
						return fmt.Errorf("failed to get StatefulSet: %w", err)
					}
					sts.Spec.Template.Spec.Containers[0].Image = "malicious.invalid/sidecar:latest"
					return c.Update(ctx, sts)
				})
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Direct modification of OpenBao-managed resources is prohibited"))
		})

		It("prevents managed Pod deletion during maintenance without cluster maintenance permission", func() {
			Expect(tenantFW.SetMaintenanceEnabled(ctx, victim.Name, true)).To(Succeed())

			var targetPod corev1.Pod
			Eventually(func(g Gomega) {
				pods := &corev1.PodList{}
				g.Expect(admin.List(ctx, pods,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{
						"openbao.org/cluster": victim.Name,
					},
				)).To(Succeed())
				g.Expect(pods.Items).NotTo(BeEmpty())
				targetPod = pods.Items[0]
				g.Expect(targetPod.Annotations).To(HaveKeyWithValue(constants.AnnotationMaintenance, "true"))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			err := e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, impersonatedUser, []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
				return c.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: targetPod.Name, Namespace: targetPod.Namespace}})
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Direct modification of OpenBao-managed resources is prohibited"))
		})

		It("allows managed Pod deletion during maintenance with cluster maintenance permission", func() {
			maintenanceRole := &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-victim-maintenance",
					Namespace: tenantNamespace,
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups:     []string{"openbao.org"},
						Resources:     []string{"openbaoclusters"},
						ResourceNames: []string{victim.Name},
						Verbs:         []string{"get", "maintenance"},
					},
					{
						APIGroups:     []string{"openbao.org"},
						Resources:     []string{"openbaoclusters/status"},
						ResourceNames: []string{victim.Name},
						Verbs:         []string{"get"},
					},
				},
			}
			createRoleBindingForGroup(ctx, admin, tenantNamespace, maintenanceRole)
			Expect(tenantFW.SetMaintenanceEnabled(ctx, victim.Name, true)).To(Succeed())

			var targetPod corev1.Pod
			Eventually(func(g Gomega) {
				pods := &corev1.PodList{}
				g.Expect(admin.List(ctx, pods,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{
						"openbao.org/cluster": victim.Name,
					},
				)).To(Succeed())
				g.Expect(pods.Items).NotTo(BeEmpty())
				targetPod = pods.Items[0]
				g.Expect(targetPod.Annotations).To(HaveKeyWithValue(constants.AnnotationMaintenance, "true"))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())
			originalUID := targetPod.UID

			err := e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, impersonatedUser, []string{"system:authenticated", impersonatedGroup}, func(c client.Client) error {
				return c.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: targetPod.Name, Namespace: targetPod.Namespace}})
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				pods := &corev1.PodList{}
				g.Expect(admin.List(ctx, pods,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{
						"openbao.org/cluster": victim.Name,
					},
				)).To(Succeed())
				g.Expect(pods.Items).NotTo(BeEmpty())
				g.Expect(pods.Items[0].UID).NotTo(Equal(originalUID))
			}, 4*time.Minute, 2*time.Second).Should(Succeed())
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

	// --- Admission Dependency Runtime Recheck ---
	Context("Admission Dependency Runtime Recheck", Label("admission", "pentest"), func() {
		var (
			tenantFW        *framework.Framework
			tenantNamespace string
			clusterName     = "admission-runtime-loss"
		)

		BeforeAll(func() {
			var err error
			tenantFW, err = framework.New(ctx, admin, "tenant-admission-runtime", operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			tenantNamespace = tenantFW.Namespace

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
						OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
							Enabled: true,
						},
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
					Maintenance: &openbaov1alpha1.MaintenanceConfig{
						Enabled: true,
					},
					Network: &openbaov1alpha1.NetworkConfig{
						APIServerCIDR: apiServerCIDR,
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
				},
			}
			Expect(admin.Create(ctx, cluster)).To(Succeed())

			Eventually(func() error {
				return admin.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, &openbaov1alpha1.OpenBaoCluster{})
			}, 30*time.Second, time.Second).Should(Succeed())

			_, err = tenantFW.WaitForStatefulSetReady(ctx, clusterName, 1, framework.DefaultWaitTimeout, framework.DefaultPollInterval)
			Expect(err).NotTo(HaveOccurred())
			Expect(tenantFW.TriggerReconcile(ctx, clusterName)).To(Succeed())
			tenantFW.WaitForCondition(clusterName, openbaov1alpha1.ConditionAvailable, metav1.ConditionTrue)
		})

		AfterAll(func() {
			if tenantFW != nil {
				_ = tenantFW.Cleanup(ctx)
			}
		})

		It("pauses managed-resource reconciliation when a required admission binding disappears, then recovers when restored", Label(
			"case:admission-runtime-binding-loss",
			"covers:admission-runtime-recheck",
			"covers:managed-resource-pause-on-policy-loss",
		), func() {
			const bindingSuffix = "openbao-lock-managed-resource-mutations-binding"

			binding, err := findAdmissionPolicyBinding(ctx, admin, bindingSuffix, admission.DefaultNamePrefixes())
			Expect(err).NotTo(HaveOccurred())
			originalBinding := binding.DeepCopy()
			originalBinding.SetResourceVersion("")
			originalBinding.SetUID("")
			originalBinding.SetCreationTimestamp(metav1.Time{})
			originalBinding.SetManagedFields(nil)

			By("removing a required admission binding after the cluster is healthy")
			Expect(admin.Delete(ctx, binding)).To(Succeed())
			DeferCleanup(func() {
				current := &admissionregistrationv1.ValidatingAdmissionPolicyBinding{}
				err := admin.Get(ctx, types.NamespacedName{Name: originalBinding.Name}, current)
				if apierrors.IsNotFound(err) {
					Expect(admin.Create(ctx, originalBinding)).To(Succeed())
					return
				}
				Expect(err).NotTo(HaveOccurred())
			})

			By("waiting for live dependency checks to report the missing binding")
			Eventually(func(g Gomega) {
				checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()

				status, err := admission.CheckDependencies(checkCtx, admin, admission.DefaultDependencies(), admission.DefaultNamePrefixes())
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(status.OverallReady).To(BeFalse())
				g.Expect(status.SummaryMessage()).To(ContainSubstring(bindingSuffix))
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("requesting a scale-up that would normally mutate the managed StatefulSet")
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				current := &openbaov1alpha1.OpenBaoCluster{}
				if err := admin.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, current); err != nil {
					return err
				}
				original := current.DeepCopy()
				current.Spec.Replicas = 2
				return admin.Patch(ctx, current, client.MergeFrom(original))
			})).To(Succeed())
			Expect(tenantFW.TriggerReconcile(ctx, clusterName)).To(Succeed())

			By("proving the controller fails closed and does not mutate the StatefulSet")
			Consistently(func(g Gomega) {
				sts := &appsv1.StatefulSet{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, sts)).To(Succeed())
				g.Expect(sts.Spec.Replicas).NotTo(BeNil())
				g.Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
			}, 45*time.Second, 5*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				updated := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(admin.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, updated)).To(Succeed())
				available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(available.Reason).To(Equal("NotReady"))
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("restoring the admission binding and verifying recovery")
			Expect(admin.Create(ctx, originalBinding)).To(Succeed())
			Eventually(func(g Gomega) {
				checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()

				status, err := admission.CheckDependencies(checkCtx, admin, admission.DefaultDependencies(), admission.DefaultNamePrefixes())
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(status.OverallReady).To(BeTrue(), status.SummaryMessage())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			Expect(tenantFW.TriggerReconcile(ctx, clusterName)).To(Succeed())
			_, err = tenantFW.WaitForStatefulSetReady(ctx, clusterName, 2, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval)
			Expect(err).NotTo(HaveOccurred())
			tenantFW.WaitForCondition(clusterName, openbaov1alpha1.ConditionAvailable, metav1.ConditionTrue)
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
			By("creating a cluster to trigger tenant RBAC provisioning")
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

			By("verifying the tenant role does not grant broad Secret access")
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

			By("verifying the dedicated Secrets writer role only grants the expected allowlisted access")
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
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			By("verifying the Secrets writer RoleBinding points at the allowlist role")
			Eventually(func(g Gomega) {
				rb := &rbacv1.RoleBinding{}
				g.Expect(admin.Get(ctx, writerRBKey, rb)).To(Succeed())
				g.Expect(rb.RoleRef.Name).To(Equal(provisioner.TenantSecretsWriterRoleName))
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		})

		It("restricts OpenBao pod ServiceAccount pod patching to cluster pods", Label("pentest"), func() {
			clusterName := "rbac-pod-mutation"
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

			By("waiting for the cluster StatefulSet to exist and expose the Pod service account")
			var clusterSA string
			Eventually(func(g Gomega) {
				sts := &appsv1.StatefulSet{}
				err := admin.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: tenantNamespace}, sts)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sts.Spec.Template.Spec.ServiceAccountName).NotTo(BeEmpty())
				clusterSA = sts.Spec.Template.Spec.ServiceAccountName
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			clusterUser := fmt.Sprintf("system:serviceaccount:%s:%s", tenantNamespace, clusterSA)
			impCfg := rest.CopyConfig(cfg)
			impCfg.Impersonate = rest.ImpersonationConfig{
				UserName: clusterUser,
				Groups: []string{
					"system:serviceaccounts",
					fmt.Sprintf("system:serviceaccounts:%s", tenantNamespace),
					"system:authenticated",
				},
			}
			clientset, err := kubernetes.NewForConfig(impCfg)
			Expect(err).NotTo(HaveOccurred())

			By("creating a non-OpenBao pod in the tenant namespace")
			otherPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "not-" + clusterName + "-0",
					Namespace: tenantNamespace,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					AutomountServiceAccountToken: func() *bool {
						v := false
						return &v
					}(),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: func() *bool {
							v := true
							return &v
						}(),
						RunAsUser: func() *int64 {
							v := int64(65532)
							return &v
						}(),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "pause",
							Image: "registry.k8s.io/pause:3.9",
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: func() *bool {
									v := false
									return &v
								}(),
								ReadOnlyRootFilesystem: func() *bool {
									v := true
									return &v
								}(),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
				},
			}
			err = admin.Create(ctx, otherPod)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
			DeferCleanup(func() {
				_ = admin.Delete(ctx, otherPod)
			})

			By("waiting for an OpenBao pod to exist")
			openBaoPodName := ""
			Eventually(func(g Gomega) {
				podList := &corev1.PodList{}
				g.Expect(admin.List(ctx, podList,
					client.InNamespace(tenantNamespace),
					client.MatchingLabels{"openbao.org/cluster": clusterName},
				)).To(Succeed())
				g.Expect(podList.Items).NotTo(BeEmpty())
				openBaoPodName = podList.Items[0].Name
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			patch := []byte(`{"metadata":{"labels":{"openbao-active":"true"}}}`)
			_, err = clientset.CoreV1().Pods(tenantNamespace).Patch(
				ctx,
				openBaoPodName,
				types.MergePatchType,
				patch,
				metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}},
			)
			Expect(err).NotTo(HaveOccurred(), "expected OpenBao pod ServiceAccount to be able to patch the OpenBao pod labels (dry-run)")

			_, err = clientset.CoreV1().Pods(tenantNamespace).Patch(
				ctx,
				otherPod.Name,
				types.MergePatchType,
				patch,
				metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}},
			)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsForbidden(err)).To(BeTrue(), "expected OpenBao pod ServiceAccount pod patch to be restricted by resourceNames")
		})

		It("has required ValidatingAdmissionPolicy dependencies installed and correctly bound", func() {
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			status, err := admission.CheckDependencies(checkCtx, admin, admission.DefaultDependencies(), []string{"openbao-operator-", ""})
			Expect(err).NotTo(HaveOccurred())
			Expect(status.OverallReady).To(BeTrue(), status.SummaryMessage())

			dependencyNames := make([]string, 0, len(status.Dependencies))
			for _, dep := range status.Dependencies {
				dependencyNames = append(dependencyNames, dep.Dependency.Name)
			}

			expectedDependencyNames := make([]string, 0, len(admission.DefaultDependencies()))
			for _, dep := range admission.DefaultDependencies() {
				expectedDependencyNames = append(expectedDependencyNames, dep.Name)
			}

			Expect(dependencyNames).To(ConsistOf(expectedDependencyNames))
		})
	})
})
