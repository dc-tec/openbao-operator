package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

var (
	reconcileDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "openbao",
			Name:      "reconcile_duration_seconds",
			Help:      "Duration of reconciliation loops in seconds",
			// Buckets chosen to capture fast reconciles and longer tail up to 60s.
			Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 60},
		},
		[]string{"namespace", "name", "controller"},
	)

	reconcileErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Name:      "reconcile_errors_total",
			Help:      "Total number of reconciliation errors",
		},
		[]string{"namespace", "name", "controller", "reason"},
	)

	clusterReadyReplicasGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "cluster_ready_replicas",
			Help:      "Number of Ready replicas for an OpenBaoCluster",
		},
		[]string{"namespace", "name"},
	)

	clusterPhaseGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "cluster_phase",
			Help:      "Current phase of an OpenBaoCluster (1 = active phase)",
		},
		[]string{"namespace", "name", "phase"},
	)

	driftDetectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Name:      "drift_detected_total",
			Help:      "Total number of drift events detected by Sentinel",
		},
		[]string{"namespace", "name", "resource_kind"},
	)

	driftCorrectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Name:      "drift_corrected_total",
			Help:      "Total number of drift events corrected by the operator",
		},
		[]string{"namespace", "name"},
	)

	driftLastDetectedTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "drift_last_detected_timestamp",
			Help:      "Unix timestamp when drift was last detected",
		},
		[]string{"namespace", "name"},
	)

	// Restore metrics
	restoreTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Name:      "restore_total",
			Help:      "Total number of restore operations attempted",
		},
		[]string{"namespace", "name"},
	)

	restoreSuccessTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Name:      "restore_success_total",
			Help:      "Total number of successful restore operations",
		},
		[]string{"namespace", "name"},
	)

	restoreFailureTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Name:      "restore_failure_total",
			Help:      "Total number of failed restore operations",
		},
		[]string{"namespace", "name"},
	)

	restoreDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "openbao",
			Name:      "restore_duration_seconds",
			Help:      "Duration of restore operations in seconds",
			Buckets:   []float64{10, 30, 60, 120, 300, 600, 1200},
		},
		[]string{"namespace", "name"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		reconcileDurationHistogram,
		reconcileErrorsTotal,
		clusterReadyReplicasGauge,
		clusterPhaseGauge,
		driftDetectedTotal,
		driftCorrectedTotal,
		driftLastDetectedTimestamp,
		// Restore metrics
		restoreTotal,
		restoreSuccessTotal,
		restoreFailureTotal,
		restoreDurationHistogram,
	)
}

// ReconcileMetrics provides helpers to record reconcile-level metrics for a
// specific controller and OpenBaoCluster.
type ReconcileMetrics struct {
	namespace  string
	name       string
	controller string
}

// NewReconcileMetrics creates a new ReconcileMetrics instance.
func NewReconcileMetrics(namespace, name, controller string) *ReconcileMetrics {
	return &ReconcileMetrics{
		namespace:  namespace,
		name:       name,
		controller: controller,
	}
}

// ObserveDuration records the duration of a reconcile loop in seconds.
func (m *ReconcileMetrics) ObserveDuration(durationSeconds float64) {
	reconcileDurationHistogram.
		WithLabelValues(m.namespace, m.name, m.controller).
		Observe(durationSeconds)
}

// IncrementError increments the reconcile error counter with the given reason.
// Reason values should be low-cardinality strings (for example, "KubernetesAPIError").
func (m *ReconcileMetrics) IncrementError(reason string) {
	reconcileErrorsTotal.
		WithLabelValues(m.namespace, m.name, m.controller, reason).
		Inc()
}

// ClusterMetrics provides helpers to record per-cluster state metrics.
type ClusterMetrics struct {
	namespace string
	name      string
}

// NewClusterMetrics creates a new ClusterMetrics instance.
func NewClusterMetrics(namespace, name string) *ClusterMetrics {
	return &ClusterMetrics{
		namespace: namespace,
		name:      name,
	}
}

// SetReadyReplicas records the number of Ready replicas for the cluster.
func (m *ClusterMetrics) SetReadyReplicas(readyReplicas int32) {
	clusterReadyReplicasGauge.
		WithLabelValues(m.namespace, m.name).
		Set(float64(readyReplicas))
}

// SetPhase records the current phase for the cluster. The gauge is set to 1
// for the provided phase. Other historical phase series will naturally age
// out in Prometheus retention.
func (m *ClusterMetrics) SetPhase(phase openbaov1alpha1.ClusterPhase) {
	clusterPhaseGauge.
		WithLabelValues(m.namespace, m.name, string(phase)).
		Set(1.0)
}

// RecordDriftDetected records that Sentinel detected drift on a specific resource.
func (m *ClusterMetrics) RecordDriftDetected(resourceKind string) {
	driftDetectedTotal.
		WithLabelValues(m.namespace, m.name, resourceKind).
		Inc()
}

// RecordDriftCorrected records that the operator corrected drift.
func (m *ClusterMetrics) RecordDriftCorrected() {
	driftCorrectedTotal.
		WithLabelValues(m.namespace, m.name).
		Inc()
}

// SetDriftLastDetectedTimestamp records the timestamp when drift was last detected.
func (m *ClusterMetrics) SetDriftLastDetectedTimestamp(timestampSeconds float64) {
	driftLastDetectedTimestamp.
		WithLabelValues(m.namespace, m.name).
		Set(timestampSeconds)
}

// Clear removes all per-cluster metrics for this cluster. This should be
// called during finalization to avoid leaving stale series after deletion.
func (m *ClusterMetrics) Clear() {
	clusterReadyReplicasGauge.
		DeleteLabelValues(m.namespace, m.name)

	// Clear all known phases for this cluster.
	for _, phase := range []openbaov1alpha1.ClusterPhase{
		openbaov1alpha1.ClusterPhaseInitializing,
		openbaov1alpha1.ClusterPhaseRunning,
		openbaov1alpha1.ClusterPhaseUpgrading,
		openbaov1alpha1.ClusterPhaseBackingUp,
		openbaov1alpha1.ClusterPhaseFailed,
	} {
		clusterPhaseGauge.
			DeleteLabelValues(m.namespace, m.name, string(phase))
	}

	// Clear drift metrics
	driftLastDetectedTimestamp.
		DeleteLabelValues(m.namespace, m.name)
}

// RestoreMetrics provides helpers to record restore operation metrics.
type RestoreMetrics struct {
	namespace string
	name      string
}

// NewRestoreMetrics creates a new RestoreMetrics instance.
func NewRestoreMetrics(namespace, name string) *RestoreMetrics {
	return &RestoreMetrics{
		namespace: namespace,
		name:      name,
	}
}

// RecordStarted increments the restore total counter.
func (m *RestoreMetrics) RecordStarted() {
	restoreTotal.
		WithLabelValues(m.namespace, m.name).
		Inc()
}

// RecordSuccess increments the restore success counter and records duration.
func (m *RestoreMetrics) RecordSuccess(durationSeconds float64) {
	restoreSuccessTotal.
		WithLabelValues(m.namespace, m.name).
		Inc()
	restoreDurationHistogram.
		WithLabelValues(m.namespace, m.name).
		Observe(durationSeconds)
}

// RecordFailure increments the restore failure counter.
func (m *RestoreMetrics) RecordFailure() {
	restoreFailureTotal.
		WithLabelValues(m.namespace, m.name).
		Inc()
}
