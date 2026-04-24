package admission

import (
	"context"
	"strings"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const defaultRefreshInterval = 30 * time.Second

// Tracker caches the current admission dependency status and refreshes it on demand.
type Tracker struct {
	reader          client.Reader
	dependencies    []Dependency
	namePrefixes    []string
	refreshInterval time.Duration

	mu        sync.RWMutex
	status    *Status
	refreshMu sync.Mutex
}

// NewTracker creates an admission dependency tracker.
func NewTracker(reader client.Reader, dependencies []Dependency, namePrefixes []string, refreshInterval time.Duration) *Tracker {
	if refreshInterval <= 0 {
		refreshInterval = defaultRefreshInterval
	}
	return &Tracker{
		reader:          reader,
		dependencies:    append([]Dependency(nil), dependencies...),
		namePrefixes:    append([]string(nil), namePrefixes...),
		refreshInterval: refreshInterval,
	}
}

// Current returns the last cached admission dependency status, if any.
func (t *Tracker) Current() *Status {
	if t == nil {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()
	return cloneStatus(t.status)
}

// Set stores the supplied admission dependency status.
func (t *Tracker) Set(status Status) {
	if t == nil {
		return
	}

	t.mu.Lock()
	t.status = cloneStatus(&status)
	t.mu.Unlock()
	SetAdmissionDependenciesReady(status.OverallReady)
}

// MarkReadyForUnsafeMode seeds the tracker for unsafe mode installs where dependency checks are disabled.
func (t *Tracker) MarkReadyForUnsafeMode() {
	if t == nil {
		return
	}
	t.Set(unsafeModeStatus())
}

// EnsureFresh refreshes the cached status when it is stale or absent.
func (t *Tracker) EnsureFresh(ctx context.Context) (*Status, error) {
	if t == nil {
		return nil, nil
	}
	if UnsafeAdmissionDisabled() {
		status := unsafeModeStatus()
		t.Set(status)
		return cloneStatus(&status), nil
	}

	t.mu.RLock()
	current := cloneStatus(t.status)
	refreshInterval := t.refreshInterval
	t.mu.RUnlock()

	if current != nil && time.Since(current.CheckedAt) < refreshInterval {
		return current, nil
	}

	t.refreshMu.Lock()
	defer t.refreshMu.Unlock()

	t.mu.RLock()
	current = cloneStatus(t.status)
	refreshInterval = t.refreshInterval
	t.mu.RUnlock()

	if current != nil && time.Since(current.CheckedAt) < refreshInterval {
		return current, nil
	}

	return t.refreshLocked(ctx, current)
}

// Refresh re-checks admission dependencies immediately, bypassing the cached age window.
func (t *Tracker) Refresh(ctx context.Context) (*Status, error) {
	if t == nil {
		return nil, nil
	}
	if UnsafeAdmissionDisabled() {
		status := unsafeModeStatus()
		t.Set(status)
		return cloneStatus(&status), nil
	}

	t.refreshMu.Lock()
	defer t.refreshMu.Unlock()

	t.mu.RLock()
	current := cloneStatus(t.status)
	t.mu.RUnlock()

	return t.refreshLocked(ctx, current)
}

func (t *Tracker) refreshLocked(ctx context.Context, current *Status) (*Status, error) {
	status, err := CheckDependencies(ctx, t.reader, t.dependencies, t.namePrefixes)
	if err != nil {
		if current != nil {
			// Preserve the last known-good dependency state across transient API
			// read failures so concurrent reconcilers do not all fail closed on
			// the same timeout spike.
			current.CheckedAt = time.Now()
			t.Set(*current)
			return cloneStatus(current), nil
		}
		status = Status{
			CheckedAt:    time.Now(),
			OverallReady: false,
		}
	}
	if current != nil && !status.OverallReady && hasOnlyDependencyReadFailures(status) {
		// Preserve the last known dependency state when the refresh reached the
		// API server but only surfaced transient read failures. This keeps
		// runtime reconcilers from failing closed on API hiccups while still
		// allowing a successful later refresh to revoke readiness.
		current.CheckedAt = time.Now()
		t.Set(*current)
		return cloneStatus(current), nil
	}

	t.Set(status)
	return cloneStatus(&status), err
}

func hasOnlyDependencyReadFailures(status Status) bool {
	sawReadFailure := false
	for _, dep := range status.Dependencies {
		if dep.Ready {
			continue
		}
		if len(dep.Issues) == 0 {
			return false
		}
		for _, issue := range dep.Issues {
			if !strings.HasPrefix(issue, "failed to read ") {
				return false
			}
			sawReadFailure = true
		}
	}
	return sawReadFailure
}

func cloneStatus(status *Status) *Status {
	if status == nil {
		return nil
	}

	cloned := *status
	if len(status.Dependencies) == 0 {
		return &cloned
	}

	cloned.Dependencies = make([]DependencyStatus, len(status.Dependencies))
	for i := range status.Dependencies {
		cloned.Dependencies[i] = status.Dependencies[i]
		cloned.Dependencies[i].Issues = append([]string(nil), status.Dependencies[i].Issues...)
	}

	return &cloned
}

func unsafeModeStatus() Status {
	return Status{
		CheckedAt:    time.Now(),
		OverallReady: true,
		UnsafeMode:   true,
	}
}
