package errors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

// ReasonedError wraps an error with a low-cardinality reason string that can be
// surfaced in status without forcing consumers to parse error text.
type ReasonedError struct {
	Reason string
	Err    error
}

func (e *ReasonedError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Reason
	}
	if strings.TrimSpace(e.Reason) == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Reason, e.Err)
}

func (e *ReasonedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// WithReason annotates err with a reason string.
func WithReason(reason string, err error) error {
	if err == nil {
		return nil
	}
	return &ReasonedError{Reason: reason, Err: err}
}

// Reason extracts a reason string from an error annotated via WithReason.
func Reason(err error) (string, bool) {
	var rerr *ReasonedError
	if errors.As(err, &rerr) {
		if rerr != nil && strings.TrimSpace(rerr.Reason) != "" {
			return rerr.Reason, true
		}
	}
	return "", false
}

// Transient errors indicate temporary conditions that should be retried.
// These errors typically result in requeue with a delay.

// ErrTransientConnection indicates a transient connection error that should be retried.
// This includes timeouts, connection refused, DNS resolution failures, and network unreachable errors.
var ErrTransientConnection = errors.New("transient connection error")

// ErrTransientRemoteOverloaded indicates the remote service is overloaded and requests should be retried later.
// This is used for non-Kubernetes, non-connection transient failures such as HTTP 429 and 5xx responses from
// OpenBao (excluding endpoints like /sys/health where status codes represent state rather than failure).
var ErrTransientRemoteOverloaded = errors.New("transient remote overloaded")

// ErrTransientClusterState indicates a cluster is temporarily not in the state
// required for the requested operation. This covers externally observable
// convergence states such as leader election or readiness settling.
var ErrTransientClusterState = errors.New("transient cluster state")

// ErrTransientKubernetesAPI indicates a transient Kubernetes API error that should be retried.
// This includes rate limiting, temporary server errors, and network issues.
var ErrTransientKubernetesAPI = errors.New("transient Kubernetes API error")

// Permanent errors indicate configuration or state issues that require user intervention.
// These errors should NOT be requeued automatically; reconciliation should wait for user changes.

// ErrPermanentConfig indicates a permanent configuration error that requires user intervention.
// This includes invalid configuration values, missing required fields, or incompatible settings.
var ErrPermanentConfig = errors.New("permanent configuration error")

// ErrPermanentPrerequisitesMissing indicates that required prerequisites are missing
// and reconciliation should wait for them to be created. This is similar to transient
// but indicates a dependency that may require user action (e.g., external TLS provider).
var ErrPermanentPrerequisitesMissing = errors.New("permanent prerequisites missing")

// IsTransientConnection checks if an error is a transient connection error.
// This includes network timeouts, connection refused, DNS failures, and similar issues.
func IsTransientConnection(err error) bool {
	if err == nil {
		return false
	}

	// Check for our well-known error
	if errors.Is(err, ErrTransientConnection) {
		return true
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, net.ErrClosed) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// Check for common transient network errno values.
	transientErrnos := []error{
		syscall.ECONNREFUSED,
		syscall.ECONNRESET,
		syscall.ECONNABORTED,
		syscall.ETIMEDOUT,
		syscall.EHOSTUNREACH,
		syscall.ENETUNREACH,
		syscall.EPIPE,
	}
	for _, errno := range transientErrnos {
		if errors.Is(err, errno) {
			return true
		}
	}

	// Check for net.Error types that indicate transient issues
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
		// Note: netErr.Temporary() is deprecated since Go 1.18 and not recommended
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

// IsTransientKubernetesAPI checks if an error is a transient Kubernetes API error.
func IsTransientKubernetesAPI(err error) bool {
	if err == nil {
		return false
	}

	// Check for our well-known error
	if errors.Is(err, ErrTransientKubernetesAPI) {
		return true
	}

	return apierrors.IsTooManyRequests(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsInternalError(err) ||
		apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsUnexpectedServerError(err) ||
		errors.Is(err, context.DeadlineExceeded)
}

// IsTransientRemoteOverloaded checks if an error indicates a remote service overload condition.
func IsTransientRemoteOverloaded(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, ErrTransientRemoteOverloaded)
}

// IsTransientClusterState checks if an error indicates the target cluster is
// temporarily not ready for the requested operation.
func IsTransientClusterState(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, ErrTransientClusterState)
}

// WrapTransientConnection wraps an error as a transient connection error.
// If the error is already a transient connection error, it is returned as-is.
func WrapTransientConnection(err error) error {
	if err == nil {
		return nil
	}

	if IsTransientConnection(err) {
		return err
	}

	return fmt.Errorf("%w: %w", ErrTransientConnection, err)
}

// WrapTransientRemoteOverloaded wraps an error as a transient remote overloaded error.
// If the error is already a transient remote overloaded error, it is returned as-is.
func WrapTransientRemoteOverloaded(err error) error {
	if err == nil {
		return nil
	}

	if IsTransientRemoteOverloaded(err) {
		return err
	}

	return fmt.Errorf("%w: %w", ErrTransientRemoteOverloaded, err)
}

// WrapTransientClusterState wraps an error as a transient cluster state error.
// If the error is already a transient cluster state error, it is returned as-is.
func WrapTransientClusterState(err error) error {
	if err == nil {
		return nil
	}

	if IsTransientClusterState(err) {
		return err
	}

	return fmt.Errorf("%w: %w", ErrTransientClusterState, err)
}

// WrapTransientKubernetesAPI wraps an error as a transient Kubernetes API error.
func WrapTransientKubernetesAPI(err error) error {
	if err == nil {
		return nil
	}

	if IsTransientKubernetesAPI(err) {
		return err
	}

	return fmt.Errorf("%w: %w", ErrTransientKubernetesAPI, err)
}

// WrapPermanentConfig wraps an error as a permanent configuration error.
func WrapPermanentConfig(err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%w: %w", ErrPermanentConfig, err)
}

// WrapPermanentPrerequisitesMissing wraps an error as a permanent prerequisites missing error.
func WrapPermanentPrerequisitesMissing(err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%w: %w", ErrPermanentPrerequisitesMissing, err)
}

// IsTransient checks if an error is transient (should be retried).
func IsTransient(err error) bool {
	return IsTransientConnection(err) ||
		IsTransientRemoteOverloaded(err) ||
		IsTransientClusterState(err) ||
		IsTransientKubernetesAPI(err)
}

// IsPermanent checks if an error is permanent (requires user intervention).
// Returns true for permanent configuration or prerequisites missing errors.
func IsPermanent(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, ErrPermanentConfig) || errors.Is(err, ErrPermanentPrerequisitesMissing)
}

// ShouldRequeue determines if an error should trigger a requeue.
// Transient errors should requeue; permanent errors should not.
// Returns (shouldRequeue, requeueAfter).
func ShouldRequeue(err error) (bool, time.Duration) {
	if err == nil {
		return false, 0
	}

	if IsTransient(err) {
		// For transient connection errors, requeue with a short delay
		if IsTransientConnection(err) {
			return true, 5 * time.Second
		}
		// For remote overloaded errors, requeue with a longer delay to reduce pressure
		if IsTransientRemoteOverloaded(err) {
			return true, 15 * time.Second
		}
		// For transient cluster convergence states, requeue with a short delay
		if IsTransientClusterState(err) {
			return true, 5 * time.Second
		}
		// For transient Kubernetes API errors, requeue with a short delay
		if IsTransientKubernetesAPI(err) {
			return true, 5 * time.Second
		}
	}

	// Permanent errors should not requeue automatically
	if IsPermanent(err) {
		return false, 0
	}

	// For unknown errors, default to requeue (controller-runtime will handle backoff)
	return true, 0
}

// IsCRDMissingError checks if an error indicates that a CRD is not installed.
// This is a permanent configuration error that requires user intervention.
func IsCRDMissingError(err error) bool {
	if err == nil {
		return false
	}

	if meta.IsNoMatchError(err) || isRuntimeNotRegisteredError(err) {
		return true
	}

	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "could not find the requested resource")
}

// WrapCRDMissing wraps an error as a permanent config error for missing CRDs.
func WrapCRDMissing(err error) error {
	if err == nil {
		return nil
	}

	if IsCRDMissingError(err) {
		return WrapPermanentConfig(fmt.Errorf("CRD not installed: %w", err))
	}

	return err
}

func isRuntimeNotRegisteredError(err error) bool {
	for current := err; current != nil; current = errors.Unwrap(current) {
		if runtime.IsNotRegisteredError(current) {
			return true
		}
	}
	return false
}
