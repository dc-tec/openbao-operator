package admission

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WaitForDependencies checks admission dependencies until they are ready, the timeout elapses, or ctx is cancelled.
// It returns the last observed Status (even on timeout/cancellation) to support diagnostics.
func WaitForDependencies(ctx context.Context, c client.Reader, deps []Dependency, namePrefixes []string, timeout time.Duration, interval time.Duration) (Status, error) {
	if ctx == nil {
		return Status{}, fmt.Errorf("context is required")
	}
	if timeout <= 0 {
		return CheckDependencies(ctx, c, deps, namePrefixes)
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}

	deadline := time.Now().Add(timeout)
	var last Status

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return last, context.DeadlineExceeded
		}

		checkTimeout := 10 * time.Second
		if remaining < checkTimeout {
			checkTimeout = remaining
		}

		checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		status, err := CheckDependencies(checkCtx, c, deps, namePrefixes)
		cancel()
		if err != nil {
			return status, err
		}
		last = status
		if status.OverallReady {
			return status, nil
		}

		sleepFor := interval
		if sleepFor > remaining {
			sleepFor = remaining
		}

		timer := time.NewTimer(sleepFor)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last, ctx.Err()
		case <-timer.C:
		}
	}
}
