package infra

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

// testScheme is a shared scheme used across tests.
var testScheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)
	return scheme
}()

func TestUsesStaticSeal(t *testing.T) {
	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		want    bool
	}{
		{
			name: "nil unseal config",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Unseal: nil,
				},
			},
			want: true,
		},
		{
			name: "empty unseal type",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "",
					},
				},
			},
			want: true,
		},
		{
			name: "static unseal type",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "static",
					},
				},
			},
			want: true,
		},
		{
			name: "non-static unseal type",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Unseal: &openbaov1alpha1.UnsealConfig{
						Type: "aws-kms",
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := usesStaticSeal(tt.cluster)
			if got != tt.want {
				t.Errorf("usesStaticSeal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateUnsealKey(t *testing.T) {
	key, err := generateUnsealKey()
	if err != nil {
		t.Fatalf("generateUnsealKey() error = %v", err)
	}

	if len(key) != 32 {
		t.Errorf("generateUnsealKey() key length = %v, want 32", len(key))
	}

	// Generate multiple keys and verify they're different
	key2, err := generateUnsealKey()
	if err != nil {
		t.Fatalf("generateUnsealKey() error = %v", err)
	}

	if len(key2) != 32 {
		t.Errorf("generateUnsealKey() key2 length = %v, want 32", len(key2))
	}

	// Keys should be different (very unlikely to be the same)
	if string(key) == string(key2) {
		t.Error("generateUnsealKey() should generate different keys")
	}
}

func TestEnsureUnsealSecret_CreatesSecret(t *testing.T) {
	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	cluster := newMinimalCluster("test-cluster", "default")

	err := manager.ensureUnsealSecret(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("ensureUnsealSecret() error = %v", err)
	}

	// Verify Secret was created
	secretName := unsealSecretName(cluster)
	secret := &corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      secretName,
	}, secret)

	if err != nil {
		t.Fatalf("expected Secret to exist: %v", err)
	}

	if secret.Type != corev1.SecretTypeOpaque {
		t.Errorf("ensureUnsealSecret() secret.Type = %v, want %v", secret.Type, corev1.SecretTypeOpaque)
	}

	if secret.Immutable == nil || !*secret.Immutable {
		t.Error("ensureUnsealSecret() secret should be immutable")
	}

	if len(secret.Data[unsealSecretKey]) != 32 {
		t.Errorf("ensureUnsealSecret() key length = %v, want 32", len(secret.Data[unsealSecretKey]))
	}
}

func newTestClientWithObjects(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	builder := fake.NewClientBuilder().WithScheme(testScheme)
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

func TestEnsureUnsealSecret_HandlesAlreadyExists(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")
	secretName := unsealSecretName(cluster)

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			unsealSecretKey: []byte("existing-key"),
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClientWithObjects(t, existingSecret)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	// Should not error when secret already exists (blind create pattern)
	err := manager.ensureUnsealSecret(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("ensureUnsealSecret() with existing secret should not error, got: %v", err)
	}
}

func TestEnsureConfigMap_CreatesConfigMap(t *testing.T) {
	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	cluster := newMinimalCluster("test-cluster", "default")
	configContent := "test config content"

	err := manager.ensureConfigMap(ctx, logger, cluster, configContent)
	if err != nil {
		t.Fatalf("ensureConfigMap() error = %v", err)
	}

	// Verify ConfigMap was created
	cmName := configMapName(cluster)
	configMap := &corev1.ConfigMap{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cmName,
	}, configMap)

	if err != nil {
		t.Fatalf("expected ConfigMap to exist: %v", err)
	}

	if configMap.Data[configFileName] != configContent {
		t.Errorf("ensureConfigMap() config content = %v, want %v", configMap.Data[configFileName], configContent)
	}
}

func TestEnsureConfigMap_UpdatesConfigMap(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")
	cmName := configMapName(cluster)

	existingConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cluster.Namespace,
		},
		Data: map[string]string{
			configFileName: "old config content",
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClientWithObjects(t, existingConfigMap)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	newConfigContent := "new config content"
	err := manager.ensureConfigMap(ctx, logger, cluster, newConfigContent)
	if err != nil {
		t.Fatalf("ensureConfigMap() error = %v", err)
	}

	// Verify ConfigMap was updated
	configMap := &corev1.ConfigMap{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cmName,
	}, configMap)

	if err != nil {
		t.Fatalf("expected ConfigMap to exist: %v", err)
	}

	if configMap.Data[configFileName] != newConfigContent {
		t.Errorf("ensureConfigMap() config content = %v, want %v", configMap.Data[configFileName], newConfigContent)
	}
}

func TestEnsureConfigMap_IsIdempotent(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")
	cmName := configMapName(cluster)
	configContent := "test config content"

	existingConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cluster.Namespace,
		},
		Data: map[string]string{
			configFileName: configContent,
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClientWithObjects(t, existingConfigMap)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	// With SSA, applying the same state multiple times is idempotent
	// First apply should succeed
	err := manager.ensureConfigMap(ctx, logger, cluster, configContent)
	if err != nil {
		t.Fatalf("ensureConfigMap() first apply error = %v", err)
	}

	// Second apply with same content should also succeed (idempotent)
	err = manager.ensureConfigMap(ctx, logger, cluster, configContent)
	if err != nil {
		t.Fatalf("ensureConfigMap() second apply error = %v", err)
	}

	// Verify ConfigMap still exists and has correct content
	configMap := &corev1.ConfigMap{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cmName,
	}, configMap)
	if err != nil {
		t.Fatalf("expected ConfigMap to exist after idempotent applies: %v", err)
	}
	if configMap.Data[configFileName] != configContent {
		t.Errorf("expected ConfigMap content to remain %q, got %q", configContent, configMap.Data[configFileName])
	}
}

func TestEnsureSelfInitConfigMap_Disabled(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: false,
	}
	cmName := configInitMapName(cluster)

	existingConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cluster.Namespace,
		},
		Data: map[string]string{
			configFileName: "old init config",
		},
	}

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClientWithObjects(t, existingConfigMap)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	err := manager.ensureSelfInitConfigMap(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("ensureSelfInitConfigMap() error = %v", err)
	}

	// Verify ConfigMap was deleted
	configMap := &corev1.ConfigMap{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cmName,
	}, configMap)

	if !apierrors.IsNotFound(err) {
		t.Errorf("expected ConfigMap to be deleted, got error: %v", err)
	}
}

func TestEnsureSelfInitConfigMap_NotConfigured(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")
	cluster.Spec.SelfInit = nil

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	// Should not error when self-init is not configured
	err := manager.ensureSelfInitConfigMap(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("ensureSelfInitConfigMap() error = %v", err)
	}
}

func TestEnsureSelfInitConfigMap_HardenedProfileWithBootstrap(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")
	cluster.Spec.Profile = openbaov1alpha1.ProfileHardened
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled:          true,
		BootstrapJWTAuth: true, // Opt-in to JWT bootstrap
		Requests: []openbaov1alpha1.SelfInitRequest{
			{
				Name:      "enable-stdout-audit",
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/audit/stdout",
				Data: &apiextensionsv1.JSON{
					Raw: []byte(`{"type":"file"}`),
				},
			},
		},
	}

	oidcIssuer := "https://kubernetes.default.svc"
	oidcJWTKeys := []string{"-----BEGIN PUBLIC KEY-----\ntest-public-key\n-----END PUBLIC KEY-----\n"}

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", oidcIssuer, oidcJWTKeys)

	err := manager.ensureSelfInitConfigMap(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("ensureSelfInitConfigMap() error = %v", err)
	}

	// Verify ConfigMap was created
	cmName := configInitMapName(cluster)
	configMap := &corev1.ConfigMap{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cmName,
	}, configMap)
	if err != nil {
		t.Fatalf("expected ConfigMap to exist: %v", err)
	}

	content := configMap.Data[configFileName]
	if content == "" {
		t.Fatal("expected ConfigMap to have config content")
	}

	// Verify bootstrap HCL is included
	expectedSnippets := []string{
		`initialize "operator-bootstrap"`,
		`request "enable-jwt-auth"`,
		`request "config-jwt-auth"`,
		`request "create-operator-policy"`,
		`request "create-operator-role"`,
		`jwt_validation_pubkeys`,
		`bound_issuer`,
		`bound_audiences`,
		`openbao-internal`,
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected ConfigMap content to contain %q, got:\n%s", snippet, content)
		}
	}

	// Verify user requests are also included
	if !strings.Contains(content, `initialize "enable-stdout-audit"`) {
		t.Errorf("expected ConfigMap content to contain user request, got:\n%s", content)
	}
}

func TestEnsureSelfInitConfigMap_DevelopmentProfileWithBackupJWTAuthBootstraps(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")
	cluster.Spec.Profile = openbaov1alpha1.ProfileDevelopment
	cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
		ExecutorImage: "openbao/backup-executor:v0.1.0",
		Schedule:      "0 3 * * *",
		JWTAuthRole:   "backup",
		Target: openbaov1alpha1.BackupTarget{
			Endpoint:     "https://s3.amazonaws.com",
			Bucket:       "test-bucket",
			PathPrefix:   "backups",
			UsePathStyle: false,
		},
	}
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled:          true,
		BootstrapJWTAuth: true, // Opt-in to JWT bootstrap (required for backup JWT role creation)
		Requests: []openbaov1alpha1.SelfInitRequest{
			{
				Name:      "create-admin-policy",
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/policies/acl/admin",
				Data: &apiextensionsv1.JSON{
					Raw: []byte(`{"policy":"path \\\"*\\\" { capabilities = [\\\"create\\\"] }"}`),
				},
			},
		},
	}

	oidcIssuer := "https://kubernetes.default.svc"
	oidcJWTKeys := []string{"-----BEGIN PUBLIC KEY-----\ntest-public-key\n-----END PUBLIC KEY-----\n"}

	ctx := context.Background()
	logger := logr.Discard()
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", oidcIssuer, oidcJWTKeys)

	err := manager.ensureSelfInitConfigMap(ctx, logger, cluster)
	if err != nil {
		t.Fatalf("ensureSelfInitConfigMap() error = %v", err)
	}

	cmName := configInitMapName(cluster)
	configMap := &corev1.ConfigMap{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cmName,
	}, configMap)
	if err != nil {
		t.Fatalf("expected ConfigMap to exist: %v", err)
	}

	content := configMap.Data[configFileName]
	if content == "" {
		t.Fatal("expected ConfigMap to have config content")
	}

	expectedSnippets := []string{
		`initialize "operator-bootstrap"`,
		`request "enable-jwt-auth"`,
		`request "config-jwt-auth"`,
		`auth/jwt/role/backup`,
		`sys/policies/acl/backup`,
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected ConfigMap content to contain %q, got:\n%s", snippet, content)
		}
	}
}

func TestDeleteConfigMap(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")
	cmName := configMapName(cluster)

	existingConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cluster.Namespace,
		},
		Data: map[string]string{
			configFileName: "test config",
		},
	}

	ctx := context.Background()
	k8sClient := newTestClientWithObjects(t, existingConfigMap)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	err := manager.deleteConfigMap(ctx, cluster)
	if err != nil {
		t.Fatalf("deleteConfigMap() error = %v", err)
	}

	// Verify ConfigMap was deleted
	configMap := &corev1.ConfigMap{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cmName,
	}, configMap)

	if !apierrors.IsNotFound(err) {
		t.Errorf("expected ConfigMap to be deleted, got error: %v", err)
	}
}

func TestDeleteConfigMap_NotFound(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")

	ctx := context.Background()
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	// Should not error when ConfigMap doesn't exist
	err := manager.deleteConfigMap(ctx, cluster)
	if err != nil {
		t.Fatalf("deleteConfigMap() with missing ConfigMap should not error, got: %v", err)
	}
}

func TestDeleteSecrets(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")

	unsealSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      unsealSecretName(cluster),
			Namespace: cluster.Namespace,
		},
	}

	tlsCASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tlsCASecretName(cluster),
			Namespace: cluster.Namespace,
		},
	}

	tlsServerSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tlsServerSecretName(cluster),
			Namespace: cluster.Namespace,
		},
	}

	ctx := context.Background()
	k8sClient := newTestClientWithObjects(t, unsealSecret, tlsCASecret, tlsServerSecret)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	err := manager.deleteSecrets(ctx, cluster)
	if err != nil {
		t.Fatalf("deleteSecrets() error = %v", err)
	}

	// Verify all secrets were deleted
	secrets := []string{
		unsealSecretName(cluster),
		tlsCASecretName(cluster),
		tlsServerSecretName(cluster),
	}

	for _, secretName := range secrets {
		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      secretName,
		}, secret)

		if !apierrors.IsNotFound(err) {
			t.Errorf("expected Secret %s to be deleted, got error: %v", secretName, err)
		}
	}
}

func TestDeleteSecrets_PartialMissing(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")

	// Only create one secret
	unsealSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      unsealSecretName(cluster),
			Namespace: cluster.Namespace,
		},
	}

	ctx := context.Background()
	k8sClient := newTestClientWithObjects(t, unsealSecret)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	// Should not error when some secrets are missing
	err := manager.deleteSecrets(ctx, cluster)
	if err != nil {
		t.Fatalf("deleteSecrets() with missing secrets should not error, got: %v", err)
	}
}
