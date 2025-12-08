package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
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
)

func init() {
	metrics.Registry.MustRegister(
		reconcileDurationHistogram,
		reconcileErrorsTotal,
		clusterReadyReplicasGauge,
		clusterPhaseGauge,
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
}
