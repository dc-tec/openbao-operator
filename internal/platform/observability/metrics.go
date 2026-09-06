package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

var (
	clusterMetricPhases = [...]openbaov1alpha1.ClusterPhase{
		openbaov1alpha1.ClusterPhaseInitializing,
		openbaov1alpha1.ClusterPhaseRunning,
		openbaov1alpha1.ClusterPhaseUpgrading,
		openbaov1alpha1.ClusterPhaseBackingUp,
		openbaov1alpha1.ClusterPhaseFailed,
	}

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

	clusterReadReplicasDesiredGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "cluster_read_replicas_desired",
			Help:      "Desired number of steady read replicas for an OpenBaoCluster",
		},
		[]string{"namespace", "name"},
	)

	clusterReadReplicasReadyGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "cluster_read_replicas_ready",
			Help:      "Number of Ready steady read replicas for an OpenBaoCluster",
		},
		[]string{"namespace", "name"},
	)

	clusterReadReplicasRegisteredGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "cluster_read_replicas_registered",
			Help:      "Number of steady read replicas registered in Raft membership for an OpenBaoCluster",
		},
		[]string{"namespace", "name"},
	)

	clusterReadReplicasHealthyGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "cluster_read_replicas_healthy",
			Help:      "Number of steady read replicas Autopilot considers healthy for an OpenBaoCluster",
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

	// Restore metrics
	restoreStateGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "restore_state",
			Help:      "Current restore state per cluster (0=none, 1=running, 2=success, 3=failed)",
		},
		[]string{"namespace", "name"},
	)

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
		clusterReadReplicasDesiredGauge,
		clusterReadReplicasReadyGauge,
		clusterReadReplicasRegisteredGauge,
		clusterReadReplicasHealthyGauge,
		clusterPhaseGauge,
		// Restore metrics
		restoreStateGauge,
		restoreTotal,
		restoreSuccessTotal,
		restoreFailureTotal,
		restoreDurationHistogram,
	)
}

// ReconcileMetrics records metrics for one reconciliation of a controller's
// resource. Create a new helper for each reconciliation.
type ReconcileMetrics struct {
	namespace  string
	name       string
	controller string
	cleared    bool
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
	if m.cleared {
		return
	}
	reconcileDurationHistogram.
		WithLabelValues(m.namespace, m.name, m.controller).
		Observe(durationSeconds)
}

// IncrementError increments the reconcile error counter with the given reason.
// Reason values should be low-cardinality strings (for example, "KubernetesAPIError").
func (m *ReconcileMetrics) IncrementError(reason string) {
	if m.cleared {
		return
	}
	reconcileErrorsTotal.
		WithLabelValues(m.namespace, m.name, m.controller, reason).
		Inc()
}

// Clear removes this controller's metrics for an absent or finalized resource.
// Further observations through this helper are ignored, including deferred ones.
func (m *ReconcileMetrics) Clear() {
	if m.cleared {
		return
	}
	m.cleared = true
	reconcileDurationHistogram.DeleteLabelValues(m.namespace, m.name, m.controller)
	reconcileErrorsTotal.DeletePartialMatch(prometheus.Labels{
		"namespace":  m.namespace,
		"name":       m.name,
		"controller": m.controller,
	})
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

// SetReadReplicaCounts records the observed steady read-replica counts for the cluster.
func (m *ClusterMetrics) SetReadReplicaCounts(desiredReplicas, readyReplicas, registeredReplicas, healthyReplicas int32) {
	clusterReadReplicasDesiredGauge.
		WithLabelValues(m.namespace, m.name).
		Set(float64(desiredReplicas))
	clusterReadReplicasReadyGauge.
		WithLabelValues(m.namespace, m.name).
		Set(float64(readyReplicas))
	clusterReadReplicasRegisteredGauge.
		WithLabelValues(m.namespace, m.name).
		Set(float64(registeredReplicas))
	clusterReadReplicasHealthyGauge.
		WithLabelValues(m.namespace, m.name).
		Set(float64(healthyReplicas))
}

// SetPhase records 1 for the current phase and 0 for other known phases.
// Only known phase labels are exported. An empty or unrecognized phase leaves
// all phase gauges at 0.
func (m *ClusterMetrics) SetPhase(phase openbaov1alpha1.ClusterPhase) {
	known := false
	for _, knownPhase := range clusterMetricPhases {
		if knownPhase == phase {
			known = true
		}
		clusterPhaseGauge.
			WithLabelValues(m.namespace, m.name, string(knownPhase)).
			Set(0)
	}
	if known {
		clusterPhaseGauge.
			WithLabelValues(m.namespace, m.name, string(phase)).
			Set(1)
	}
}

// Clear removes all per-cluster metrics for this cluster. This should be
// called during finalization to avoid leaving stale series after deletion.
func (m *ClusterMetrics) Clear() {
	clusterReadyReplicasGauge.
		DeleteLabelValues(m.namespace, m.name)
	clusterReadReplicasDesiredGauge.
		DeleteLabelValues(m.namespace, m.name)
	clusterReadReplicasReadyGauge.
		DeleteLabelValues(m.namespace, m.name)
	clusterReadReplicasRegisteredGauge.
		DeleteLabelValues(m.namespace, m.name)
	clusterReadReplicasHealthyGauge.
		DeleteLabelValues(m.namespace, m.name)

	// Clear all known phases for this cluster.
	for _, phase := range clusterMetricPhases {
		clusterPhaseGauge.
			DeleteLabelValues(m.namespace, m.name, string(phase))
	}
}

// RestoreMetrics provides helpers to record restore operation metrics.
type RestoreMetrics struct {
	namespace string
	name      string
}

func (m *RestoreMetrics) setState(state float64) {
	restoreStateGauge.
		WithLabelValues(m.namespace, m.name).
		Set(state)
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
	m.setState(1)
	restoreTotal.
		WithLabelValues(m.namespace, m.name).
		Inc()
}

// RecordSuccess increments the restore success counter and records duration.
func (m *RestoreMetrics) RecordSuccess(durationSeconds float64) {
	m.setState(2)
	restoreSuccessTotal.
		WithLabelValues(m.namespace, m.name).
		Inc()
	restoreDurationHistogram.
		WithLabelValues(m.namespace, m.name).
		Observe(durationSeconds)
}

// RecordFailure increments the restore failure counter.
func (m *RestoreMetrics) RecordFailure() {
	m.setState(3)
	restoreFailureTotal.
		WithLabelValues(m.namespace, m.name).
		Inc()
}

// RecordFailureWithDuration increments the restore failure counter and records duration.
func (m *RestoreMetrics) RecordFailureWithDuration(durationSeconds float64) {
	m.setState(3)
	restoreFailureTotal.
		WithLabelValues(m.namespace, m.name).
		Inc()
	restoreDurationHistogram.
		WithLabelValues(m.namespace, m.name).
		Observe(durationSeconds)
}
