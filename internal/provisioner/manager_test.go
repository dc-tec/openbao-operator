package provisioner

import (
	"context"
	"reflect"
	"sort"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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
	_ = openbaov1alpha1.AddToScheme(scheme)
	return scheme
}()

const testNamespace = "test-namespace"
const podSecurityRestrictedLevel = "restricted"

func newTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	builder := fake.NewClientBuilder().WithScheme(testScheme)
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

func TestEnsureTenantRBAC_CreatesRoleAndRoleBinding(t *testing.T) {
	namespace := testNamespace
	// Create namespace for Pod Security labels test
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	k8sClient := newTestClient(t, ns)
	logger := logr.Discard()
	manager, err := NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	ctx := context.Background()

	// Create namespace if it doesn't exist (for Pod Security labels)
	existingNS := &corev1.Namespace{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, existingNS)
	if err != nil && apierrors.IsNotFound(err) {
		newNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		if createErr := k8sClient.Create(ctx, newNS); createErr != nil {
			t.Fatalf("failed to create namespace: %v", createErr)
		}
	}

	err = manager.EnsureTenantRBAC(ctx, namespace)
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

	// Verify Pod Security labels were applied to namespace
	nsForLabels := &corev1.Namespace{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, nsForLabels)
	if err != nil {
		t.Fatalf("expected Namespace to exist: %v", err)
	}

	expectedLabels := map[string]string{
		"pod-security.kubernetes.io/enforce": podSecurityRestrictedLevel,
		"pod-security.kubernetes.io/audit":   podSecurityRestrictedLevel,
		"pod-security.kubernetes.io/warn":    podSecurityRestrictedLevel,
	}

	for key, expectedValue := range expectedLabels {
		if actualValue, exists := nsForLabels.Labels[key]; !exists {
			t.Errorf("Namespace missing Pod Security label %q", key)
		} else if actualValue != expectedValue {
			t.Errorf("Namespace label %q = %q, want %q", key, actualValue, expectedValue)
		}
	}
}

func TestEnsureTenantRBAC_UpdatesRoleWhenRulesChange(t *testing.T) {
	namespace := testNamespace
	// Create namespace first (required for Pod Security label updates)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
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

	k8sClient := newTestClient(t, ns, existingRole)
	logger := logr.Discard()
	manager, err := NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	ctx := context.Background()

	err = manager.EnsureTenantRBAC(ctx, namespace)
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

	// Verify rules were updated
	if len(role.Rules) != 15 {
		t.Errorf("Role rules count = %v, want 15", len(role.Rules))
	}

	// Verify at least one rule has the expected OpenBaoCluster permissions
	hasExpectedRule := false
	for _, rule := range role.Rules {
		if contains(rule.APIGroups, "openbao.org") &&
			contains(rule.Resources, "openbaoclusters") &&
			contains(rule.Verbs, "get") &&
			contains(rule.Verbs, "create") {
			hasExpectedRule = true
			break
		}
	}
	if !hasExpectedRule {
		t.Error("Role was not updated with expected OpenBaoCluster rule")
	}
}

func TestEnsureTenantRBAC_UpdatesRoleBindingWhenSubjectsChange(t *testing.T) {
	namespace := testNamespace
	// Create namespace first (required for Pod Security label updates)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
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

	k8sClient := newTestClient(t, ns, existingRoleBinding)
	logger := logr.Discard()
	manager, err := NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	ctx := context.Background()

	err = manager.EnsureTenantRBAC(ctx, namespace)
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
	// NewManager uses default "openbao-operator-controller" if OPERATOR_SERVICE_ACCOUNT_NAME is not set
	expectedName := "openbao-operator-controller"
	if subject.Name != expectedName {
		t.Errorf("RoleBinding subject.Name = %v, want %v", subject.Name, expectedName)
	}
	if subject.Namespace != "openbao-operator-system" {
		t.Errorf("RoleBinding subject.Namespace = %v, want openbao-operator-system", subject.Namespace)
	}
}

func TestEnsureTenantRBAC_HandlesAlreadyExistsGracefully(t *testing.T) {
	namespace := testNamespace
	// Create namespace first (required for Pod Security label updates)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	existingRole := GenerateTenantRole(namespace)
	existingRoleBinding := GenerateTenantRoleBinding(namespace, OperatorServiceAccount{
		Name:      "controller-manager",
		Namespace: "openbao-operator-system",
	})

	k8sClient := newTestClient(t, ns, existingRole, existingRoleBinding)
	logger := logr.Discard()
	manager, err := NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	ctx := context.Background()

	// Should not error when resources already exist with correct content
	err = manager.EnsureTenantRBAC(ctx, namespace)
	if err != nil {
		t.Fatalf("EnsureTenantRBAC() error = %v", err)
	}
}

func TestCleanupTenantRBAC_DeletesRoleAndRoleBinding(t *testing.T) {
	namespace := testNamespace
	existingRole := GenerateTenantRole(namespace)
	existingRoleBinding := GenerateTenantRoleBinding(namespace, OperatorServiceAccount{
		Name:      "controller-manager",
		Namespace: "openbao-operator-system",
	})

	k8sClient := newTestClient(t, existingRole, existingRoleBinding)
	logger := logr.Discard()
	manager, err := NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	ctx := context.Background()

	err = manager.CleanupTenantRBAC(ctx, namespace)
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
	namespace := testNamespace
	k8sClient := newTestClient(t)
	logger := logr.Discard()
	manager, err := NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	ctx := context.Background()

	// Should not error when resources don't exist
	err = manager.CleanupTenantRBAC(ctx, namespace)
	if err != nil {
		t.Fatalf("CleanupTenantRBAC() error = %v", err)
	}
}

func TestEnsureTenantRBAC_AppliesPodSecurityLabels(t *testing.T) {
	namespace := testNamespace
	// Create namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				// Pre-existing label that should be preserved
				"existing-label": "value",
			},
		},
	}
	k8sClient := newTestClient(t, ns)
	logger := logr.Discard()
	manager, err := NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	ctx := context.Background()

	err = manager.EnsureTenantRBAC(ctx, namespace)
	if err != nil {
		t.Fatalf("EnsureTenantRBAC() error = %v", err)
	}

	// Verify Pod Security labels were applied
	updatedNS := &corev1.Namespace{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, updatedNS)
	if err != nil {
		t.Fatalf("expected Namespace to exist: %v", err)
	}

	expectedLabels := map[string]string{
		"pod-security.kubernetes.io/enforce": podSecurityRestrictedLevel,
		"pod-security.kubernetes.io/audit":   podSecurityRestrictedLevel,
		"pod-security.kubernetes.io/warn":    podSecurityRestrictedLevel,
	}

	for key, expectedValue := range expectedLabels {
		if actualValue, exists := updatedNS.Labels[key]; !exists {
			t.Errorf("Namespace missing Pod Security label %q", key)
		} else if actualValue != expectedValue {
			t.Errorf("Namespace label %q = %q, want %q", key, actualValue, expectedValue)
		}
	}

	// Verify pre-existing labels are preserved
	if updatedNS.Labels["existing-label"] != "value" {
		t.Errorf("Namespace pre-existing label was not preserved")
	}
}

func TestEnsureTenantRBAC_UpdatesPodSecurityLabels(t *testing.T) {
	namespace := testNamespace
	// Create namespace with incorrect Pod Security labels
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"pod-security.kubernetes.io/enforce": "privileged", // Wrong value
				"pod-security.kubernetes.io/audit":   "baseline",   // Wrong value
				// Missing warn label
			},
		},
	}
	k8sClient := newTestClient(t, ns)
	logger := logr.Discard()
	manager, err := NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	ctx := context.Background()

	err = manager.EnsureTenantRBAC(ctx, namespace)
	if err != nil {
		t.Fatalf("EnsureTenantRBAC() error = %v", err)
	}

	// Verify Pod Security labels were updated to restricted
	updatedNS := &corev1.Namespace{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, updatedNS)
	if err != nil {
		t.Fatalf("expected Namespace to exist: %v", err)
	}

	if updatedNS.Labels["pod-security.kubernetes.io/enforce"] != podSecurityRestrictedLevel {
		t.Errorf("Pod Security enforce label was not updated to restricted")
	}
	if updatedNS.Labels["pod-security.kubernetes.io/audit"] != podSecurityRestrictedLevel {
		t.Errorf("Pod Security audit label was not updated to restricted")
	}
	if updatedNS.Labels["pod-security.kubernetes.io/warn"] != podSecurityRestrictedLevel {
		t.Errorf("Pod Security warn label was not added")
	}
}

func TestEnsureTenantSecretRBAC_CreatesRolesAndRoleBindings(t *testing.T) {
	namespace := testNamespace
	clusterName := "bao"

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.0",
			Image:    "openbao:2.4.0",
			Replicas: 1,
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
					Endpoint: "https://s3.example",
					Bucket:   "bucket",
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
	}

	k8sClient := newTestClient(t, cluster)
	logger := logr.Discard()
	manager, err := NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	ctx := context.Background()
	if err := manager.EnsureTenantSecretRBAC(ctx, namespace); err != nil {
		t.Fatalf("EnsureTenantSecretRBAC() error = %v", err)
	}

	expectedWriterNames := []string{
		clusterName + constants.SuffixRootToken,
		clusterName + constants.SuffixTLSCA,
		clusterName + constants.SuffixTLSServer,
		clusterName + constants.SuffixUnsealKey,
	}
	sort.Strings(expectedWriterNames)

	expectedReaderNames := []string{
		"backup-creds",
		"backup-token",
		"unseal-creds",
		"upgrade-token",
	}
	sort.Strings(expectedReaderNames)

	writerRole := &rbacv1.Role{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: TenantSecretsWriterRoleName}, writerRole); err != nil {
		t.Fatalf("expected writer Role to exist: %v", err)
	}
	if got := extractSecretResourceNames(writerRole.Rules); !reflect.DeepEqual(got, expectedWriterNames) {
		t.Errorf("writer Role allowlist = %v, want %v", got, expectedWriterNames)
	}

	writerRoleBinding := &rbacv1.RoleBinding{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: TenantSecretsWriterRoleBindingName}, writerRoleBinding); err != nil {
		t.Fatalf("expected writer RoleBinding to exist: %v", err)
	}
	if writerRoleBinding.RoleRef.Name != TenantSecretsWriterRoleName {
		t.Errorf("writer RoleBinding RoleRef.Name = %v, want %v", writerRoleBinding.RoleRef.Name, TenantSecretsWriterRoleName)
	}

	readerRole := &rbacv1.Role{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: TenantSecretsReaderRoleName}, readerRole); err != nil {
		t.Fatalf("expected reader Role to exist: %v", err)
	}
	if got := extractSecretResourceNames(readerRole.Rules); !reflect.DeepEqual(got, expectedReaderNames) {
		t.Errorf("reader Role allowlist = %v, want %v", got, expectedReaderNames)
	}

	readerRoleBinding := &rbacv1.RoleBinding{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: TenantSecretsReaderRoleBindingName}, readerRoleBinding); err != nil {
		t.Fatalf("expected reader RoleBinding to exist: %v", err)
	}
	if readerRoleBinding.RoleRef.Name != TenantSecretsReaderRoleName {
		t.Errorf("reader RoleBinding RoleRef.Name = %v, want %v", readerRoleBinding.RoleRef.Name, TenantSecretsReaderRoleName)
	}
}

func TestEnsureTenantSecretRBAC_DeletesRolesAndRoleBindingsWhenNoClustersRemain(t *testing.T) {
	namespace := testNamespace

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bao",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.0",
			Image:    "openbao:2.4.0",
			Replicas: 1,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
				Mode:    openbaov1alpha1.TLSModeOperatorManaged,
			},
		},
	}

	k8sClient := newTestClient(t, cluster)
	logger := logr.Discard()
	manager, err := NewManager(k8sClient, nil, logger)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	ctx := context.Background()

	if err := manager.EnsureTenantSecretRBAC(ctx, namespace); err != nil {
		t.Fatalf("EnsureTenantSecretRBAC() error = %v", err)
	}

	if err := k8sClient.Delete(ctx, cluster); err != nil {
		t.Fatalf("failed to delete OpenBaoCluster: %v", err)
	}

	if err := manager.EnsureTenantSecretRBAC(ctx, namespace); err != nil {
		t.Fatalf("EnsureTenantSecretRBAC() error = %v", err)
	}

	for _, name := range []string{TenantSecretsReaderRoleBindingName, TenantSecretsWriterRoleBindingName} {
		roleBinding := &rbacv1.RoleBinding{}
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, roleBinding)
		if !apierrors.IsNotFound(err) {
			t.Errorf("expected RoleBinding %s to be deleted, got error: %v", name, err)
		}
	}

	for _, name := range []string{TenantSecretsReaderRoleName, TenantSecretsWriterRoleName} {
		role := &rbacv1.Role{}
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, role)
		if !apierrors.IsNotFound(err) {
			t.Errorf("expected Role %s to be deleted, got error: %v", name, err)
		}
	}
}

func extractSecretResourceNames(rules []rbacv1.PolicyRule) []string {
	var out []string
	for i := range rules {
		rule := rules[i]
		if !contains(rule.Resources, "secrets") {
			continue
		}
		if len(rule.ResourceNames) == 0 {
			continue
		}
		out = append(out, rule.ResourceNames...)
	}
	sort.Strings(out)
	return out
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
