package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dc-tec/openbao-operator/internal/platform/entrypoint"
)

func TestDetectPlatform(t *testing.T) {
	for _, tc := range []struct {
		name, body, want, wantError string
		status                      int
	}{
		{name: "kubernetes", body: `{"kind":"APIGroupList","apiVersion":"v1","groups":[{"name":"apps"}]}`,
			status: http.StatusOK, want: "kubernetes"},
		{name: "openshift", body: `{"kind":"APIGroupList","apiVersion":"v1","groups":[{"name":"security.openshift.io"}]}`,
			status: http.StatusOK, want: "openshift"},
		{name: "API unavailable", status: http.StatusServiceUnavailable, wantError: "discover target platform"},
		{name: "API forbidden", status: http.StatusForbidden, wantError: "discover target platform"},
		{name: "malformed response", body: "invalid json", status: http.StatusOK, wantError: "discover target platform"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/apis", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Accept"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(server.Close)
			platform, err := resolvePlatform(t.Context(), &rest.Config{Host: server.URL}, "auto")
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Empty(t, platform, "failed discovery must not select Kubernetes")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, platform)
		})
	}
}

func TestDetectPlatformInvalidClientConfig(t *testing.T) {
	config := &rest.Config{Host: "http://[invalid"}
	platform, err := detectPlatform(t.Context(), config)
	require.ErrorContains(t, err, "create platform discovery client")
	require.Empty(t, platform)
	for _, configured := range []string{"kubernetes", "openshift"} {
		platform, err = resolvePlatform(t.Context(), config, configured)
		require.NoError(t, err, "explicit platform must bypass discovery")
		require.Equal(t, configured, platform)
	}
}

type discoveryRoundTripper func(*http.Request) (*http.Response, error)

func (f discoveryRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDetectPlatformRequestDeadline(t *testing.T) {
	config := &rest.Config{
		Host: "https://api.invalid",
		Transport: discoveryRoundTripper(func(r *http.Request) (*http.Response, error) {
			deadline, ok := r.Context().Deadline()
			require.True(t, ok, "platform discovery must have a deadline")
			require.Positive(t, time.Until(deadline))
			require.LessOrEqual(t, time.Until(deadline), platformDiscoveryTimeout)
			return nil, fmt.Errorf("stop after checking deadline")
		}),
	}
	_, err := detectPlatform(t.Context(), config)
	require.ErrorContains(t, err, "stop after checking deadline")
}

func TestDetectPlatformCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	platform, err := detectPlatform(ctx, &rest.Config{Host: server.URL})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, platform)
}

func TestInitializeAdmissionTracker(t *testing.T) {
	t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "false")
	reader := fake.NewClientBuilder().WithScheme(scheme).Build()
	for _, mode := range []string{entrypoint.AdmissionEnforcementFail, entrypoint.AdmissionEnforcementWarn} {
		t.Run(mode, func(t *testing.T) {
			tracker, err := initializeAdmissionTracker(t.Context(), reader, mode, 0)
			if mode == entrypoint.AdmissionEnforcementFail {
				require.ErrorContains(t, err, "admission policy dependencies not ready")
				require.Nil(t, tracker)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, tracker.Current())
			require.False(t, tracker.Current().OverallReady)
		})
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	tracker, err := initializeAdmissionTracker(ctx, reader, entrypoint.AdmissionEnforcementFail, time.Minute)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, tracker)

	t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "true")
	tracker, err = initializeAdmissionTracker(t.Context(), nil, entrypoint.AdmissionEnforcementFail, time.Minute)
	require.NoError(t, err)
	require.True(t, tracker.Current().OverallReady)
	require.True(t, tracker.Current().UnsafeMode)
}
