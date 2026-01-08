package provisioner

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dc-tec/openbao-operator/internal/constants"
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

	// TenantSecretsReaderRoleName grants read-only access to explicitly enumerated Secret names.
	// This is used to reduce the blast radius of Secret access in tenant namespaces.
	TenantSecretsReaderRoleName = "openbao-operator-tenant-secrets-reader"
	// TenantSecretsReaderRoleBindingName binds TenantSecretsReaderRoleName to the controller ServiceAccount.
	TenantSecretsReaderRoleBindingName = "openbao-operator-tenant-secrets-reader-rolebinding"
	// TenantSecretsWriterRoleName grants write access to explicitly enumerated Secret names owned by the operator.
	TenantSecretsWriterRoleName = "openbao-operator-tenant-secrets-writer"
	// TenantSecretsWriterRoleBindingName binds TenantSecretsWriterRoleName to the controller ServiceAccount.
	TenantSecretsWriterRoleBindingName = "openbao-operator-tenant-secrets-writer-rolebinding"
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

	rules := newPolicyRulesBuilder().
		// 1. Manage OpenBao Clusters and Restores
		Group("openbao.org").
		Resources("openbaoclusters", "openbaoclusters/status", "openbaoclusters/finalizers").
		Verbs(commonVerbs...).
		Group("openbao.org").
		Resources("openbaorestores", "openbaorestores/status", "openbaorestores/finalizers").
		Verbs(commonVerbs...).
		// 2. Manage Workload Infrastructure
		Group("apps").
		Resources("statefulsets").
		Verbs(commonVerbs...).
		Group("apps").
		Resources("deployments").
		Verbs(commonVerbs...).
		Group("").
		Resources("services", "configmaps", "serviceaccounts").
		Verbs(commonVerbs...).
		// Events for operator visibility (kubectl describe, auditing, troubleshooting).
		// Events are namespaced and do not expand access to tenant resources.
		Group("").
		Resources("events").
		Verbs("create", "patch").
		// 3. Pod access for health checks, leader detection, and cleanup
		// The operator needs delete permission to clean up pods during OpenBaoCluster deletion,
		// especially when pods become orphaned after StatefulSet deletion is blocked by the webhook.
		Group("").
		Resources("pods").
		Verbs("get", "list", "watch", "update", "patch", "delete").
		// 4. PVC access for StatefulSets
		Group("").
		Resources("persistentvolumeclaims").
		Verbs(commonVerbs...).
		// 5. Jobs for backups
		Group("batch").
		Resources("jobs").
		Verbs(commonVerbs...).
		// 6. PodDisruptionBudgets for StatefulSet protection
		Group("policy").
		Resources("poddisruptionbudgets").
		Verbs(commonVerbs...).
		// 7. Networking resources
		Group("networking.k8s.io").
		Resources("ingresses", "networkpolicies").
		Verbs(commonVerbs...).
		// 8. Gateway API
		Group("gateway.networking.k8s.io").
		Resources("httproutes", "tlsroutes", "backendtlspolicies").
		Verbs(commonVerbs...).
		// 9. RBAC for OpenBao pod discovery
		Group("rbac.authorization.k8s.io").
		Resources("roles", "rolebindings").
		Verbs(commonVerbs...).
		// 10. Endpoints for service discovery
		Group("").
		Resources("endpoints").
		Verbs("get", "list", "watch").
		// 11. EndpointSlices for service discovery (Required for modern K8s)
		Group("discovery.k8s.io").
		Resources("endpointslices").
		Verbs("get", "list", "watch").
		Rules()

	return &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      TenantRoleName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: constants.LabelValueAppComponentProvisioner,
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
		Rules: rules,
	}
}

// GenerateTenantRoleBinding generates a namespaced RoleBinding that binds
// the operator's ServiceAccount to the tenant Role.
func GenerateTenantRoleBinding(namespace string, operatorSA OperatorServiceAccount) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
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

// GenerateTenantSecretsReaderRole generates a namespaced Role that grants read-only access to a
// specific set of Secret names. This is used to avoid granting broad Secret access in tenant namespaces.
func GenerateTenantSecretsReaderRole(namespace string, secretNames []string) *rbacv1.Role {
	rules := []rbacv1.PolicyRule{
		{
			APIGroups:     []string{""},
			Resources:     []string{"secrets"},
			Verbs:         []string{"get"},
			ResourceNames: secretNames,
		},
	}

	return &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      TenantSecretsReaderRoleName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: "provisioner",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
		Rules: rules,
	}
}

// GenerateTenantSecretsReaderRoleBinding generates a namespaced RoleBinding that binds the
// operator's controller ServiceAccount to the secrets reader Role.
func GenerateTenantSecretsReaderRoleBinding(namespace string, operatorSA OperatorServiceAccount) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      TenantSecretsReaderRoleBindingName,
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
			Name:     TenantSecretsReaderRoleName,
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

// GenerateTenantSecretsWriterRole generates a namespaced Role that grants write access to a
// specific set of Secret names owned by the operator.
func GenerateTenantSecretsWriterRole(namespace string, secretNames []string) *rbacv1.Role {
	rules := []rbacv1.PolicyRule{}
	if len(secretNames) > 0 {
		// RBAC cannot scope "create" by resourceNames (create is against the collection),
		// so we split create from the name-scoped mutation verbs.
		rules = append(rules,
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"create"},
			},
			rbacv1.PolicyRule{
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				Verbs:         []string{"delete", "get", "patch", "update"},
				ResourceNames: secretNames,
			},
		)
	}

	return &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      TenantSecretsWriterRoleName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: "provisioner",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
		Rules: rules,
	}
}

// GenerateTenantSecretsWriterRoleBinding generates a namespaced RoleBinding that binds the
// operator's controller ServiceAccount to the secrets writer Role.
func GenerateTenantSecretsWriterRoleBinding(namespace string, operatorSA OperatorServiceAccount) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      TenantSecretsWriterRoleBindingName,
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
			Name:     TenantSecretsWriterRoleName,
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
// - Minimal write access: only "patch" on OpenBaoCluster status to emit drift triggers
// - list/watch on OpenBaoCluster required for controller-runtime cache to function (best-effort)
func GenerateSentinelRole(namespace string) *rbacv1.Role {
	rules := newPolicyRulesBuilder().
		Group("apps").
		Resources("statefulsets").
		Verbs("get", "list", "watch").
		Group("").
		Resources("services", "configmaps").
		Verbs("get", "list", "watch").
		Group("openbao.org").
		Resources("openbaoclusters").
		Verbs("get", "list", "watch").
		Group("openbao.org").
		Resources("openbaoclusters/status").
		Verbs("patch").
		Rules()

	return &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.SentinelRoleName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: "sentinel",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
		Rules: rules,
	}
}

// GenerateSentinelRoleBinding generates a RoleBinding for the Sentinel ServiceAccount.
func GenerateSentinelRoleBinding(namespace string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
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
