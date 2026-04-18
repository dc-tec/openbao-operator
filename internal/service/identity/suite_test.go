//go:build integration
// +build integration

package identity

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
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

	if assetsDir := getFirstFoundEnvTestBinaryDir(); assetsDir != "" {
		testEnv.BinaryAssetsDirectory = assetsDir
	}

	cfg, err := testEnv.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to start envtest:", err)
		os.Exit(1)
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		_ = testEnv.Stop()
		fmt.Fprintln(os.Stderr, "failed to create client:", err)
		os.Exit(1)
	}

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
	ns := "t" + sum
	ns = strings.ToLower(ns)
	if len(ns) > 63 {
		ns = ns[:63]
	}
	return ns
}

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

func createClusterCRForTest(t *testing.T, k8sClient client.Client, cluster *openbaov1alpha1.OpenBaoCluster) {
	t.Helper()
	ctx := context.Background()

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
