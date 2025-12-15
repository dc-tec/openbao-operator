package openbao

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/openbao/operator/internal/constants"
)

const (
	// DefaultConnectionTimeout is the default timeout for establishing connections.
	DefaultConnectionTimeout = 5 * time.Second
	// DefaultRequestTimeout is the default timeout for individual API requests.
	DefaultRequestTimeout = 10 * time.Second
	// DefaultSnapshotTimeout is the default timeout for snapshot operations.
	DefaultSnapshotTimeout = 30 * time.Minute
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
	// ClusterID is the unique identifier for the cluster.
	ClusterID string `json:"cluster_id,omitempty"`
}

// Client provides access to OpenBao's system API endpoints.
// It is used by the UpgradeManager to check node health and perform leader step-down.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

var (
	systemCertPool     *x509.CertPool
	systemCertPoolOnce sync.Once
)

func getSystemCertPool() *x509.CertPool {
	systemCertPoolOnce.Do(func() {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		systemCertPool = pool
	})
	return systemCertPool
}

// ClientConfig holds configuration for creating a new Client.
type ClientConfig struct {
	// BaseURL is the OpenBao API URL (e.g., "https://pod-0.cluster.ns.svc:8200").
	BaseURL string
	// Token is the authentication token for OpenBao API calls.
	Token string
	// CACert is the PEM-encoded CA certificate for TLS verification.
	// If empty, the system certificate pool is used.
	CACert []byte
	// ConnectionTimeout is the timeout for establishing connections.
	// Defaults to DefaultConnectionTimeout if zero.
	ConnectionTimeout time.Duration
	// RequestTimeout is the timeout for individual requests.
	// Defaults to DefaultRequestTimeout if zero.
	RequestTimeout time.Duration
}

// NewClient creates a new OpenBao API client with the given configuration.
// The client is configured to trust the provided CA certificate for TLS verification.
func NewClient(config ClientConfig) (*Client, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	if config.CACert == nil {
		config.CACert = []byte{}
	}

	connectionTimeout := config.ConnectionTimeout
	if connectionTimeout == 0 {
		connectionTimeout = DefaultConnectionTimeout
	}

	requestTimeout := config.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = DefaultRequestTimeout
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

	// Set ServerName to the hostname from the URL to ensure proper certificate verification.
	// This is important when connecting to pod DNS names where the certificate SANs must match.
	if parsedURL.Hostname() != "" {
		tlsConfig.ServerName = parsedURL.Hostname()
	}

	// Start from the system cert pool when available so that custom CAs are
	// additive instead of replacing the system roots.
	systemPool := getSystemCertPool()
	if len(config.CACert) > 0 {
		if !systemPool.AppendCertsFromPEM(config.CACert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
	}

	tlsConfig.RootCAs = systemPool

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
	}, nil
}

// Health queries the OpenBao health endpoint and returns the current node state.
// This endpoint does not require authentication by default.
//
// The health endpoint returns different HTTP status codes based on state:
// - 200: Initialized, unsealed, active
// - 429: Standby node
// - 473: Performance standby
// - 501: Not initialized
// - 503: Sealed
//
// We parse the response body regardless of status code since it contains
// the health information we need.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+constants.APIPathSysHealth, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create health request: %w", err)
	}

	// The health endpoint can optionally accept a token for authenticated checks
	if c.token != "" {
		req.Header.Set("X-Vault-Token", c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query health endpoint: %w", err)
	}
	defer func() {
		// Drain and close the body to enable connection reuse
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// Read the response body regardless of status code
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read health response: %w", err)
	}

	var health HealthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		return nil, fmt.Errorf("failed to parse health response: %w", err)
	}

	return &health, nil
}

// IsLeader determines if the connected node is the Raft leader.
// A node is the leader if it is initialized, unsealed, and not in standby mode.
func (c *Client) IsLeader(ctx context.Context) (bool, error) {
	health, err := c.Health(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check leader status: %w", err)
	}

	// A node is the leader if:
	// - It is initialized
	// - It is unsealed
	// - It is not a standby
	// - It is not a performance standby
	return health.Initialized && !health.Sealed && !health.Standby && !health.PerformanceStandby, nil
}

// StepDown requests the leader to step down and trigger a new election.
// This is used during upgrades to gracefully transfer leadership before
// updating the leader pod.
//
// The step-down endpoint requires authentication with a token that has
// the sys/step-down capability.
//
// Returns nil on success, or an error if the request fails.
// Note: A 204 No Content response indicates success.
func (c *Client) StepDown(ctx context.Context) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for step-down operation")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+constants.APIPathSysStepDown, nil)
	if err != nil {
		return fmt.Errorf("failed to create step-down request: %w", err)
	}

	req.Header.Set("X-Vault-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute step-down request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// 204 No Content is the expected success response
	// 200 OK is also acceptable
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("step-down request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// IsHealthy returns true if the node is initialized, unsealed, and reachable.
func (c *Client) IsHealthy(ctx context.Context) (bool, error) {
	health, err := c.Health(ctx)
	if err != nil {
		return false, err
	}

	return health.Initialized && !health.Sealed, nil
}

// SnapshotResponse wraps the snapshot response stream.
type SnapshotResponse struct {
	// Body is the snapshot data stream. The caller must close it after reading.
	Body io.ReadCloser
	// ContentLength is the expected size of the snapshot, or -1 if unknown.
	ContentLength int64
}

// InitRequest represents the payload sent to PUT /v1/sys/init.
// When using static auto-unseal (seal "static"), secret_shares and secret_threshold
// must not be included in the request as they are not applicable.
// For traditional unseal (shamir), these fields are required.
type InitRequest struct {
	// SecretShares is the number of unseal key shares to generate.
	// Required for traditional Shamir unseal, must be omitted for static seal.
	// +optional
	SecretShares *int `json:"secret_shares,omitempty"`
	// SecretThreshold is the minimum number of shares required to reconstruct the key.
	// Required for traditional Shamir unseal, must be omitted for static seal.
	// +optional
	SecretThreshold *int `json:"secret_threshold,omitempty"`
}

// InitResponse represents the response from PUT /v1/sys/init.
// It includes highly sensitive credentials and must never be logged.
type InitResponse struct {
	UnsealKeysB64 []string `json:"unseal_keys_b64"`
	RootToken     string   `json:"root_token"`
}

// Snapshot retrieves a Raft snapshot from the leader.
// This streams the snapshot data directly from OpenBao without buffering.
// The caller is responsible for closing the returned SnapshotResponse.Body.
//
// The snapshot endpoint requires authentication with a token that has
// read capability on sys/storage/raft/snapshot.
//
// This method should be called on the leader node for best results.
func (c *Client) Snapshot(ctx context.Context) (*SnapshotResponse, error) {
	if c.token == "" {
		return nil, fmt.Errorf("authentication token required for snapshot operation")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+constants.APIPathRaftSnapshot, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot request: %w", err)
	}

	req.Header.Set("X-Vault-Token", c.token)

	// Use a new HTTP client with extended timeout for snapshots
	// The snapshot could be large and take a while to transfer
	snapshotClient := &http.Client{
		Transport: c.httpClient.Transport,
		Timeout:   DefaultSnapshotTimeout,
	}

	resp, err := snapshotClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute snapshot request: %w", err)
	}

	// Check for non-success status codes
	if resp.StatusCode != http.StatusOK {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("snapshot request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return &SnapshotResponse{
		Body:          resp.Body,
		ContentLength: resp.ContentLength,
	}, nil
}

// Init initializes an OpenBao cluster by calling PUT /v1/sys/init.
// This endpoint must only be called on an uninitialized cluster. The caller is
// responsible for handling the returned root token securely.
//
// When using static seal, the request should have nil values for SecretShares
// and SecretThreshold (they will be omitted from the JSON payload).
// For traditional Shamir unseal, both fields must be set to values > 0.
func (c *Client) Init(ctx context.Context, req InitRequest) (*InitResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal init request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+constants.APIPathSysInit, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create init request: %w", err)
	}

	// /v1/sys/init is unauthenticated; no token header is required.
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute init request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("init request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read init response: %w", err)
	}

	var initResp InitResponse
	if err := json.Unmarshal(respBody, &initResp); err != nil {
		return nil, fmt.Errorf("failed to parse init response: %w", err)
	}

	return &initResp, nil
}

// Token returns the authentication token (for creating new clients with the same auth).
func (c *Client) Token() string {
	return c.token
}

// BaseURL returns the base URL of the client.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// JWTAuthLoginResponse represents the response from POST /v1/auth/jwt/login.
type JWTAuthLoginResponse struct {
	Auth struct {
		ClientToken string `json:"client_token"`
		LeaseID     string `json:"lease_id"`
		Renewable   bool   `json:"renewable"`
		TTL         int    `json:"ttl"`
	} `json:"auth"`
}

// LoginJWT authenticates to OpenBao using JWT authentication.
// It sends a POST request to /v1/auth/jwt/login with the role name and JWT token.
// Returns the OpenBao client token from the response.
//
// TIGHTENED: Removed retry loop. If this fails, the Controller will requeue.
// The Kubernetes Controller runtime is a retry loop. If a login fails, return
// the error, exit the Reconcile, and let the workqueue handle the backoff.
// This keeps the client code dumb and the controller logic consistent.
//
// The role must be configured in OpenBao and must bind to the ServiceAccount
// that issued the JWT token.
func (c *Client) LoginJWT(ctx context.Context, role, jwtToken string) (string, error) {
	if role == "" || jwtToken == "" {
		return "", fmt.Errorf("role and jwtToken are required for JWT authentication")
	}

	// Build request body
	reqBody := map[string]string{
		"role": role,
		"jwt":  jwtToken,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT auth request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+constants.APIPathAuthJWTLogin, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create JWT auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute JWT auth request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("JWT auth request failed with status %d: %s", resp.StatusCode, string(body))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read JWT auth response: %w", err)
	}

	var authResp JWTAuthLoginResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return "", fmt.Errorf("failed to parse JWT auth response: %w", err)
	}

	if authResp.Auth.ClientToken == "" {
		return "", fmt.Errorf("JWT auth response missing client_token")
	}

	return authResp.Auth.ClientToken, nil
}
