package provisioner

import (
	"testing"

	"github.com/openbao/operator/internal/constants"
)

//nolint:gocyclo // Table-driven test with multiple assertions
func TestGenerateTenantRole(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		wantName  string
		wantRules int
	}{
		{
			name:      "default namespace",
			namespace: "default",
			wantName:  TenantRoleName,
			wantRules: 16, // Expected number of PolicyRules
		},
		{
			name:      "custom namespace",
			namespace: "tenant-1",
			wantName:  TenantRoleName,
			wantRules: 16,
		},
		{
			name:      "namespace with special characters",
			namespace: "my-namespace-123",
			wantName:  TenantRoleName,
			wantRules: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role := GenerateTenantRole(tt.namespace)

			if role == nil {
				t.Fatalf("GenerateTenantRole() returned nil")
			}

			if role.Name != tt.wantName {
				t.Errorf("GenerateTenantRole() name = %v, want %v", role.Name, tt.wantName)
			}

			if role.Namespace != tt.namespace {
				t.Errorf("GenerateTenantRole() namespace = %v, want %v", role.Namespace, tt.namespace)
			}

			if len(role.Rules) != tt.wantRules {
				t.Errorf("GenerateTenantRole() rules count = %v, want %v", len(role.Rules), tt.wantRules)
			}

			// Verify labels
			expectedLabels := map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: "provisioner",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			}
			for k, v := range expectedLabels {
				if role.Labels[k] != v {
					t.Errorf("GenerateTenantRole() label[%s] = %v, want %v", k, role.Labels[k], v)
				}
			}

			// Verify key rules exist
			hasOpenBaoClusterRule := false
			hasStatefulSetRule := false
			hasDeploymentRule := false
			hasSecretRule := false
			secretRuleHasWatch := false
			secretRuleHasList := false
			hasPodRule := false

			for _, rule := range role.Rules {
				// Check for OpenBaoCluster rule (uses commonVerbs, not "*")
				if contains(rule.APIGroups, "openbao.org") &&
					contains(rule.Resources, "openbaoclusters") &&
					contains(rule.Verbs, "get") &&
					contains(rule.Verbs, "create") {
					hasOpenBaoClusterRule = true
				}

				// Check for StatefulSet rule (uses commonVerbs, not "*")
				if contains(rule.APIGroups, "apps") &&
					contains(rule.Resources, "statefulsets") &&
					contains(rule.Verbs, "get") &&
					contains(rule.Verbs, "create") {
					hasStatefulSetRule = true
				}

				// Check for Deployment rule (for Sentinel)
				if contains(rule.APIGroups, "apps") &&
					contains(rule.Resources, "deployments") &&
					contains(rule.Verbs, "get") &&
					contains(rule.Verbs, "create") &&
					contains(rule.Verbs, "delete") {
					hasDeploymentRule = true
				}

				// Check for Secret rule
				if contains(rule.APIGroups, "") &&
					contains(rule.Resources, "secrets") &&
					contains(rule.Verbs, "get") &&
					contains(rule.Verbs, "create") {
					hasSecretRule = true
					secretRuleHasWatch = contains(rule.Verbs, "watch")
					secretRuleHasList = contains(rule.Verbs, "list")
				}

				// Check for Pod rule (includes delete for cleanup)
				if contains(rule.APIGroups, "") &&
					contains(rule.Resources, "pods") &&
					contains(rule.Verbs, "get") &&
					contains(rule.Verbs, "list") &&
					contains(rule.Verbs, "delete") {
					hasPodRule = true
				}

			}

			if !hasOpenBaoClusterRule {
				t.Error("GenerateTenantRole() missing OpenBaoCluster rule")
			}
			if !hasStatefulSetRule {
				t.Error("GenerateTenantRole() missing StatefulSet rule")
			}
			if !hasDeploymentRule {
				t.Error("GenerateTenantRole() missing Deployment rule")
			}
			if !hasSecretRule {
				t.Error("GenerateTenantRole() missing Secret rule")
			}
			if secretRuleHasWatch {
				t.Error("GenerateTenantRole() Secret rule must not grant watch")
			}
			if secretRuleHasList {
				t.Error("GenerateTenantRole() Secret rule must not grant list")
			}
			if !hasPodRule {
				t.Error("GenerateTenantRole() missing Pod rule")
			}
		})
	}
}

func TestGenerateTenantRoleBinding(t *testing.T) {
	tests := []struct {
		name       string
		namespace  string
		operatorSA OperatorServiceAccount
		wantName   string
	}{
		{
			name:      "default service account",
			namespace: "default",
			operatorSA: OperatorServiceAccount{
				Name:      "controller-manager",
				Namespace: "openbao-operator-system",
			},
			wantName: TenantRoleBindingName,
		},
		{
			name:      "custom service account",
			namespace: "tenant-1",
			operatorSA: OperatorServiceAccount{
				Name:      "custom-operator",
				Namespace: "custom-namespace",
			},
			wantName: TenantRoleBindingName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roleBinding := GenerateTenantRoleBinding(tt.namespace, tt.operatorSA)

			if roleBinding == nil {
				t.Fatalf("GenerateTenantRoleBinding() returned nil")
			}

			if roleBinding.Name != tt.wantName {
				t.Errorf("GenerateTenantRoleBinding() name = %v, want %v", roleBinding.Name, tt.wantName)
			}

			if roleBinding.Namespace != tt.namespace {
				t.Errorf("GenerateTenantRoleBinding() namespace = %v, want %v", roleBinding.Namespace, tt.namespace)
			}

			// Verify RoleRef
			if roleBinding.RoleRef.APIGroup != "rbac.authorization.k8s.io" {
				t.Errorf("GenerateTenantRoleBinding() RoleRef.APIGroup = %v, want rbac.authorization.k8s.io", roleBinding.RoleRef.APIGroup)
			}
			if roleBinding.RoleRef.Kind != "Role" {
				t.Errorf("GenerateTenantRoleBinding() RoleRef.Kind = %v, want Role", roleBinding.RoleRef.Kind)
			}
			if roleBinding.RoleRef.Name != TenantRoleName {
				t.Errorf("GenerateTenantRoleBinding() RoleRef.Name = %v, want %v", roleBinding.RoleRef.Name, TenantRoleName)
			}

			// Verify Subjects
			if len(roleBinding.Subjects) != 1 {
				t.Fatalf("GenerateTenantRoleBinding() subjects count = %v, want 1", len(roleBinding.Subjects))
			}

			subject := roleBinding.Subjects[0]
			if subject.Kind != "ServiceAccount" {
				t.Errorf("GenerateTenantRoleBinding() subject.Kind = %v, want ServiceAccount", subject.Kind)
			}
			if subject.Name != tt.operatorSA.Name {
				t.Errorf("GenerateTenantRoleBinding() subject.Name = %v, want %v", subject.Name, tt.operatorSA.Name)
			}
			if subject.Namespace != tt.operatorSA.Namespace {
				t.Errorf("GenerateTenantRoleBinding() subject.Namespace = %v, want %v", subject.Namespace, tt.operatorSA.Namespace)
			}

			// Verify labels
			expectedLabels := map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: "provisioner",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			}
			for k, v := range expectedLabels {
				if roleBinding.Labels[k] != v {
					t.Errorf("GenerateTenantRoleBinding() label[%s] = %v, want %v", k, roleBinding.Labels[k], v)
				}
			}
		})
	}
}
