package provisioner

import (
	"context"
	"fmt"
	"sort"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	clusterpkg "github.com/dc-tec/openbao-operator/internal/adapter/cluster"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

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

	restoreList := &openbaov1alpha1.OpenBaoRestoreList{}
	if err := m.client.List(ctx, restoreList, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list OpenBaoRestores in namespace %s: %w", namespace, err)
	}

	writerNames := map[string]struct{}{}
	readerNames := map[string]struct{}{}
	clustersByName := map[string]*openbaov1alpha1.OpenBaoCluster{}
	for i := range clusterList.Items {
		accumulateTenantSecretNames(&clusterList.Items[i], writerNames, readerNames)
		clustersByName[clusterList.Items[i].Name] = &clusterList.Items[i]
	}
	for i := range restoreList.Items {
		accumulateRestoreTenantSecretNames(&restoreList.Items[i], clustersByName, readerNames)
	}
	if err := m.accumulateClaimSourceSecretNames(ctx, namespace, readerNames); err != nil {
		return err
	}

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
			secretNames:        sortedUniqueSecretNames(writerNames),
			roleFactory:        GenerateTenantSecretsWriterRole,
			roleBindingFactory: GenerateTenantSecretsWriterRoleBinding,
		},
		{
			roleName:           TenantSecretsReaderRoleName,
			roleBindingName:    TenantSecretsReaderRoleBindingName,
			secretNames:        sortedUniqueSecretNames(readerNames),
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
			continue
		}
		reader[perm.Name] = struct{}{}
	}
	accumulateRetentionSecretNames(cluster, writer)
}

func accumulateRetentionSecretNames(cluster *openbaov1alpha1.OpenBaoCluster, writer map[string]struct{}) {
	if cluster == nil || cluster.Name == "" || writer == nil {
		return
	}
	// Retain deletion probes generated recovery Secret candidates before removing
	// the cluster finalizer. Keep these exact names authorized even when a profile
	// does not create them, so Kubernetes can return NotFound instead of Forbidden.
	writer[cluster.Name+constants.SuffixUnsealKey] = struct{}{}
	writer[cluster.Name+constants.SuffixRootToken] = struct{}{}
}

func accumulateRestoreTenantSecretNames(restore *openbaov1alpha1.OpenBaoRestore, clustersByName map[string]*openbaov1alpha1.OpenBaoCluster, reader map[string]struct{}) {
	if restore == nil {
		return
	}
	if ref := restore.Spec.Source.Target.CredentialsSecretRef; ref != nil {
		reader[ref.Name] = struct{}{}
	}
	if restoreUsesStaticTokenAuth(restore, clustersByName[restore.Spec.Cluster]) {
		reader[restore.Spec.TokenSecretRef.Name] = struct{}{}
	}
}

func restoreUsesStaticTokenAuth(restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if restore == nil || restore.Spec.TokenSecretRef == nil || restore.Spec.TokenSecretRef.Name == "" {
		return false
	}

	return portauth.EffectiveJWTRole(
		restore.Spec.JWTAuthRole,
		cluster != nil && portauth.OperatorJWTBootstrapEnabled(cluster),
		portauth.RoleNameRestore,
	) == ""
}

func (m *Manager) accumulateClaimSourceSecretNames(ctx context.Context, targetNamespace string, reader map[string]struct{}) error {
	for _, claimNamespace := range m.claimSearchNamespaces(targetNamespace) {
		claims := &openbaov1alpha1.OpenBaoClusterClaimList{}
		if err := m.client.List(ctx, claims, client.InNamespace(claimNamespace)); err != nil {
			if meta.IsNoMatchError(err) {
				m.logger.Info(
					"Skipping tenant claim secret RBAC sync because claim APIs are unavailable",
					"namespace", targetNamespace,
					"claimNamespace", claimNamespace,
				)
				return nil
			}
			return fmt.Errorf("failed to list OpenBaoClusterClaims in namespace %s while syncing tenant secret RBAC for namespace %s: %w", claimNamespace, targetNamespace, err)
		}

		for i := range claims.Items {
			claim := &claims.Items[i]
			if claim.DeletionTimestamp != nil {
				continue
			}

			target, ok, err := m.claimTargetNamespace(ctx, claim)
			if err != nil {
				return err
			}
			if !ok || target != targetNamespace {
				continue
			}

			if err := m.accumulateClaimBootstrapSourceSecretNames(ctx, claim, targetNamespace, reader); err != nil {
				return err
			}
			accumulateClaimProjectedBootstrapSecretNames(claim, reader)
		}
	}

	return nil
}

func (m *Manager) claimSearchNamespaces(targetNamespace string) []string {
	namespaces := make([]string, 0, 2)
	if operatorNamespace := m.operatorSA.Namespace; operatorNamespace != "" {
		namespaces = append(namespaces, operatorNamespace)
	}
	if targetNamespace != "" && targetNamespace != m.operatorSA.Namespace {
		namespaces = append(namespaces, targetNamespace)
	}
	return namespaces
}

func (m *Manager) claimTargetNamespace(ctx context.Context, claim *openbaov1alpha1.OpenBaoClusterClaim) (string, bool, error) {
	if claim == nil || claim.Spec.TenantRef.Name == "" {
		return "", false, nil
	}

	tenant := &openbaov1alpha1.OpenBaoTenant{}
	key := types.NamespacedName{Namespace: claim.Namespace, Name: claim.Spec.TenantRef.Name}
	if err := m.client.Get(ctx, key, tenant); err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("get OpenBaoTenant %s/%s while syncing tenant secret RBAC: %w", key.Namespace, key.Name, err)
	}
	if tenant.Spec.TargetNamespace == "" {
		return "", false, nil
	}
	return tenant.Spec.TargetNamespace, true, nil
}

func (m *Manager) accumulateClaimBootstrapSourceSecretNames(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	targetNamespace string,
	reader map[string]struct{},
) error {
	serviceProfileName, ok, err := m.claimServiceProfileName(ctx, claim)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	serviceProfile := &openbaov1alpha1.OpenBaoServiceProfile{}
	if err := m.client.Get(ctx, types.NamespacedName{Name: serviceProfileName}, serviceProfile); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get OpenBaoServiceProfile %s while syncing tenant secret RBAC: %w", serviceProfileName, err)
	}
	if serviceProfile.Spec.Bootstrap.Mode != openbaov1alpha1.OpenBaoBootstrapModeSelfInit || serviceProfile.Spec.Bootstrap.ProfileRef == nil || serviceProfile.Spec.Bootstrap.ProfileRef.Name == "" {
		return nil
	}

	bootstrapProfile := &openbaov1alpha1.OpenBaoBootstrapProfile{}
	if err := m.client.Get(ctx, types.NamespacedName{Name: serviceProfile.Spec.Bootstrap.ProfileRef.Name}, bootstrapProfile); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get OpenBaoBootstrapProfile %s while syncing tenant secret RBAC: %w", serviceProfile.Spec.Bootstrap.ProfileRef.Name, err)
	}

	if bootstrapProfile.Spec.Auth != nil {
		for _, method := range bootstrapProfile.Spec.Auth.Methods {
			if name, ok := sameClusterSourceSecretName(targetNamespace, method.ConfigRef); ok {
				reader[name] = struct{}{}
			}
		}
	}
	if bootstrapProfile.Spec.Policies != nil {
		for _, bundle := range bootstrapProfile.Spec.Policies.Bundles {
			if name, ok := sameClusterSourceSecretName(targetNamespace, &bundle.ContentRef); ok {
				reader[name] = struct{}{}
			}
		}
	}
	if bootstrapProfile.Spec.Audit != nil {
		for _, device := range bootstrapProfile.Spec.Audit.Devices {
			if name, ok := sameClusterSourceSecretName(targetNamespace, device.SinkRef); ok {
				reader[name] = struct{}{}
			}
		}
	}

	return nil
}

func (m *Manager) claimServiceProfileName(ctx context.Context, claim *openbaov1alpha1.OpenBaoClusterClaim) (string, bool, error) {
	if claim == nil {
		return "", false, nil
	}
	if claim.Spec.ServiceProfileRef.Name != "" {
		return claim.Spec.ServiceProfileRef.Name, true, nil
	}
	if claim.Spec.ServiceOfferingRef == nil || claim.Spec.ServiceOfferingRef.Name == "" {
		return "", false, nil
	}

	offering := &openbaov1alpha1.OpenBaoServiceOffering{}
	if err := m.client.Get(ctx, types.NamespacedName{Name: claim.Spec.ServiceOfferingRef.Name}, offering); err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("get OpenBaoServiceOffering %s while syncing tenant secret RBAC: %w", claim.Spec.ServiceOfferingRef.Name, err)
	}
	if offering.Spec.CurrentRevisionRef.Name == "" {
		return "", false, nil
	}
	return offering.Spec.CurrentRevisionRef.Name, true, nil
}

func sameClusterSourceSecretName(targetNamespace string, ref *openbaov1alpha1.TypedObjectReference) (string, bool) {
	if ref == nil || ref.Kind != "Secret" || ref.Name == "" {
		return "", false
	}
	if ref.Namespace != "" && ref.Namespace != targetNamespace {
		return "", false
	}
	return ref.Name, true
}

func accumulateClaimProjectedBootstrapSecretNames(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	reader map[string]struct{},
) {
	if claim == nil || claim.Status.Applied.RenderedDependencies == nil {
		return
	}
	for _, ref := range claim.Status.Applied.RenderedDependencies.BootstrapProjectionRefs {
		if ref.Kind != "Secret" || ref.Name == "" {
			continue
		}
		reader[ref.Name] = struct{}{}
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

	if len(secretNames) == 0 {
		existing := &rbacv1.Role{}
		if err := m.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: roleName}, existing); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get Role %s/%s: %w", namespace, roleName, err)
		}
		m.logger.Info("Deleting tenant secrets Role (no tenant resources reference Secrets)", "namespace", namespace, "role", roleName)
		if err := m.client.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete Role %s/%s: %w", namespace, roleName, err)
		}
		return nil
	}

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

	if !shouldExist {
		existing := &rbacv1.RoleBinding{}
		if err := m.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: roleBindingName}, existing); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get RoleBinding %s/%s: %w", namespace, roleBindingName, err)
		}
		m.logger.Info("Deleting tenant secrets RoleBinding (no tenant resources reference Secrets)", "namespace", namespace, "rolebinding", roleBindingName)
		if err := m.client.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete RoleBinding %s/%s: %w", namespace, roleBindingName, err)
		}
		return nil
	}

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
