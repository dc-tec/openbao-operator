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
	"time"

	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
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

	smart *smartClientState
}

// ClientConfig holds configuration for creating a new Client.
type ClientConfig struct {
	// ClusterKey is an optional per-cluster identifier used to share smart-client state
	// (rate limiting and circuit breakers) across multiple Client instances.
	//
	// Recommended format: "<namespace>/<name>".
	// If empty, BaseURL hostname is used as a fallback (best-effort).
	ClusterKey string

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

	// SmartClientDisabled disables rate limiting and circuit breaker behavior.
	// By default, smart client features are enabled with conservative defaults.
	SmartClientDisabled bool
	// RateLimitQPS is the per-cluster rate limit applied to OpenBao API calls.
	// Defaults to 2.0 if zero or negative.
	RateLimitQPS float64
	// RateLimitBurst is the per-cluster burst size for the rate limiter.
	// Defaults to 4 if zero or negative.
	RateLimitBurst int
	// CircuitBreakerFailureThreshold is the number of consecutive failures before opening the circuit.
	// Defaults to 50 if zero or negative.
	CircuitBreakerFailureThreshold int
	// CircuitBreakerOpenDuration is the amount of time the circuit stays open before probing again.
	// Defaults to 30s if zero.
	CircuitBreakerOpenDuration time.Duration
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

	if config.ClusterKey == "" && parsedURL.Host != "" {
		// Best-effort fallback for callers that don't provide a cluster identifier.
		// Include the port to avoid sharing smart-client state across unrelated endpoints
		// (e.g., multiple httptest servers in unit tests).
		config.ClusterKey = parsedURL.Host
	}

	var smart *smartClientState
	if !config.SmartClientDisabled {
		smart = getOrCreateSmartState(config)
	}

	return &Client{
		baseURL:    config.BaseURL,
		token:      config.Token,
		httpClient: httpClient,
		smart:      smart,
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
	req, err := c.newRequest(ctx, http.MethodGet, constants.APIPathSysHealth, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create health request: %w", err)
	}

	// The health endpoint can optionally accept a token for authenticated checks
	if c.token != "" {
		req.Header.Set("X-Vault-Token", c.token)
	}

	_, body, err := c.doAndReadAll(req, nil, "failed to query health endpoint")
	if err != nil {
		return nil, err
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

	req, err := c.newRequest(ctx, http.MethodPut, constants.APIPathSysStepDown, nil)
	if err != nil {
		return fmt.Errorf("failed to create step-down request: %w", err)
	}

	req.Header.Set("X-Vault-Token", c.token)

	resp, body, err := c.doAndReadAll(req, nil, "failed to execute step-down request")
	if err != nil {
		return err
	}

	// 204 No Content is the expected success response
	// 200 OK is also acceptable
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
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

// IsSealed checks if the OpenBao cluster is sealed.
// This implements the ClusterActions interface.
func (c *Client) IsSealed(ctx context.Context) (bool, error) {
	health, err := c.Health(ctx)
	if err != nil {
		return false, err
	}

	return health.Sealed, nil
}

// StepDownLeader requests the leader to step down and trigger a new election.
// This is a wrapper around StepDown that implements the ClusterActions interface.
func (c *Client) StepDownLeader(ctx context.Context) error {
	return c.StepDown(ctx)
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

// Snapshot implements the ClusterActions interface by writing the snapshot to the provided writer.
// This streams the snapshot data directly from OpenBao to the writer without buffering.
// The snapshot endpoint requires authentication with a token that has
// read capability on sys/storage/raft/snapshot.
//
// This method should be called on the leader node for best results.
func (c *Client) Snapshot(ctx context.Context, writer io.Writer) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for snapshot operation")
	}

	req, err := c.newRequest(ctx, http.MethodGet, constants.APIPathRaftSnapshot, nil)
	if err != nil {
		return fmt.Errorf("failed to create snapshot request: %w", err)
	}

	req.Header.Set("X-Vault-Token", c.token)

	// Use a new HTTP client with extended timeout for snapshots
	// The snapshot could be large and take a while to transfer
	snapshotClient := &http.Client{
		Transport: c.httpClient.Transport,
		Timeout:   DefaultSnapshotTimeout,
	}

	resp, err := c.doRequest(req, snapshotClient, "failed to execute snapshot request")
	if err != nil {
		return err
	}
	defer func() {
		drainAndClose(resp)
	}()

	// Check for non-success status codes
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if c.smart != nil {
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				c.smart.after(req, false)
				return operatorerrors.WrapTransientRemoteOverloaded(
					fmt.Errorf("snapshot request failed due to remote overload (status %d): %s", resp.StatusCode, string(body)),
				)
			}
			c.smart.after(req, true)
		}
		return fmt.Errorf("snapshot request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Stream the snapshot data directly to the writer
	if _, err := io.Copy(writer, resp.Body); err != nil {
		if c.smart != nil {
			c.smart.after(req, false)
		}
		if operatorerrors.IsTransientConnection(err) {
			return operatorerrors.WrapTransientConnection(fmt.Errorf("failed to write snapshot data: %w", err))
		}
		return fmt.Errorf("failed to write snapshot data: %w", err)
	}

	if c.smart != nil {
		c.smart.after(req, true)
	}
	return nil
}

// Restore restores a snapshot to the cluster using the force restore API.
// This method calls POST /sys/storage/raft/snapshot-force which replaces ALL Raft data.
// WARNING: This operation is destructive and irreversible.
//
// The restore endpoint requires authentication with a token that has
// update capability on sys/storage/raft/snapshot-force.
//
// The reader should provide the raw snapshot data (binary gzip format).
// For large snapshots, streaming is used to avoid loading the entire file into memory.
func (c *Client) Restore(ctx context.Context, reader io.Reader) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for restore operation")
	}

	req, err := c.newRequest(ctx, http.MethodPost, constants.APIPathRaftSnapshotForceRestore, reader)
	if err != nil {
		return fmt.Errorf("failed to create restore request: %w", err)
	}

	req.Header.Set("X-Vault-Token", c.token)
	req.Header.Set("Content-Type", "application/octet-stream")

	// Use a new HTTP client with extended timeout for restores
	// The restore could take a while for large snapshots
	restoreClient := &http.Client{
		Transport: c.httpClient.Transport,
		Timeout:   DefaultSnapshotTimeout,
	}

	resp, body, err := c.doAndReadAll(req, restoreClient, "failed to execute restore request")
	if err != nil {
		return err
	}

	// Check for success status codes (200 OK or 204 No Content)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("restore request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
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

	httpReq, err := c.newRequest(ctx, http.MethodPut, constants.APIPathSysInit, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create init request: %w", err)
	}

	// /v1/sys/init is unauthenticated; no token header is required.
	httpReq.Header.Set("Content-Type", "application/json")

	resp, respBody, err := c.doAndReadAll(httpReq, nil, "failed to execute init request")
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("init request failed with status %d: %s", resp.StatusCode, string(respBody))
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
func (c *Client) LoginJWT(ctx context.Context, role, jwtToken string) (string, int, error) {
	if role == "" || jwtToken == "" {
		return "", 0, fmt.Errorf("role and jwtToken are required for JWT authentication")
	}

	// Build request body
	reqBody := map[string]string{
		"role": role,
		"jwt":  jwtToken,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal JWT auth request: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, constants.APIPathAuthJWTLogin, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create JWT auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, respBody, err := c.doAndReadAll(req, nil, "failed to execute JWT auth request")
	if err != nil {
		return "", 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("JWT auth request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var authResp JWTAuthLoginResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return "", 0, fmt.Errorf("failed to parse JWT auth response: %w", err)
	}

	if authResp.Auth.ClientToken == "" {
		return "", 0, fmt.Errorf("JWT auth response missing client_token")
	}

	return authResp.Auth.ClientToken, authResp.Auth.TTL, nil
}

// JoinRaftClusterRequest represents the payload sent to PUT /v1/sys/storage/raft/join.
type JoinRaftClusterRequest struct {
	// LeaderAPIAddr is the address of the leader node to join.
	LeaderAPIAddr string `json:"leader_api_addr"`
	// Retry indicates whether to retry joining if the initial attempt fails.
	Retry bool `json:"retry,omitempty"`
	// NonVoter indicates whether this node should join as a non-voter.
	// Non-voters receive snapshots and logs but do not participate in quorum.
	NonVoter bool `json:"non_voter,omitempty"`
}

// JoinRaftClusterResponse represents the response from PUT /v1/sys/storage/raft/join.
type JoinRaftClusterResponse struct {
	Joined bool `json:"joined"`
}

// JoinRaftCluster joins a node to the Raft cluster.
// This endpoint requires authentication with a token that has
// update capability on sys/storage/raft/join.
//
// The leaderAPIAddr should be the full API address of the leader node
// (e.g., "https://pod-0.cluster.ns.svc:8200").
//
// When nonVoter is true, the node joins as a non-voter, which means it
// receives snapshots and logs but does not participate in quorum decisions.
// This is used during blue/green upgrades to safely synchronize data
// before promoting the node to a voter.
func (c *Client) JoinRaftCluster(ctx context.Context, leaderAPIAddr string, retry bool, nonVoter bool) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for raft join operation")
	}

	if leaderAPIAddr == "" {
		return fmt.Errorf("leaderAPIAddr is required")
	}

	reqBody := JoinRaftClusterRequest{
		LeaderAPIAddr: leaderAPIAddr,
		Retry:         retry,
		NonVoter:      nonVoter,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal raft join request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, http.MethodPut, constants.APIPathRaftJoin, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create raft join request: %w", err)
	}

	httpReq.Header.Set("X-Vault-Token", c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, body, err := c.doAndReadAll(httpReq, nil, "failed to execute raft join request")
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("raft join request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to check if join actually succeeded.
	// OpenBao returns HTTP 200 with {"joined": false} when node is already initialized
	// as a standalone cluster - this happens if the node self-elected before the join job ran.
	var joinResp JoinRaftClusterResponse
	if err := json.Unmarshal(body, &joinResp); err != nil {
		// If we can't parse the response, log warning but don't fail
		// as older versions may not return this field
		return nil
	}

	if !joinResp.Joined {
		return fmt.Errorf("node was not joined to cluster (already initialized as standalone)")
	}

	return nil
}

// RaftServer represents a server in the Raft configuration.
type RaftServer struct {
	// NodeID is the unique identifier for this node.
	NodeID string `json:"node_id"`
	// Address is the API address of this node.
	Address string `json:"address"`
	// Leader indicates whether this node is the current leader.
	Leader bool `json:"leader,omitempty"`
	// ProtocolVersion is the Raft protocol version.
	ProtocolVersion string `json:"protocol_version,omitempty"`
	// Voter indicates whether this node is a voter (participates in quorum).
	Voter bool `json:"voter,omitempty"`
	// LastIndex is the last log index on this node.
	LastIndex uint64 `json:"last_index,omitempty"`
	// LastTerm is the last log term on this node.
	LastTerm uint64 `json:"last_term,omitempty"`
}

// RaftConfiguration represents the current Raft cluster configuration.
type RaftConfiguration struct {
	// Servers is the list of servers in the cluster.
	Servers []RaftServer `json:"servers"`
	// Index is the configuration index.
	Index uint64 `json:"index"`
}

// RaftConfigurationResponse represents the response from GET /v1/sys/storage/raft/configuration.
type RaftConfigurationResponse struct {
	// Config is the Raft configuration.
	Config RaftConfiguration `json:"config"`
}

// ReadRaftConfiguration reads the current Raft cluster configuration.
// This endpoint requires authentication with a token that has
// read capability on sys/storage/raft/configuration.
//
// This is used to check synchronization status by comparing
// last_log_index values between Blue and Green nodes.
func (c *Client) ReadRaftConfiguration(ctx context.Context) (*RaftConfigurationResponse, error) {
	if c.token == "" {
		return nil, fmt.Errorf("authentication token required for raft configuration read")
	}

	req, err := c.newRequest(ctx, http.MethodGet, constants.APIPathRaftConfiguration, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft configuration request: %w", err)
	}

	req.Header.Set("X-Vault-Token", c.token)

	resp, body, err := c.doAndReadAll(req, nil, "failed to execute raft configuration request")
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("raft configuration request failed with status %d: %s", resp.StatusCode, string(body))
	}

	type raftConfigEnvelope struct {
		Data *RaftConfigurationResponse `json:"data,omitempty"`
		RaftConfigurationResponse
	}

	var envelope raftConfigEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse raft configuration response: %w", err)
	}

	if envelope.Data != nil {
		return envelope.Data, nil
	}

	return &envelope.RaftConfigurationResponse, nil
}

// RaftAutopilotServerState represents the server state returned by Raft Autopilot.
type RaftAutopilotServerState struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	NodeStatus  string `json:"node_status"`
	LastContact string `json:"last_contact"`
	LastTerm    uint64 `json:"last_term"`
	LastIndex   uint64 `json:"last_index"`
	Healthy     bool   `json:"healthy"`
	StableSince string `json:"stable_since"`
	Status      string `json:"status"`
	// Meta may contain arbitrary server metadata. We keep it as raw JSON because
	// the schema is not stable across OpenBao versions.
	Meta json.RawMessage `json:"meta,omitempty"`
}

// RaftAutopilotStateResponse represents the response from GET /v1/sys/storage/raft/autopilot/state.
type RaftAutopilotStateResponse struct {
	Healthy          bool                                `json:"healthy"`
	FailureTolerance int                                 `json:"failure_tolerance"`
	Servers          map[string]RaftAutopilotServerState `json:"servers"`
	Leader           string                              `json:"leader"`
	Voters           []string                            `json:"voters"`
	NonVoters        []string                            `json:"non_voters"`
}

// ReadRaftAutopilotState reads the Raft Autopilot cluster state.
// This endpoint requires authentication with a token that has
// read capability on sys/storage/raft/autopilot/state.
func (c *Client) ReadRaftAutopilotState(ctx context.Context) (*RaftAutopilotStateResponse, error) {
	if c.token == "" {
		return nil, fmt.Errorf("authentication token required for raft autopilot state read")
	}

	req, err := c.newRequest(ctx, http.MethodGet, constants.APIPathRaftAutopilotState, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft autopilot state request: %w", err)
	}

	req.Header.Set("X-Vault-Token", c.token)

	resp, body, err := c.doAndReadAll(req, nil, "failed to execute raft autopilot state request")
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrAutopilotNotAvailable
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("raft autopilot state request failed with status %d: %s", resp.StatusCode, string(body))
	}

	type raftAutopilotEnvelope struct {
		Data *RaftAutopilotStateResponse `json:"data,omitempty"`
		RaftAutopilotStateResponse
	}

	var envelope raftAutopilotEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse raft autopilot state response: %w", err)
	}

	if envelope.Data != nil {
		return envelope.Data, nil
	}

	return &envelope.RaftAutopilotStateResponse, nil
}

// AutopilotConfig represents the configuration for Raft Autopilot.
// This is used with POST /v1/sys/storage/raft/autopilot/configuration.
type AutopilotConfig struct {
	// CleanupDeadServers controls whether dead servers are removed from
	// the Raft peer list periodically. Requires MinQuorum to be set.
	CleanupDeadServers bool `json:"cleanup_dead_servers"`
	// DeadServerLastContactThreshold is the limit on the amount of time
	// a server can go without leader contact before being considered failed.
	// Minimum: "1m". Default: "24h".
	DeadServerLastContactThreshold string `json:"dead_server_last_contact_threshold,omitempty"`
	// MinQuorum is the minimum number of servers allowed in a cluster before
	// autopilot can prune dead servers. This should be at least 3.
	MinQuorum int `json:"min_quorum,omitempty"`
	// LastContactThreshold is the limit on the amount of time a server can
	// go without leader contact before being considered unhealthy.
	LastContactThreshold string `json:"last_contact_threshold,omitempty"`
	// ServerStabilizationTime is the minimum amount of time a server must
	// be in a stable, healthy state before it can be added to the cluster.
	ServerStabilizationTime string `json:"server_stabilization_time,omitempty"`
}

// ConfigureRaftAutopilot sets the Raft Autopilot configuration.
// This endpoint requires authentication with a token that has
// update capability on sys/storage/raft/autopilot/configuration.
//
// This is used to enable dead server cleanup and configure thresholds.
func (c *Client) ConfigureRaftAutopilot(ctx context.Context, config AutopilotConfig) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for raft autopilot configuration")
	}

	bodyBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal autopilot config: %w", err)
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, constants.APIPathRaftAutopilotConfig, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create autopilot config request: %w", err)
	}

	httpReq.Header.Set("X-Vault-Token", c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, body, err := c.doAndReadAll(httpReq, nil, "failed to execute autopilot config request")
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("autopilot config request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// RemoveRaftPeerRequest represents the payload sent to POST /v1/sys/storage/raft/remove-peer.
type RemoveRaftPeerRequest struct {
	// ServerID is the node ID of the peer to remove.
	ServerID string `json:"server_id"`
}

// RemoveRaftPeer removes a peer from the Raft cluster.
// This endpoint requires authentication with a token that has
// update capability on sys/storage/raft/remove-peer.
//
// This is used during blue/green upgrades to eject Blue nodes
// after the cutover to Green is complete.
func (c *Client) RemoveRaftPeer(ctx context.Context, serverID string) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for raft remove-peer operation")
	}

	if serverID == "" {
		return fmt.Errorf("serverID is required")
	}

	reqBody := RemoveRaftPeerRequest{
		ServerID: serverID,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal raft remove-peer request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, constants.APIPathRaftRemovePeer, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create raft remove-peer request: %w", err)
	}

	httpReq.Header.Set("X-Vault-Token", c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, body, err := c.doAndReadAll(httpReq, nil, "failed to execute raft remove-peer request")
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("raft remove-peer request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// PromoteRaftPeerRequest represents the payload sent to POST /v1/sys/storage/raft/promote.
type PromoteRaftPeerRequest struct {
	// ServerID is the node ID of the peer to promote to voter.
	ServerID string `json:"server_id"`
}

// PromoteRaftPeer promotes a non-voter peer to a voter in the Raft cluster.
// This endpoint requires authentication with a token that has
// update capability on sys/storage/raft/configuration.
//
// This is used during blue/green upgrades to promote Green nodes from
// non-voters to voters after they have synchronized with the leader.
func (c *Client) PromoteRaftPeer(ctx context.Context, serverID string) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for raft promote operation")
	}

	if serverID == "" {
		return fmt.Errorf("serverID is required")
	}

	reqBody := PromoteRaftPeerRequest{
		ServerID: serverID,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal raft promote request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, constants.APIPathRaftPromotePeer, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create raft promote request: %w", err)
	}

	httpReq.Header.Set("X-Vault-Token", c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, body, err := c.doAndReadAll(httpReq, nil, "failed to execute raft promote request")
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("raft promote request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DemoteRaftPeerRequest represents the payload sent to POST /v1/sys/storage/raft/demote.
type DemoteRaftPeerRequest struct {
	// ServerID is the node ID of the peer to demote to non-voter.
	ServerID string `json:"server_id"`
}

// DemoteRaftPeer demotes a voter peer to a non-voter in the Raft cluster.
// This endpoint requires authentication with a token that has
// update capability on sys/storage/raft/configuration.
//
// This is used during blue/green upgrades to demote Blue nodes from
// voters to non-voters before removal, preventing them from becoming leader.
func (c *Client) DemoteRaftPeer(ctx context.Context, serverID string) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for raft demote operation")
	}

	if serverID == "" {
		return fmt.Errorf("serverID is required")
	}

	reqBody := DemoteRaftPeerRequest{
		ServerID: serverID,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal raft demote request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, constants.APIPathRaftDemotePeer, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create raft demote request: %w", err)
	}

	httpReq.Header.Set("X-Vault-Token", c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, body, err := c.doAndReadAll(httpReq, nil, "failed to execute raft demote request")
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("raft demote request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// UpdateRaftConfigurationRequest represents the payload sent to PUT /v1/sys/storage/raft/configuration.
type UpdateRaftConfigurationRequest struct {
	// Servers is the list of servers in the cluster with updated configuration.
	Servers []RaftServer `json:"servers"`
}

// UpdateRaftConfiguration updates the Raft cluster configuration.
// This endpoint requires authentication with a token that has
// update capability on sys/storage/raft/configuration.
//
// This is used during blue/green upgrades to promote Green nodes to voters
// or demote Blue nodes to non-voters.
func (c *Client) UpdateRaftConfiguration(ctx context.Context, servers []RaftServer) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for raft configuration update")
	}

	if len(servers) == 0 {
		return fmt.Errorf("servers list cannot be empty")
	}

	reqBody := UpdateRaftConfigurationRequest{
		Servers: servers,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal raft configuration update request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, http.MethodPut, constants.APIPathRaftUpdateConfig, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create raft configuration update request: %w", err)
	}

	httpReq.Header.Set("X-Vault-Token", c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, body, err := c.doAndReadAll(httpReq, nil, "failed to execute raft configuration update request")
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("raft configuration update request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
