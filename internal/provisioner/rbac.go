package provisioner

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openbao/operator/internal/constants"
)

const (
	// TenantLabelKey is the label key used to identify tenant namespaces.
	TenantLabelKey = "openbao.org/tenant"
	// TenantLabelValue is the label value that marks a namespace as a tenant.
	TenantLabelValue = "true"
	// TenantRoleName is the name of the Role created in each tenant namespace.
	TenantRoleName = "openbao-operator-tenant-role"
	// TenantRoleBindingName is the name of the RoleBinding created in each tenant namespace.
	TenantRoleBindingName = "openbao-operator-tenant-rolebinding"
)

// OperatorServiceAccount represents the operator's ServiceAccount identity.
type OperatorServiceAccount struct {
	// Name is the name of the ServiceAccount (e.g., "controller-manager").
	Name string
	// Namespace is the namespace where the ServiceAccount exists (e.g., "openbao-operator-system").
	Namespace string
}

// GenerateTenantRole generates a namespaced Role that grants the operator
// the permissions needed to manage OpenBaoCluster resources in a tenant namespace.
func GenerateTenantRole(namespace string) *rbacv1.Role {
	// Common set of verbs for managing resources (excludes "deletecollection" for safety)
	commonVerbs := []string{"create", "delete", "get", "list", "patch", "update", "watch"}

	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TenantRoleName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: "provisioner",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
		Rules: []rbacv1.PolicyRule{
			// 1. Manage OpenBao Clusters and Restores
			{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters", "openbaoclusters/status", "openbaoclusters/finalizers"},
				Verbs:     commonVerbs, // Changed from "*"
			},
			{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaorestores", "openbaorestores/status", "openbaorestores/finalizers"},
				Verbs:     commonVerbs,
			},
			// 2. Manage Workload Infrastructure
			{
				APIGroups: []string{"apps"},
				Resources: []string{"statefulsets"},
				Verbs:     commonVerbs, // Changed from "*"
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
				Verbs:     commonVerbs, // For Sentinel Deployment management
			},
			{
				APIGroups: []string{""},
				Resources: []string{"services", "configmaps", "serviceaccounts"},
				Verbs:     commonVerbs, // Changed from "*"
			},
			// 3. Limited Secret Access (Hardened)
			// Removed "list" to prevent secret enumeration/scraping.
			// "watch" is included to allow Sentinel drift detection and doesn't enable enumeration.
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"create", "delete", "get", "patch", "update", "watch"},
			},
			// 4. Pod access for health checks, leader detection, and cleanup
			// The operator needs delete permission to clean up pods during OpenBaoCluster deletion,
			// especially when pods become orphaned after StatefulSet deletion is blocked by the webhook.
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch", "update", "patch", "delete"},
			},
			// 5. PVC access for StatefulSets
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumeclaims"},
				Verbs:     commonVerbs,
			},
			// 6. Jobs for backups
			{
				APIGroups: []string{"batch"},
				Resources: []string{"jobs"},
				Verbs:     commonVerbs, // Changed from "*"
			},
			// 7. Networking resources
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses", "networkpolicies"},
				Verbs:     commonVerbs, // Changed from "*"
			},
			// 8. Gateway API
			{
				APIGroups: []string{"gateway.networking.k8s.io"},
				Resources: []string{"httproutes", "tlsroutes", "backendtlspolicies"},
				Verbs:     commonVerbs, // Changed from "*"
			},
			// 9. RBAC for OpenBao pod discovery
			{
				APIGroups: []string{"rbac.authorization.k8s.io"},
				Resources: []string{"roles", "rolebindings"},
				Verbs:     commonVerbs,
			},
			// 10. Endpoints for service discovery
			{
				APIGroups: []string{""},
				Resources: []string{"endpoints"},
				Verbs:     []string{"get", "list", "watch"},
			},
			// 11. EndpointSlices for service discovery (Required for modern K8s)
			{
				APIGroups: []string{"discovery.k8s.io"},
				Resources: []string{"endpointslices"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}
}

// GenerateTenantRoleBinding generates a namespaced RoleBinding that binds
// the operator's ServiceAccount to the tenant Role.
func GenerateTenantRoleBinding(namespace string, operatorSA OperatorServiceAccount) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TenantRoleBindingName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: "provisioner",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     TenantRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      operatorSA.Name,
				Namespace: operatorSA.Namespace,
			},
		},
	}
}

// GenerateSentinelRole generates the read-only + trigger role for the Sentinel.
// Follows least-privilege principles:
// - Read-only access to resources Sentinel watches (StatefulSets, Services, ConfigMaps)
// - No access to Secrets (Sentinel does not detect drift in unseal keys or root tokens)
// - No access to Pods or Ingresses (not used by Sentinel)
// - Minimal write access: only "patch" on OpenBaoCluster to add trigger annotation
// - list/watch on OpenBaoCluster required for controller-runtime cache to function
func GenerateSentinelRole(namespace string) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.SentinelRoleName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: "sentinel",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
		Rules: []rbacv1.PolicyRule{
			// Read-Only Access to StatefulSets (apps)
			{
				APIGroups: []string{"apps"},
				Resources: []string{"statefulsets"},
				Verbs:     []string{"get", "list", "watch"},
			},
			// Read-Only Access to Services and ConfigMaps (core)
			{
				APIGroups: []string{""},
				Resources: []string{"services", "configmaps"},
				Verbs:     []string{"get", "list", "watch"},
			},
			// Trigger Access (Limited by Admission Policy)
			// Sentinel needs list/watch for controller-runtime cache to function
			{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters"},
				Verbs:     []string{"get", "list", "patch", "watch"},
			},
		},
	}
}

// GenerateSentinelRoleBinding generates a RoleBinding for the Sentinel ServiceAccount.
func GenerateSentinelRoleBinding(namespace string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.SentinelRoleBindingName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: "sentinel",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     constants.SentinelRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      constants.SentinelServiceAccountName,
				Namespace: namespace,
			},
		},
	}
}
