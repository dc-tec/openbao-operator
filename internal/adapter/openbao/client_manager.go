package openbao

import (
	"sync"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// ClientManager centralizes OpenBao client lifecycle
type ClientManager struct {
	mu     sync.RWMutex
	states map[string]*clientState

	// defaults holds the default configuration for smart client state.
	defaults portopenbao.ClientConfig
}

// NewClientManager creates a new ClientManager with the given default limits.
// The defaults are applied when creating client state for new clusters.
func NewClientManager(defaults portopenbao.ClientConfig) *ClientManager {
	return &ClientManager{
		states:   make(map[string]*clientState),
		defaults: defaults,
	}
}

// FactoryFor returns a ClientFactory bound to the given clusterKey.
// The factory shares client state with other factories for the same clusterKey.
//
// The clusterKey should be a stable identifier for the cluster, typically
// in the format "namespace/name". The caCert is the PEM-encoded CA certificate
// for TLS verification. tlsServerName optionally overrides the hostname used
// for TLS verification when clients connect through a different Service name.
func (m *ClientManager) FactoryFor(clusterKey string, caCert []byte, tlsServerName ...string) *ClientFactory {
	if m == nil {
		return nil
	}

	state := m.getOrCreateState(clusterKey)

	template := m.defaults
	template.ClusterKey = clusterKey
	template.CACert = caCert
	if len(tlsServerName) > 0 {
		template.TLSServerName = tlsServerName[0]
	}

	return newClientFactoryWithState(template, state)
}

// getOrCreateState returns existing client state for the cluster,
// or creates a new one if it doesn't exist.
func (m *ClientManager) getOrCreateState(clusterKey string) *clientState {
	if clusterKey == "" {
		return nil
	}

	// Fast path: check with read lock
	m.mu.RLock()
	if state, ok := m.states[clusterKey]; ok {
		m.mu.RUnlock()
		return state
	}
	m.mu.RUnlock()

	// Slow path: create with write lock
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if state, ok := m.states[clusterKey]; ok {
		return state
	}

	state := newClientState(m.defaults)
	m.states[clusterKey] = state
	return state
}

// Close clears all client state and releases resources.
// After Close is called, the ClientManager should not be used.
func (m *ClientManager) Close() {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear all state
	m.states = make(map[string]*clientState)
}
