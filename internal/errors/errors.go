package errors

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Transient errors indicate temporary conditions that should be retried.
// These errors typically result in requeue with a delay.

// ErrTransientConnection indicates a transient connection error that should be retried.
// This includes timeouts, connection refused, DNS resolution failures, and network unreachable errors.
var ErrTransientConnection = errors.New("transient connection error")

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

	// Check for our sentinel error
	if errors.Is(err, ErrTransientConnection) {
		return true
	}

	errStr := strings.ToLower(err.Error())

	// Check for common transient connection error patterns
	transientPatterns := []string{
		"connection refused",
		"connection reset",
		"connection timeout",
		"context deadline exceeded",
		"timeout",
		"i/o timeout",
		"no such host",
		"network is unreachable",
		"temporary failure",
		"dial tcp",
		"connection closed",
		"broken pipe",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	// Check for net.Error types that indicate transient issues
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
		if netErr.Temporary() {
			return true
		}
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

	// Check for our sentinel error
	if errors.Is(err, ErrTransientKubernetesAPI) {
		return true
	}

	errStr := strings.ToLower(err.Error())

	// Check for Kubernetes API transient error patterns
	transientPatterns := []string{
		"rate limit",
		"too many requests",
		"server error",
		"service unavailable",
		"internal server error",
		"context deadline exceeded",
		"timeout",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
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
// Returns true for transient connection or Kubernetes API errors.
func IsTransient(err error) bool {
	return IsTransientConnection(err) || IsTransientKubernetesAPI(err)
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

	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "no matches for kind") ||
		strings.Contains(errStr, "no kind is registered for the type") ||
		strings.Contains(errStr, "could not find the requested resource")
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
