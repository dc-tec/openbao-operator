package provisioner

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// EnsureTenantRBAC ensures that a Role and RoleBinding exist in the given namespace
// for the operator to manage OpenBaoCluster resources.
func (m *Manager) EnsureTenantRBAC(ctx context.Context, tenant *openbaov1alpha1.OpenBaoTenant) error {
	namespace := tenant.Spec.TargetNamespace

	role := GenerateTenantRole(namespace)
	m.logger.Info("Applying tenant Role", "namespace", namespace, "role", TenantRoleName)
	if err := m.applyResource(ctx, role); err != nil {
		return fmt.Errorf("failed to apply tenant Role %s/%s: %w", namespace, TenantRoleName, err)
	}

	roleBinding := GenerateTenantRoleBinding(namespace, m.operatorSA)
	m.logger.Info("Applying tenant RoleBinding", "namespace", namespace, "rolebinding", TenantRoleBindingName)
	if err := m.applyResource(ctx, roleBinding); err != nil {
		return fmt.Errorf("failed to apply tenant RoleBinding %s/%s: %w", namespace, TenantRoleBindingName, err)
	}

	if err := m.ensureNamespacePodSecurityLabels(ctx, namespace); err != nil {
		return err
	}
	if err := m.EnsureTenantSecretRBAC(ctx, namespace); err != nil {
		return err
	}
	if err := m.EnsureTenantQuotas(ctx, namespace, tenant.Spec.Quota, tenant.Spec.LimitRange); err != nil {
		return err
	}

	return nil
}

func (m *Manager) ensureNamespacePodSecurityLabels(ctx context.Context, namespace string) error {
	// Use Get/Update instead of SSA because namespace labels are commonly shared with other controllers.
	ns := &corev1.Namespace{}
	if err := m.client.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		return fmt.Errorf("failed to get namespace %s: %w", namespace, err)
	}

	needsUpdate := false
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}

	const (
		enforceKey = "pod-security.kubernetes.io/enforce"
		auditKey   = "pod-security.kubernetes.io/audit"
		warnKey    = "pod-security.kubernetes.io/warn"
		levelValue = "restricted"
	)

	if ns.Labels[enforceKey] != levelValue {
		ns.Labels[enforceKey] = levelValue
		needsUpdate = true
	}
	if ns.Labels[auditKey] != levelValue {
		ns.Labels[auditKey] = levelValue
		needsUpdate = true
	}
	if ns.Labels[warnKey] != levelValue {
		ns.Labels[warnKey] = levelValue
		needsUpdate = true
	}

	if !needsUpdate {
		return nil
	}

	m.logger.Info("Applying Pod Security Standards labels to namespace", "namespace", namespace)
	if err := m.client.Update(ctx, ns); err != nil {
		return fmt.Errorf("failed to update namespace %s with Pod Security labels: %w", namespace, err)
	}
	return nil
}
