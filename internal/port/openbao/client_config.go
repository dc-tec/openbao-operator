package openbao

import "time"

const (
	// DefaultConnectionTimeout is the default timeout for establishing connections.
	DefaultConnectionTimeout = 5 * time.Second
	// DefaultRequestTimeout is the default timeout for individual API requests.
	DefaultRequestTimeout = 10 * time.Second
	// DefaultSnapshotTimeout is the default timeout for snapshot operations.
	DefaultSnapshotTimeout = 30 * time.Minute
)

// ClientConfig describes connectivity and auth options for OpenBao API clients.
type ClientConfig struct {
	// ClusterKey is an optional per-cluster identifier used to share client state
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
