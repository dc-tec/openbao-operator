package backup

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// backupState tracks the current/last-known backup state per cluster.
	// Values: 0=none/unknown, 1=success, 2=failure, 3=in_progress
	backupState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Subsystem: "backup",
			Name:      "state",
			Help:      "Current backup state per cluster (0=none, 1=success, 2=failure, 3=in_progress)",
		},
		[]string{"namespace", "name"},
	)

	// backupLastAttemptTimestamp tracks the Unix timestamp of the last backup attempt completion (success or failure).
	backupLastAttemptTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Subsystem: "backup",
			Name:      "last_attempt_timestamp",
			Help:      "Unix timestamp of the last backup attempt completion (success or failure)",
		},
		[]string{"namespace", "name"},
	)

	// backupLastSuccessTimestamp tracks the Unix timestamp of the last successful backup.
	backupLastSuccessTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Subsystem: "backup",
			Name:      "last_success_timestamp",
			Help:      "Unix timestamp of the last successful backup",
		},
		[]string{"namespace", "name"},
	)

	// backupLastDurationSeconds tracks the duration of the last backup in seconds.
	backupLastDurationSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Subsystem: "backup",
			Name:      "last_duration_seconds",
			Help:      "Duration of the last backup in seconds",
		},
		[]string{"namespace", "name"},
	)

	// backupLastSizeBytes tracks the size of the last backup in bytes.
	backupLastSizeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Subsystem: "backup",
			Name:      "last_size_bytes",
			Help:      "Size of the last backup in bytes",
		},
		[]string{"namespace", "name"},
	)

	// backupSuccessTotal tracks the total number of successful backups.
	backupSuccessTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Subsystem: "backup",
			Name:      "success_total",
			Help:      "Total number of successful backups",
		},
		[]string{"namespace", "name"},
	)

	// backupFailureTotal tracks the total number of backup failures.
	backupFailureTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Subsystem: "backup",
			Name:      "failure_total",
			Help:      "Total number of backup failures",
		},
		[]string{"namespace", "name"},
	)

	// backupConsecutiveFailures tracks the current consecutive failure count.
	backupConsecutiveFailures = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Subsystem: "backup",
			Name:      "consecutive_failures",
			Help:      "Current number of consecutive backup failures",
		},
		[]string{"namespace", "name"},
	)

	// backupInProgress indicates if a backup is currently in progress.
	backupInProgress = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Subsystem: "backup",
			Name:      "in_progress",
			Help:      "Whether a backup is currently in progress (1) or not (0)",
		},
		[]string{"namespace", "name"},
	)

	// backupRetentionDeletedTotal tracks the total number of backups deleted by retention policy.
	backupRetentionDeletedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Subsystem: "backup",
			Name:      "retention_deleted_total",
			Help:      "Total number of backups deleted by retention policy",
		},
		[]string{"namespace", "name"},
	)
)

func init() {
	// Register all metrics with the controller-runtime metrics registry
	metrics.Registry.MustRegister(
		backupState,
		backupLastAttemptTimestamp,
		backupLastSuccessTimestamp,
		backupLastDurationSeconds,
		backupLastSizeBytes,
		backupSuccessTotal,
		backupFailureTotal,
		backupConsecutiveFailures,
		backupInProgress,
		backupRetentionDeletedTotal,
	)
}

// Metrics provides methods to record backup-related metrics.
type Metrics struct {
	namespace string
	name      string
}

// SetState sets the backup state.
func (m *Metrics) SetState(state float64) {
	backupState.WithLabelValues(m.namespace, m.name).Set(state)
}

// SetLastAttemptTimestamp sets the timestamp of the last backup attempt completion.
func (m *Metrics) SetLastAttemptTimestamp(unixTimestamp float64) {
	backupLastAttemptTimestamp.WithLabelValues(m.namespace, m.name).Set(unixTimestamp)
}

// NewMetrics creates a new Metrics instance for the given cluster.
func NewMetrics(namespace, name string) *Metrics {
	return &Metrics{
		namespace: namespace,
		name:      name,
	}
}

// SetLastSuccessTimestamp sets the timestamp of the last successful backup.
func (m *Metrics) SetLastSuccessTimestamp(unixTimestamp float64) {
	backupLastSuccessTimestamp.WithLabelValues(m.namespace, m.name).Set(unixTimestamp)
}

// SetLastDuration sets the duration of the last backup in seconds.
func (m *Metrics) SetLastDuration(durationSeconds float64) {
	backupLastDurationSeconds.WithLabelValues(m.namespace, m.name).Set(durationSeconds)
}

// SetLastSize sets the size of the last backup in bytes.
func (m *Metrics) SetLastSize(sizeBytes int64) {
	backupLastSizeBytes.WithLabelValues(m.namespace, m.name).Set(float64(sizeBytes))
}

// IncrementSuccessTotal increments the total successful backups counter.
func (m *Metrics) IncrementSuccessTotal() {
	backupSuccessTotal.WithLabelValues(m.namespace, m.name).Inc()
}

// IncrementFailureTotal increments the total failed backups counter.
func (m *Metrics) IncrementFailureTotal() {
	backupFailureTotal.WithLabelValues(m.namespace, m.name).Inc()
}

// SetConsecutiveFailures sets the current consecutive failure count.
func (m *Metrics) SetConsecutiveFailures(count int32) {
	backupConsecutiveFailures.WithLabelValues(m.namespace, m.name).Set(float64(count))
}

// SetInProgress sets whether a backup is currently in progress.
func (m *Metrics) SetInProgress(inProgress bool) {
	value := 0.0
	if inProgress {
		value = 1.0
	}
	backupInProgress.WithLabelValues(m.namespace, m.name).Set(value)
}

// IncrementRetentionDeleted increments the retention deleted counter by the given amount.
func (m *Metrics) IncrementRetentionDeleted(count int) {
	backupRetentionDeletedTotal.WithLabelValues(m.namespace, m.name).Add(float64(count))
}

// RecordSuccess records metrics for a successful backup.
func (m *Metrics) RecordSuccess(durationSeconds float64, sizeBytes int64, timestamp float64) {
	m.SetState(1)
	m.SetLastAttemptTimestamp(timestamp)
	m.SetLastSuccessTimestamp(timestamp)
	m.SetLastDuration(durationSeconds)
	m.SetLastSize(sizeBytes)
	m.IncrementSuccessTotal()
	m.SetConsecutiveFailures(0)
	m.SetInProgress(false)
}

// RecordFailure records metrics for a failed backup.
func (m *Metrics) RecordFailure(consecutiveFailures int32) {
	// Note: caller should set last attempt timestamp when available.
	m.SetState(2)
	m.IncrementFailureTotal()
	m.SetConsecutiveFailures(consecutiveFailures)
	m.SetInProgress(false)
}

// Clear removes all metrics for this cluster (used on deletion).
func (m *Metrics) Clear() {
	backupState.DeleteLabelValues(m.namespace, m.name)
	backupLastAttemptTimestamp.DeleteLabelValues(m.namespace, m.name)
	backupLastSuccessTimestamp.DeleteLabelValues(m.namespace, m.name)
	backupLastDurationSeconds.DeleteLabelValues(m.namespace, m.name)
	backupLastSizeBytes.DeleteLabelValues(m.namespace, m.name)
	backupSuccessTotal.DeleteLabelValues(m.namespace, m.name)
	backupFailureTotal.DeleteLabelValues(m.namespace, m.name)
	backupConsecutiveFailures.DeleteLabelValues(m.namespace, m.name)
	backupInProgress.DeleteLabelValues(m.namespace, m.name)
	backupRetentionDeletedTotal.DeleteLabelValues(m.namespace, m.name)
}
