package openbao

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

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

type smartClientState struct {
	limiter *rate.Limiter

	mu       sync.Mutex
	breakers map[string]*circuitBreaker

	failureThreshold int
	openDuration     time.Duration
}

var smartClientStates sync.Map // map[string]*smartClientState

func smartStateKey(clusterKey string) string {
	if clusterKey == "" {
		return ""
	}
	return clusterKey
}

func getOrCreateSmartState(cfg ClientConfig) *smartClientState {
	key := smartStateKey(cfg.ClusterKey)
	if key == "" {
		return nil
	}

	if existing, ok := smartClientStates.Load(key); ok {
		return existing.(*smartClientState)
	}

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

	state := &smartClientState{
		limiter:          rate.NewLimiter(rate.Limit(qps), burst),
		breakers:         make(map[string]*circuitBreaker),
		failureThreshold: failureThreshold,
		openDuration:     openDuration,
	}

	// Avoid overwriting if another goroutine won the race.
	actual, _ := smartClientStates.LoadOrStore(key, state)
	return actual.(*smartClientState)
}

func (s *smartClientState) requestKey(req *http.Request) string {
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

func (s *smartClientState) allow(ctx context.Context, req *http.Request) error {
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

func (s *smartClientState) after(req *http.Request, success bool) {
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
