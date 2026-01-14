//go:build integration
// +build integration

package integration

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/infra"
)

func TestInfraNetwork_GatewayAPICRDsMissing_HTTPRouteMode_IsDegraded(t *testing.T) {
	ctx := context.Background()

	k8sClient, scheme := startIsolatedEnv(t, isolatedEnvOptions{
		crdDirs: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
	})

	namespace := createNamespace(t, ctx, k8sClient)

	cluster := newMinimalClusterObj(namespace, "gw-crd-missing-httproute")
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled: true,
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name: "traefik-gateway",
		},
		Hostname: "bao.example.local",
	}
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}
	createTLSSecretForClient(t, ctx, k8sClient, namespace, cluster.Name)

	mgr := infra.NewManager(k8sClient, scheme, "openbao-operator-system", "", nil)
	err := mgr.Reconcile(ctx, logr.Discard(), cluster, "", "")
	if err == nil {
		t.Fatalf("expected reconcile to fail with ErrGatewayAPIMissing, got nil")
	}
	if !errors.Is(err, infra.ErrGatewayAPIMissing) {
		t.Fatalf("expected ErrGatewayAPIMissing, got %T: %v", err, err)
	}
}

func TestInfraNetwork_GatewayAPICRDsMissing_TLSPassthroughMode_IsDegraded(t *testing.T) {
	ctx := context.Background()

	k8sClient, scheme := startIsolatedEnv(t, isolatedEnvOptions{
		crdDirs: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
	})

	namespace := createNamespace(t, ctx, k8sClient)

	cluster := newMinimalClusterObj(namespace, "gw-crd-missing-tlsroute")
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled: true,
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name: "traefik-gateway",
		},
		Hostname:       "bao.example.local",
		TLSPassthrough: true,
	}
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}
	createTLSSecretForClient(t, ctx, k8sClient, namespace, cluster.Name)

	mgr := infra.NewManager(k8sClient, scheme, "openbao-operator-system", "", nil)
	err := mgr.Reconcile(ctx, logr.Discard(), cluster, "", "")
	if err == nil {
		t.Fatalf("expected reconcile to fail with ErrGatewayAPIMissing, got nil")
	}
	if !errors.Is(err, infra.ErrGatewayAPIMissing) {
		t.Fatalf("expected ErrGatewayAPIMissing, got %T: %v", err, err)
	}
}

func TestInfraNetwork_GatewayAPIBackendTLSPolicyCRDMissing_IsDegraded(t *testing.T) {
	ctx := context.Background()

	gatewayCRDsWithoutBackendTLS := copyGatewayCRDsExcluding(t, []string{"backendtlspolicies"})

	k8sClient, scheme := startIsolatedEnv(t, isolatedEnvOptions{
		crdDirs: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			gatewayCRDsWithoutBackendTLS,
		},
	})

	namespace := createNamespace(t, ctx, k8sClient)

	cluster := newMinimalClusterObj(namespace, "gw-crd-missing-backend-tls")
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled: true,
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name: "traefik-gateway",
		},
		Hostname: "bao.example.local",
		BackendTLS: &openbaov1alpha1.BackendTLSConfig{
			Enabled: boolPtr(true),
		},
	}
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}
	createTLSSecretForClient(t, ctx, k8sClient, namespace, cluster.Name)
	createCASecretForClient(t, ctx, k8sClient, namespace, cluster.Name, []byte("ca-1"))

	mgr := infra.NewManager(k8sClient, scheme, "openbao-operator-system", "", nil)
	err := mgr.Reconcile(ctx, logr.Discard(), cluster, "", "")
	if err == nil {
		t.Fatalf("expected reconcile to fail with ErrGatewayAPIMissing, got nil")
	}
	if !errors.Is(err, infra.ErrGatewayAPIMissing) {
		t.Fatalf("expected ErrGatewayAPIMissing, got %T: %v", err, err)
	}
}

type isolatedEnvOptions struct {
	crdDirs []string
}

func startIsolatedEnv(t *testing.T, opts isolatedEnvOptions) (client.Client, *runtime.Scheme) {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)
	_ = gatewayv1alpha2.Install(scheme)

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     opts.crdDirs,
		ErrorIfCRDPathMissing: true,
	}
	if assetsDir := getFirstFoundEnvTestBinaryDir(); assetsDir != "" {
		testEnv.BinaryAssetsDirectory = assetsDir
	}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() {
		_ = testEnv.Stop()
	})

	cfg.QPS = 20
	cfg.Burst = 40

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	seedKubernetesServiceForClient(t, context.Background(), k8sClient)

	return k8sClient, scheme
}

func seedKubernetesServiceForClient(t *testing.T, ctx context.Context, c client.Client) {
	t.Helper()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubernetes",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "https", Port: 443},
			},
		},
	}
	_ = c.Create(ctx, svc)
}

func createNamespace(t *testing.T, ctx context.Context, c client.Client) string {
	t.Helper()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "it-gw-crd-missing-",
		},
	}
	if err := c.Create(ctx, ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Delete(context.Background(), ns)
	})
	return ns.Name
}

func createTLSSecretForClient(t *testing.T, ctx context.Context, c client.Client, namespace, clusterName string) {
	t.Helper()

	secretName := clusterName + constants.SuffixTLSServer
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("test-cert"),
			"tls.key": []byte("test-key"),
			"ca.crt":  []byte("test-ca"),
		},
	}
	if err := c.Create(ctx, secret); err != nil {
		t.Fatalf("create TLS secret: %v", err)
	}
}

func createCASecretForClient(t *testing.T, ctx context.Context, c client.Client, namespace, clusterName string, caPEM []byte) {
	t.Helper()

	secretName := clusterName + constants.SuffixTLSCA
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt": caPEM,
		},
	}
	if err := c.Create(ctx, secret); err != nil {
		t.Fatalf("create CA secret: %v", err)
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func copyGatewayCRDsExcluding(t *testing.T, excludeSubstrings []string) string {
	t.Helper()

	srcDir := filepath.Join("..", "manifests", "gateway-api", "v1.4.1", "crds")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("read gateway CRD dir %q: %v", srcDir, err)
	}

	dstDir := t.TempDir()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		skip := false
		for _, s := range excludeSubstrings {
			if strings.Contains(strings.ToLower(name), strings.ToLower(s)) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		srcPath := filepath.Join(srcDir, name)
		dstPath := filepath.Join(dstDir, name)

		data, readErr := os.ReadFile(srcPath)
		if readErr != nil {
			t.Fatalf("read %q: %v", srcPath, readErr)
		}

		mode := fs.FileMode(0o644)
		if info, statErr := os.Stat(srcPath); statErr == nil {
			mode = info.Mode()
		}

		if writeErr := os.WriteFile(dstPath, data, mode); writeErr != nil {
			t.Fatalf("write %q: %v", dstPath, writeErr)
		}
	}

	return dstDir
}
