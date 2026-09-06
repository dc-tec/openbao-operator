package entrypoint

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/internal/platform/admission"
)

type readinessInformer struct {
	cache.Informer
	synced, stopped bool
}

func (i readinessInformer) HasSynced() bool { return i.synced }
func (i readinessInformer) IsStopped() bool { return i.stopped }

func TestManagerReadiness(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		running, synced, stopped bool
		unsafe                   string
		status                   *admission.Status
		wantError                string
	}{
		{name: "not started", wantError: "manager cache synchronization"},
		{name: "cache pending", running: true, wantError: "watched resource cache"},
		{name: "cache stopped", running: true, synced: true, stopped: true, wantError: "watched resource cache"},
		{name: "absent admission", running: true, synced: true, wantError: "have not been checked"},
		{name: "unready admission", running: true, synced: true,
			status: &admission.Status{CheckedAt: time.Now()}, wantError: "not ready"},
		{name: "stale admission", running: true, synced: true,
			status: &admission.Status{CheckedAt: time.Now().Add(-31 * time.Second), OverallReady: true}, wantError: "stale"},
		{name: "future admission", running: true, synced: true,
			status: &admission.Status{CheckedAt: time.Now().Add(time.Minute), OverallReady: true}, wantError: "stale"},
		{name: "fresh ready", running: true, synced: true,
			status: &admission.Status{CheckedAt: time.Now(), OverallReady: true}},
		{name: "unsafe cache pending", running: true, unsafe: "true", wantError: "watched resource cache"},
		{name: "unsafe not started", synced: true, unsafe: "true", wantError: "manager cache synchronization"},
		{name: "unsafe ready", running: true, synced: true, unsafe: "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", tc.unsafe)
			r := &managerReadiness{
				informers:       []cache.Informer{readinessInformer{synced: tc.synced, stopped: tc.stopped}},
				admissionStatus: tc.status,
			}
			r.running.Store(tc.running)
			for range 3 {
				err := r.Check(httptest.NewRequest("GET", "/readyz", nil))
				if tc.wantError == "" {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, tc.wantError)
				}
			}
			require.False(t, r.NeedLeaderElection())
		})
	}
}

type readinessErrorReader struct {
	client.Reader
	calls    int
	deadline time.Time
}

func (r *readinessErrorReader) Get(ctx context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	r.calls++
	r.deadline, _ = ctx.Deadline()
	return fmt.Errorf("API unavailable")
}

func TestReadinessRefreshIsBoundedAndProbesDoNotReadAPI(t *testing.T) {
	t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "")
	reader := &readinessErrorReader{}
	r := &managerReadiness{reader: reader}
	r.running.Store(true)
	r.refresh(t.Context())
	require.Equal(t, 12, reader.calls, "each missing binding stops its dependency check")
	require.Positive(t, time.Until(reader.deadline))
	require.LessOrEqual(t, time.Until(reader.deadline), 10*time.Second)
	for range 10 {
		require.ErrorContains(t, r.Check(nil), "admission dependencies are not ready")
	}
	require.Equal(t, 12, reader.calls)
	require.Equal(t, 15*time.Second, admissionReadinessRefreshInterval)
	require.Equal(t, 30*time.Second, admissionReadinessMaxAge)
}

func TestReadinessStopsOnCancellation(t *testing.T) {
	t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "true")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := &managerReadiness{}
	require.NoError(t, r.Start(ctx))
	require.False(t, r.running.Load())
	require.Error(t, r.Check(nil))
}
