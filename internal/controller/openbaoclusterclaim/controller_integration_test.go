//go:build integration
// +build integration

package openbaoclusterclaim

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/connectionpublishing"
)

func TestSetupWithManager_RecreatesDeletedClaimManagedLocalCluster(t *testing.T) {
	ctx := context.Background()
	namespace := "claim-watch-recreate"
	liveClient := startOpenBaoClusterClaimManager(t)
	createClaimTestNamespace(t, ctx, liveClient, namespace)

	createSameClusterBaselineCatalog(t, ctx, liveClient)
	createClaimTenant(t, ctx, liveClient, namespace)

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments-bao",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-v1"},
		},
	}
	require.NoError(t, liveClient.Create(ctx, claim))

	clusterKey := types.NamespacedName{Namespace: namespace, Name: claim.Name}
	original := waitForLocalCluster(t, ctx, liveClient, clusterKey)
	originalUID := original.UID

	require.NoError(t, liveClient.Delete(ctx, original))
	require.Eventually(t, func() bool {
		current := &openbaov1alpha1.OpenBaoCluster{}
		return apierrors.IsNotFound(liveClient.Get(ctx, clusterKey, current))
	}, 20*time.Second, 200*time.Millisecond, "expected local OpenBaoCluster to be deleted before recreation")

	require.Eventually(t, func() bool {
		current := &openbaov1alpha1.OpenBaoCluster{}
		if err := liveClient.Get(ctx, clusterKey, current); err != nil {
			return false
		}
		return current.UID != "" && current.UID != originalUID
	}, 30*time.Second, 200*time.Millisecond, "expected claim-managed OpenBaoCluster watch to recreate deleted workload")
}

func TestSetupWithManager_PublishesIngressConnectionFromClusterWatch(t *testing.T) {
	ctx := context.Background()
	namespace := "claim-ingress-publication"
	liveClient := startOpenBaoClusterClaimManager(t)
	createClaimTestNamespace(t, ctx, liveClient, namespace)

	createSameClusterIngressCatalog(t, ctx, liveClient)
	createClaimTenant(t, ctx, liveClient, namespace)

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments-bao",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-ingress-v1"},
		},
	}
	require.NoError(t, liveClient.Create(ctx, claim))

	clusterKey := types.NamespacedName{Namespace: namespace, Name: claim.Name}
	_ = waitForLocalCluster(t, ctx, liveClient, clusterKey)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         namespace,
			Name:              connectionpublishing.LocalPublicServiceName(claim.Name),
			CreationTimestamp: metav1.NewTime(time.Date(2026, time.April, 20, 17, 0, 0, 0, time.UTC)),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name: "https",
				Port: constants.PortAPI,
			}},
		},
	}
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         namespace,
			Name:              connectionpublishing.LocalCASecretName(claim.Name),
			CreationTimestamp: metav1.NewTime(time.Date(2026, time.April, 20, 18, 0, 0, 0, time.UTC)),
		},
		Data: map[string][]byte{
			"ca.crt": []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"),
		},
	}
	require.NoError(t, liveClient.Create(ctx, service))
	require.NoError(t, liveClient.Create(ctx, caSecret))

	var current openbaov1alpha1.OpenBaoCluster
	require.NoError(t, liveClient.Get(ctx, clusterKey, &current))
	current.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
	current.Status.Conditions = []metav1.Condition{{
		Type:               string(openbaov1alpha1.ConditionIngressIntegrationReady),
		Status:             metav1.ConditionTrue,
		Reason:             "IngressIntegrationReady",
		LastTransitionTime: metav1.NewTime(time.Now().UTC()),
	}}
	require.NoError(t, liveClient.Status().Update(ctx, &current))

	claimKey := client.ObjectKeyFromObject(claim)
	require.Eventually(t, func() bool {
		updated := &openbaov1alpha1.OpenBaoClusterClaim{}
		if err := liveClient.Get(ctx, claimKey, updated); err != nil {
			return false
		}
		return updated.Status.Connection.Endpoint == "https://payments-bao.example.internal"
	}, 30*time.Second, 200*time.Millisecond, "expected cluster watch to publish ingress endpoint")

	secretKey := types.NamespacedName{Namespace: namespace, Name: connectionpublishing.SecretName(claim.Name)}
	require.Eventually(t, func() bool {
		secret := &corev1.Secret{}
		if err := liveClient.Get(ctx, secretKey, secret); err != nil {
			return false
		}
		return string(secret.Data["endpoint"]) == "https://payments-bao.example.internal"
	}, 20*time.Second, 200*time.Millisecond, "expected claim-owned connection secret to be published")
}

func TestSetupWithManager_CleansProjectedBootstrapArtifactsOnClaimDeletion(t *testing.T) {
	ctx := context.Background()
	namespace := "claim-bootstrap-cleanup"
	liveClient := startOpenBaoClusterClaimManager(t)
	createClaimTestNamespace(t, ctx, liveClient, namespace)

	createSameClusterSecretBootstrapCatalog(t, ctx, liveClient)
	createClaimTenant(t, ctx, liveClient, namespace)
	require.NoError(t, liveClient.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubernetes-auth-default",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"default_role":       []byte("operator"),
			"token_reviewer_jwt": []byte("secret-token"),
		},
	}))

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments-bao",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-configref-v1"},
		},
	}
	require.NoError(t, liveClient.Create(ctx, claim))

	cluster := waitForLocalCluster(t, ctx, liveClient, types.NamespacedName{Namespace: namespace, Name: claim.Name})
	projectedRef := waitForProjectedBootstrapSecretRef(t, cluster)
	projectedKey := types.NamespacedName{Namespace: namespace, Name: projectedRef.Name}

	projected := &corev1.Secret{}
	require.Eventually(t, func() bool {
		return liveClient.Get(ctx, projectedKey, projected) == nil
	}, 20*time.Second, 200*time.Millisecond, "expected projected bootstrap secret to exist")

	require.NoError(t, liveClient.Delete(ctx, claim))

	require.Eventually(t, func() bool {
		currentClaim := &openbaov1alpha1.OpenBaoClusterClaim{}
		return apierrors.IsNotFound(liveClient.Get(ctx, client.ObjectKeyFromObject(claim), currentClaim))
	}, 30*time.Second, 200*time.Millisecond, "expected claim to be deleted")
	require.Eventually(t, func() bool {
		currentCluster := &openbaov1alpha1.OpenBaoCluster{}
		return apierrors.IsNotFound(liveClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: claim.Name}, currentCluster))
	}, 30*time.Second, 200*time.Millisecond, "expected local OpenBaoCluster to be deleted")
	require.Eventually(t, func() bool {
		secret := &corev1.Secret{}
		return apierrors.IsNotFound(liveClient.Get(ctx, projectedKey, secret))
	}, 30*time.Second, 200*time.Millisecond, "expected projected bootstrap artifact to be cleaned up")
}

func TestSetupWithManager_ReactsWhenReferencedTenantAppears(t *testing.T) {
	ctx := context.Background()
	operatorNamespace := "claim-tenant-watch-operator"
	targetNamespace := "claim-tenant-watch-target"
	liveClient := startOpenBaoClusterClaimManager(t)
	createClaimTestNamespace(t, ctx, liveClient, operatorNamespace)
	createClaimTestNamespace(t, ctx, liveClient, targetNamespace)

	createSameClusterBaselineCatalog(t, ctx, liveClient)

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments-bao",
			Namespace: operatorNamespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-v1"},
		},
	}
	require.NoError(t, liveClient.Create(ctx, claim))

	require.NoError(t, liveClient.Create(ctx, &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments",
			Namespace: operatorNamespace,
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{
			TargetNamespace: targetNamespace,
		},
	}))

	clusterKey := types.NamespacedName{Namespace: targetNamespace, Name: claim.Name}
	require.Eventually(t, func() bool {
		current := &openbaov1alpha1.OpenBaoCluster{}
		return liveClient.Get(ctx, clusterKey, current) == nil
	}, 30*time.Second, 200*time.Millisecond, "expected tenant watch to requeue claim after tenant creation")
}

func startOpenBaoClusterClaimManager(t *testing.T) client.Client {
	t.Helper()

	scheme := newClaimIntegrationScheme(t)
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}
	if assetsDir := getFirstFoundClaimEnvTestBinaryDir(); assetsDir != "" {
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

	reconciler := &OpenBaoClusterClaimReconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		EnableServiceClaims: true,
	}
	require.NoError(t, reconciler.SetupWithManager(mgr))

	managerCtx, cancel := context.WithCancel(context.Background())
	managerErr := make(chan error, 1)
	go func() {
		managerErr <- mgr.Start(managerCtx)
	}()
	require.True(t, mgr.GetCache().WaitForCacheSync(managerCtx), "expected claim controller cache to sync before creating test resources")
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

func newClaimIntegrationScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	return scheme
}

func getFirstFoundClaimEnvTestBinaryDir() string {
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

func createClaimTestNamespace(t *testing.T, ctx context.Context, c client.Client, namespace string) {
	t.Helper()

	require.NoError(t, c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}))
	t.Cleanup(func() {
		_ = c.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	})
}

func createClaimTenant(t *testing.T, ctx context.Context, c client.Client, namespace string) {
	t.Helper()

	require.NoError(t, c.Create(ctx, &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{
			TargetNamespace: namespace,
		},
	}))
}

func createSameClusterBaselineCatalog(t *testing.T, ctx context.Context, c client.Client) {
	t.Helper()

	for _, obj := range []client.Object{
		sameClusterBaselineServiceProfile(),
		sameClusterBaselineBootstrapProfile(),
		sameClusterBaselineExposureClass(),
		sameClusterBaselineBackupProfile(),
	} {
		require.NoError(t, c.Create(ctx, obj))
	}
}

func createSameClusterIngressCatalog(t *testing.T, ctx context.Context, c client.Client) {
	t.Helper()

	for _, obj := range []client.Object{
		sameClusterIngressServiceProfileFixture(),
		sameClusterBaselineBootstrapProfile(),
		sameClusterIngressExposureClassFixture(),
		sameClusterIngressEntrypointFixture(),
		sameClusterIngressPolicyFixture(),
		sameClusterBaselineBackupProfile(),
	} {
		require.NoError(t, c.Create(ctx, obj))
	}
}

func createSameClusterSecretBootstrapCatalog(t *testing.T, ctx context.Context, c client.Client) {
	t.Helper()

	for _, obj := range []client.Object{
		sameClusterSecretConfigRefServiceProfileFixture(),
		sameClusterSecretConfigRefBootstrapProfileFixture(),
		sameClusterBaselineExposureClass(),
		sameClusterBaselineBackupProfile(),
	} {
		require.NoError(t, c.Create(ctx, obj))
	}
}

func waitForLocalCluster(t *testing.T, ctx context.Context, c client.Client, key types.NamespacedName) *openbaov1alpha1.OpenBaoCluster {
	t.Helper()

	current := &openbaov1alpha1.OpenBaoCluster{}
	require.Eventually(t, func() bool {
		return c.Get(ctx, key, current) == nil
	}, 30*time.Second, 200*time.Millisecond, "expected same-cluster OpenBaoCluster to be materialized")
	return current.DeepCopy()
}

func waitForProjectedBootstrapSecretRef(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster) *openbaov1alpha1.TypedObjectReference {
	t.Helper()

	if cluster.Spec.SelfInit == nil {
		t.Fatal("cluster spec selfInit = nil, want projected bootstrap refs")
	}
	for _, req := range cluster.Spec.SelfInit.Requests {
		if req.AuthMethod != nil && req.AuthMethod.ConfigFromRef != nil {
			return req.AuthMethod.ConfigFromRef.DeepCopy()
		}
	}
	t.Fatal("expected projected bootstrap secret ref in self-init requests")
	return nil
}

func sameClusterBaselineServiceProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	readReplicas := int32(1)
	preUpgradeSnapshot := false

	return &openbaov1alpha1.OpenBaoServiceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-ha-v1"},
		Spec: openbaov1alpha1.OpenBaoServiceProfileSpec{
			Cluster: openbaov1alpha1.OpenBaoServiceProfileClusterSpec{
				Version:         "2.6.0",
				Voters:          3,
				ReadReplicas:    &readReplicas,
				SecurityProfile: openbaov1alpha1.ProfileDevelopment,
			},
			Storage: openbaov1alpha1.OpenBaoServiceProfileStorageSpec{
				PrimarySize:     "20Gi",
				ReadReplicaSize: "10Gi",
			},
			Bootstrap: openbaov1alpha1.OpenBaoServiceProfileBootstrapSpec{
				Mode:       openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
				ProfileRef: &openbaov1alpha1.LocalReference{Name: "oidc-standard-users-v1"},
			},
			Exposure: openbaov1alpha1.OpenBaoServiceProfileExposureSpec{
				ClassRef: openbaov1alpha1.LocalReference{Name: "internal-tls-v1"},
			},
			Backup: openbaov1alpha1.OpenBaoServiceProfileBackupSpec{
				ProfileRef: openbaov1alpha1.LocalReference{Name: "standard-daily-v1"},
			},
			Lifecycle: openbaov1alpha1.OpenBaoServiceProfileLifecycleSpec{
				UpgradeStrategy:    openbaov1alpha1.UpdateStrategyRollingUpdate,
				PreUpgradeSnapshot: &preUpgradeSnapshot,
			},
		},
	}
}

func sameClusterSecretConfigRefServiceProfileFixture() *openbaov1alpha1.OpenBaoServiceProfile {
	profile := sameClusterBaselineServiceProfile()
	profile.Name = "standard-ha-configref-v1"
	return profile
}

func sameClusterIngressServiceProfileFixture() *openbaov1alpha1.OpenBaoServiceProfile {
	profile := sameClusterBaselineServiceProfile()
	profile.Name = "standard-ha-ingress-v1"
	profile.Spec.Exposure.ClassRef = openbaov1alpha1.LocalReference{Name: "edge-ingress-v1"}
	return profile
}

func sameClusterBaselineBootstrapProfile() *openbaov1alpha1.OpenBaoBootstrapProfile {
	return &openbaov1alpha1.OpenBaoBootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "oidc-standard-users-v1"},
		Spec: openbaov1alpha1.OpenBaoBootstrapProfileSpec{
			OperatorLifecycleAuth: openbaov1alpha1.OpenBaoBootstrapLifecycleAuthSpec{
				Mode: openbaov1alpha1.OpenBaoBootstrapLifecycleAuthModeJWT,
				JWT:  &openbaov1alpha1.OpenBaoBootstrapLifecycleJWTSpec{Audience: "openbao-operator"},
			},
			SecretEngines: &openbaov1alpha1.OpenBaoBootstrapSecretEnginesSpec{
				Mounts: []openbaov1alpha1.OpenBaoBootstrapSecretEngineMountSpec{{
					Type: "kv",
					Path: "secret",
				}},
			},
		},
	}
}

func sameClusterSecretConfigRefBootstrapProfileFixture() *openbaov1alpha1.OpenBaoBootstrapProfile {
	profile := sameClusterBaselineBootstrapProfile()
	profile.Spec.Auth = &openbaov1alpha1.OpenBaoBootstrapAuthSpec{
		Methods: []openbaov1alpha1.OpenBaoBootstrapAuthMethodSpec{{
			Type: "kubernetes",
			Path: "kubernetes",
			ConfigRef: &openbaov1alpha1.TypedObjectReference{
				Kind: "Secret",
				Name: "kubernetes-auth-default",
			},
		}},
	}
	return profile
}

func sameClusterBaselineExposureClass() *openbaov1alpha1.OpenBaoExposureClass {
	return &openbaov1alpha1.OpenBaoExposureClass{
		ObjectMeta: metav1.ObjectMeta{Name: "internal-tls-v1"},
		Spec: openbaov1alpha1.OpenBaoExposureClassSpec{
			PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeClusterInternal,
			HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
				Mode: openbaov1alpha1.OpenBaoExposureHostnamePolicyModeGenerated,
			},
			ServicePolicy: &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
				Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
				BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
			},
		},
	}
}

func sameClusterIngressExposureClassFixture() *openbaov1alpha1.OpenBaoExposureClass {
	return &openbaov1alpha1.OpenBaoExposureClass{
		ObjectMeta: metav1.ObjectMeta{Name: "edge-ingress-v1"},
		Spec: openbaov1alpha1.OpenBaoExposureClassSpec{
			PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeIngress,
			HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
				Mode:         openbaov1alpha1.OpenBaoExposureHostnamePolicyModeGenerated,
				DomainSuffix: "example.internal",
			},
			TLSPolicy: &openbaov1alpha1.OpenBaoExposureTLSPolicySpec{
				Mode:       openbaov1alpha1.OpenBaoExposureTLSModeOperatorManaged,
				MinVersion: openbaov1alpha1.OpenBaoExposureTLSMinimumVersionTLS12,
			},
			EntrypointRef:    &openbaov1alpha1.LocalReference{Name: "internal-ingress-v1"},
			IngressPolicyRef: &openbaov1alpha1.LocalReference{Name: "nginx-backend-tls-v1"},
			Routing: &openbaov1alpha1.OpenBaoExposureRoutingSpec{
				Path: "/",
			},
			ServicePolicy: &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
				Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
				BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
			},
		},
	}
}

func sameClusterIngressEntrypointFixture() *openbaov1alpha1.OpenBaoEntrypoint {
	return &openbaov1alpha1.OpenBaoEntrypoint{
		ObjectMeta: metav1.ObjectMeta{Name: "internal-ingress-v1"},
		Spec: openbaov1alpha1.OpenBaoEntrypointSpec{
			Mode: openbaov1alpha1.OpenBaoEntrypointModeIngress,
			ObjectRef: openbaov1alpha1.OpenBaoEntrypointObjectReference{
				APIGroup:  "networking.k8s.io",
				Kind:      "IngressClass",
				Name:      "nginx",
				Namespace: "",
			},
		},
	}
}

func sameClusterIngressPolicyFixture() *openbaov1alpha1.OpenBaoIngressPolicy {
	return &openbaov1alpha1.OpenBaoIngressPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx-backend-tls-v1"},
		Spec: openbaov1alpha1.OpenBaoIngressPolicySpec{
			PathType: openbaov1alpha1.IngressPathTypePrefix,
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/backend-protocol": "HTTPS",
			},
			BackendTLS: &openbaov1alpha1.OpenBaoIngressPolicyBackendTLSSpec{
				PublicationMode: openbaov1alpha1.OpenBaoIngressBackendTLSPublicationModeAnnotation,
			},
			ReadinessMode: openbaov1alpha1.IngressReadinessModeLoadBalancerPublished,
		},
	}
}

func sameClusterBaselineBackupProfile() *openbaov1alpha1.OpenBaoBackupProfile {
	return &openbaov1alpha1.OpenBaoBackupProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-daily-v1"},
	}
}
