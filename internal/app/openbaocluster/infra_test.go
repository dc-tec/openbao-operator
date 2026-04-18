package openbaocluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
)

type scaleDownRuntimeStub struct {
	prepareFunc func(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, statefulSetName string, currentReplicas, desiredReplicas int32) error
}

func (s scaleDownRuntimeStub) PrepareScaleDown(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, statefulSetName string, currentReplicas, desiredReplicas int32) error {
	if s.prepareFunc != nil {
		return s.prepareFunc(ctx, logger, cluster, statefulSetName, currentReplicas, desiredReplicas)
	}
	return nil
}

func TestHandleScaleDownSafety(t *testing.T) {
	clusterName := "test-cluster"
	namespace := "default"

	tests := []struct {
		name            string
		currentReplicas int32
		desiredReplicas int32
		statusReplicas  int32
		readyReplicas   int32
		currentStatus   int32
		generation      int64
		observedGen     int64
		runtimeError    string
		expectedApplied int32
		expectedError   string
	}{
		{
			name:            "no scale down",
			currentReplicas: 3,
			desiredReplicas: 3,
			statusReplicas:  3,
			readyReplicas:   3,
			currentStatus:   3,
			generation:      1,
			observedGen:     1,
			expectedApplied: 3,
			expectedError:   "",
		},
		{
			name:            "scale up",
			currentReplicas: 3,
			desiredReplicas: 4,
			statusReplicas:  3,
			readyReplicas:   3,
			currentStatus:   3,
			generation:      1,
			observedGen:     1,
			expectedApplied: 4,
			expectedError:   "",
		},
		{
			name:            "scale down uses one safe step",
			currentReplicas: 3,
			desiredReplicas: 1,
			statusReplicas:  3,
			readyReplicas:   3,
			currentStatus:   3,
			generation:      1,
			observedGen:     1,
			expectedApplied: 2,
			expectedError:   "",
		},
		{
			name:            "scale down waits for statefulset to settle between steps",
			currentReplicas: 2,
			desiredReplicas: 1,
			statusReplicas:  2,
			readyReplicas:   1,
			currentStatus:   1,
			generation:      2,
			observedGen:     2,
			expectedApplied: 2,
			expectedError:   "waiting for StatefulSet default/test-cluster to settle at 2 replicas before next scale-down step",
		},
		{
			name:            "scale down requeues when runtime blocks",
			currentReplicas: 3,
			desiredReplicas: 2,
			statusReplicas:  3,
			readyReplicas:   3,
			currentStatus:   3,
			generation:      1,
			observedGen:     1,
			runtimeError:    "waiting for leader step-down on test-cluster-2 to complete",
			expectedApplied: 3,
			expectedError:   "waiting for leader step-down on test-cluster-2 to complete",
		},
		{
			name:            "scale down blocks without runtime",
			currentReplicas: 3,
			desiredReplicas: 2,
			statusReplicas:  3,
			readyReplicas:   3,
			currentStatus:   3,
			generation:      1,
			observedGen:     1,
			expectedApplied: 3,
			expectedError:   "scale-down runtime is not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: namespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Replicas: tt.desiredReplicas,
				},
			}

			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:       clusterName,
					Namespace:  namespace,
					Generation: tt.generation,
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &tt.currentReplicas,
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: tt.observedGen,
					Replicas:           tt.statusReplicas,
					ReadyReplicas:      tt.readyReplicas,
					CurrentReplicas:    tt.currentStatus,
				},
			}

			r := &infraReconciler{}
			if tt.name != "scale down blocks without runtime" {
				r.deps.ScaleDown = InfraScaleDownRuntime{
					Runtime: scaleDownRuntimeStub{
						prepareFunc: func(_ context.Context, _ logr.Logger, gotCluster *openbaov1alpha1.OpenBaoCluster, statefulSetName string, currentReplicas, desiredReplicas int32) error {
							assert.Same(t, cluster, gotCluster)
							assert.Equal(t, clusterName, statefulSetName)
							assert.Equal(t, tt.currentReplicas, currentReplicas)
							if tt.currentReplicas > tt.desiredReplicas {
								expectedDesired := tt.currentReplicas - 1
								if expectedDesired < tt.desiredReplicas {
									expectedDesired = tt.desiredReplicas
								}
								assert.Equal(t, expectedDesired, desiredReplicas)
							}
							if tt.runtimeError != "" {
								return errors.New(tt.runtimeError)
							}
							return nil
						},
					},
				}
			}

			appliedReplicas, err := r.handleScaleDownSafety(context.Background(), cluster, tt.desiredReplicas, sts)
			assert.Equal(t, tt.expectedApplied, appliedReplicas)
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestInfraReconciler_ResolveOIDC_LazyDiscoveryForSelfInit(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled: true,
				},
			},
		},
	}

	var called int
	r := &infraReconciler{deps: InfraDependencies{
		OIDC: InfraOIDCRuntime{
			RestConfig:  &rest.Config{Host: "https://kubernetes.default.svc"},
			OIDCIssuer:  "",
			OIDCJWTKeys: nil,
			DiscoverOIDCConfig: func(ctx context.Context, cfg *rest.Config) (*OIDCConfig, error) {
				called++
				return &OIDCConfig{
					IssuerURL: "https://issuer.example",
					JWKSURL:   "https://issuer.example/keys",
				}, nil
			},
		},
	}}

	oidc, err := r.resolveOIDC(context.Background(), cluster)
	assert.NoError(t, err)
	assert.Equal(t, 1, called)
	assert.Equal(t, "https://issuer.example", oidc.IssuerURL)
	assert.Equal(t, "https://issuer.example/keys", oidc.JWKSURL)
}

func TestInfraReconciler_VerifyInitContainerImageDigest_UsesResolvedDefaultImage(t *testing.T) {
	t.Setenv(constants.EnvOperatorVersion, "1.2.3")

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileHardened,
		},
	}

	var verifiedImage string
	r := &infraReconciler{deps: InfraDependencies{
		ImageVerification: InfraImageVerificationRuntime{
			VerifyOperatorImage: func(_ context.Context, _ logr.Logger, _ imageverify.Verifier, _ *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
				verifiedImage = imageRef
				return "ghcr.io/dc-tec/openbao-init@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil
			},
		},
	}}

	initImage, err := r.resolveInitContainerImage(cluster)
	if err != nil {
		t.Fatalf("resolveInitContainerImage() error = %v", err)
	}
	if initImage != "ghcr.io/dc-tec/openbao-init:1.2.3" {
		t.Fatalf("resolveInitContainerImage() = %q, want %q", initImage, "ghcr.io/dc-tec/openbao-init:1.2.3")
	}

	digest, err := r.verifyInitContainerImageDigest(context.Background(), logr.Discard(), cluster, initImage)
	if err != nil {
		t.Fatalf("verifyInitContainerImageDigest() error = %v", err)
	}
	if digest != "ghcr.io/dc-tec/openbao-init@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("verifyInitContainerImageDigest() = %q", digest)
	}
	if verifiedImage != initImage {
		t.Fatalf("verified image = %q, want %q", verifiedImage, initImage)
	}
}

func TestComputeStatefulSetSpec_BlueGreenCleanupRetainsStatefulSetName(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bluegreen-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase:        openbaov1alpha1.PhaseCleanup,
				BlueRevision: "blue123",
			},
		},
	}

	spec := (&infraReconciler{}).computeStatefulSetSpec(logr.Discard(), cluster, "sha256:main", "sha256:init")
	assert.True(t, spec.SkipReconciliation)
	assert.Equal(t, "blue123", spec.Revision)
	assert.Equal(t, "bluegreen-cluster-blue123", spec.Name)
}

func TestInfraReconciler_ResolveTargetMainImage_BlueGreenPrefersActivePods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
				BlueGreen: &openbaov1alpha1.BlueGreenConfig{
					AutoPromote: false,
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3", // upgrade pending
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase:        openbaov1alpha1.PhaseIdle,
				BlueRevision: "rev-old",
				BlueImage:    "", // missing in status; should be inferred
			},
		},
	}

	activePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-rev-old-0",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openbao",
				"app.kubernetes.io/instance":   "test-cluster",
				"app.kubernetes.io/managed-by": "openbao-operator",
				"openbao.org/cluster":          "test-cluster",
				"openbao.org/revision":         "rev-old",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "openbao", Image: "openbao/openbao:2.4.3"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, activePod).
		Build()

	r := &infraReconciler{deps: InfraDependencies{
		Kubernetes: InfraKubernetesRuntime{
			Client: k8sClient,
			Scheme: scheme,
		},
	}}

	got := r.resolveTargetMainImage(context.Background(), logr.Discard(), cluster)
	assert.Equal(t, "openbao/openbao:2.4.3", got)
	assert.Equal(t, "openbao/openbao:2.4.3", cluster.Status.BlueGreen.BlueImage)
}

func TestInfraReconciler_Reconcile_BlocksInvalidVersionSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		wantReason string
	}{
		{
			name: "downgrade is rejected",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.4.4",
					Image:   "openbao/openbao:2.4.4",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.5.0",
				},
			},
			wantReason: upgrade.ReasonDowngradeBlocked,
		},
		{
			name: "image version mismatch is rejected",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.5.0",
					Image:   "openbao/openbao:2.4.4",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.4",
				},
			},
			wantReason: upgrade.ReasonImageVersionMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &infraReconciler{}
			_, err := r.Reconcile(context.Background(), logr.Discard(), tt.cluster)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !errors.Is(err, operatorerrors.ErrPermanentConfig) {
				t.Fatalf("expected permanent config error, got %v", err)
			}
			reason, ok := operatorerrors.Reason(err)
			if !ok {
				t.Fatalf("expected reasoned error, got %v", err)
			}
			if reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}

func TestInfraReconciler_Reconcile_MapsAPIServerNetworkConfigurationError(t *testing.T) {
	t.Setenv(constants.EnvOperatorVersion, "1.2.3")

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileDevelopment,
			Version: "2.5.0",
			Image:   "openbao/openbao:2.5.0",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()

	r := &infraReconciler{
		deps: InfraDependencies{
			Kubernetes: InfraKubernetesRuntime{
				Client:            k8sClient,
				Scheme:            scheme,
				OperatorNamespace: "openbao-operator-system",
			},
		},
	}

	_, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, operatorerrors.ErrPermanentConfig) {
		t.Fatalf("expected permanent config error, got %v", err)
	}
	reason, ok := operatorerrors.Reason(err)
	if !ok {
		t.Fatalf("expected reasoned error, got %v", err)
	}
	if reason != constants.ReasonAPIServerNetworkConfigurationInvalid {
		t.Fatalf("reason = %q, want %s", reason, constants.ReasonAPIServerNetworkConfigurationInvalid)
	}
	if !strings.Contains(err.Error(), "spec.network.apiServerEndpointIPs") {
		t.Fatalf("error %q does not mention apiServerEndpointIPs", err)
	}
}

func TestInfraReconciler_Reconcile_DoesNotRequeuePermanentScaleDownPrerequisites(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	currentReplicas := int32(3)
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile:  openbaov1alpha1.ProfileDevelopment,
			Version:  "2.5.0",
			Image:    "openbao/openbao:2.5.0",
			Replicas: 1,
			InitContainer: &openbaov1alpha1.InitContainerConfig{
				Image: "openbao-init:test",
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized: true,
		},
	}
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       cluster.Name,
			Namespace:  cluster.Namespace,
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &currentReplicas,
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			Replicas:           3,
			ReadyReplicas:      3,
			CurrentReplicas:    3,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, statefulSet).
		Build()

	r := &infraReconciler{
		deps: InfraDependencies{
			Kubernetes: InfraKubernetesRuntime{
				Client: k8sClient,
				Scheme: scheme,
			},
			ScaleDown: InfraScaleDownRuntime{
				Runtime: scaleDownRuntimeStub{
					prepareFunc: func(context.Context, logr.Logger, *openbaov1alpha1.OpenBaoCluster, string, int32, int32) error {
						return operatorerrors.WrapPermanentPrerequisitesMissing(errors.New("missing raft permissions"))
					},
				},
			},
		},
	}

	result, err := r.Reconcile(context.Background(), logr.Discard(), cluster)
	assert.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
}

func TestInfraReconciler_MapManagerReconcileError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantReason string
	}{
		{name: "oidc bootstrap audience mismatch", err: inframanager.ErrOIDCBootstrapAudienceMismatch, wantReason: constants.ReasonOIDCBootstrapConfigurationInvalid},
		{name: "gateway api missing", err: inframanager.ErrGatewayAPIMissing, wantReason: constants.ReasonGatewayAPIMissing},
		{name: "api server network invalid", err: inframanager.ErrAPIServerNetworkConfigurationInvalid, wantReason: constants.ReasonAPIServerNetworkConfigurationInvalid},
		{name: "prerequisites missing", err: workloadsvc.ErrStatefulSetPrerequisitesMissing, wantReason: constants.ReasonPrerequisitesMissing},
		{name: "acme domain not resolvable", err: inframanager.ErrACMEDomainNotResolvable, wantReason: constants.ReasonACMEDomainNotResolvable},
		{name: "acme gateway not configured", err: inframanager.ErrACMEGatewayNotConfiguredForPassthrough, wantReason: constants.ReasonACMEGatewayNotConfiguredForPassthrough},
	}

	r := &infraReconciler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.mapManagerReconcileError(tt.err)
			reason, ok := operatorerrors.Reason(got)
			if !ok {
				t.Fatalf("expected reasoned error, got %v", got)
			}
			if reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tt.wantReason)
			}
			if !errors.Is(got, tt.err) {
				t.Fatalf("wrapped error %v should preserve %v", got, tt.err)
			}
		})
	}

	t.Run("unmapped error passes through", func(t *testing.T) {
		original := errors.New("boom")
		if got := r.mapManagerReconcileError(original); got != original {
			t.Fatalf("got %v, want original %v", got, original)
		}
	})
}

func TestInfraReconciler_ResolveOIDC_MissingRestConfigReturnsBootstrapReason(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled: true,
				},
			},
		},
	}

	r := &infraReconciler{}

	_, err := r.resolveOIDC(context.Background(), cluster)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, operatorerrors.ErrPermanentConfig) {
		t.Fatalf("expected permanent config error, got %v", err)
	}
	reason, ok := operatorerrors.Reason(err)
	if !ok || reason != constants.ReasonOIDCBootstrapConfigurationInvalid {
		t.Fatalf("reason = %q,%v want %s,true", reason, ok, constants.ReasonOIDCBootstrapConfigurationInvalid)
	}
}

func TestInfraReconciler_ResolveOIDC_ForbiddenDiscoveryReturnsBootstrapReason(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled: true,
				},
			},
		},
	}

	r := &infraReconciler{
		deps: InfraDependencies{
			OIDC: InfraOIDCRuntime{
				RestConfig: &rest.Config{Host: "https://kubernetes.default.svc"},
				DiscoverOIDCConfig: func(ctx context.Context, cfg *rest.Config) (*OIDCConfig, error) {
					return nil, errors.New("forbidden")
				},
				DiscoveryStatusCode: func(err error) (int, bool) {
					return http.StatusForbidden, true
				},
			},
		},
	}

	_, err := r.resolveOIDC(context.Background(), cluster)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, operatorerrors.ErrPermanentConfig) {
		t.Fatalf("expected permanent config error, got %v", err)
	}
	reason, ok := operatorerrors.Reason(err)

	if !ok || reason != constants.ReasonOIDCBootstrapConfigurationInvalid {
		t.Fatalf("reason = %q,%v want %s,true", reason, ok, constants.ReasonOIDCBootstrapConfigurationInvalid)
	}
}

func TestInfraReconciler_ResolveOIDC_EmptyIssuerReturnsBootstrapReason(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled: true,
				},
			},
		},
	}

	r := &infraReconciler{
		deps: InfraDependencies{
			OIDC: InfraOIDCRuntime{
				RestConfig: &rest.Config{Host: "https://kubernetes.default.svc"},
				DiscoverOIDCConfig: func(ctx context.Context, cfg *rest.Config) (*OIDCConfig, error) {
					return &OIDCConfig{JWKSURL: "https://issuer.example/keys"}, nil
				},
			},
		},
	}

	_, err := r.resolveOIDC(context.Background(), cluster)
	assertOIDCBootstrapConfigurationError(t, err)
}

func TestInfraReconciler_ResolveOIDC_NoJWTValidationMaterialReturnsBootstrapReason(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled: true,
				},
			},
		},
	}

	r := &infraReconciler{
		deps: InfraDependencies{
			OIDC: InfraOIDCRuntime{
				RestConfig: &rest.Config{Host: "https://kubernetes.default.svc"},
				DiscoverOIDCConfig: func(ctx context.Context, cfg *rest.Config) (*OIDCConfig, error) {
					return &OIDCConfig{IssuerURL: "https://issuer.example"}, nil
				},
			},
		},
	}

	_, err := r.resolveOIDC(context.Background(), cluster)
	assertOIDCBootstrapConfigurationError(t, err)
}

func TestInfraReconciler_ResolveOIDC_MalformedJWKSReturnsBootstrapReason(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled: true,
				},
			},
		},
	}

	r := &infraReconciler{
		deps: InfraDependencies{
			OIDC: InfraOIDCRuntime{
				RestConfig: &rest.Config{Host: "https://kubernetes.default.svc"},
				DiscoverOIDCConfig: func(ctx context.Context, cfg *rest.Config) (*OIDCConfig, error) {
					return nil, fmt.Errorf("%w: failed to parse jwks document: %w", portauth.ErrDiscoveryContentInvalid, malformedJSONError())
				},
			},
		},
	}

	_, err := r.resolveOIDC(context.Background(), cluster)
	assertOIDCBootstrapConfigurationError(t, err)
}

func TestInfraReconciler_ResolveOIDC_TransientJWKSFetchFailureStaysTransient(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled: true,
				},
			},
		},
	}

	r := &infraReconciler{
		deps: InfraDependencies{
			OIDC: InfraOIDCRuntime{
				RestConfig: &rest.Config{Host: "https://kubernetes.default.svc"},
				DiscoverOIDCConfig: func(ctx context.Context, cfg *rest.Config) (*OIDCConfig, error) {
					return nil, fmt.Errorf("failed to fetch JWKS keys: failed to fetch jwks endpoint: %w", context.DeadlineExceeded)
				},
			},
		},
	}

	_, err := r.resolveOIDC(context.Background(), cluster)
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, operatorerrors.ErrPermanentConfig) {
		t.Fatalf("expected transient error, got permanent config: %v", err)
	}
	if !operatorerrors.IsTransient(err) {
		t.Fatalf("expected transient error, got %v", err)
	}
}

func assertOIDCBootstrapConfigurationError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, operatorerrors.ErrPermanentConfig) {
		t.Fatalf("expected permanent config error, got %v", err)
	}
	reason, ok := operatorerrors.Reason(err)
	if !ok || reason != constants.ReasonOIDCBootstrapConfigurationInvalid {
		t.Fatalf("reason = %q,%v want %s,true", reason, ok, constants.ReasonOIDCBootstrapConfigurationInvalid)
	}
}

func malformedJSONError() error {
	var payload map[string]any
	return json.Unmarshal([]byte("{"), &payload)
}
