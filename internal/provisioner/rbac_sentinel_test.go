package provisioner

import (
	"testing"

	"github.com/openbao/operator/internal/constants"
)

func TestGenerateSentinelRole(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		wantName  string
		wantRules int
	}{
		{
			name:      "default namespace",
			namespace: "default",
			wantName:  constants.SentinelRoleName,
			wantRules: 3, // StatefulSets, Services/ConfigMaps, OpenBaoCluster
		},
		{
			name:      "custom namespace",
			namespace: "tenant-1",
			wantName:  constants.SentinelRoleName,
			wantRules: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role := GenerateSentinelRole(tt.namespace)

			if role == nil {
				t.Fatalf("GenerateSentinelRole() returned nil")
			}

			if role.Name != tt.wantName {
				t.Errorf("GenerateSentinelRole() name = %v, want %v", role.Name, tt.wantName)
			}

			if role.Namespace != tt.namespace {
				t.Errorf("GenerateSentinelRole() namespace = %v, want %v", role.Namespace, tt.namespace)
			}

			if len(role.Rules) != tt.wantRules {
				t.Errorf("GenerateSentinelRole() rules count = %v, want %v", len(role.Rules), tt.wantRules)
			}

			// Verify labels
			expectedLabels := map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: "sentinel",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			}
			for k, v := range expectedLabels {
				if role.Labels[k] != v {
					t.Errorf("GenerateSentinelRole() label[%s] = %v, want %v", k, role.Labels[k], v)
				}
			}

			// Verify least-privilege rules
			hasStatefulSetRule := false
			hasServiceConfigMapRule := false
			hasOpenBaoClusterRule := false

			for _, rule := range role.Rules {
				// Check for StatefulSet read rule (apps)
				if contains(rule.APIGroups, "apps") &&
					contains(rule.Resources, "statefulsets") &&
					!contains(rule.Resources, "deployments") &&
					contains(rule.Verbs, "get") &&
					contains(rule.Verbs, "list") &&
					contains(rule.Verbs, "watch") &&
					!contains(rule.Verbs, "create") &&
					!contains(rule.Verbs, "update") &&
					!contains(rule.Verbs, "patch") &&
					!contains(rule.Verbs, "delete") {
					hasStatefulSetRule = true
				}

				// Check for Service/ConfigMap read rule (core)
				if contains(rule.APIGroups, "") &&
					contains(rule.Resources, "services") &&
					contains(rule.Resources, "configmaps") &&
					!contains(rule.Resources, "secrets") &&
					contains(rule.Verbs, "get") &&
					contains(rule.Verbs, "list") &&
					contains(rule.Verbs, "watch") &&
					!contains(rule.Verbs, "create") &&
					!contains(rule.Verbs, "update") &&
					!contains(rule.Verbs, "patch") &&
					!contains(rule.Verbs, "delete") {
					hasServiceConfigMapRule = true
				}

				// Check for OpenBaoCluster patch rule (needs list/watch for controller-runtime cache)
				if contains(rule.APIGroups, "openbao.org") &&
					contains(rule.Resources, "openbaoclusters") &&
					contains(rule.Verbs, "get") &&
					contains(rule.Verbs, "list") &&
					contains(rule.Verbs, "patch") &&
					contains(rule.Verbs, "watch") &&
					!contains(rule.Verbs, "create") &&
					!contains(rule.Verbs, "update") &&
					!contains(rule.Verbs, "delete") {
					hasOpenBaoClusterRule = true
				}
			}

			if !hasStatefulSetRule {
				t.Error("GenerateSentinelRole() missing StatefulSet read rule")
			}
			if !hasServiceConfigMapRule {
				t.Error("GenerateSentinelRole() missing Service/ConfigMap read rule")
			}
			if !hasOpenBaoClusterRule {
				t.Error("GenerateSentinelRole() missing OpenBaoCluster patch rule")
			}
		})
	}
}

func TestGenerateSentinelRoleBinding(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		wantName  string
	}{
		{
			name:      "default namespace",
			namespace: "default",
			wantName:  constants.SentinelRoleBindingName,
		},
		{
			name:      "custom namespace",
			namespace: "tenant-1",
			wantName:  constants.SentinelRoleBindingName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roleBinding := GenerateSentinelRoleBinding(tt.namespace)

			if roleBinding == nil {
				t.Fatalf("GenerateSentinelRoleBinding() returned nil")
			}

			if roleBinding.Name != tt.wantName {
				t.Errorf("GenerateSentinelRoleBinding() name = %v, want %v", roleBinding.Name, tt.wantName)
			}

			if roleBinding.Namespace != tt.namespace {
				t.Errorf("GenerateSentinelRoleBinding() namespace = %v, want %v", roleBinding.Namespace, tt.namespace)
			}

			// Verify RoleRef
			if roleBinding.RoleRef.APIGroup != "rbac.authorization.k8s.io" {
				t.Errorf("GenerateSentinelRoleBinding() RoleRef.APIGroup = %v, want rbac.authorization.k8s.io", roleBinding.RoleRef.APIGroup)
			}
			if roleBinding.RoleRef.Kind != "Role" {
				t.Errorf("GenerateSentinelRoleBinding() RoleRef.Kind = %v, want Role", roleBinding.RoleRef.Kind)
			}
			if roleBinding.RoleRef.Name != constants.SentinelRoleName {
				t.Errorf("GenerateSentinelRoleBinding() RoleRef.Name = %v, want %v", roleBinding.RoleRef.Name, constants.SentinelRoleName)
			}

			// Verify Subjects
			if len(roleBinding.Subjects) != 1 {
				t.Fatalf("GenerateSentinelRoleBinding() subjects count = %v, want 1", len(roleBinding.Subjects))
			}

			subject := roleBinding.Subjects[0]
			if subject.Kind != "ServiceAccount" {
				t.Errorf("GenerateSentinelRoleBinding() subject.Kind = %v, want ServiceAccount", subject.Kind)
			}
			if subject.Name != constants.SentinelServiceAccountName {
				t.Errorf("GenerateSentinelRoleBinding() subject.Name = %v, want %v", subject.Name, constants.SentinelServiceAccountName)
			}
			if subject.Namespace != tt.namespace {
				t.Errorf("GenerateSentinelRoleBinding() subject.Namespace = %v, want %v", subject.Namespace, tt.namespace)
			}

			// Verify labels
			expectedLabels := map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: "sentinel",
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			}
			for k, v := range expectedLabels {
				if roleBinding.Labels[k] != v {
					t.Errorf("GenerateSentinelRoleBinding() label[%s] = %v, want %v", k, roleBinding.Labels[k], v)
				}
			}
		})
	}
}
