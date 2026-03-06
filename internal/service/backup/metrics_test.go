package backup

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestBackupMetrics_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		run    func(m *Metrics)
		assert func(t *testing.T, namespace, name string)
	}{
		{
			name: "set in progress true and false",
			run: func(m *Metrics) {
				m.SetInProgress(true)
				m.SetInProgress(false)
			},
			assert: func(t *testing.T, namespace, name string) {
				t.Helper()
				if got := testutil.ToFloat64(backupInProgress.WithLabelValues(namespace, name)); got != 0 {
					t.Fatalf("backupInProgress = %v, want 0", got)
				}
			},
		},
		{
			name: "record success updates gauges and counters",
			run: func(m *Metrics) {
				m.RecordSuccess(12.5, 2048, 1700000000)
				m.IncrementRetentionDeleted(3)
			},
			assert: func(t *testing.T, namespace, name string) {
				t.Helper()

				if got := testutil.ToFloat64(backupState.WithLabelValues(namespace, name)); got != 1 {
					t.Fatalf("backupState = %v, want 1", got)
				}
				if got := testutil.ToFloat64(backupLastAttemptTimestamp.WithLabelValues(namespace, name)); got != 1700000000 {
					t.Fatalf("backupLastAttemptTimestamp = %v, want 1700000000", got)
				}
				if got := testutil.ToFloat64(backupLastSuccessTimestamp.WithLabelValues(namespace, name)); got != 1700000000 {
					t.Fatalf("backupLastSuccessTimestamp = %v, want 1700000000", got)
				}
				if got := testutil.ToFloat64(backupLastDurationSeconds.WithLabelValues(namespace, name)); got != 12.5 {
					t.Fatalf("backupLastDurationSeconds = %v, want 12.5", got)
				}
				if got := testutil.ToFloat64(backupLastSizeBytes.WithLabelValues(namespace, name)); got != 2048 {
					t.Fatalf("backupLastSizeBytes = %v, want 2048", got)
				}
				if got := testutil.ToFloat64(backupSuccessTotal.WithLabelValues(namespace, name)); got != 1 {
					t.Fatalf("backupSuccessTotal = %v, want 1", got)
				}
				if got := testutil.ToFloat64(backupConsecutiveFailures.WithLabelValues(namespace, name)); got != 0 {
					t.Fatalf("backupConsecutiveFailures = %v, want 0", got)
				}
				if got := testutil.ToFloat64(backupInProgress.WithLabelValues(namespace, name)); got != 0 {
					t.Fatalf("backupInProgress = %v, want 0", got)
				}
				if got := testutil.ToFloat64(backupRetentionDeletedTotal.WithLabelValues(namespace, name)); got != 3 {
					t.Fatalf("backupRetentionDeletedTotal = %v, want 3", got)
				}
			},
		},
		{
			name: "record failure updates failure metrics",
			run: func(m *Metrics) {
				m.RecordFailure(4)
			},
			assert: func(t *testing.T, namespace, name string) {
				t.Helper()

				if got := testutil.ToFloat64(backupState.WithLabelValues(namespace, name)); got != 2 {
					t.Fatalf("backupState = %v, want 2", got)
				}
				if got := testutil.ToFloat64(backupFailureTotal.WithLabelValues(namespace, name)); got != 1 {
					t.Fatalf("backupFailureTotal = %v, want 1", got)
				}
				if got := testutil.ToFloat64(backupConsecutiveFailures.WithLabelValues(namespace, name)); got != 4 {
					t.Fatalf("backupConsecutiveFailures = %v, want 4", got)
				}
				if got := testutil.ToFloat64(backupInProgress.WithLabelValues(namespace, name)); got != 0 {
					t.Fatalf("backupInProgress = %v, want 0", got)
				}
			},
		},
		{
			name: "clear removes label values",
			run: func(m *Metrics) {
				m.SetState(3)
				m.IncrementFailureTotal()
				m.SetConsecutiveFailures(2)
				m.Clear()
			},
			assert: func(t *testing.T, namespace, name string) {
				t.Helper()

				if got := testutil.ToFloat64(backupState.WithLabelValues(namespace, name)); got != 0 {
					t.Fatalf("backupState = %v, want 0", got)
				}
				if got := testutil.ToFloat64(backupFailureTotal.WithLabelValues(namespace, name)); got != 0 {
					t.Fatalf("backupFailureTotal = %v, want 0", got)
				}
				if got := testutil.ToFloat64(backupConsecutiveFailures.WithLabelValues(namespace, name)); got != 0 {
					t.Fatalf("backupConsecutiveFailures = %v, want 0", got)
				}
			},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			namespace := "metrics-ns"
			name := fmt.Sprintf("cluster-%d", i)
			m := NewMetrics(namespace, name)
			// Reset any previous values for this series.
			m.Clear()

			tt.run(m)
			tt.assert(t, namespace, name)

			// Ensure no test leaks labels into the next one.
			m.Clear()
		})
	}
}
