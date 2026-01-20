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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	clusterpkg "github.com/dc-tec/openbao-operator/internal/cluster"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
)

// Manager handles the provisioning of RBAC resources for tenant namespaces.
type Manager struct {
	client             client.Client
	impersonatedClient client.Client // Client configured for impersonation
	operatorSA         OperatorServiceAccount
	delegateSA         DelegateServiceAccount
	logger             logr.Logger
}

// DelegateServiceAccount represents the delegate ServiceAccount used for impersonation.
type DelegateServiceAccount struct {
	Name      string
	Namespace string
}

// NewManager creates a new provisioner Manager.
// It creates a separate client configured for impersonation to enforce least privilege.
// restConfig is the REST config used to create the impersonated client.
func NewManager(ctx context.Context, c client.Client, restConfig *rest.Config, logger logr.Logger) (*Manager, error) {
	// Get operator namespace from environment or use default
	saNamespace := os.Getenv("OPERATOR_NAMESPACE")
	if saNamespace == "" {
		saNamespace = "openbao-operator-system"
	}

	// Discover the controller ServiceAccount name dynamically
	// The base name is "controller", which becomes "openbao-operator-controller" after kustomize prefix
	controllerSAName := "openbao-operator-controller"
	controllerSANamespace := saNamespace

	// Verify the ServiceAccount exists
	if restConfig != nil {
		controllerSA := &corev1.ServiceAccount{}
		if err := c.Get(ctx, types.NamespacedName{
			Namespace: controllerSANamespace,
			Name:      controllerSAName,
		}, controllerSA); err != nil {
			return nil, fmt.Errorf("failed to discover controller ServiceAccount %s/%s: %w", controllerSANamespace, controllerSAName, err)
		}
	}

	// Delegate ServiceAccount for impersonation
	// The base name is "provisioner-delegate", which becomes "openbao-operator-provisioner-delegate" after kustomize prefix
	delegateName := "openbao-operator-provisioner-delegate"
	delegateNamespace := saNamespace

	// Verify the delegate ServiceAccount exists
	if restConfig != nil {
		delegateSA := &corev1.ServiceAccount{}
		if err := c.Get(ctx, types.NamespacedName{
			Namespace: delegateNamespace,
			Name:      delegateName,
		}, delegateSA); err != nil {
			return nil, fmt.Errorf("failed to discover delegate ServiceAccount %s/%s: %w", delegateNamespace, delegateName, err)
		}
	}

	// Create impersonated client for RBAC operations
	// SECURITY: This client impersonates the delegate ServiceAccount, which is bound
	// to the tenant-template ClusterRole. The API server enforces that the delegate
	// can only create Roles/RoleBindings with permissions it possesses.
	var impersonatedClient client.Client
	if restConfig != nil {
		var err error
		impersonatedClient, err = createImpersonatedClient(restConfig, c.Scheme(), delegateNamespace, delegateName)
		if err != nil {
			return nil, fmt.Errorf("failed to create impersonated client: %w", err)
		}
	} else {
		// For tests with fake clients, use the base client (impersonation not supported in fake clients)
		impersonatedClient = c
	}

	return &Manager{
		client:             c,
		impersonatedClient: impersonatedClient,
		operatorSA: OperatorServiceAccount{
			Name:      controllerSAName,
			Namespace: controllerSANamespace,
		},
		delegateSA: DelegateServiceAccount{
			Name:      delegateName,
			Namespace: delegateNamespace,
		},
		logger: logger,
	}, nil
}

// createImpersonatedClient creates a controller-runtime client configured to impersonate
// the delegate ServiceAccount. This enforces least privilege by ensuring the Provisioner
// can only create Roles/RoleBindings with permissions the delegate possesses.
//
// Client-side throttling is configured to prevent overwhelming the API server during
// mass-provisioning events. The rate limits are conservative to ensure stability.
func createImpersonatedClient(baseConfig *rest.Config, scheme *runtime.Scheme, namespace, name string) (client.Client, error) {
	// Create a copy of the config to avoid modifying the original
	config := rest.CopyConfig(baseConfig)

	// Configure impersonation
	config.Impersonate = rest.ImpersonationConfig{
		UserName: fmt.Sprintf("system:serviceaccount:%s:%s", namespace, name),
	}

	// Configure client-side throttling to prevent overwhelming the API server
	// during mass-provisioning events. These values are conservative to ensure
	// stability while still allowing reasonable throughput.
	if config.QPS == 0 {
		config.QPS = 5.0 // 5 requests per second
	}
	if config.Burst == 0 {
		config.Burst = 10 // Allow bursts up to 10 requests
	}

	// Create a new client with impersonation
	impersonatedClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create impersonated client: %w", err)
	}

	return impersonatedClient, nil
}

// applyResource uses Server-Side Apply with the impersonated client.
// Unlike infra.applyResource, this does NOT set owner references since
// tenant RBAC resources should not be garbage-collected with any single cluster.
func (m *Manager) applyResource(ctx context.Context, obj client.Object) error {
	patchOpts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner("openbao-provisioner"),
	}

	if err := m.impersonatedClient.Patch(ctx, obj, client.Apply, patchOpts...); err != nil {
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
	m.logger.Info("Applying tenant Role", "namespace", namespace, "role", TenantRoleName,
		"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
	if err := m.applyResource(ctx, role); err != nil {
		return fmt.Errorf("failed to apply tenant Role %s/%s: %w", namespace, TenantRoleName, err)
	}

	// Apply RoleBinding using Server-Side Apply
	roleBinding := GenerateTenantRoleBinding(namespace, m.operatorSA)
	m.logger.Info("Applying tenant RoleBinding", "namespace", namespace, "rolebinding", TenantRoleBindingName,
		"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
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
// This creates/updates two Roles in the tenant namespace (via delegate impersonation):
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
		if err := m.impersonatedClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: roleName}, existing); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get Role %s/%s: %w", namespace, roleName, err)
		}
		m.logger.Info("Deleting tenant secrets Role (no clusters reference Secrets)", "namespace", namespace, "role", roleName)
		if err := m.impersonatedClient.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
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
	m.logger.Info("Applying tenant secrets Role", "namespace", namespace, "role", roleName,
		"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
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
		if err := m.impersonatedClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: roleBindingName}, existing); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get RoleBinding %s/%s: %w", namespace, roleBindingName, err)
		}
		m.logger.Info("Deleting tenant secrets RoleBinding (no clusters reference Secrets)", "namespace", namespace, "rolebinding", roleBindingName)
		if err := m.impersonatedClient.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
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
	m.logger.Info("Applying tenant secrets RoleBinding", "namespace", namespace, "rolebinding", roleBindingName,
		"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
	if err := m.applyResource(ctx, roleBinding); err != nil {
		return fmt.Errorf("failed to apply secrets RoleBinding %s/%s: %w", namespace, roleBindingName, err)
	}

	return nil
}

// IsTenantNamespaceProvisioned returns true if the tenant namespace has been provisioned
// (i.e., the core tenant RoleBinding exists).
//
// SECURITY: This uses the impersonated (delegate) client to avoid requiring the Provisioner
// ServiceAccount to list/watch/get RBAC resources cluster-wide via the controller-runtime cache.
func (m *Manager) IsTenantNamespaceProvisioned(ctx context.Context, namespace string) (bool, error) {
	if namespace == "" {
		return false, fmt.Errorf("namespace is required")
	}

	existing := &rbacv1.RoleBinding{}
	if err := m.impersonatedClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: TenantRoleBindingName}, existing); err != nil {
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
		err := m.impersonatedClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, roleBinding)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("failed to get RoleBinding %s/%s: %w", namespace, name, err)
		}
		m.logger.Info("Deleting tenant secrets RoleBinding", "namespace", namespace, "rolebinding", name)
		if err := m.impersonatedClient.Delete(ctx, roleBinding); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete RoleBinding %s/%s: %w", namespace, name, err)
		}
	}

	secretRoles := []string{
		TenantSecretsReaderRoleName,
		TenantSecretsWriterRoleName,
	}
	for _, name := range secretRoles {
		role := &rbacv1.Role{}
		err := m.impersonatedClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, role)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("failed to get Role %s/%s: %w", namespace, name, err)
		}
		m.logger.Info("Deleting tenant secrets Role", "namespace", namespace, "role", name)
		if err := m.impersonatedClient.Delete(ctx, role); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete Role %s/%s: %w", namespace, name, err)
		}
	}

	// Delete RoleBinding first
	roleBinding := &rbacv1.RoleBinding{}
	err := m.impersonatedClient.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      TenantRoleBindingName,
	}, roleBinding)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get RoleBinding %s/%s: %w", namespace, TenantRoleBindingName, err)
		}
	} else {
		m.logger.Info("Deleting tenant RoleBinding", "namespace", namespace, "rolebinding", TenantRoleBindingName)
		if err := m.impersonatedClient.Delete(ctx, roleBinding); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete RoleBinding %s/%s: %w", namespace, TenantRoleBindingName, err)
		}
	}

	// Delete Role
	role := &rbacv1.Role{}
	err = m.impersonatedClient.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      TenantRoleName,
	}, role)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get Role %s/%s: %w", namespace, TenantRoleName, err)
		}
	} else {
		m.logger.Info("Deleting tenant Role", "namespace", namespace, "role", TenantRoleName)
		if err := m.impersonatedClient.Delete(ctx, role); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete Role %s/%s: %w", namespace, TenantRoleName, err)
		}
	}

	return nil
}
