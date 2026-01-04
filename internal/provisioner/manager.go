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
	"github.com/dc-tec/openbao-operator/internal/constants"
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
func NewManager(c client.Client, restConfig *rest.Config, logger logr.Logger) (*Manager, error) {
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
		if err := c.Get(context.Background(), types.NamespacedName{
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
		if err := c.Get(context.Background(), types.NamespacedName{
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

// EnsureTenantRBAC ensures that a Role and RoleBinding exist in the given namespace
// for the operator to manage OpenBaoCluster resources.
//
//nolint:gocyclo // Explicit validation and update paths improve auditability for security-sensitive RBAC.
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

		m.logger.Info("Creating tenant Role", "namespace", namespace, "role", TenantRoleName,
			"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
		// SECURITY: Use impersonated client to enforce least privilege
		// The API server will check if the delegate has permission to create this Role
		if err := m.impersonatedClient.Create(ctx, role); err != nil {
			// Wrap transient Kubernetes API errors
			if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
				return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to create Role %s/%s (via impersonation): %w", namespace, TenantRoleName, err))
			}
			return fmt.Errorf("failed to create Role %s/%s (via impersonation): %w", namespace, TenantRoleName, err)
		}
	} else {
		// Update Role if rules have changed
		if !rolesEqual(existingRole.Rules, role.Rules) {
			m.logger.Info("Updating tenant Role", "namespace", namespace, "role", TenantRoleName,
				"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
			existingRole.Rules = role.Rules
			// Preserve existing labels and merge with new ones
			if existingRole.Labels == nil {
				existingRole.Labels = make(map[string]string)
			}
			for k, v := range role.Labels {
				existingRole.Labels[k] = v
			}
			// SECURITY: Use impersonated client to enforce least privilege
			if err := m.impersonatedClient.Update(ctx, existingRole); err != nil {
				// Wrap transient Kubernetes API errors
				if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
					return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to update Role %s/%s (via impersonation): %w", namespace, TenantRoleName, err))
				}
				return fmt.Errorf("failed to update Role %s/%s (via impersonation): %w", namespace, TenantRoleName, err)
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

		m.logger.Info("Creating tenant RoleBinding", "namespace", namespace, "rolebinding", TenantRoleBindingName,
			"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
		// SECURITY: Use impersonated client to enforce least privilege
		if err := m.impersonatedClient.Create(ctx, roleBinding); err != nil {
			// Wrap transient Kubernetes API errors
			if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
				return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to create RoleBinding %s/%s (via impersonation): %w", namespace, TenantRoleBindingName, err))
			}
			return fmt.Errorf("failed to create RoleBinding %s/%s (via impersonation): %w", namespace, TenantRoleBindingName, err)
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
			m.logger.Info("Updating tenant RoleBinding", "namespace", namespace, "rolebinding", TenantRoleBindingName,
				"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
			// SECURITY: Use impersonated client to enforce least privilege
			if err := m.impersonatedClient.Update(ctx, existingRoleBinding); err != nil {
				// Wrap transient Kubernetes API errors
				if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
					return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to update RoleBinding %s/%s (via impersonation): %w", namespace, TenantRoleBindingName, err))
				}
				if apierrors.IsForbidden(err) {
					return fmt.Errorf("failed to update RoleBinding %s/%s (via impersonation): %w. "+
						"Ensure the delegate ServiceAccount %s/%s exists and is bound to the tenant-template ClusterRole "+
						"via config/rbac/provisioner_delegate_clusterrolebinding.yaml", namespace, TenantRoleBindingName, err,
						m.delegateSA.Namespace, m.delegateSA.Name)
				}
				return fmt.Errorf("failed to update RoleBinding %s/%s (via impersonation): %w", namespace, TenantRoleBindingName, err)
			}
		}
	}

	// Apply Pod Security Standards labels to namespace
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

	if err := m.ensureSecretsRole(ctx, namespace, TenantSecretsWriterRoleName, writerList, GenerateTenantSecretsWriterRole); err != nil {
		return err
	}
	if err := m.ensureSecretsRoleBinding(ctx, namespace, TenantSecretsWriterRoleBindingName, TenantSecretsWriterRoleName, len(writerList) > 0, GenerateTenantSecretsWriterRoleBinding); err != nil {
		return err
	}

	if err := m.ensureSecretsRole(ctx, namespace, TenantSecretsReaderRoleName, readerList, GenerateTenantSecretsReaderRole); err != nil {
		return err
	}
	if err := m.ensureSecretsRoleBinding(ctx, namespace, TenantSecretsReaderRoleBindingName, TenantSecretsReaderRoleName, len(readerList) > 0, GenerateTenantSecretsReaderRoleBinding); err != nil {
		return err
	}

	return nil
}

func accumulateTenantSecretNames(cluster *openbaov1alpha1.OpenBaoCluster, writer, reader map[string]struct{}) {
	if cluster == nil || cluster.Name == "" {
		return
	}

	add := func(set map[string]struct{}, name string) {
		if name == "" {
			return
		}
		set[name] = struct{}{}
	}

	// Cluster-owned Secrets.
	tlsCA := cluster.Name + constants.SuffixTLSCA
	tlsServer := cluster.Name + constants.SuffixTLSServer

	// Only OperatorManaged TLS implies the operator must write the TLS Secrets.
	// External/ACME are read-only from the operator's perspective.
	if cluster.Spec.TLS.Mode == "" || cluster.Spec.TLS.Mode == openbaov1alpha1.TLSModeOperatorManaged {
		add(writer, tlsCA)
		add(writer, tlsServer)
	} else {
		add(reader, tlsCA)
		add(reader, tlsServer)
	}

	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		add(writer, cluster.Name+constants.SuffixRootToken)
	}

	if isStaticUnseal(cluster) {
		add(writer, cluster.Name+constants.SuffixUnsealKey)
	}

	// Referenced Secrets in the OpenBaoCluster spec (read-only).
	if cluster.Spec.Unseal != nil && cluster.Spec.Unseal.CredentialsSecretRef != nil {
		add(reader, cluster.Spec.Unseal.CredentialsSecretRef.Name)
	}

	if cluster.Spec.Backup != nil {
		if cluster.Spec.Backup.Target.CredentialsSecretRef != nil {
			add(reader, cluster.Spec.Backup.Target.CredentialsSecretRef.Name)
		}
		if cluster.Spec.Backup.TokenSecretRef != nil {
			add(reader, cluster.Spec.Backup.TokenSecretRef.Name)
		}
	}

	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.TokenSecretRef != nil {
		add(reader, cluster.Spec.Upgrade.TokenSecretRef.Name)
	}
}

func isStaticUnseal(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Spec.Unseal == nil {
		return true
	}
	if cluster.Spec.Unseal.Type == "" {
		return true
	}
	return cluster.Spec.Unseal.Type == "static"
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

	role := factory(namespace, secretNames)
	existing := &rbacv1.Role{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      roleName,
	}, existing)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get Role %s/%s: %w", namespace, roleName, err)
		}

		m.logger.Info("Creating tenant secrets Role", "namespace", namespace, "role", roleName,
			"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
		if err := m.impersonatedClient.Create(ctx, role); err != nil {
			if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
				return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to create Role %s/%s (via impersonation): %w", namespace, roleName, err))
			}
			return fmt.Errorf("failed to create Role %s/%s (via impersonation): %w", namespace, roleName, err)
		}
		return nil
	}

	if !rolesEqual(existing.Rules, role.Rules) {
		existing.Rules = role.Rules
		if existing.Labels == nil {
			existing.Labels = make(map[string]string)
		}
		for k, v := range role.Labels {
			existing.Labels[k] = v
		}
		m.logger.Info("Updating tenant secrets Role", "namespace", namespace, "role", roleName,
			"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
		if err := m.impersonatedClient.Update(ctx, existing); err != nil {
			if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
				return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to update Role %s/%s (via impersonation): %w", namespace, roleName, err))
			}
			return fmt.Errorf("failed to update Role %s/%s (via impersonation): %w", namespace, roleName, err)
		}
	}

	return nil
}

type secretsRoleBindingFactory func(namespace string, operatorSA OperatorServiceAccount) *rbacv1.RoleBinding

func (m *Manager) ensureSecretsRoleBinding(ctx context.Context, namespace string, roleBindingName string, roleName string, shouldExist bool, factory secretsRoleBindingFactory) error {
	if roleBindingName == "" || roleName == "" {
		return fmt.Errorf("role binding name and role name are required")
	}

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

	roleBinding := factory(namespace, m.operatorSA)
	existing := &rbacv1.RoleBinding{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      roleBindingName,
	}, existing)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get RoleBinding %s/%s: %w", namespace, roleBindingName, err)
		}

		m.logger.Info("Creating tenant secrets RoleBinding", "namespace", namespace, "rolebinding", roleBindingName,
			"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
		if err := m.impersonatedClient.Create(ctx, roleBinding); err != nil {
			if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
				return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to create RoleBinding %s/%s (via impersonation): %w", namespace, roleBindingName, err))
			}
			return fmt.Errorf("failed to create RoleBinding %s/%s (via impersonation): %w", namespace, roleBindingName, err)
		}
		return nil
	}

	needsUpdate := false
	if existing.RoleRef.Name != roleBinding.RoleRef.Name ||
		existing.RoleRef.Kind != roleBinding.RoleRef.Kind ||
		existing.RoleRef.APIGroup != roleBinding.RoleRef.APIGroup {
		existing.RoleRef = roleBinding.RoleRef
		needsUpdate = true
	}

	if len(existing.Subjects) != len(roleBinding.Subjects) {
		existing.Subjects = roleBinding.Subjects
		needsUpdate = true
	} else if len(roleBinding.Subjects) > 0 {
		existingSubject := existing.Subjects[0]
		desiredSubject := roleBinding.Subjects[0]
		if existingSubject.Kind != desiredSubject.Kind ||
			existingSubject.Name != desiredSubject.Name ||
			existingSubject.Namespace != desiredSubject.Namespace {
			existing.Subjects = roleBinding.Subjects
			needsUpdate = true
		}
	}

	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	for k, v := range roleBinding.Labels {
		if existing.Labels[k] != v {
			existing.Labels[k] = v
			needsUpdate = true
		}
	}

	if needsUpdate {
		m.logger.Info("Updating tenant secrets RoleBinding", "namespace", namespace, "rolebinding", roleBindingName,
			"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
		if err := m.impersonatedClient.Update(ctx, existing); err != nil {
			if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
				return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to update RoleBinding %s/%s (via impersonation): %w", namespace, roleBindingName, err))
			}
			return fmt.Errorf("failed to update RoleBinding %s/%s (via impersonation): %w", namespace, roleBindingName, err)
		}
	}

	return nil
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

	if len(a.ResourceNames) != len(b.ResourceNames) {
		return false
	}
	for i := range a.ResourceNames {
		if a.ResourceNames[i] != b.ResourceNames[i] {
			return false
		}
	}

	if len(a.NonResourceURLs) != len(b.NonResourceURLs) {
		return false
	}
	for i := range a.NonResourceURLs {
		if a.NonResourceURLs[i] != b.NonResourceURLs[i] {
			return false
		}
	}

	return true
}
