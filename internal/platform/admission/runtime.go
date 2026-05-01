package admission

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RefreshStatus returns the current admission dependency status using the shared
// tracker when available, or a direct dependency check otherwise.
func RefreshStatus(ctx context.Context, tracker *Tracker, reader client.Reader) (*Status, error) {
	if tracker != nil {
		return tracker.EnsureFresh(ctx)
	}
	if UnsafeAdmissionDisabled() {
		status := unsafeModeStatus()
		SetAdmissionDependenciesReady(status.OverallReady)
		return cloneStatus(&status), nil
	}

	// Preserve existing unit-test behavior when callers seed the legacy global
	// readiness signal without constructing a tracker.
	if AdmissionDependenciesReady() {
		status := Status{
			CheckedAt:    time.Now(),
			OverallReady: true,
		}
		return cloneStatus(&status), nil
	}
	if reader == nil {
		return nil, fmt.Errorf("kubernetes client reader is required")
	}

	status, err := CheckDependencies(ctx, reader, DefaultDependencies(), DefaultNamePrefixes())
	if err != nil {
		status = Status{
			CheckedAt:    time.Now(),
			OverallReady: false,
		}
	}
	SetAdmissionDependenciesReady(status.OverallReady)
	return cloneStatus(&status), err
}
