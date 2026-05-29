package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGinkgoPhaseEventsExtractsBySteps(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "ginkgo.json")
	if err := os.WriteFile(path, []byte(`[
  {
    "SpecReports": [
      {
        "SpecEvents": [
          {
            "SpecEventType": "By",
            "TimelineLocation": {"Order": 2, "Time": "2026-05-29T08:00:02Z"},
            "Message": "waiting for restore completion"
          },
          {
            "SpecEventType": "Node",
            "TimelineLocation": {"Order": 1, "Time": "2026-05-29T08:00:01Z"},
            "Message": "ignored node"
          },
          {
            "SpecEventType": "By",
            "TimelineLocation": {"Order": 3, "Time": "2026-05-29T08:00:03Z"},
            "Message": "waiting for restore completion"
          }
        ]
      }
    ]
  }
]`), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	phases, err := parseGinkgoPhaseEvents(path)
	if err != nil {
		t.Fatalf("parseGinkgoPhaseEvents() error = %v", err)
	}
	if len(phases) != 2 {
		t.Fatalf("phase count = %d, want 2", len(phases))
	}
	if got := phases[0].Name; got != "ginkgo_by_waiting_for_restore_completion" {
		t.Fatalf("first phase = %q", got)
	}
	if got := phases[1].Name; got != "ginkgo_by_waiting_for_restore_completion_2" {
		t.Fatalf("second phase = %q", got)
	}
	if phases[0].Source != "ginkgo_by" {
		t.Fatalf("phase source = %q, want ginkgo_by", phases[0].Source)
	}
}

func TestPhaseNameFromTextBoundsLength(t *testing.T) {
	t.Parallel()

	got := phaseNameFromText("ginkgo_by", "Restore phase: "+strings.Repeat("very slow ", 40))
	if len(got) > 96 {
		t.Fatalf("phaseNameFromText() length = %d, want <= 96", len(got))
	}
}
