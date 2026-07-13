//go:build integration
// +build integration

package openbaocluster_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	security "github.com/dc-tec/openbao-operator/internal/adapter/security"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/controller/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portsecurity "github.com/dc-tec/openbao-operator/internal/port/security"
)

func TestSetupWithManager_ReconcilesClusterInSingleTenantMode(t *testing.T) {
	ctx := context.Background()
	namespace := "single-tenant-test"
	liveClient := startOpenBaoClusterManager(t, namespace, true)

	cluster := newManagerTestCluster(namespace, "single-tenant-cluster")
	require.NoError(t, liveClient.Create(ctx, cluster))
	t.Cleanup(func() {
		_ = liveClient.Delete(context.Background(), cluster)
	})

	waitForClusterFinalizer(t, ctx, liveClient, cluster)
	waitForManagedConfigMap(t, ctx, liveClient, namespace, cluster.Name)
}

func TestSetupWithManager_ReconcilesClusterInMultiTenantMode(t *testing.T) {
	ctx := context.Background()
	namespace := "multi-tenant-test"
	liveClient := startOpenBaoClusterManager(t, namespace, false)

	cluster := newManagerTestCluster(namespace, "multi-tenant-cluster")
	require.NoError(t, liveClient.Create(ctx, cluster))
	t.Cleanup(func() {
		_ = liveClient.Delete(context.Background(), cluster)
	})

	waitForClusterFinalizer(t, ctx, liveClient, cluster)
	waitForManagedConfigMap(t, ctx, liveClient, namespace, cluster.Name)
}

func TestSetupWithManager_SingleTenantRecreatesDeletedConfigMap(t *testing.T) {
	ctx := context.Background()
	namespace := "single-tenant-watch-test"
	liveClient := startOpenBaoClusterManager(t, namespace, true)

	cluster := newManagerTestCluster(namespace, "watch-recreate-cluster")
	require.NoError(t, liveClient.Create(ctx, cluster))
	t.Cleanup(func() {
		_ = liveClient.Delete(context.Background(), cluster)
	})

	configMap := waitForManagedConfigMap(t, ctx, liveClient, namespace, cluster.Name)
	originalUID := configMap.UID

	require.NoError(t, liveClient.Delete(ctx, configMap))
	waitForNotFound(t, ctx, liveClient, configMap)

	configMapKey := client.ObjectKey{Namespace: namespace, Name: cluster.Name + constants.SuffixConfigMap}
	require.Eventually(t, func() bool {
		current := &corev1.ConfigMap{}
		if err := liveClient.Get(ctx, configMapKey, current); err != nil {
			return false
		}
		return current.UID != originalUID
	}, 20*time.Second, 200*time.Millisecond, "expected single-tenant Owns() watch to recreate deleted ConfigMap")
}

func startOpenBaoClusterManager(t *testing.T, namespace string, singleTenant bool) client.Client {
	t.Helper()
	t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "true")

	scheme := newIntegrationScheme(t)
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "test", "manifests", "gateway-api", "v1.5.1", "crds"),
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

	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}
	require.NoError(t, liveClient.Create(context.Background(), testNamespace))
	t.Cleanup(func() {
		_ = liveClient.Delete(context.Background(), testNamespace)
	})
	if !singleTenant {
		require.NoError(t, ensureProvisionedTenantNamespace(context.Background(), liveClient, namespace))
	}

	mgrOptions := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	}
	skipNameValidation := true
	mgrOptions.Controller.SkipNameValidation = &skipNameValidation
	if singleTenant {
		mgrOptions.Cache = cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				namespace: {},
			},
		}
	}

	mgr, err := ctrl.NewManager(cfg, mgrOptions)
	require.NoError(t, err)

	imageVerifier := security.NewImageVerifier(logr.Discard(), mgr.GetClient(), nil)
	reconciler := &openbaocluster.OpenBaoClusterReconciler{
		Client: mgr.GetClient(),
		ControllerRuntime: openbaocluster.ControllerRuntime{
			APIReader:        mgr.GetAPIReader(),
			Recorder:         mgr.GetEventRecorder("openbaocluster-test"),
			SingleTenantMode: singleTenant,
		},
		Applications: appopenbaocluster.NewRuntimeApplications(appopenbaocluster.RuntimeApplicationsConfig{
			Kubernetes: appopenbaocluster.RuntimeKubernetesConfig{
				Client:            mgr.GetClient(),
				APIReader:         mgr.GetAPIReader(),
				Scheme:            mgr.GetScheme(),
				RestConfig:        cfg,
				OperatorNamespace: "openbao-operator-system",
				Recorder:          mgr.GetEventRecorder("openbaocluster-test"),
			},
			ImageVerification: appopenbaocluster.RuntimeImageVerificationConfig{
				ImageVerifier:         imageVerifier,
				OperatorImageVerifier: imageVerifier,
				Infra: appopenbaocluster.InfraImageVerificationRuntime{
					OperatorImageVerifier:              imageVerifier,
					VerifyOperatorImage:                portsecurity.VerifyOperatorImageForCluster,
					IsMainImageVerificationEnabled:     portsecurity.IsMainImageVerificationEnabled,
					IsOperatorImageVerificationEnabled: portsecurity.IsOperatorImageVerificationEnabled,
				},
			},
		}),
	}
	require.NoError(t, reconciler.SetupWithManager(mgr))

	managerCtx, cancel := context.WithCancel(context.Background())
	managerErr := make(chan error, 1)
	go func() {
		managerErr <- mgr.Start(managerCtx)
	}()
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

func ensureProvisionedTenantNamespace(ctx context.Context, c client.Client, namespace string) error {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.TenantRoleBindingName,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     constants.TenantRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "openbao-operator-controller",
				Namespace: "openbao-operator-system",
			},
		},
	}
	err := c.Create(ctx, roleBinding)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func newIntegrationScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	require.NoError(t, gatewayv1.Install(scheme))
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

func newManagerTestCluster(namespace, name string) *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile:  openbaov1alpha1.ProfileDevelopment,
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 1,
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
			TLS: openbaov1alpha1.TLSConfig{
				Enabled:        true,
				RotationPeriod: "720h",
			},
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
			},
			InitContainer: &openbaov1alpha1.InitContainerConfig{
				Image: "openbao/openbao-init:latest",
			},
		},
	}
}

func waitForClusterFinalizer(t *testing.T, ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) {
	t.Helper()

	clusterKey := client.ObjectKeyFromObject(cluster)
	require.Eventually(t, func() bool {
		current := &openbaov1alpha1.OpenBaoCluster{}
		if err := c.Get(ctx, clusterKey, current); err != nil {
			return false
		}
		return slices.Contains(current.Finalizers, openbaov1alpha1.OpenBaoClusterFinalizer)
	}, 20*time.Second, 200*time.Millisecond, "expected manager-driven reconcile to add the finalizer")
}

func waitForManagedConfigMap(t *testing.T, ctx context.Context, c client.Client, namespace, clusterName string) *corev1.ConfigMap {
	t.Helper()

	configMapKey := client.ObjectKey{
		Namespace: namespace,
		Name:      clusterName + constants.SuffixConfigMap,
	}
	current := &corev1.ConfigMap{}
	require.Eventually(t, func() bool {
		if err := c.Get(ctx, configMapKey, current); err != nil {
			return false
		}
		_, hasConfig := current.Data["config.hcl"]
		return hasConfig
	}, 20*time.Second, 200*time.Millisecond, "expected manager-driven reconcile to create the managed ConfigMap")

	result := current.DeepCopy()
	require.NoError(t, c.Get(ctx, configMapKey, result))
	return result
}

func waitForNotFound(t *testing.T, ctx context.Context, c client.Client, obj client.Object) {
	t.Helper()

	key := client.ObjectKeyFromObject(obj)
	require.Eventually(t, func() bool {
		current := obj.DeepCopyObject().(client.Object)
		err := c.Get(ctx, key, current)
		return apierrors.IsNotFound(err)
	}, 10*time.Second, 200*time.Millisecond, "expected object %s/%s to be deleted", key.Namespace, key.Name)
}
