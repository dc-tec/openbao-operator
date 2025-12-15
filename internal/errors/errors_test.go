package errors

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestIsTransientConnection(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "sentinel error",
			err:  ErrTransientConnection,
			want: true,
		},
		{
			name: "wrapped sentinel error",
			err:  fmt.Errorf("context: %w", ErrTransientConnection),
			want: true,
		},
		{
			name: "connection refused",
			err:  errors.New("connection refused"),
			want: true,
		},
		{
			name: "connection reset",
			err:  errors.New("connection reset by peer"),
			want: true,
		},
		{
			name: "connection timeout",
			err:  errors.New("connection timeout"),
			want: true,
		},
		{
			name: "context deadline exceeded",
			err:  errors.New("context deadline exceeded"),
			want: true,
		},
		{
			name: "i/o timeout",
			err:  errors.New("i/o timeout"),
			want: true,
		},
		{
			name: "no such host",
			err:  errors.New("no such host"),
			want: true,
		},
		{
			name: "network is unreachable",
			err:  errors.New("network is unreachable"),
			want: true,
		},
		{
			name: "dial tcp error",
			err:  errors.New("dial tcp 127.0.0.1:8080: connect: connection refused"),
			want: true,
		},
		{
			name: "DNS error",
			err:  &net.DNSError{Err: "no such host", Name: "example.com"},
			want: true,
		},
		{
			name: "timeout net.Error",
			err:  &timeoutError{},
			want: true,
		},
		{
			name: "temporary net.Error",
			err:  &temporaryError{},
			want: true,
		},
		{
			name: "non-transient error",
			err:  errors.New("invalid configuration"),
			want: false,
		},
		{
			name: "permanent config error",
			err:  ErrPermanentConfig,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTransientConnection(tt.err)
			if got != tt.want {
				t.Errorf("IsTransientConnection() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTransientKubernetesAPI(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "sentinel error",
			err:  ErrTransientKubernetesAPI,
			want: true,
		},
		{
			name: "wrapped sentinel error",
			err:  fmt.Errorf("context: %w", ErrTransientKubernetesAPI),
			want: true,
		},
		{
			name: "rate limit",
			err:  errors.New("rate limit exceeded"),
			want: true,
		},
		{
			name: "too many requests",
			err:  errors.New("too many requests"),
			want: true,
		},
		{
			name: "server error",
			err:  errors.New("server error"),
			want: true,
		},
		{
			name: "service unavailable",
			err:  errors.New("service unavailable"),
			want: true,
		},
		{
			name: "internal server error",
			err:  errors.New("internal server error"),
			want: true,
		},
		{
			name: "context deadline exceeded",
			err:  errors.New("context deadline exceeded"),
			want: true,
		},
		{
			name: "timeout",
			err:  errors.New("timeout"),
			want: true,
		},
		{
			name: "non-transient error",
			err:  errors.New("invalid configuration"),
			want: false,
		},
		{
			name: "connection error (not K8s API)",
			err:  errors.New("connection refused"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTransientKubernetesAPI(tt.err)
			if got != tt.want {
				t.Errorf("IsTransientKubernetesAPI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCRDMissingError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "no matches for kind",
			err:  errors.New("no matches for kind"),
			want: true,
		},
		{
			name: "no kind is registered",
			err:  errors.New("no kind is registered for the type"),
			want: true,
		},
		{
			name: "could not find the requested resource",
			err:  errors.New("could not find the requested resource"),
			want: true,
		},
		{
			name: "case insensitive",
			err:  errors.New("NO MATCHES FOR KIND"),
			want: true,
		},
		{
			name: "non-CRD error",
			err:  errors.New("resource not found"),
			want: false,
		},
		{
			name: "connection error",
			err:  errors.New("connection refused"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCRDMissingError(tt.err)
			if got != tt.want {
				t.Errorf("IsCRDMissingError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWrapTransientConnection(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		wantWrapped     bool // Whether error should be wrapped with ErrTransientConnection
		wantIsTransient bool // Whether result should be detected as transient
	}{
		{
			name:            "nil error",
			err:             nil,
			wantWrapped:     false,
			wantIsTransient: false,
		},
		{
			name:            "already transient connection error",
			err:             ErrTransientConnection,
			wantWrapped:     false, // Returned as-is
			wantIsTransient: true,
		},
		{
			name:            "connection refused (already detected as transient)",
			err:             errors.New("connection refused"),
			wantWrapped:     false, // Already detected as transient, so returned as-is
			wantIsTransient: true,
		},
		{
			name:            "non-transient error",
			err:             errors.New("invalid config"),
			wantWrapped:     true, // Should be wrapped
			wantIsTransient: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapTransientConnection(tt.err)
			if tt.err == nil {
				if got != nil {
					t.Errorf("WrapTransientConnection(nil) = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("WrapTransientConnection() = nil, want error")
				return
			}
			// Check if error is detected as transient
			if IsTransientConnection(got) != tt.wantIsTransient {
				t.Errorf("WrapTransientConnection() result IsTransientConnection() = %v, want %v", IsTransientConnection(got), tt.wantIsTransient)
			}
			// If error should be wrapped, check that it's wrapped with ErrTransientConnection
			if tt.wantWrapped {
				if !errors.Is(got, ErrTransientConnection) {
					t.Errorf("WrapTransientConnection() should wrap error with ErrTransientConnection")
				}
			}
		})
	}
}

func TestWrapTransientKubernetesAPI(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		wantWrapped     bool // Whether error should be wrapped with ErrTransientKubernetesAPI
		wantIsTransient bool // Whether result should be detected as transient
	}{
		{
			name:            "nil error",
			err:             nil,
			wantWrapped:     false,
			wantIsTransient: false,
		},
		{
			name:            "already transient K8s API error",
			err:             ErrTransientKubernetesAPI,
			wantWrapped:     false, // Returned as-is
			wantIsTransient: true,
		},
		{
			name:            "rate limit error (already detected as transient)",
			err:             errors.New("rate limit exceeded"),
			wantWrapped:     false, // Already detected as transient, so returned as-is
			wantIsTransient: true,
		},
		{
			name:            "non-transient error",
			err:             errors.New("invalid config"),
			wantWrapped:     true, // Should be wrapped
			wantIsTransient: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapTransientKubernetesAPI(tt.err)
			if tt.err == nil {
				if got != nil {
					t.Errorf("WrapTransientKubernetesAPI(nil) = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("WrapTransientKubernetesAPI() = nil, want error")
				return
			}
			// Check if error is detected as transient
			if IsTransientKubernetesAPI(got) != tt.wantIsTransient {
				t.Errorf("WrapTransientKubernetesAPI() result IsTransientKubernetesAPI() = %v, want %v", IsTransientKubernetesAPI(got), tt.wantIsTransient)
			}
			// If error should be wrapped, check that it's wrapped with ErrTransientKubernetesAPI
			if tt.wantWrapped {
				if !errors.Is(got, ErrTransientKubernetesAPI) {
					t.Errorf("WrapTransientKubernetesAPI() should wrap error with ErrTransientKubernetesAPI")
				}
			}
		})
	}
}

func TestWrapPermanentConfig(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantIsErr bool
	}{
		{
			name:      "nil error",
			err:       nil,
			wantIsErr: false,
		},
		{
			name:      "regular error",
			err:       errors.New("invalid config"),
			wantIsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapPermanentConfig(tt.err)
			if (got != nil) != tt.wantIsErr {
				t.Errorf("WrapPermanentConfig() error = %v, wantIsErr %v", got, tt.wantIsErr)
				return
			}
			if got != nil && !errors.Is(got, ErrPermanentConfig) {
				t.Errorf("WrapPermanentConfig() wrapped error should be ErrPermanentConfig")
			}
		})
	}
}

func TestWrapPermanentPrerequisitesMissing(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantIsErr bool
	}{
		{
			name:      "nil error",
			err:       nil,
			wantIsErr: false,
		},
		{
			name:      "regular error",
			err:       errors.New("prerequisites missing"),
			wantIsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapPermanentPrerequisitesMissing(tt.err)
			if (got != nil) != tt.wantIsErr {
				t.Errorf("WrapPermanentPrerequisitesMissing() error = %v, wantIsErr %v", got, tt.wantIsErr)
				return
			}
			if got != nil && !errors.Is(got, ErrPermanentPrerequisitesMissing) {
				t.Errorf("WrapPermanentPrerequisitesMissing() wrapped error should be ErrPermanentPrerequisitesMissing")
			}
		})
	}
}

func TestWrapCRDMissing(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantWrapped bool
		wantIsErr   bool
	}{
		{
			name:        "nil error",
			err:         nil,
			wantWrapped: false,
			wantIsErr:   false,
		},
		{
			name:        "CRD missing error",
			err:         errors.New("no matches for kind"),
			wantWrapped: true,
			wantIsErr:   true,
		},
		{
			name:        "non-CRD error",
			err:         errors.New("connection refused"),
			wantWrapped: false,
			wantIsErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapCRDMissing(tt.err)
			if (got != nil) != tt.wantIsErr {
				t.Errorf("WrapCRDMissing() error = %v, wantIsErr %v", got, tt.wantIsErr)
				return
			}
			if got == nil {
				return
			}
			if tt.wantWrapped {
				if !errors.Is(got, ErrPermanentConfig) {
					t.Errorf("WrapCRDMissing() should wrap CRD missing errors as ErrPermanentConfig")
				}
			} else {
				// Non-CRD errors should be returned as-is
				if got != tt.err {
					t.Errorf("WrapCRDMissing() should return non-CRD errors as-is")
				}
			}
		})
	}
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "transient connection",
			err:  ErrTransientConnection,
			want: true,
		},
		{
			name: "transient K8s API",
			err:  ErrTransientKubernetesAPI,
			want: true,
		},
		{
			name: "connection refused",
			err:  errors.New("connection refused"),
			want: true,
		},
		{
			name: "rate limit",
			err:  errors.New("rate limit exceeded"),
			want: true,
		},
		{
			name: "permanent config",
			err:  ErrPermanentConfig,
			want: false,
		},
		{
			name: "non-transient error",
			err:  errors.New("invalid config"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTransient(tt.err)
			if got != tt.want {
				t.Errorf("IsTransient() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsPermanent(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "permanent config",
			err:  ErrPermanentConfig,
			want: true,
		},
		{
			name: "permanent prerequisites missing",
			err:  ErrPermanentPrerequisitesMissing,
			want: true,
		},
		{
			name: "wrapped permanent config",
			err:  WrapPermanentConfig(errors.New("invalid")),
			want: true,
		},
		{
			name: "transient connection",
			err:  ErrTransientConnection,
			want: false,
		},
		{
			name: "non-permanent error",
			err:  errors.New("some error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPermanent(tt.err)
			if got != tt.want {
				t.Errorf("IsPermanent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldRequeue(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantRequeue bool
		wantAfter   time.Duration
	}{
		{
			name:        "nil error",
			err:         nil,
			wantRequeue: false,
			wantAfter:   0,
		},
		{
			name:        "transient connection",
			err:         ErrTransientConnection,
			wantRequeue: true,
			wantAfter:   5 * time.Second,
		},
		{
			name:        "transient K8s API",
			err:         ErrTransientKubernetesAPI,
			wantRequeue: true,
			wantAfter:   5 * time.Second,
		},
		{
			name:        "connection refused",
			err:         errors.New("connection refused"),
			wantRequeue: true,
			wantAfter:   5 * time.Second,
		},
		{
			name:        "rate limit",
			err:         errors.New("rate limit exceeded"),
			wantRequeue: true,
			wantAfter:   5 * time.Second,
		},
		{
			name:        "permanent config",
			err:         ErrPermanentConfig,
			wantRequeue: false,
			wantAfter:   0,
		},
		{
			name:        "permanent prerequisites missing",
			err:         ErrPermanentPrerequisitesMissing,
			wantRequeue: false,
			wantAfter:   0,
		},
		{
			name:        "unknown error",
			err:         errors.New("unknown error"),
			wantRequeue: true,
			wantAfter:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRequeue, gotAfter := ShouldRequeue(tt.err)
			if gotRequeue != tt.wantRequeue {
				t.Errorf("ShouldRequeue() requeue = %v, want %v", gotRequeue, tt.wantRequeue)
			}
			if gotAfter != tt.wantAfter {
				t.Errorf("ShouldRequeue() after = %v, want %v", gotAfter, tt.wantAfter)
			}
		})
	}
}

// Helper types for testing net.Error interface

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return false }

type temporaryError struct{}

func (e *temporaryError) Error() string   { return "temporary" }
func (e *temporaryError) Timeout() bool   { return false }
func (e *temporaryError) Temporary() bool { return true }

// Test context cancellation errors
func TestIsTransientConnection_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Context cancellation (context.Canceled) is not a connection error per se,
	// but it indicates an operation was cancelled, which could be retried.
	// However, our current implementation doesn't detect it as transient connection.
	// This is acceptable behavior - context.Canceled is different from network errors.
	err := ctx.Err()
	// context.Canceled is "context canceled", which doesn't match our patterns
	// This is expected - cancellation is intentional, not a network failure
	if IsTransientConnection(err) {
		t.Logf("Note: context.Canceled is detected as transient (this may or may not be desired)")
	}
}

// Test context timeout errors
func TestIsTransientConnection_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for timeout
	time.Sleep(10 * time.Millisecond)

	err := ctx.Err()
	if !IsTransientConnection(err) {
		t.Errorf("context.DeadlineExceeded should be detected as transient connection error")
	}
}

// Test real network errors
func TestIsTransientConnection_RealNetworkError(t *testing.T) {
	// Try to dial a non-existent address
	conn, err := net.DialTimeout("tcp", "127.0.0.1:99999", 10*time.Millisecond)
	if conn != nil {
		_ = conn.Close()
	}
	if err != nil {
		if !IsTransientConnection(err) {
			t.Errorf("real network error should be detected as transient: %v", err)
		}
	}
}
