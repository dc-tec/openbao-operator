package admission

import (
	"context"
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

	mu     sync.RWMutex
	status *Status
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

	return t.Refresh(ctx)
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

	status, err := CheckDependencies(ctx, t.reader, t.dependencies, t.namePrefixes)
	if err != nil {
		status = Status{
			CheckedAt:    time.Now(),
			OverallReady: false,
		}
	}

	t.Set(status)
	return cloneStatus(&status), err
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
