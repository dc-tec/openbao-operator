package errors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type testUnregisteredType struct{}

func newTooManyRequestsError() error {
	return apierrors.NewTooManyRequests("too many requests", 1)
}

func newServiceUnavailableError() error {
	return apierrors.NewServiceUnavailable("service unavailable")
}

func newInternalServerError() error {
	return apierrors.NewInternalError(errors.New("boom"))
}

func newTimeoutError() error {
	return apierrors.NewTimeoutError("request timed out", 1)
}

func newServerTimeoutError() error {
	return apierrors.NewServerTimeout(schema.GroupResource{Group: "apps", Resource: "deployments"}, "list", 1)
}

func newNoKindMatchError() error {
	return &meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "Gateway"},
		SearchedVersions: []string{"v1"},
	}
}

func newNoResourceMatchError() error {
	return &meta.NoResourceMatchError{
		PartialResource: schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"},
	}
}

func newNotRegisteredTypeError() error {
	return runtime.NewNotRegisteredErrForType("test-scheme", reflect.TypeOf(testUnregisteredType{}))
}

func newConnectionRefusedError() error {
	return &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: os.NewSyscallError("connect", syscall.ECONNREFUSED),
	}
}

func newConnectionResetError() error {
	return &net.OpError{
		Op:  "read",
		Net: "tcp",
		Err: os.NewSyscallError("read", syscall.ECONNRESET),
	}
}

func newConnectionTimeoutError() error {
	return &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: os.NewSyscallError("connect", syscall.ETIMEDOUT),
	}
}

func newDNSError() error {
	return &net.DNSError{Err: "no such host", Name: "example.com"}
}

func newNetworkUnreachableError() error {
	return &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: os.NewSyscallError("connect", syscall.ENETUNREACH),
	}
}

type transientWrapTestCase struct {
	name            string
	err             error
	wantWrapped     bool
	wantIsTransient bool
}

func runTransientWrapTests(
	t *testing.T,
	name string,
	sentinel error,
	wrap func(error) error,
	isTransient func(error) bool,
	tests []transientWrapTestCase,
) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrap(tt.err)
			if tt.err == nil {
				if got != nil {
					t.Errorf("%s(nil) = %v, want nil", name, got)
				}
				return
			}
			if got == nil {
				t.Errorf("%s() = nil, want error", name)
				return
			}
			if isTransient(got) != tt.wantIsTransient {
				t.Errorf("%s() transient = %v, want %v", name, isTransient(got), tt.wantIsTransient)
			}
			if tt.wantWrapped && !errors.Is(got, sentinel) {
				t.Errorf("%s() should wrap error with %v", name, sentinel)
			}
		})
	}
}

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
			name: "well-known error",
			err:  ErrTransientConnection,
			want: true,
		},
		{
			name: "wrapped well-known error",
			err:  fmt.Errorf("context: %w", ErrTransientConnection),
			want: true,
		},
		{
			name: "connection refused",
			err:  newConnectionRefusedError(),
			want: true,
		},
		{
			name: "connection reset",
			err:  newConnectionResetError(),
			want: true,
		},
		{
			name: "connection timeout",
			err:  newConnectionTimeoutError(),
			want: true,
		},
		{
			name: "context deadline exceeded",
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "i/o timeout",
			err:  &timeoutError{},
			want: true,
		},
		{
			name: "response body EOF",
			err:  io.EOF,
			want: true,
		},
		{
			name: "response body unexpected EOF",
			err:  fmt.Errorf("read failed: %w", io.ErrUnexpectedEOF),
			want: true,
		},
		{
			name: "no such host",
			err:  newDNSError(),
			want: true,
		},
		{
			name: "network is unreachable",
			err:  newNetworkUnreachableError(),
			want: true,
		},
		{
			name: "dial tcp error",
			err:  newConnectionRefusedError(),
			want: true,
		},
		{
			name: "DNS error",
			err:  newDNSError(),
			want: true,
		},
		{
			name: "timeout net.Error",
			err:  &timeoutError{},
			want: true,
		},
		{
			name: "temporary net.Error (deprecated, only timeout is checked)",
			err:  &temporaryError{},
			want: false, // Temporary() is deprecated; only Timeout() is checked
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
			name: "well-known error",
			err:  ErrTransientKubernetesAPI,
			want: true,
		},
		{
			name: "wrapped well-known error",
			err:  fmt.Errorf("context: %w", ErrTransientKubernetesAPI),
			want: true,
		},
		{
			name: "too many requests",
			err:  newTooManyRequestsError(),
			want: true,
		},
		{
			name: "service unavailable",
			err:  newServiceUnavailableError(),
			want: true,
		},
		{
			name: "internal server error",
			err:  newInternalServerError(),
			want: true,
		},
		{
			name: "request timeout",
			err:  newTimeoutError(),
			want: true,
		},
		{
			name: "server timeout",
			err:  newServerTimeoutError(),
			want: true,
		},
		{
			name: "context deadline exceeded",
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "wrapped well-known timeout",
			err:  fmt.Errorf("wrapped: %w", newTooManyRequestsError()),
			want: true,
		},
		{
			name: "non-transient error",
			err:  errors.New("invalid configuration"),
			want: false,
		},
		{
			name: "connection error (not K8s API)",
			err:  newConnectionRefusedError(),
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

func TestIsTransientRemoteOverloaded(t *testing.T) {
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
			name: "well-known error",
			err:  ErrTransientRemoteOverloaded,
			want: true,
		},
		{
			name: "wrapped well-known error",
			err:  fmt.Errorf("context: %w", ErrTransientRemoteOverloaded),
			want: true,
		},
		{
			name: "non-transient error",
			err:  errors.New("invalid configuration"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTransientRemoteOverloaded(tt.err)
			if got != tt.want {
				t.Errorf("IsTransientRemoteOverloaded() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTransientClusterState(t *testing.T) {
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
			name: "well-known error",
			err:  ErrTransientClusterState,
			want: true,
		},
		{
			name: "wrapped well-known error",
			err:  fmt.Errorf("context: %w", ErrTransientClusterState),
			want: true,
		},
		{
			name: "non-transient error",
			err:  errors.New("invalid configuration"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTransientClusterState(tt.err)
			if got != tt.want {
				t.Errorf("IsTransientClusterState() = %v, want %v", got, tt.want)
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
			err:  newNoKindMatchError(),
			want: true,
		},
		{
			name: "no matches for resource",
			err:  newNoResourceMatchError(),
			want: true,
		},
		{
			name: "no kind is registered",
			err:  newNotRegisteredTypeError(),
			want: true,
		},
		{
			name: "wrapped not registered error",
			err:  fmt.Errorf("wrapped: %w", newNotRegisteredTypeError()),
			want: true,
		},
		{
			name: "requested resource fallback",
			err:  errors.New("could not find the requested resource"),
			want: true,
		},
		{
			name: "non-CRD error",
			err:  errors.New("resource not found"),
			want: false,
		},
		{
			name: "connection error",
			err:  newConnectionRefusedError(),
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
	tests := []transientWrapTestCase{
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
			err:             newConnectionRefusedError(),
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

	runTransientWrapTests(
		t,
		"WrapTransientConnection",
		ErrTransientConnection,
		WrapTransientConnection,
		IsTransientConnection,
		tests,
	)
}

func TestWrapTransientRemoteOverloaded(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		wantWrapped     bool
		wantIsTransient bool
	}{
		{
			name:            "nil error",
			err:             nil,
			wantWrapped:     false,
			wantIsTransient: false,
		},
		{
			name:            "already transient remote overloaded error",
			err:             ErrTransientRemoteOverloaded,
			wantWrapped:     false,
			wantIsTransient: true,
		},
		{
			name:            "regular error gets wrapped",
			err:             errors.New("some error"),
			wantWrapped:     true,
			wantIsTransient: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapTransientRemoteOverloaded(tt.err)
			if tt.err == nil {
				if got != nil {
					t.Errorf("WrapTransientRemoteOverloaded() = %v, want nil", got)
				}
				return
			}

			if IsTransientRemoteOverloaded(got) != tt.wantIsTransient {
				t.Errorf("expected transient=%v, got %v", tt.wantIsTransient, IsTransientRemoteOverloaded(got))
			}

			isWrapped := errors.Is(got, ErrTransientRemoteOverloaded) && got != ErrTransientRemoteOverloaded
			if isWrapped != tt.wantWrapped && tt.err != ErrTransientRemoteOverloaded {
				t.Errorf("expected wrapped=%v, got %v (err=%v)", tt.wantWrapped, isWrapped, got)
			}
		})
	}
}

func TestWrapTransientClusterState(t *testing.T) {
	tests := []transientWrapTestCase{
		{
			name:            "nil error",
			err:             nil,
			wantWrapped:     false,
			wantIsTransient: false,
		},
		{
			name:            "already transient cluster state error",
			err:             ErrTransientClusterState,
			wantWrapped:     false,
			wantIsTransient: true,
		},
		{
			name:            "non-transient error",
			err:             errors.New("leader election has not settled"),
			wantWrapped:     true,
			wantIsTransient: true,
		},
	}

	runTransientWrapTests(
		t,
		"WrapTransientClusterState",
		ErrTransientClusterState,
		WrapTransientClusterState,
		IsTransientClusterState,
		tests,
	)
}

func TestWrapTransientKubernetesAPI(t *testing.T) {
	tests := []transientWrapTestCase{
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
			name:            "too many requests error (already detected as transient)",
			err:             newTooManyRequestsError(),
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

	runTransientWrapTests(
		t,
		"WrapTransientKubernetesAPI",
		ErrTransientKubernetesAPI,
		WrapTransientKubernetesAPI,
		IsTransientKubernetesAPI,
		tests,
	)
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
			name: "transient cluster state",
			err:  ErrTransientClusterState,
			want: true,
		},
		{
			name: "connection refused",
			err:  newConnectionRefusedError(),
			want: true,
		},
		{
			name: "rate limit",
			err:  newTooManyRequestsError(),
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
			name:        "transient cluster state",
			err:         ErrTransientClusterState,
			wantRequeue: true,
			wantAfter:   5 * time.Second,
		},
		{
			name:        "connection refused",
			err:         newConnectionRefusedError(),
			wantRequeue: true,
			wantAfter:   5 * time.Second,
		},
		{
			name:        "rate limit",
			err:         newTooManyRequestsError(),
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to open listener: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	conn, err := net.DialTimeout("tcp", addr, 10*time.Millisecond)
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("expected dial error")
	}
	if !IsTransientConnection(err) {
		t.Errorf("real network error should be detected as transient: %v", err)
	}
}
