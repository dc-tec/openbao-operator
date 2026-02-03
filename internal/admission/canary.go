package admission

import (
	"context"
	"fmt"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// VerifyProvisionerRBACEnforcement performs a dry-run request that SHOULD be denied by the
// Provisioner RBAC ValidatingAdmissionPolicy.
//
// This provides stronger assurance than checking presence/bindings alone, because it validates
// that the API server is actively enforcing Deny decisions.
//
// The check is intentionally scoped to RBAC Role creation only and never persists objects.
func VerifyProvisionerRBACEnforcement(ctx context.Context, clientset kubernetes.Interface, namespace string) error {
	if ctx == nil {
		return fmt.Errorf("context is required")
	}
	if clientset == nil {
		return fmt.Errorf("kubernetes clientset is required")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openbao-operator-admission-canary",
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{},
	}

	_, err := clientset.RbacV1().Roles(namespace).Create(ctx, role, metav1.CreateOptions{
		DryRun: []string{metav1.DryRunAll},
	})
	if err == nil {
		return fmt.Errorf("expected canary Role create to be denied by ValidatingAdmissionPolicy, but it was allowed (dry-run)")
	}
	if apierrors.IsForbidden(err) || apierrors.IsInvalid(err) {
		// VAP denial surfaces as Forbidden with the policy message embedded.
		if strings.Contains(err.Error(), "Provisioner can only create Roles") ||
			strings.Contains(err.Error(), "The Provisioner can only create Roles") {
			return nil
		}
		// Some clusters may deny via the namespace restriction rule when testing in a protected namespace.
		if strings.Contains(err.Error(), "may not manage tenant RBAC in system namespaces") {
			return nil
		}
		return fmt.Errorf("canary Role create was denied, but not by the expected VAP message: %w", err)
	}

	return fmt.Errorf("unexpected error from canary Role create (dry-run): %w", err)
}
