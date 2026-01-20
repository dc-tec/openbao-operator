package provisioner

import (
	"sort"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			wantRules: 15, // Expected number of PolicyRules
		},
		{
			name:      "custom namespace",
			namespace: "tenant-1",
			wantName:  TenantRoleName,
			wantRules: 15,
		},
		{
			name:      "namespace with special characters",
			namespace: "my-namespace-123",
			wantName:  TenantRoleName,
			wantRules: 15,
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

				// Check for Deployment rule (used for controller-managed Deployments where enabled)
				if contains(rule.APIGroups, "apps") &&
					contains(rule.Resources, "deployments") &&
					contains(rule.Verbs, "get") &&
					contains(rule.Verbs, "create") &&
					contains(rule.Verbs, "delete") {
					hasDeploymentRule = true
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

func TestGetRequiredSecretPermissions(t *testing.T) {
	tests := []struct {
		name        string
		cluster     *openbaov1alpha1.OpenBaoCluster
		wantWriters []string
		wantReaders []string
	}{
		{
			name:    "nil cluster returns nil",
			cluster: nil,
		},
		{
			name:    "empty name returns nil",
			cluster: &openbaov1alpha1.OpenBaoCluster{},
		},
		{
			name: "OperatorManaged TLS mode - TLS secrets are writable",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("test-cluster"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeOperatorManaged,
					},
				},
			},
			wantWriters: []string{
				"test-cluster-root-token",
				"test-cluster-tls-ca",
				"test-cluster-tls-server",
				"test-cluster-unseal-key",
			},
		},
		{
			name: "empty TLS mode defaults to OperatorManaged",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("bao"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    "", // empty defaults to OperatorManaged
					},
				},
			},
			wantWriters: []string{
				"bao-root-token",
				"bao-tls-ca",
				"bao-tls-server",
				"bao-unseal-key",
			},
		},
		{
			name: "External TLS mode - TLS secrets are readable",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("ext"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeExternal,
					},
				},
			},
			wantWriters: []string{
				"ext-root-token",
				"ext-unseal-key",
			},
			wantReaders: []string{
				"ext-tls-ca",
				"ext-tls-server",
			},
		},
		{
			name: "ACME TLS mode - TLS secrets are readable",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("acme"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
					},
				},
			},
			wantWriters: []string{
				"acme-root-token",
				"acme-unseal-key",
			},
			wantReaders: []string{
				"acme-tls-ca",
				"acme-tls-server",
			},
		},
		{
			name: "SelfInit enabled - no root token written",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("selfinit"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeOperatorManaged,
					},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
					},
				},
			},
			wantWriters: []string{
				"selfinit-tls-ca",
				"selfinit-tls-server",
				"selfinit-unseal-key",
			},
		},
		{
			name: "Cloud unseal (awskms) - no unseal key written",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("cloud"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeOperatorManaged,
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "awskms",
						CredentialsSecretRef: &corev1.LocalObjectReference{
							Name: "aws-creds",
						},
					},
				},
			},
			wantWriters: []string{
				"cloud-root-token",
				"cloud-tls-ca",
				"cloud-tls-server",
			},
			wantReaders: []string{
				"aws-creds",
			},
		},
		{
			name: "Static unseal - unseal key written",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("static"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeOperatorManaged,
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "static",
					},
				},
			},
			wantWriters: []string{
				"static-root-token",
				"static-tls-ca",
				"static-tls-server",
				"static-unseal-key",
			},
		},
		{
			name: "Backup with credentials and token",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("backup"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeOperatorManaged,
					},
					Backup: &openbaov1alpha1.BackupSchedule{
						Schedule: "0 3 * * *",
						Target: openbaov1alpha1.BackupTarget{
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: "backup-creds",
							},
						},
						TokenSecretRef: &corev1.LocalObjectReference{
							Name: "backup-token",
						},
					},
				},
			},
			wantWriters: []string{
				"backup-root-token",
				"backup-tls-ca",
				"backup-tls-server",
				"backup-unseal-key",
			},
			wantReaders: []string{
				"backup-creds",
				"backup-token",
			},
		},
		{
			name: "Upgrade with token",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("upgrade"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeOperatorManaged,
					},
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						TokenSecretRef: &corev1.LocalObjectReference{
							Name: "upgrade-token",
						},
					},
				},
			},
			wantWriters: []string{
				"upgrade-root-token",
				"upgrade-tls-ca",
				"upgrade-tls-server",
				"upgrade-unseal-key",
			},
			wantReaders: []string{
				"upgrade-token",
			},
		},
		{
			name: "Full configuration with all secret types",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("full"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeOperatorManaged,
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "static",
						CredentialsSecretRef: &corev1.LocalObjectReference{
							Name: "unseal-creds",
						},
					},
					Backup: &openbaov1alpha1.BackupSchedule{
						Schedule: "0 3 * * *",
						Target: openbaov1alpha1.BackupTarget{
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: "backup-creds",
							},
						},
						TokenSecretRef: &corev1.LocalObjectReference{
							Name: "backup-token",
						},
					},
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						TokenSecretRef: &corev1.LocalObjectReference{
							Name: "upgrade-token",
						},
					},
				},
			},
			wantWriters: []string{
				"full-root-token",
				"full-tls-ca",
				"full-tls-server",
				"full-unseal-key",
			},
			wantReaders: []string{
				"backup-creds",
				"backup-token",
				"unseal-creds",
				"upgrade-token",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perms := GetRequiredSecretPermissions(tt.cluster)

			gotWriters := filterByPermission(perms, PermissionWrite)
			gotReaders := filterByPermission(perms, PermissionRead)

			sort.Strings(gotWriters)
			sort.Strings(gotReaders)
			sort.Strings(tt.wantWriters)
			sort.Strings(tt.wantReaders)

			if !stringSlicesEqual(gotWriters, tt.wantWriters) {
				t.Errorf("writers = %v, want %v", gotWriters, tt.wantWriters)
			}
			if !stringSlicesEqual(gotReaders, tt.wantReaders) {
				t.Errorf("readers = %v, want %v", gotReaders, tt.wantReaders)
			}
		})
	}
}

func TestIsStaticUnseal(t *testing.T) {
	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		want    bool
	}{
		{
			name:    "nil cluster is static",
			cluster: nil,
			want:    true,
		},
		{
			name: "nil Unseal is static",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("test"),
				Spec:       openbaov1alpha1.OpenBaoClusterSpec{},
			},
			want: true,
		},
		{
			name: "empty type is static",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("test"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "",
					},
				},
			},
			want: true,
		},
		{
			name: "explicit static type",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("test"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "static",
					},
				},
			},
			want: true,
		},
		{
			name: "awskms is not static",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("test"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "awskms",
					},
				},
			},
			want: false,
		},
		{
			name: "gcpkms is not static",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: objectMeta("test"),
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "gcpkms",
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsStaticUnseal(tt.cluster)
			if got != tt.want {
				t.Errorf("IsStaticUnseal() = %v, want %v", got, tt.want)
			}
		})
	}
}

// objectMeta creates a minimal ObjectMeta for testing.
func objectMeta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: "default",
	}
}

// filterByPermission extracts secret names with the given permission type.
func filterByPermission(perms []SecretPermission, permission string) []string {
	var result []string
	for _, p := range perms {
		if p.Permission == permission {
			result = append(result, p.Name)
		}
	}
	return result
}

// stringSlicesEqual compares two string slices for equality.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
