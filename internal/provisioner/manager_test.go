package provisioner

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// testScheme is a shared scheme used across tests.
var testScheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	return scheme
}()

func newTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	builder := fake.NewClientBuilder().WithScheme(testScheme)
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

func TestEnsureTenantRBAC_CreatesRoleAndRoleBinding(t *testing.T) {
	k8sClient := newTestClient(t)
	logger := logr.Discard()
	manager := NewManager(k8sClient, logger)

	namespace := "test-namespace"
	ctx := context.Background()

	err := manager.EnsureTenantRBAC(ctx, namespace)
	if err != nil {
		t.Fatalf("EnsureTenantRBAC() error = %v", err)
	}

	// Verify Role was created
	role := &rbacv1.Role{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      TenantRoleName,
	}, role)
	if err != nil {
		t.Fatalf("expected Role to exist: %v", err)
	}

	if role.Name != TenantRoleName {
		t.Errorf("Role name = %v, want %v", role.Name, TenantRoleName)
	}
	if role.Namespace != namespace {
		t.Errorf("Role namespace = %v, want %v", role.Namespace, namespace)
	}

	// Verify RoleBinding was created
	roleBinding := &rbacv1.RoleBinding{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      TenantRoleBindingName,
	}, roleBinding)
	if err != nil {
		t.Fatalf("expected RoleBinding to exist: %v", err)
	}

	if roleBinding.Name != TenantRoleBindingName {
		t.Errorf("RoleBinding name = %v, want %v", roleBinding.Name, TenantRoleBindingName)
	}
	if roleBinding.Namespace != namespace {
		t.Errorf("RoleBinding namespace = %v, want %v", roleBinding.Namespace, namespace)
	}
}

func TestEnsureTenantRBAC_UpdatesRoleWhenRulesChange(t *testing.T) {
	namespace := "test-namespace"
	existingRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TenantRoleName,
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters"},
				Verbs:     []string{"get"}, // Different from expected
			},
		},
	}

	k8sClient := newTestClient(t, existingRole)
	logger := logr.Discard()
	manager := NewManager(k8sClient, logger)

	ctx := context.Background()

	err := manager.EnsureTenantRBAC(ctx, namespace)
	if err != nil {
		t.Fatalf("EnsureTenantRBAC() error = %v", err)
	}

	// Verify Role was updated
	role := &rbacv1.Role{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      TenantRoleName,
	}, role)
	if err != nil {
		t.Fatalf("expected Role to exist: %v", err)
	}

	// Verify rules were updated (should have 11 rules now)
	if len(role.Rules) != 11 {
		t.Errorf("Role rules count = %v, want 11", len(role.Rules))
	}

	// Verify at least one rule has the expected OpenBaoCluster permissions
	hasExpectedRule := false
	for _, rule := range role.Rules {
		if contains(rule.APIGroups, "openbao.org") &&
			contains(rule.Resources, "openbaoclusters") &&
			contains(rule.Verbs, "*") {
			hasExpectedRule = true
			break
		}
	}
	if !hasExpectedRule {
		t.Error("Role was not updated with expected OpenBaoCluster rule")
	}
}

func TestEnsureTenantRBAC_UpdatesRoleBindingWhenSubjectsChange(t *testing.T) {
	namespace := "test-namespace"
	existingRoleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TenantRoleBindingName,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     TenantRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "old-operator",
				Namespace: "old-namespace",
			},
		},
	}

	k8sClient := newTestClient(t, existingRoleBinding)
	logger := logr.Discard()
	manager := NewManager(k8sClient, logger)

	ctx := context.Background()

	err := manager.EnsureTenantRBAC(ctx, namespace)
	if err != nil {
		t.Fatalf("EnsureTenantRBAC() error = %v", err)
	}

	// Verify RoleBinding was updated
	roleBinding := &rbacv1.RoleBinding{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      TenantRoleBindingName,
	}, roleBinding)
	if err != nil {
		t.Fatalf("expected RoleBinding to exist: %v", err)
	}

	// Verify subject was updated
	if len(roleBinding.Subjects) != 1 {
		t.Fatalf("RoleBinding subjects count = %v, want 1", len(roleBinding.Subjects))
	}

	subject := roleBinding.Subjects[0]
	if subject.Name != "controller-manager" {
		t.Errorf("RoleBinding subject.Name = %v, want controller-manager", subject.Name)
	}
	if subject.Namespace != "openbao-operator-system" {
		t.Errorf("RoleBinding subject.Namespace = %v, want openbao-operator-system", subject.Namespace)
	}
}

func TestEnsureTenantRBAC_HandlesAlreadyExistsGracefully(t *testing.T) {
	namespace := "test-namespace"
	existingRole := GenerateTenantRole(namespace)
	existingRoleBinding := GenerateTenantRoleBinding(namespace, OperatorServiceAccount{
		Name:      "controller-manager",
		Namespace: "openbao-operator-system",
	})

	k8sClient := newTestClient(t, existingRole, existingRoleBinding)
	logger := logr.Discard()
	manager := NewManager(k8sClient, logger)

	ctx := context.Background()

	// Should not error when resources already exist with correct content
	err := manager.EnsureTenantRBAC(ctx, namespace)
	if err != nil {
		t.Fatalf("EnsureTenantRBAC() error = %v", err)
	}
}

func TestCleanupTenantRBAC_DeletesRoleAndRoleBinding(t *testing.T) {
	namespace := "test-namespace"
	existingRole := GenerateTenantRole(namespace)
	existingRoleBinding := GenerateTenantRoleBinding(namespace, OperatorServiceAccount{
		Name:      "controller-manager",
		Namespace: "openbao-operator-system",
	})

	k8sClient := newTestClient(t, existingRole, existingRoleBinding)
	logger := logr.Discard()
	manager := NewManager(k8sClient, logger)

	ctx := context.Background()

	err := manager.CleanupTenantRBAC(ctx, namespace)
	if err != nil {
		t.Fatalf("CleanupTenantRBAC() error = %v", err)
	}

	// Verify RoleBinding was deleted
	roleBinding := &rbacv1.RoleBinding{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      TenantRoleBindingName,
	}, roleBinding)
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected RoleBinding to be deleted, got error: %v", err)
	}

	// Verify Role was deleted
	role := &rbacv1.Role{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      TenantRoleName,
	}, role)
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected Role to be deleted, got error: %v", err)
	}
}

func TestCleanupTenantRBAC_HandlesNotFoundGracefully(t *testing.T) {
	namespace := "test-namespace"
	k8sClient := newTestClient(t)
	logger := logr.Discard()
	manager := NewManager(k8sClient, logger)

	ctx := context.Background()

	// Should not error when resources don't exist
	err := manager.CleanupTenantRBAC(ctx, namespace)
	if err != nil {
		t.Fatalf("CleanupTenantRBAC() error = %v", err)
	}
}

func TestRolesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []rbacv1.PolicyRule
		b    []rbacv1.PolicyRule
		want bool
	}{
		{
			name: "equal rules",
			a: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"openbao.org"},
					Resources: []string{"openbaoclusters"},
					Verbs:     []string{"*"},
				},
			},
			b: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"openbao.org"},
					Resources: []string{"openbaoclusters"},
					Verbs:     []string{"*"},
				},
			},
			want: true,
		},
		{
			name: "different lengths",
			a: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"openbao.org"},
					Resources: []string{"openbaoclusters"},
					Verbs:     []string{"*"},
				},
			},
			b:    []rbacv1.PolicyRule{},
			want: false,
		},
		{
			name: "different APIGroups",
			a: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"openbao.org"},
					Resources: []string{"openbaoclusters"},
					Verbs:     []string{"*"},
				},
			},
			b: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"apps"},
					Resources: []string{"openbaoclusters"},
					Verbs:     []string{"*"},
				},
			},
			want: false,
		},
		{
			name: "different Resources",
			a: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"openbao.org"},
					Resources: []string{"openbaoclusters"},
					Verbs:     []string{"*"},
				},
			},
			b: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"openbao.org"},
					Resources: []string{"statefulsets"},
					Verbs:     []string{"*"},
				},
			},
			want: false,
		},
		{
			name: "different Verbs",
			a: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"openbao.org"},
					Resources: []string{"openbaoclusters"},
					Verbs:     []string{"*"},
				},
			},
			b: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"openbao.org"},
					Resources: []string{"openbaoclusters"},
					Verbs:     []string{"get"},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rolesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("rolesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPolicyRuleEqual(t *testing.T) {
	tests := []struct {
		name string
		a    rbacv1.PolicyRule
		b    rbacv1.PolicyRule
		want bool
	}{
		{
			name: "equal rules",
			a: rbacv1.PolicyRule{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters"},
				Verbs:     []string{"*"},
			},
			b: rbacv1.PolicyRule{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters"},
				Verbs:     []string{"*"},
			},
			want: true,
		},
		{
			name: "different APIGroups length",
			a: rbacv1.PolicyRule{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters"},
				Verbs:     []string{"*"},
			},
			b: rbacv1.PolicyRule{
				APIGroups: []string{"openbao.org", "apps"},
				Resources: []string{"openbaoclusters"},
				Verbs:     []string{"*"},
			},
			want: false,
		},
		{
			name: "different APIGroups values",
			a: rbacv1.PolicyRule{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters"},
				Verbs:     []string{"*"},
			},
			b: rbacv1.PolicyRule{
				APIGroups: []string{"apps"},
				Resources: []string{"openbaoclusters"},
				Verbs:     []string{"*"},
			},
			want: false,
		},
		{
			name: "different Resources length",
			a: rbacv1.PolicyRule{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters"},
				Verbs:     []string{"*"},
			},
			b: rbacv1.PolicyRule{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters", "statefulsets"},
				Verbs:     []string{"*"},
			},
			want: false,
		},
		{
			name: "different Verbs length",
			a: rbacv1.PolicyRule{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters"},
				Verbs:     []string{"*"},
			},
			b: rbacv1.PolicyRule{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters"},
				Verbs:     []string{"get", "list"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policyRuleEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("policyRuleEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

// contains is a helper function to check if a slice contains a value
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
