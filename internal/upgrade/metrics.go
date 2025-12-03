package upgrade

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// UpgradeStatus represents the status of an upgrade for the status gauge.
type UpgradeStatus int

const (
	// UpgradeStatusNone indicates no upgrade is in progress.
	UpgradeStatusNone UpgradeStatus = 0
	// UpgradeStatusRunning indicates an upgrade is in progress.
	UpgradeStatusRunning UpgradeStatus = 1
	// UpgradeStatusSuccess indicates the last upgrade succeeded.
	UpgradeStatusSuccess UpgradeStatus = 2
	// UpgradeStatusFailed indicates the last upgrade failed.
	UpgradeStatusFailed UpgradeStatus = 3
)

var (
	// upgradeStatusGauge tracks the current upgrade status per cluster.
	// Values: 0=none, 1=running, 2=success, 3=failed
	upgradeStatusGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Subsystem: "upgrade",
			Name:      "status",
			Help:      "Current upgrade status per cluster (0=none, 1=running, 2=success, 3=failed)",
		},
		[]string{"namespace", "name"},
	)

	// upgradeDurationHistogram tracks the total duration of upgrades.
	upgradeDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "openbao",
			Subsystem: "upgrade",
			Name:      "duration_seconds",
			Help:      "Total duration of upgrade operations in seconds",
			Buckets:   []float64{60, 120, 300, 600, 900, 1200, 1800, 3600},
		},
		[]string{"namespace", "name", "from_version", "to_version"},
	)

	// upgradePodDurationHistogram tracks the duration per pod during upgrades.
	upgradePodDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "openbao",
			Subsystem: "upgrade",
			Name:      "pod_duration_seconds",
			Help:      "Duration to upgrade each pod in seconds",
			Buckets:   []float64{10, 30, 60, 120, 180, 300, 600},
		},
		[]string{"namespace", "name", "pod"},
	)

	// upgradeStepDownCounter tracks the total number of leader step-downs.
	upgradeStepDownCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Subsystem: "upgrade",
			Name:      "stepdown_total",
			Help:      "Total number of leader step-down operations during upgrades",
		},
		[]string{"namespace", "name"},
	)

	// upgradeStepDownFailuresCounter tracks failed step-down operations.
	upgradeStepDownFailuresCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Subsystem: "upgrade",
			Name:      "stepdown_failures_total",
			Help:      "Total number of failed leader step-down operations",
		},
		[]string{"namespace", "name"},
	)

	// upgradeInProgressGauge indicates if an upgrade is currently in progress.
	upgradeInProgressGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Subsystem: "upgrade",
			Name:      "in_progress",
			Help:      "Whether an upgrade is currently in progress (1) or not (0)",
		},
		[]string{"namespace", "name"},
	)

	// upgradePodsCompletedGauge tracks how many pods have been upgraded.
	upgradePodsCompletedGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Subsystem: "upgrade",
			Name:      "pods_completed",
			Help:      "Number of pods that have been upgraded in the current upgrade",
		},
		[]string{"namespace", "name"},
	)

	// upgradeTotalPodsGauge tracks the total number of pods to be upgraded.
	upgradeTotalPodsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Subsystem: "upgrade",
			Name:      "pods_total",
			Help:      "Total number of pods to be upgraded",
		},
		[]string{"namespace", "name"},
	)
)

func init() {
	// Register all metrics with the controller-runtime metrics registry
	metrics.Registry.MustRegister(
		upgradeStatusGauge,
		upgradeDurationHistogram,
		upgradePodDurationHistogram,
		upgradeStepDownCounter,
		upgradeStepDownFailuresCounter,
		upgradeInProgressGauge,
		upgradePodsCompletedGauge,
		upgradeTotalPodsGauge,
	)
}

// Metrics provides methods to record upgrade-related metrics.
type Metrics struct {
	namespace string
	name      string
}

// NewMetrics creates a new Metrics instance for the given cluster.
func NewMetrics(namespace, name string) *Metrics {
	return &Metrics{
		namespace: namespace,
		name:      name,
	}
}

// SetStatus sets the current upgrade status.
func (m *Metrics) SetStatus(status UpgradeStatus) {
	upgradeStatusGauge.WithLabelValues(m.namespace, m.name).Set(float64(status))
}

// RecordDuration records the total duration of an upgrade.
func (m *Metrics) RecordDuration(durationSeconds float64, fromVersion, toVersion string) {
	upgradeDurationHistogram.WithLabelValues(m.namespace, m.name, fromVersion, toVersion).Observe(durationSeconds)
}

// RecordPodDuration records the duration to upgrade a specific pod.
func (m *Metrics) RecordPodDuration(durationSeconds float64, podName string) {
	upgradePodDurationHistogram.WithLabelValues(m.namespace, m.name, podName).Observe(durationSeconds)
}

// IncrementStepDownTotal increments the step-down counter.
func (m *Metrics) IncrementStepDownTotal() {
	upgradeStepDownCounter.WithLabelValues(m.namespace, m.name).Inc()
}

// IncrementStepDownFailures increments the step-down failures counter.
func (m *Metrics) IncrementStepDownFailures() {
	upgradeStepDownFailuresCounter.WithLabelValues(m.namespace, m.name).Inc()
}

// SetInProgress sets whether an upgrade is in progress.
func (m *Metrics) SetInProgress(inProgress bool) {
	value := 0.0
	if inProgress {
		value = 1.0
	}
	upgradeInProgressGauge.WithLabelValues(m.namespace, m.name).Set(value)
}

// SetPodsCompleted sets the number of pods that have been upgraded.
func (m *Metrics) SetPodsCompleted(count int) {
	upgradePodsCompletedGauge.WithLabelValues(m.namespace, m.name).Set(float64(count))
}

// SetTotalPods sets the total number of pods to be upgraded.
func (m *Metrics) SetTotalPods(count int) {
	upgradeTotalPodsGauge.WithLabelValues(m.namespace, m.name).Set(float64(count))
}

// Clear resets all metrics for this cluster (used on deletion).
func (m *Metrics) Clear() {
	upgradeStatusGauge.DeleteLabelValues(m.namespace, m.name)
	upgradeInProgressGauge.DeleteLabelValues(m.namespace, m.name)
	upgradePodsCompletedGauge.DeleteLabelValues(m.namespace, m.name)
	upgradeTotalPodsGauge.DeleteLabelValues(m.namespace, m.name)
	upgradeStepDownCounter.DeleteLabelValues(m.namespace, m.name)
	upgradeStepDownFailuresCounter.DeleteLabelValues(m.namespace, m.name)
}
