package openbao

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"time"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// HealthResponse represents the response from GET /v1/sys/health.
// The health endpoint returns different status codes based on cluster state:
// - 200: initialized, unsealed, and active
// - 429: unsealed and standby
// - 472: data recovery mode replication secondary and target sealed
// - 473: performance standby
// - 501: not initialized
// - 503: sealed
type HealthResponse struct {
	// Initialized indicates whether OpenBao has been initialized.
	Initialized bool `json:"initialized"`
	// Sealed indicates whether OpenBao is sealed.
	Sealed bool `json:"sealed"`
	// Standby indicates whether this node is a standby (not the leader).
	Standby bool `json:"standby"`
	// PerformanceStandby indicates if this is a performance standby node.
	PerformanceStandby bool `json:"performance_standby"`
	// ReplicationPerformanceMode is the replication mode.
	ReplicationPerformanceMode string `json:"replication_performance_mode,omitempty"`
	// ReplicationDRMode is the DR replication mode.
	ReplicationDRMode string `json:"replication_dr_mode,omitempty"`
	// ServerTimeUTC is the server time in UTC.
	ServerTimeUTC int64 `json:"server_time_utc,omitempty"`
	// Version is the OpenBao version.
	Version string `json:"version,omitempty"`
	// ClusterName is the name of the Raft cluster.
	ClusterName string `json:"cluster_name,omitempty"`
	// LeaderAddress is the address of the leader node.
	LeaderAddress string `json:"leader_address,omitempty"`
	// ClusterID is the unique identifier for the cluster.
	ClusterID string `json:"cluster_id,omitempty"`
}

// Client provides access to OpenBao's system API endpoints.
// It is used by the UpgradeManager to check node health and perform leader step-down.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client

	state *clientState
}

// NewClient creates a new OpenBao API client with the given configuration.
// The client is configured to trust the provided CA certificate for TLS verification.
func NewClient(config portopenbao.ClientConfig) (*Client, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	if config.CACert == nil {
		config.CACert = []byte{}
	}

	connectionTimeout := config.ConnectionTimeout
	if connectionTimeout == 0 {
		connectionTimeout = portopenbao.DefaultConnectionTimeout
	}

	requestTimeout := config.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = portopenbao.DefaultRequestTimeout
	}

	// Parse the base URL to extract the hostname for server name verification
	parsedURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid baseURL %q: %w", config.BaseURL, err)
	}

	// Configure TLS with the provided CA certificate
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Set ServerName explicitly when the operator needs to verify against a
	// stable TLS identity instead of the connection hostname (for example, ACME
	// clusters and direct pod connections during day-2 operations).
	if config.TLSServerName != "" {
		tlsConfig.ServerName = config.TLSServerName
	} else if parsedURL.Hostname() != "" {
		tlsConfig.ServerName = parsedURL.Hostname()
	}

	// If a per-cluster CA bundle is provided, trust only that bundle.
	// If empty, use system roots (default behavior when RootCAs is nil).
	if len(config.CACert) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(config.CACert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = pool
	}

	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		TLSHandshakeTimeout: connectionTimeout,
		DisableKeepAlives:   false,
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   requestTimeout,
	}

	var state *clientState

	return &Client{
		baseURL:    config.BaseURL,
		token:      config.Token,
		httpClient: httpClient,
		state:      state,
	}, nil
}

// newClientWithState creates a Client with explicit client state.
// This is used by ClientFactory when created via ClientManager.
func newClientWithState(config portopenbao.ClientConfig, state *clientState) (*Client, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	if config.CACert == nil {
		config.CACert = []byte{}
	}

	connectionTimeout := config.ConnectionTimeout
	if connectionTimeout == 0 {
		connectionTimeout = portopenbao.DefaultConnectionTimeout
	}

	requestTimeout := config.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = portopenbao.DefaultRequestTimeout
	}

	// Parse the base URL to extract the hostname for server name verification
	parsedURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid baseURL %q: %w", config.BaseURL, err)
	}

	// Configure TLS with the provided CA certificate
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if config.TLSServerName != "" {
		tlsConfig.ServerName = config.TLSServerName
	} else if parsedURL.Hostname() != "" {
		tlsConfig.ServerName = parsedURL.Hostname()
	}

	// If a per-cluster CA bundle is provided, trust only that bundle.
	if len(config.CACert) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(config.CACert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = pool
	}

	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		TLSHandshakeTimeout: connectionTimeout,
		DisableKeepAlives:   false,
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   requestTimeout,
	}

	return &Client{
		baseURL:    config.BaseURL,
		token:      config.Token,
		httpClient: httpClient,
		state:      state,
	}, nil
}
