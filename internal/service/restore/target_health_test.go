package restore

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestTargetHealthRequiresHealthyVotersAndOneLeader(t *testing.T) {
	leader := portopenbao.HealthStatus{Initialized: true, ClusterID: "source", Version: "2.7.0"}
	standby := leader
	standby.Standby = true
	for _, tc := range []struct {
		name      string
		voters    []portopenbao.HealthStatus
		readError error
		wantError string
	}{
		{name: "healthy", voters: []portopenbao.HealthStatus{standby, leader, standby}},
		{name: "sealed", voters: []portopenbao.HealthStatus{leader, {Initialized: true, Sealed: true}}, wantError: "not initialized and unsealed"},
		{name: "uninitialized", voters: []portopenbao.HealthStatus{{}}, wantError: "not initialized and unsealed"},
		{name: "no leader", voters: []portopenbao.HealthStatus{standby}, wantError: "no healthy active voter"},
		{name: "multiple leaders", voters: []portopenbao.HealthStatus{leader, leader}, wantError: "multiple active voters"},
		{name: "read failure", voters: []portopenbao.HealthStatus{leader}, readError: errors.New("connection failed"), wantError: "read target voter 0 health"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, cluster := healthFixture(t)
			cluster.Spec.Replicas = int32(len(tc.voters))
			m := NewManager(c, c.Scheme(), nil, nil, "")
			reads := 0
			m.readHealth = func(_ context.Context, _ portopenbao.ClientConfig) (*portopenbao.HealthStatus, error) {
				health := tc.voters[reads]
				reads++
				return &health, tc.readError
			}
			got, err := m.targetHealth(t.Context(), cluster)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, got)
				if tc.readError != nil {
					require.ErrorIs(t, err, tc.readError)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, &leader, got)
			require.Equal(t, len(tc.voters), reads)
		})
	}
}

func TestTargetHealthResolvesTrustAndDirectVoterAddresses(t *testing.T) {
	for _, systemRoots := range []bool{false, true} {
		name := "managed CA"
		if systemRoots {
			name = "system roots"
		}
		t.Run(name, func(t *testing.T) {
			c, cluster := healthFixture(t)
			cluster.Spec.TLS = api.TLSConfig{Enabled: true, Mode: api.TLSModeOperatorManaged}
			var expectedCA []byte
			if systemRoots {
				cluster.Spec.TLS.Mode = api.TLSModeACME
			} else {
				expectedCA = []byte("target CA")
				require.NoError(t, c.Create(t.Context(), &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: cluster.Name + "-tls-ca", Namespace: cluster.Namespace},
					Data:       map[string][]byte{"ca.crt": expectedCA},
				}))
			}
			m := NewManager(c, c.Scheme(), nil, nil, "")
			m.clientConfig.CACert = []byte("must be replaced")
			m.readHealth = func(ctx context.Context, config portopenbao.ClientConfig) (*portopenbao.HealthStatus, error) {
				require.Equal(t, expectedCA, config.CACert)
				require.Equal(t, "https://fresh-0.fresh.recovery.svc:8200", config.BaseURL)
				require.Equal(t, portopenbao.ComputeTLSServerName(cluster), config.TLSServerName)
				_, bounded := ctx.Deadline()
				require.True(t, bounded)
				return &portopenbao.HealthStatus{Initialized: true}, nil
			}
			_, err := m.targetHealth(t.Context(), cluster)
			require.NoError(t, err)
		})
	}
}

func healthFixture(t *testing.T) (client.WithWatch, *api.OpenBaoCluster) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, api.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	cluster := &api.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "fresh", Namespace: "recovery"},
		Spec:       api.OpenBaoClusterSpec{Replicas: 1, TLS: api.TLSConfig{Mode: api.TLSModeACME}},
	}
	return fake.NewClientBuilder().WithScheme(scheme).Build(), cluster
}
