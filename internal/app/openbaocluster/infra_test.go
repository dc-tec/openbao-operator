package openbaocluster

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	"github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func TestHandleScaleDownSafety(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	clusterName := "test-cluster"
	namespace := "default"

	tests := []struct {
		name            string
		currentReplicas int32
		desiredReplicas int32
		victimLeader    bool
		victimError     bool // simulate network error
		expectedError   string
	}{
		{
			name:            "No scale down",
			currentReplicas: 3,
			desiredReplicas: 3,
			victimLeader:    false, // Irrelevant
			expectedError:   "",
		},
		{
			name:            "Scale up",
			currentReplicas: 3,
			desiredReplicas: 4,
			victimLeader:    false, // Irrelevant
			expectedError:   "",
		},
		{
			name:            "Scale down, victim is follower",
			currentReplicas: 3,
			desiredReplicas: 2,
			victimLeader:    false,
			expectedError:   "",
		},
		{
			name:            "Scale down, victim is leader",
			currentReplicas: 3,
			desiredReplicas: 2,
			victimLeader:    true,
			expectedError:   "waiting for leader step-down on test-cluster-2 to complete",
		},
		{
			name:            "Scale down, victim unreachable",
			currentReplicas: 3,
			desiredReplicas: 2,
			victimError:     true,
			expectedError:   "", // Should proceed (fail open)
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

			// Mock StatefulSet
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: namespace,
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &tt.currentReplicas,
				},
			}

			// Mock Client
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(cluster, sts).
				Build()

				// Mock OpenBao Client for victim pod
			clientFunc := func(_ context.Context, c *openbaov1alpha1.OpenBaoCluster, podName string) (ScaleDownPodClient, error) {
				if tt.victimError {
					return nil, fmt.Errorf("network error")
				}

				// Mock server to handle health/step-down
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/v1/sys/health":
						if tt.victimLeader {
							// Active Leader: 200 OK, initialized=true, sealed=false, standby=false
							w.WriteHeader(http.StatusOK)
							_, _ = w.Write([]byte(`{"initialized": true, "sealed": false, "standby": false}`))
						} else {
							// Follower: 429, initialized=true, sealed=false, standby=true
							w.WriteHeader(http.StatusTooManyRequests)
							_, _ = w.Write([]byte(`{"initialized": true, "sealed": false, "standby": true}`))
						}
					case "/v1/sys/step-down":
						if r.Method == http.MethodPut {
							w.WriteHeader(http.StatusNoContent)
						} else {
							w.WriteHeader(http.StatusMethodNotAllowed)
						}
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))
				// Note: server is not closed, leaking resources in test but acceptable for short unit test

				// Important: Use server URL
				clientConfig := portopenbao.ClientConfig{
					BaseURL: server.URL,
					Token:   "root", // required for step-down
				}

				return openbao.NewClient(clientConfig)
			}

			r := &infraReconciler{deps: InfraDependencies{
				Kubernetes: InfraKubernetesRuntime{
					Client: k8sClient,
					Scheme: scheme,
				},
				Events: InfraEventRuntime{
					Recorder: nil, // not needed for this test part
				},
				Pods: InfraPodRuntime{
					ClientForPodFunc: clientFunc,
				},
			}}

			err := r.handleScaleDownSafety(context.Background(), cluster, tt.desiredReplicas, sts)
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
		reasons: InfraReasonPolicy{
			APIServerNetworkConfiguration: "APIServerNetworkConfigurationInvalid",
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
	if reason != "APIServerNetworkConfigurationInvalid" {
		t.Fatalf("reason = %q, want APIServerNetworkConfigurationInvalid", reason)
	}
	if !strings.Contains(err.Error(), "spec.network.apiServerEndpointIPs") {
		t.Fatalf("error %q does not mention apiServerEndpointIPs", err)
	}
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

	r := &infraReconciler{
		reasons: InfraReasonPolicy{
			OIDCBootstrapConfiguration: "OIDCBootstrapConfigurationInvalid",
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
	if !ok || reason != "OIDCBootstrapConfigurationInvalid" {
		t.Fatalf("reason = %q,%v want OIDCBootstrapConfigurationInvalid,true", reason, ok)
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
		reasons: InfraReasonPolicy{
			OIDCBootstrapConfiguration: "OIDCBootstrapConfigurationInvalid",
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
	if !ok || reason != "OIDCBootstrapConfigurationInvalid" {
		t.Fatalf("reason = %q,%v want OIDCBootstrapConfigurationInvalid,true", reason, ok)
	}
}
