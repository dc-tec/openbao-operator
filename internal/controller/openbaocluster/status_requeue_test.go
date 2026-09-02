package openbaocluster

import (
	"testing"
	"time"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestSafetyNetRequeueAfter(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 123456789)
	gotA := safetyNetRequeueAfter(now)
	gotB := safetyNetRequeueAfter(now)
	if gotA != gotB {
		t.Fatalf("expected deterministic output for the same timestamp: %v vs %v", gotA, gotB)
	}

	tests := []time.Time{
		time.Unix(1700000000, 0),
		time.Unix(1700000000, 1),
		time.Unix(1700000000, int64(constants.RequeueSafetyNetJitter/2)),
		time.Unix(1700000000, int64(constants.RequeueSafetyNetJitter-1)),
	}
	for _, ts := range tests {
		got := safetyNetRequeueAfter(ts)
		minRequeue := constants.RequeueSafetyNetBase
		maxRequeue := constants.RequeueSafetyNetBase + constants.RequeueSafetyNetJitter
		if got < minRequeue || got >= maxRequeue {
			t.Fatalf("safetyNetRequeueAfter(%v)=%v, expected in [%v, %v)", ts, got, minRequeue, maxRequeue)
		}
	}
}

func TestSteadyStateStatusRefreshRequeueAfter(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 123456789)
	got := steadyStateStatusRefreshRequeueAfter(now)
	if got != constants.RequeueStandard {
		t.Fatalf("steadyStateStatusRefreshRequeueAfter(%v)=%v, want %v", now, got, constants.RequeueStandard)
	}
}
