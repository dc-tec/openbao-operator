package provisioner

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Manager handles the provisioning of RBAC resources for tenant namespaces.
type Manager struct {
	client     client.Client
	operatorSA OperatorServiceAccount
	logger     logr.Logger
}

// NewManager creates a new provisioner Manager.
func NewManager(client client.Client, logger logr.Logger) *Manager {
	// Get operator ServiceAccount name and namespace from environment or use defaults
	saName := os.Getenv("OPERATOR_SERVICE_ACCOUNT_NAME")
	if saName == "" {
		saName = "controller-manager"
	}

	saNamespace := os.Getenv("OPERATOR_NAMESPACE")
	if saNamespace == "" {
		saNamespace = "openbao-operator-system"
	}

	return &Manager{
		client: client,
		operatorSA: OperatorServiceAccount{
			Name:      saName,
			Namespace: saNamespace,
		},
		logger: logger,
	}
}

// EnsureTenantRBAC ensures that a Role and RoleBinding exist in the given namespace
// for the operator to manage OpenBaoCluster resources.
func (m *Manager) EnsureTenantRBAC(ctx context.Context, namespace string) error {
	// Ensure Role exists
	role := GenerateTenantRole(namespace)
	existingRole := &rbacv1.Role{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      TenantRoleName,
	}, existingRole)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get Role %s/%s: %w", namespace, TenantRoleName, err)
		}

		m.logger.Info("Creating tenant Role", "namespace", namespace, "role", TenantRoleName)
		if err := m.client.Create(ctx, role); err != nil {
			return fmt.Errorf("failed to create Role %s/%s: %w", namespace, TenantRoleName, err)
		}
	} else {
		// Update Role if rules have changed
		if !rolesEqual(existingRole.Rules, role.Rules) {
			m.logger.Info("Updating tenant Role", "namespace", namespace, "role", TenantRoleName)
			existingRole.Rules = role.Rules
			// Preserve existing labels and merge with new ones
			if existingRole.Labels == nil {
				existingRole.Labels = make(map[string]string)
			}
			for k, v := range role.Labels {
				existingRole.Labels[k] = v
			}
			if err := m.client.Update(ctx, existingRole); err != nil {
				return fmt.Errorf("failed to update Role %s/%s: %w", namespace, TenantRoleName, err)
			}
		}
	}

	// Ensure RoleBinding exists
	roleBinding := GenerateTenantRoleBinding(namespace, m.operatorSA)
	existingRoleBinding := &rbacv1.RoleBinding{}
	err = m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      TenantRoleBindingName,
	}, existingRoleBinding)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get RoleBinding %s/%s: %w", namespace, TenantRoleBindingName, err)
		}

		m.logger.Info("Creating tenant RoleBinding", "namespace", namespace, "rolebinding", TenantRoleBindingName)
		if err := m.client.Create(ctx, roleBinding); err != nil {
			return fmt.Errorf("failed to create RoleBinding %s/%s: %w", namespace, TenantRoleBindingName, err)
		}
	} else {
		// Update RoleBinding if it has changed
		needsUpdate := false
		if existingRoleBinding.RoleRef.Name != roleBinding.RoleRef.Name ||
			existingRoleBinding.RoleRef.Kind != roleBinding.RoleRef.Kind ||
			existingRoleBinding.RoleRef.APIGroup != roleBinding.RoleRef.APIGroup {
			existingRoleBinding.RoleRef = roleBinding.RoleRef
			needsUpdate = true
		}

		// Check if subjects match
		if len(existingRoleBinding.Subjects) != len(roleBinding.Subjects) {
			existingRoleBinding.Subjects = roleBinding.Subjects
			needsUpdate = true
		} else if len(roleBinding.Subjects) > 0 {
			existingSubject := existingRoleBinding.Subjects[0]
			desiredSubject := roleBinding.Subjects[0]
			if existingSubject.Kind != desiredSubject.Kind ||
				existingSubject.Name != desiredSubject.Name ||
				existingSubject.Namespace != desiredSubject.Namespace {
				existingRoleBinding.Subjects = roleBinding.Subjects
				needsUpdate = true
			}
		}

		// Update labels
		if existingRoleBinding.Labels == nil {
			existingRoleBinding.Labels = make(map[string]string)
		}
		for k, v := range roleBinding.Labels {
			if existingRoleBinding.Labels[k] != v {
				existingRoleBinding.Labels[k] = v
				needsUpdate = true
			}
		}

		if needsUpdate {
			m.logger.Info("Updating tenant RoleBinding", "namespace", namespace, "rolebinding", TenantRoleBindingName)
			if err := m.client.Update(ctx, existingRoleBinding); err != nil {
				return fmt.Errorf("failed to update RoleBinding %s/%s: %w", namespace, TenantRoleBindingName, err)
			}
		}
	}

	return nil
}

// CleanupTenantRBAC removes the Role and RoleBinding from the given namespace.
func (m *Manager) CleanupTenantRBAC(ctx context.Context, namespace string) error {
	// Delete RoleBinding first
	roleBinding := &rbacv1.RoleBinding{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      TenantRoleBindingName,
	}, roleBinding)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get RoleBinding %s/%s: %w", namespace, TenantRoleBindingName, err)
		}
	} else {
		m.logger.Info("Deleting tenant RoleBinding", "namespace", namespace, "rolebinding", TenantRoleBindingName)
		if err := m.client.Delete(ctx, roleBinding); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete RoleBinding %s/%s: %w", namespace, TenantRoleBindingName, err)
		}
	}

	// Delete Role
	role := &rbacv1.Role{}
	err = m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      TenantRoleName,
	}, role)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get Role %s/%s: %w", namespace, TenantRoleName, err)
		}
	} else {
		m.logger.Info("Deleting tenant Role", "namespace", namespace, "role", TenantRoleName)
		if err := m.client.Delete(ctx, role); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete Role %s/%s: %w", namespace, TenantRoleName, err)
		}
	}

	return nil
}

// rolesEqual compares two PolicyRule slices for equality.
func rolesEqual(a, b []rbacv1.PolicyRule) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if !policyRuleEqual(a[i], b[i]) {
			return false
		}
	}

	return true
}

// policyRuleEqual compares two PolicyRules for equality.
func policyRuleEqual(a, b rbacv1.PolicyRule) bool {
	if len(a.APIGroups) != len(b.APIGroups) {
		return false
	}
	for i := range a.APIGroups {
		if a.APIGroups[i] != b.APIGroups[i] {
			return false
		}
	}

	if len(a.Resources) != len(b.Resources) {
		return false
	}
	for i := range a.Resources {
		if a.Resources[i] != b.Resources[i] {
			return false
		}
	}

	if len(a.Verbs) != len(b.Verbs) {
		return false
	}
	for i := range a.Verbs {
		if a.Verbs[i] != b.Verbs[i] {
			return false
		}
	}

	return true
}
