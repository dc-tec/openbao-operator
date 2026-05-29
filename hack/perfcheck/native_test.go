package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBoundedDNSLabelSanitizesRunIDs(t *testing.T) {
	t.Parallel()

	got := boundedDNSLabel("Run_2026-05-29T10:20:30Z/Alpha.Beta ")
	want := "run-2026-05-29t10-20-30z-alpha-beta"
	if got != want {
		t.Fatalf("boundedDNSLabel() = %q, want %q", got, want)
	}
	if got := boundedDNSLabel(strings.Repeat("a", 80)); len(got) > 63 {
		t.Fatalf("boundedDNSLabel() length = %d, want <= 63", len(got))
	}
	if got := boundedDNSLabel("$$$"); got != "perf" {
		t.Fatalf("boundedDNSLabel() for invalid value = %q, want perf", got)
	}
}

func TestNativeResourceNameLeavesRoomForPodSuffixes(t *testing.T) {
	t.Parallel()

	got := nativeResourceName("perf-life", strings.Repeat("long-run-id-", 8))
	if len(got) > 40 {
		t.Fatalf("nativeResourceName() length = %d, want <= 40", len(got))
	}
	if strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
		t.Fatalf("nativeResourceName() = %q, should not have edge hyphens", got)
	}
}

func TestTenantChurnNamespaceNameIsStableAndBounded(t *testing.T) {
	t.Parallel()

	native := &nativeScenarioContext{runID: strings.Repeat("tenant-run-", 8)}
	got := native.tenantChurnNamespaceName(8)
	if len(got) > 40 {
		t.Fatalf("tenantChurnNamespaceName() length = %d, want <= 40", len(got))
	}
	if !strings.HasPrefix(got, "perf-tenant-09-") {
		t.Fatalf("tenantChurnNamespaceName() = %q, want indexed prefix", got)
	}
}

func TestNativeSelfInitRequestsUseJSONPayload(t *testing.T) {
	t.Parallel()

	requests := nativeSelfInitRequests("perf-example")
	var rolePayload map[string]any
	for _, request := range requests {
		if request.Name != "create-perf-role" {
			continue
		}
		if request.Data == nil {
			t.Fatalf("create-perf-role Data is nil")
		}
		if err := json.Unmarshal(request.Data.Raw, &rolePayload); err != nil {
			t.Fatalf("unmarshal create-perf-role Data: %v", err)
		}
	}
	if rolePayload == nil {
		t.Fatalf("create-perf-role request missing")
	}
	if got, want := rolePayload["bound_subject"], "system:serviceaccount:perf-example:default"; got != want {
		t.Fatalf("bound_subject = %v, want %q", got, want)
	}
}

func TestPhaseMeasurements(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 5, 29, 8, 0, 0, 0, time.UTC)
	got := phaseMeasurements(map[string]time.Time{
		"start": start,
		"ready": start.Add(42 * time.Second),
	}, "start", map[string]string{
		"ready_seconds":   "ready",
		"missing_seconds": "missing",
	})

	assertApproxEqual(t, got["ready_seconds"], 42)
	if _, exists := got["missing_seconds"]; exists {
		t.Fatalf("phaseMeasurements() included missing phase")
	}
}

func TestPhaseMeasurementsClampNegativeDurations(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 5, 29, 8, 0, 0, 0, time.UTC)
	got := phaseMeasurements(map[string]time.Time{
		"start":   start,
		"started": start.Add(-500 * time.Millisecond),
	}, "start", map[string]string{
		"started_seconds": "started",
	})

	assertApproxEqual(t, got["started_seconds"], 0)
}

func TestDurationPercentileSeconds(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 5, 29, 8, 0, 0, 0, time.UTC)
	values := []time.Time{
		start.Add(1 * time.Second),
		start.Add(5 * time.Second),
		start.Add(9 * time.Second),
		start.Add(13 * time.Second),
	}

	assertApproxEqual(t, durationPercentileSeconds(start, values, 0.50), 5)
	assertApproxEqual(t, durationPercentileSeconds(start, values, 0.95), 13)
}

func TestResourceWriteTrackerCountsResourceVersionChanges(t *testing.T) {
	t.Parallel()

	tracker := newResourceWriteTracker()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:            "openbao-0",
		Namespace:       "perf",
		ResourceVersion: "1",
	}}

	tracker.track("Pod", pod)
	tracker.track("Pod", pod)
	if tracker.count != 1 {
		t.Fatalf("tracker count = %d, want 1 for duplicate resource version", tracker.count)
	}

	pod.ResourceVersion = "2"
	tracker.track("Pod", pod)
	if tracker.count != 2 {
		t.Fatalf("tracker count = %d, want 2 after resource version change", tracker.count)
	}
}
