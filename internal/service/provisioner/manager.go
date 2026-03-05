package provisioner

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	clusterpkg "github.com/dc-tec/openbao-operator/internal/adapter/cluster"
	"github.com/dc-tec/openbao-operator/internal/adapter/kube"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

// Manager handles the provisioning of RBAC resources for tenant namespaces.
type Manager struct {
	client     client.Client
	operatorSA OperatorServiceAccount
	logger     logr.Logger
}

// NewManager creates a new provisioner Manager.
func NewManager(ctx context.Context, c client.Client, logger logr.Logger) (*Manager, error) {
	// Get operator namespace from environment or use default
	saNamespace := os.Getenv("POD_NAMESPACE")
	if saNamespace == "" {
		saNamespace = os.Getenv("OPERATOR_NAMESPACE")
	}
	if saNamespace == "" {
		saNamespace = "openbao-operator-system"
	}

	// Discover the controller ServiceAccount name dynamically
	// The base name is "controller", which becomes "openbao-operator-controller" after kustomize prefix
	controllerSAName := os.Getenv("OPERATOR_SERVICE_ACCOUNT_NAME")
	if controllerSAName == "" {
		controllerSAName = "openbao-operator-controller"
	}
	controllerSANamespace := saNamespace

	return &Manager{
		client: c,
		operatorSA: OperatorServiceAccount{
			Name:      controllerSAName,
			Namespace: controllerSANamespace,
		},
		logger: logger,
	}, nil
}

// applyResource uses Server-Side Apply.
// Unlike infra.applyResource, this does NOT set owner references since
// tenant RBAC resources should not be garbage-collected with any single cluster.
func (m *Manager) applyResource(ctx context.Context, obj client.Object) error {
	applyConfig, err := kube.ToApplyConfiguration(obj, m.client)
	if err != nil {
		return fmt.Errorf("failed to convert object to ApplyConfiguration: %w", err)
	}

	applyOpts := []client.ApplyOption{
		client.ForceOwnership,
		client.FieldOwner("openbao-provisioner"),
	}

	if err := m.client.Apply(ctx, applyConfig, applyOpts...); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(
				fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}

// EnsureTenantRBAC ensures that a Role and RoleBinding exist in the given namespace
// for the operator to manage OpenBaoCluster resources.
func (m *Manager) EnsureTenantRBAC(ctx context.Context, tenant *openbaov1alpha1.OpenBaoTenant) error {
	namespace := tenant.Spec.TargetNamespace

	// Apply Role using Server-Side Apply
	role := GenerateTenantRole(namespace)
	m.logger.Info("Applying tenant Role", "namespace", namespace, "role", TenantRoleName)
	if err := m.applyResource(ctx, role); err != nil {
		return fmt.Errorf("failed to apply tenant Role %s/%s: %w", namespace, TenantRoleName, err)
	}

	// Apply RoleBinding using Server-Side Apply
	roleBinding := GenerateTenantRoleBinding(namespace, m.operatorSA)
	m.logger.Info("Applying tenant RoleBinding", "namespace", namespace, "rolebinding", TenantRoleBindingName)
	if err := m.applyResource(ctx, roleBinding); err != nil {
		return fmt.Errorf("failed to apply tenant RoleBinding %s/%s: %w", namespace, TenantRoleBindingName, err)
	}

	// Apply Pod Security Standards labels to namespace
	// Note: Using Get-Update for namespace labels as SSA for namespace labels
	// could potentially conflict with other controllers managing the same namespace.
	ns := &corev1.Namespace{}
	if err := m.client.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		return fmt.Errorf("failed to get namespace %s: %w", namespace, err)
	}

	// Check if labels need to be updated
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

	if needsUpdate {
		m.logger.Info("Applying Pod Security Standards labels to namespace", "namespace", namespace)
		if err := m.client.Update(ctx, ns); err != nil {
			return fmt.Errorf("failed to update namespace %s with Pod Security labels: %w", namespace, err)
		}
	}

	// Reconcile Secret allowlist RBAC for this tenant namespace.
	if err := m.EnsureTenantSecretRBAC(ctx, namespace); err != nil {
		return err
	}

	// Apply ResourceQuota and LimitRange for this tenant namespace.
	if err := m.EnsureTenantQuotas(ctx, namespace, tenant.Spec.Quota, tenant.Spec.LimitRange); err != nil {
		return err
	}

	return nil
}

// EnsureTenantSecretRBAC ensures tenant Secret access is reduced to explicit allowlists.
//
// This creates/updates two Roles in the tenant namespace:
// - TenantSecretsWriterRoleName: write access to operator-owned Secret names.
// - TenantSecretsReaderRoleName: read-only access to user-provided Secret names referenced by specs.
//
// Both Roles are bound to the operator controller ServiceAccount via RoleBindings.
func (m *Manager) EnsureTenantSecretRBAC(ctx context.Context, namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}

	clusterList := &openbaov1alpha1.OpenBaoClusterList{}
	if err := m.client.List(ctx, clusterList, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list OpenBaoClusters in namespace %s: %w", namespace, err)
	}

	writerNames := map[string]struct{}{}
	readerNames := map[string]struct{}{}

	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]
		accumulateTenantSecretNames(cluster, writerNames, readerNames)
	}

	writerList := sortedUniqueSecretNames(writerNames)
	readerList := sortedUniqueSecretNames(readerNames)

	type secretsRBACSpec struct {
		roleName           string
		roleBindingName    string
		secretNames        []string
		roleFactory        secretsRoleFactory
		roleBindingFactory secretsRoleBindingFactory
	}

	desired := []secretsRBACSpec{
		{
			roleName:           TenantSecretsWriterRoleName,
			roleBindingName:    TenantSecretsWriterRoleBindingName,
			secretNames:        writerList,
			roleFactory:        GenerateTenantSecretsWriterRole,
			roleBindingFactory: GenerateTenantSecretsWriterRoleBinding,
		},
		{
			roleName:           TenantSecretsReaderRoleName,
			roleBindingName:    TenantSecretsReaderRoleBindingName,
			secretNames:        readerList,
			roleFactory:        GenerateTenantSecretsReaderRole,
			roleBindingFactory: GenerateTenantSecretsReaderRoleBinding,
		},
	}

	for _, spec := range desired {
		if err := m.ensureSecretsRole(ctx, namespace, spec.roleName, spec.secretNames, spec.roleFactory); err != nil {
			return err
		}
		if err := m.ensureSecretsRoleBinding(ctx, namespace, spec.roleBindingName, spec.roleName, len(spec.secretNames) > 0, spec.roleBindingFactory); err != nil {
			return err
		}
	}

	return nil
}

func accumulateTenantSecretNames(cluster *openbaov1alpha1.OpenBaoCluster, writer, reader map[string]struct{}) {
	if cluster == nil || cluster.Name == "" {
		return
	}
	for _, perm := range clusterpkg.GetRequiredSecretPermissions(cluster) {
		if perm.Permission == clusterpkg.PermissionWrite {
			writer[perm.Name] = struct{}{}
		} else {
			reader[perm.Name] = struct{}{}
		}
	}
}

func sortedUniqueSecretNames(names map[string]struct{}) []string {
	if len(names) == 0 {
		return nil
	}

	out := make([]string, 0, len(names))
	for name := range names {
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type secretsRoleFactory func(namespace string, secretNames []string) *rbacv1.Role

func (m *Manager) ensureSecretsRole(ctx context.Context, namespace string, roleName string, secretNames []string, factory secretsRoleFactory) error {
	if roleName == "" {
		return fmt.Errorf("role name is required")
	}

	// Handle deletion case: if no secrets, delete the role if it exists
	if len(secretNames) == 0 {
		existing := &rbacv1.Role{}
		if err := m.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: roleName}, existing); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get Role %s/%s: %w", namespace, roleName, err)
		}
		m.logger.Info("Deleting tenant secrets Role (no clusters reference Secrets)", "namespace", namespace, "role", roleName)
		if err := m.client.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete Role %s/%s: %w", namespace, roleName, err)
		}
		return nil
	}

	// Apply Role using Server-Side Apply
	role := factory(namespace, secretNames)
	if role == nil {
		return fmt.Errorf("failed to build Role %s/%s: factory returned nil", namespace, roleName)
	}
	if role.GetName() != roleName || role.GetNamespace() != namespace {
		return fmt.Errorf("failed to build Role %s/%s: factory returned %s/%s", namespace, roleName, role.GetNamespace(), role.GetName())
	}
	m.logger.Info("Applying tenant secrets Role", "namespace", namespace, "role", roleName)
	if err := m.applyResource(ctx, role); err != nil {
		return fmt.Errorf("failed to apply secrets Role %s/%s: %w", namespace, roleName, err)
	}

	return nil
}

type secretsRoleBindingFactory func(namespace string, operatorSA OperatorServiceAccount) *rbacv1.RoleBinding

func (m *Manager) ensureSecretsRoleBinding(ctx context.Context, namespace string, roleBindingName string, roleName string, shouldExist bool, factory secretsRoleBindingFactory) error {
	if roleBindingName == "" || roleName == "" {
		return fmt.Errorf("role binding name and role name are required")
	}

	// Handle deletion case: if shouldExist is false, delete the role binding if it exists
	if !shouldExist {
		existing := &rbacv1.RoleBinding{}
		if err := m.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: roleBindingName}, existing); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get RoleBinding %s/%s: %w", namespace, roleBindingName, err)
		}
		m.logger.Info("Deleting tenant secrets RoleBinding (no clusters reference Secrets)", "namespace", namespace, "rolebinding", roleBindingName)
		if err := m.client.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete RoleBinding %s/%s: %w", namespace, roleBindingName, err)
		}
		return nil
	}

	// Apply RoleBinding using Server-Side Apply
	roleBinding := factory(namespace, m.operatorSA)
	if roleBinding == nil {
		return fmt.Errorf("failed to build RoleBinding %s/%s: factory returned nil", namespace, roleBindingName)
	}
	if roleBinding.GetName() != roleBindingName || roleBinding.GetNamespace() != namespace {
		return fmt.Errorf("failed to build RoleBinding %s/%s: factory returned %s/%s", namespace, roleBindingName, roleBinding.GetNamespace(), roleBinding.GetName())
	}
	if roleBinding.RoleRef.Name != roleName {
		return fmt.Errorf("failed to build RoleBinding %s/%s: roleRef.name=%q want %q", namespace, roleBindingName, roleBinding.RoleRef.Name, roleName)
	}
	m.logger.Info("Applying tenant secrets RoleBinding", "namespace", namespace, "rolebinding", roleBindingName)
	if err := m.applyResource(ctx, roleBinding); err != nil {
		return fmt.Errorf("failed to apply secrets RoleBinding %s/%s: %w", namespace, roleBindingName, err)
	}

	return nil
}

// IsTenantNamespaceProvisioned returns true if the tenant namespace has been provisioned
// (i.e., the core tenant RoleBinding exists).
//
// SECURITY: RBAC objects are not cached, so this avoids requiring list/watch permissions.
func (m *Manager) IsTenantNamespaceProvisioned(ctx context.Context, namespace string) (bool, error) {
	if namespace == "" {
		return false, fmt.Errorf("namespace is required")
	}

	existing := &rbacv1.RoleBinding{}
	if err := m.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: TenantRoleBindingName}, existing); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get tenant RoleBinding %s/%s: %w", namespace, TenantRoleBindingName, err)
	}
	return true, nil
}

// CleanupTenantRBAC removes the Role and RoleBinding from the given namespace.
func (m *Manager) CleanupTenantRBAC(ctx context.Context, namespace string) error {
	secretRoleBindings := []string{
		TenantSecretsReaderRoleBindingName,
		TenantSecretsWriterRoleBindingName,
	}
	for _, name := range secretRoleBindings {
		roleBinding := &rbacv1.RoleBinding{}
		err := m.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, roleBinding)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("failed to get RoleBinding %s/%s: %w", namespace, name, err)
		}
		m.logger.Info("Deleting tenant secrets RoleBinding", "namespace", namespace, "rolebinding", name)
		if err := m.client.Delete(ctx, roleBinding); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete RoleBinding %s/%s: %w", namespace, name, err)
		}
	}

	secretRoles := []string{
		TenantSecretsReaderRoleName,
		TenantSecretsWriterRoleName,
	}
	for _, name := range secretRoles {
		role := &rbacv1.Role{}
		err := m.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, role)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("failed to get Role %s/%s: %w", namespace, name, err)
		}
		m.logger.Info("Deleting tenant secrets Role", "namespace", namespace, "role", name)
		if err := m.client.Delete(ctx, role); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete Role %s/%s: %w", namespace, name, err)
		}
	}

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
