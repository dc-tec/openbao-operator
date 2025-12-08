package provisioner

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TenantRoleName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openbao-operator",
				"app.kubernetes.io/component":  "provisioner",
				"app.kubernetes.io/managed-by": "openbao-operator",
			},
		},
		Rules: []rbacv1.PolicyRule{
			// 1. Manage OpenBao Clusters
			{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters", "openbaoclusters/status", "openbaoclusters/finalizers"},
				Verbs:     []string{"*"},
			},
			// 2. Manage Workload Infrastructure
			{
				APIGroups: []string{"apps"},
				Resources: []string{"statefulsets"},
				Verbs:     []string{"*"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"services", "configmaps", "serviceaccounts"},
				Verbs:     []string{"*"},
			},
			// 3. Limited Secret Access
			// General access for user-managed secrets (e.g., backup creds)
			// Note: We cannot restrict to specific resourceNames for unseal keys because
			// Kubernetes RBAC doesn't support write-only permissions. The "blind create"
			// pattern in ensureUnsealSecret eliminates the need for GET permission.
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			// 4. Pod access for health checks and leader detection
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch", "update", "patch"},
			},
			// 5. PVC access for StatefulSets
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumeclaims"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			// 6. Jobs for backups
			{
				APIGroups: []string{"batch"},
				Resources: []string{"jobs"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			// 7. Networking resources
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses", "networkpolicies"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			// 8. Gateway API
			{
				APIGroups: []string{"gateway.networking.k8s.io"},
				Resources: []string{"httproutes", "tlsroutes", "backendtlspolicies"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			// 9. RBAC for OpenBao pod discovery
			{
				APIGroups: []string{"rbac.authorization.k8s.io"},
				Resources: []string{"roles", "rolebindings"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			// 10. Endpoints for service discovery
			{
				APIGroups: []string{""},
				Resources: []string{"endpoints"},
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
				"app.kubernetes.io/name":       "openbao-operator",
				"app.kubernetes.io/component":  "provisioner",
				"app.kubernetes.io/managed-by": "openbao-operator",
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
