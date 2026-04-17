//go:build integration
// +build integration

package infra

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"hash/fnv"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
	appsv1 "k8s.io/api/apps/v1"
)

var (
	envTestClient client.Client
	envTestScheme *runtime.Scheme
	envTestStop   func()
)

func TestMain(m *testing.M) {
	ctrl.SetLogger(zap.New(zap.WriteTo(io.Discard)))

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		fmt.Fprintln(os.Stderr, "failed to add client-go scheme:", err)
		os.Exit(1)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		fmt.Fprintln(os.Stderr, "failed to add openbao scheme:", err)
		os.Exit(1)
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Use pre-configured binaries if available
	if assetsDir := getFirstFoundEnvTestBinaryDir(); assetsDir != "" {
		testEnv.BinaryAssetsDirectory = assetsDir
	}

	cfg, err := testEnv.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to start envtest:", err)
		os.Exit(1)
	}

	cfg.QPS = 20
	cfg.Burst = 40

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		_ = testEnv.Stop()
		fmt.Fprintln(os.Stderr, "failed to create client:", err)
		os.Exit(1)
	}

	// Ensure the default namespace exists (envtest doesn't guarantee it).
	ctx := context.Background()
	defaultNS := &corev1.Namespace{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, defaultNS); err != nil {
		if apierrors.IsNotFound(err) {
			if err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}); err != nil {
				_ = testEnv.Stop()
				fmt.Fprintln(os.Stderr, "failed to create default namespace:", err)
				os.Exit(1)
			}
		} else {
			_ = testEnv.Stop()
			fmt.Fprintln(os.Stderr, "failed to get default namespace:", err)
			os.Exit(1)
		}
	}

	// Seed the kubernetes service that NetworkPolicy detection requires.
	kubernetesService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubernetes",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "https",
					Port: 443,
				},
			},
		},
	}
	if err := k8sClient.Create(ctx, kubernetesService); err != nil && !apierrors.IsAlreadyExists(err) {
		_ = testEnv.Stop()
		fmt.Fprintln(os.Stderr, "failed to seed kubernetes service:", err)
		os.Exit(1)
	}

	envTestClient = k8sClient
	envTestScheme = scheme
	envTestStop = func() {
		if err := testEnv.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, "failed to stop envtest:", err)
		}
	}

	code := m.Run()
	envTestStop()
	os.Exit(code)
}

func envtestClientForPackage(t *testing.T) (client.Client, *runtime.Scheme) {
	t.Helper()
	if envTestClient == nil || envTestScheme == nil {
		t.Fatalf("envtest not initialized")
	}
	return envTestClient, envTestScheme
}

func testNamespace(t *testing.T) string {
	t.Helper()
	h := fnv.New32a()
	_, _ = h.Write([]byte(t.Name()))
	sum := fmt.Sprintf("%x", h.Sum32())
	// DNS1123 label: start with letter, <= 63 chars.
	ns := "t" + sum
	ns = strings.ToLower(ns)
	if len(ns) > 63 {
		ns = ns[:63]
	}
	return ns
}

// -----------------------------------------------------------------------------
// Test Helpers (Naming)
// -----------------------------------------------------------------------------

func tlsCASecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + "-tls-ca" // matches constants.SuffixTLSCA
}

// -----------------------------------------------------------------------------
// Integration Test Helpers (formerly helpers_integration_test.go)
// -----------------------------------------------------------------------------

func newTestClientWithObjects(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	builder := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithReturnManagedFields()
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
func getFirstFoundEnvTestBinaryDir() string {
	if assetsDir := os.Getenv("KUBEBUILDER_ASSETS"); assetsDir != "" {
		absoluteAssetsDir, err := filepath.Abs(assetsDir)
		if err != nil {
			return ""
		}
		return absoluteAssetsDir
	}

	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := filepath.Glob(filepath.Join(basePath, "*"))
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if info, err := filepath.Abs(entry); err == nil {
			if stat, err := os.Stat(info); err == nil && stat.IsDir() {
				return info
			}
		}
	}
	return ""
}

func newTestCACertPEM(t *testing.T) []byte {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey(ca) error = %v", err)
	}

	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-ca",
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(ca) error = %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}

// newTestStatefulSetSpec creates a minimal StatefulSetSpec for testing.
func newTestStatefulSetSpec(cluster *openbaov1alpha1.OpenBaoCluster) workloadsvc.StatefulSetSpec {
	return workloadsvc.StatefulSetSpec{
		Name:               cluster.Name,
		Revision:           "",
		Image:              cluster.Spec.Image,
		InitContainerImage: "",
		Replicas:           cluster.Spec.Replicas,
		ConfigHash:         "",
		DisableSelfInit:    false,
		SkipReconciliation: false,
	}
}

// createTLSSecretForTest creates a minimal TLS server secret for testing.
// This is needed because ensureStatefulSet now checks for prerequisite resources.
func createTLSSecretForTest(t *testing.T, k8sClient client.Client, cluster *openbaov1alpha1.OpenBaoCluster) {
	t.Helper()
	secretName := tlsServerSecretName(cluster)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("test-cert"),
			"tls.key": []byte("test-key"),
			"ca.crt":  []byte("test-ca"),
		},
	}
	if err := k8sClient.Create(context.Background(), secret); err != nil {
		t.Fatalf("failed to create TLS secret for test: %v", err)
	}
}

func createClusterCRForTest(t *testing.T, k8sClient client.Client, cluster *openbaov1alpha1.OpenBaoCluster) {
	t.Helper()
	ctx := context.Background()

	// Envtest does not implicitly create namespaces.
	nsName := cluster.GetNamespace()
	if nsName != "" {
		ns := &corev1.Namespace{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns); err != nil {
			if !apierrors.IsNotFound(err) {
				t.Fatalf("failed to get namespace %q for test: %v", nsName, err)
			}
			if err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}); err != nil {
				t.Fatalf("failed to create namespace %q for test: %v", nsName, err)
			}
		}
	}

	toCreate := cluster.DeepCopy()
	if toCreate.Spec.Profile == "" {
		toCreate.Spec.Profile = openbaov1alpha1.ProfileDevelopment
	}
	toCreate.Status = openbaov1alpha1.OpenBaoClusterStatus{}
	if err := k8sClient.Create(ctx, toCreate); err != nil {
		t.Fatalf("failed to create OpenBaoCluster for test: %v", err)
	}

	cluster.SetUID(toCreate.GetUID())
	cluster.SetResourceVersion(toCreate.GetResourceVersion())
}

func statefulSetName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name
}

// -----------------------------------------------------------------------------
// Deprecated / Test-Only Cleanup Functions (moved from config.go)
// -----------------------------------------------------------------------------

// deleteConfigMap finds and deletes the config map for the cluster.
// Used only in integration tests.
func deleteConfigMap(ctx context.Context, k8sClient client.Client, cluster *openbaov1alpha1.OpenBaoCluster) error {
	configMap := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      configMapName(cluster),
	}, configMap)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if err := k8sClient.Delete(ctx, configMap); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

// deleteSecrets removes all Secrets associated with the OpenBaoCluster.
// Used only in integration tests.
func deleteSecrets(ctx context.Context, k8sClient client.Client, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil {
		return nil
	}

	secretNames := []string{}

	// Only delete operator-owned Secrets.
	{
		mode := cluster.Spec.TLS.Mode
		if mode == "" {
			mode = openbaov1alpha1.TLSModeOperatorManaged
		}

		if cluster.Spec.TLS.Enabled && mode == openbaov1alpha1.TLSModeOperatorManaged {
			secretNames = append(secretNames, tlsServerSecretName(cluster), tlsCASecretName(cluster))
		}
	}

	// Only delete the unseal key Secret when using static unseal (operator-owned).
	{
		staticUnseal := cluster.Spec.Unseal == nil || cluster.Spec.Unseal.Type == "" || cluster.Spec.Unseal.Type == "static"
		if staticUnseal {
			secretNames = append(secretNames, unsealSecretName(cluster))
		}
	}

	// Delete by name without reading Secret contents
	for _, name := range secretNames {
		if name == "" {
			continue
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: cluster.Namespace,
			},
		}
		if err := k8sClient.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
