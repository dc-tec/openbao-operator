package admission

import (
	"context"
	"testing"
	"time"
)

func TestTracker_SetAndCurrent(t *testing.T) {
	t.Parallel()

	tracker := NewTracker(nil, nil, nil, time.Minute)
	input := Status{
		CheckedAt:    time.Now(),
		OverallReady: false,
		Dependencies: []DependencyStatus{
			{
				Dependency: Dependency{Name: "policy-a"},
				Ready:      false,
				Issues:     []string{"missing binding"},
			},
		},
	}

	tracker.Set(input)
	current := tracker.Current()
	if current == nil {
		t.Fatal("Current() returned nil")
	}
	if current.OverallReady {
		t.Fatal("Current() unexpectedly marked dependencies ready")
	}
	if len(current.Dependencies) != 1 || len(current.Dependencies[0].Issues) != 1 {
		t.Fatalf("Current() lost dependency details: %#v", current)
	}

	current.Dependencies[0].Issues[0] = "mutated"
	latest := tracker.Current()
	if latest.Dependencies[0].Issues[0] != "missing binding" {
		t.Fatalf("tracker returned a mutable shared status: %#v", latest)
	}
}

func TestTracker_EnsureFreshCachesRecentStatus(t *testing.T) {
	t.Parallel()

	tracker := NewTracker(nil, nil, nil, time.Hour)
	tracker.Set(Status{
		CheckedAt:    time.Now(),
		OverallReady: true,
	})

	current, err := tracker.EnsureFresh(context.Background())
	if err != nil {
		t.Fatalf("EnsureFresh() returned unexpected error: %v", err)
	}
	if current == nil || !current.OverallReady {
		t.Fatalf("EnsureFresh() returned unexpected status: %#v", current)
	}
}
