package storage

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStorageEndpointRequestResolver map[string][]netip.Addr

func (r fakeStorageEndpointRequestResolver) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	if addrs, ok := r[host]; ok {
		return addrs, nil
	}
	return nil, &net.DNSError{
		Err:        "no such host",
		Name:       host,
		IsNotFound: true,
	}
}

type recordingStorageEndpointRequestResolver struct {
	network string
	addrs   []netip.Addr
}

func (r *recordingStorageEndpointRequestResolver) LookupNetIP(_ context.Context, network, _ string) ([]netip.Addr, error) {
	r.network = network
	return r.addrs, nil
}

func TestStorageEndpointRequestGuardDialsResolvedAddress(t *testing.T) {
	t.Parallel()

	var dialedAddress string
	resolver := &recordingStorageEndpointRequestResolver{
		addrs: []netip.Addr{netip.MustParseAddr("203.0.113.10")},
	}
	guard := storageEndpointRequestGuard{
		resolver: resolver,
		dialContext: func(_ context.Context, _ string, address string) (net.Conn, error) {
			dialedAddress = address
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() {
				_ = clientConn.Close()
				_ = serverConn.Close()
			})
			return clientConn, nil
		},
	}

	conn, err := guard.guardedDialContext(context.Background(), "tcp4", "storage.example.test:443")
	require.NoError(t, err)
	require.NotNil(t, conn)
	assert.Equal(t, "ip4", resolver.network)
	assert.Equal(t, "203.0.113.10:443", dialedAddress)
}

func TestStorageEndpointRequestGuardAllowsPrivateClusterAddress(t *testing.T) {
	t.Parallel()

	var dialedAddress string
	guard := storageEndpointRequestGuard{
		resolver: fakeStorageEndpointRequestResolver{},
		dialContext: func(_ context.Context, _ string, address string) (net.Conn, error) {
			dialedAddress = address
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() {
				_ = clientConn.Close()
				_ = serverConn.Close()
			})
			return clientConn, nil
		},
	}

	conn, err := guard.guardedDialContext(context.Background(), "tcp", "10.96.12.34:9000")
	require.NoError(t, err)
	require.NotNil(t, conn)
	assert.Equal(t, "10.96.12.34:9000", dialedAddress)
}

func TestStorageEndpointRequestGuardRejectsForbiddenDNSAtDialTime(t *testing.T) {
	t.Parallel()

	dialed := false
	guard := storageEndpointRequestGuard{
		resolver: fakeStorageEndpointRequestResolver{
			"storage.example.test": []netip.Addr{netip.MustParseAddr("169.254.169.254")},
		},
		dialContext: func(context.Context, string, string) (net.Conn, error) {
			dialed = true
			return nil, nil
		},
	}

	conn, err := guard.guardedDialContext(context.Background(), "tcp", "storage.example.test:443")
	require.Error(t, err)
	assert.Nil(t, conn)
	assert.False(t, dialed)
	assert.Contains(t, err.Error(), "resolves to forbidden address")
}

func TestStorageEndpointRequestGuardRejectsRedirectToForbiddenHost(t *testing.T) {
	t.Parallel()

	client, err := buildHTTPClient(nil, false, true)
	require.NoError(t, err)
	require.NotNil(t, client.CheckRedirect)

	req, err := http.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data", nil)
	require.NoError(t, err)

	err = client.CheckRedirect(req, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}
