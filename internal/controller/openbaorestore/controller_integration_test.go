//go:build integration
// +build integration

package openbaorestore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	openbaorestorecontroller "github.com/dc-tec/openbao-operator/internal/controller/openbaorestore"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
)

func TestOpenBaoRestore_SetupWithManager_InitializesRestoreStatusFromPending(t *testing.T) {
	setAdmissionReady(t)

	ctx := context.Background()
	liveClient := startOpenBaoRestoreManager(t)

	const namespace = "restore-test"
	createNamespace(t, ctx, liveClient, namespace)

	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-validation",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "https://objectstore.example.com",
					Bucket:   "backups",
				},
				Key: "snapshots/backup.snap",
			},
		},
	}
	require.NoError(t, liveClient.Create(ctx, restore))

	restoreKey := types.NamespacedName{Namespace: restore.Namespace, Name: restore.Name}
	require.Eventually(t, func() bool {
		current := &openbaov1alpha1.OpenBaoRestore{}
		if err := liveClient.Get(ctx, restoreKey, current); err != nil {
			return false
		}
		return current.Status.Phase != "" &&
			current.Status.Phase != openbaov1alpha1.RestorePhasePending &&
			current.Status.StartTime != nil &&
			current.Status.SnapshotKey == "snapshots/backup.snap" &&
			slices.Contains(current.Finalizers, openbaov1alpha1.OpenBaoRestoreFinalizer)
	}, 20*time.Second, 200*time.Millisecond, "expected manager-driven reconcile to initialize restore status and leave pending phase")
}

func setAdmissionReady(t *testing.T) {
	t.Helper()

	t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "")
	admission.SetAdmissionDependenciesReady(true)
	t.Cleanup(func() {
		admission.SetAdmissionDependenciesReady(false)
	})
}

func startOpenBaoRestoreManager(t *testing.T) client.Client {
	t.Helper()

	scheme := newIntegrationScheme(t)
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}
	if assetsDir := getFirstFoundEnvTestBinaryDir(); assetsDir != "" {
		testEnv.BinaryAssetsDirectory = assetsDir
	}

	cfg, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, testEnv.Stop())
	})

	liveClient, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err)

	mgrOptions := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	}
	skipNameValidation := true
	mgrOptions.Controller.SkipNameValidation = &skipNameValidation

	mgr, err := ctrl.NewManager(cfg, mgrOptions)
	require.NoError(t, err)

	reconciler := &openbaorestorecontroller.OpenBaoRestoreReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	require.NoError(t, reconciler.SetupWithManager(mgr))

	managerCtx, cancel := context.WithCancel(context.Background())
	managerErr := make(chan error, 1)
	go func() {
		managerErr <- mgr.Start(managerCtx)
	}()
	require.True(t, mgr.GetCache().WaitForCacheSync(managerCtx), "expected manager cache to sync before creating test resources")
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-managerErr:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("manager stopped with error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("manager did not stop within timeout")
		}
	})

	return liveClient
}

func newIntegrationScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	return scheme
}

func getFirstFoundEnvTestBinaryDir() string {
	if assetsDir := os.Getenv("KUBEBUILDER_ASSETS"); assetsDir != "" {
		absoluteAssetsDir, err := filepath.Abs(assetsDir)
		if err != nil {
			return ""
		}
		return absoluteAssetsDir
	}

	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			assetsDir, err := filepath.Abs(filepath.Join(basePath, entry.Name()))
			if err != nil {
				return ""
			}
			return assetsDir
		}
	}
	return ""
}

func createNamespace(t *testing.T, ctx context.Context, c client.Client, name string) {
	t.Helper()

	require.NoError(t, c.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}))
}
