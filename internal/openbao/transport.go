package openbao

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"golang.org/x/time/rate"
)

const (
	defaultRateLimitQPS   = 2.0
	defaultRateLimitBurst = 4

	// defaultCircuitBreakerFailureThreshold is intentionally conservative. With a per-cluster
	// rate limit of 2rps, 50 consecutive failures means we back off after ~25s of sustained
	// failure, which prevents spamming but still allows short blips to heal without tripping.
	defaultCircuitBreakerFailureThreshold = 50
	defaultCircuitBreakerOpenDuration     = 30 * time.Second
)

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

type circuitBreaker struct {
	failures         int
	state            circuitState
	openUntil        time.Time
	halfOpenInFlight bool
}

// clientState manages rate limiting and circuit breaking for a cluster.
// It is owned by ClientManager for explicit lifecycle management.
type clientState struct {
	limiter *rate.Limiter

	mu       sync.Mutex
	breakers map[string]*circuitBreaker

	failureThreshold int
	openDuration     time.Duration
}

// newClientState creates a new clientState with the given configuration.
// This is used by ClientManager for explicit state management.
func newClientState(cfg ClientConfig) *clientState {
	qps := cfg.RateLimitQPS
	if qps <= 0 {
		qps = defaultRateLimitQPS
	}
	burst := cfg.RateLimitBurst
	if burst <= 0 {
		burst = defaultRateLimitBurst
	}

	failureThreshold := cfg.CircuitBreakerFailureThreshold
	if failureThreshold <= 0 {
		failureThreshold = defaultCircuitBreakerFailureThreshold
	}
	openDuration := cfg.CircuitBreakerOpenDuration
	if openDuration <= 0 {
		openDuration = defaultCircuitBreakerOpenDuration
	}

	return &clientState{
		limiter:          rate.NewLimiter(rate.Limit(qps), burst),
		breakers:         make(map[string]*circuitBreaker),
		failureThreshold: failureThreshold,
		openDuration:     openDuration,
	}
}

func (s *clientState) requestKey(req *http.Request) string {
	if req == nil || req.URL == nil {
		return "unknown"
	}
	host := req.URL.Host
	if host == "" {
		host = "unknown-host"
	}
	path := req.URL.Path
	if path == "" {
		path = "/"
	}
	return fmt.Sprintf("%s %s %s", host, req.Method, path)
}

func (s *clientState) allow(ctx context.Context, req *http.Request) error {
	if s == nil {
		return nil
	}

	reqKey := s.requestKey(req)

	now := time.Now()
	s.mu.Lock()
	br := s.breakers[reqKey]
	if br == nil {
		br = &circuitBreaker{state: circuitClosed}
		s.breakers[reqKey] = br
	}

	switch br.state {
	case circuitOpen:
		if now.Before(br.openUntil) {
			until := br.openUntil
			s.mu.Unlock()
			return operatorerrors.WrapTransientRemoteOverloaded(
				fmt.Errorf("openbao circuit breaker open for %s (retry after %s)", reqKey, time.Until(until).Truncate(time.Second)),
			)
		}
		br.state = circuitHalfOpen
		br.halfOpenInFlight = false
	case circuitHalfOpen:
		if br.halfOpenInFlight {
			s.mu.Unlock()
			return operatorerrors.WrapTransientRemoteOverloaded(
				fmt.Errorf("openbao circuit breaker half-open (probe in-flight) for %s", reqKey),
			)
		}
	case circuitClosed:
	}

	wasHalfOpenProbe := false
	if br.state == circuitHalfOpen {
		br.halfOpenInFlight = true
		wasHalfOpenProbe = true
	}
	s.mu.Unlock()

	if err := s.limiter.Wait(ctx); err != nil {
		if wasHalfOpenProbe {
			s.mu.Lock()
			br.halfOpenInFlight = false
			s.mu.Unlock()
		}
		return err
	}

	return nil
}

func (s *clientState) after(req *http.Request, success bool) {
	if s == nil {
		return
	}

	reqKey := s.requestKey(req)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	br := s.breakers[reqKey]
	if br == nil {
		br = &circuitBreaker{state: circuitClosed}
		s.breakers[reqKey] = br
	}

	switch br.state {
	case circuitHalfOpen:
		br.halfOpenInFlight = false
		if success {
			br.state = circuitClosed
			br.failures = 0
			br.openUntil = time.Time{}
			return
		}
		br.state = circuitOpen
		br.failures = s.failureThreshold
		br.openUntil = now.Add(s.openDuration)
		return
	case circuitOpen:
		// Keep open; allow() handles transition to half-open when openUntil expires.
		if success {
			// Receiving a "success" while open is unexpected because allow() blocks, but
			// be defensive and close the circuit to avoid stuck-open behavior.
			br.state = circuitClosed
			br.failures = 0
			br.openUntil = time.Time{}
		}
		return
	case circuitClosed:
		if success {
			br.failures = 0
			return
		}
		br.failures++
		if br.failures >= s.failureThreshold {
			br.state = circuitOpen
			br.openUntil = now.Add(s.openDuration)
		}
		return
	default:
	}
}

func (c *Client) newRequest(ctx context.Context, method string, path string, body io.Reader) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
}

func (c *Client) doRequest(req *http.Request, httpClient *http.Client, op string) (*http.Response, error) {
	if httpClient == nil {
		httpClient = c.httpClient
	}

	if c.state != nil {
		if err := c.state.allow(req.Context(), req); err != nil {
			return nil, err
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		wrapped := fmt.Errorf("%s: %w", op, err)
		if c.state != nil {
			c.state.after(req, false)
		}
		if operatorerrors.IsTransientConnection(err) {
			return nil, operatorerrors.WrapTransientConnection(wrapped)
		}
		return nil, wrapped
	}
	return resp, nil
}

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func (c *Client) doAndReadAll(req *http.Request, httpClient *http.Client, op string) (*http.Response, []byte, error) {
	resp, err := c.doRequest(req, httpClient, op)
	if err != nil {
		return nil, nil, err
	}

	defer drainAndClose(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if c.state != nil {
			c.state.after(req, false)
		}
		return nil, nil, fmt.Errorf("%s: failed to read response body: %w", op, err)
	}

	// The health endpoint encodes state in HTTP status codes (sealed, standby, etc.),
	// so we must not classify 429/5xx responses as overload.
	if req.URL != nil && req.URL.Path != constants.APIPathSysHealth {
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if c.state != nil {
				c.state.after(req, false)
			}
			return nil, nil, operatorerrors.WrapTransientRemoteOverloaded(
				fmt.Errorf("%s: OpenBao API overloaded (status %d): %s", op, resp.StatusCode, string(body)),
			)
		}
	}

	if c.state != nil {
		c.state.after(req, true)
	}
	return resp, body, nil
}
