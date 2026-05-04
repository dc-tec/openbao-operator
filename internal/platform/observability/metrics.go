package observability

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	claimPhaseGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "claim_phase",
			Help:      "Current phase of an OpenBaoClusterClaim (1 = active phase)",
		},
		[]string{"namespace", "name", "phase"},
	)

	claimConditionGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "claim_condition",
			Help:      "Current claim condition state (1 = active condition status)",
		},
		[]string{"namespace", "name", "type", "status"},
	)

	claimRolloutStateGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "claim_rollout_state",
			Help:      "Current rollout state of an OpenBaoClusterClaim (1 = active rollout state)",
		},
		[]string{"namespace", "name", "state"},
	)

	claimMaterializationModeGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "claim_materialization_mode",
			Help:      "Current materialization mode of an OpenBaoClusterClaim (1 = active mode)",
		},
		[]string{"namespace", "name", "mode"},
	)

	claimSummaryGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "claim_summary",
			Help:      "Current claim diagnostic summary (1 = active summary)",
		},
		[]string{"namespace", "name", "severity", "reason"},
	)

	claimInfoGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "claim_info",
			Help:      "Current claim identity and catalog binding (1 = claim exists)",
		},
		[]string{"namespace", "name", "tenant", "service_offering", "service_profile"},
	)

	claimRestoreStateGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "claim_restore_state",
			Help:      "Current claim-facing restore workflow state (1 = active restore state)",
		},
		[]string{"namespace", "name", "restore", "state"},
	)

	claimUpgradeRequestStateGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "claim_upgrade_request_state",
			Help:      "Current claim upgrade request state (1 = active state)",
		},
		[]string{"namespace", "name", "claim", "state", "reason"},
	)

	claimUpgradeRequestClassificationGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "claim_upgrade_request_classification",
			Help:      "Current claim upgrade request classification (1 = active classification)",
		},
		[]string{"namespace", "name", "claim", "class"},
	)

	claimBackupRequestStateGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "claim_backup_request_state",
			Help:      "Current claim backup request state (1 = active state)",
		},
		[]string{"namespace", "name", "claim", "state", "reason"},
	)

	claimRestoreRequestStateGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "claim_restore_request_state",
			Help:      "Current claim restore request state (1 = active state)",
		},
		[]string{"namespace", "name", "claim", "state", "reason"},
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

	claimMetricsMu            sync.Mutex
	claimMetricCache          = map[string]claimMetricSnapshot{}
	upgradeRequestMetricCache = map[string]upgradeRequestMetricSnapshot{}
	backupRequestMetricCache  = map[string]backupRequestMetricSnapshot{}
	restoreRequestMetricCache = map[string]restoreRequestMetricSnapshot{}
)

type claimMetricSnapshot struct {
	phase               openbaov1alpha1.OpenBaoClusterClaimPhase
	rolloutState        openbaov1alpha1.OpenBaoClusterClaimRolloutState
	materializationMode openbaov1alpha1.OpenBaoClusterClaimMaterializationMode
	conditions          map[string]metav1.ConditionStatus
	summary             *claimSummaryLabels
	info                *claimInfoLabels
	restore             *claimRestoreLabels
}

type claimSummaryLabels struct {
	severity openbaov1alpha1.OpenBaoClusterClaimStatusSeverity
	reason   string
}

type claimInfoLabels struct {
	tenant          string
	serviceOffering string
	serviceProfile  string
}

type claimRestoreLabels struct {
	restore string
	state   openbaov1alpha1.RestorePhase
}

type upgradeRequestMetricSnapshot struct {
	claimName string
	state     openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState
	reason    string
	class     openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClass
}

type backupRequestMetricSnapshot struct {
	claimName string
	state     openbaov1alpha1.OpenBaoClusterClaimBackupRequestState
	reason    string
}

type restoreRequestMetricSnapshot struct {
	claimName string
	state     openbaov1alpha1.OpenBaoClusterClaimRestoreRequestState
	reason    string
}

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
		claimPhaseGauge,
		claimConditionGauge,
		claimRolloutStateGauge,
		claimMaterializationModeGauge,
		claimSummaryGauge,
		claimInfoGauge,
		claimRestoreStateGauge,
		claimUpgradeRequestStateGauge,
		claimUpgradeRequestClassificationGauge,
		claimBackupRequestStateGauge,
		claimRestoreRequestStateGauge,
		// Restore metrics
		restoreStateGauge,
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
	clusterReadReplicasDesiredGauge.
		DeleteLabelValues(m.namespace, m.name)
	clusterReadReplicasReadyGauge.
		DeleteLabelValues(m.namespace, m.name)
	clusterReadReplicasRegisteredGauge.
		DeleteLabelValues(m.namespace, m.name)
	clusterReadReplicasHealthyGauge.
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

// SyncClaim records the current OpenBaoClusterClaim status as first-class claim metrics.
func SyncClaim(claim *openbaov1alpha1.OpenBaoClusterClaim) {
	if claim == nil {
		return
	}

	claimMetricsMu.Lock()
	defer claimMetricsMu.Unlock()

	key := metricKey(claim.Namespace, claim.Name)
	previous := claimMetricCache[key]
	next := claimMetricSnapshot{
		phase:               claim.Status.Phase,
		rolloutState:        claim.Status.Rollout.State,
		materializationMode: claim.Status.Materialization.Mode,
		conditions:          make(map[string]metav1.ConditionStatus, len(claim.Status.Conditions)),
	}

	if previous.phase != "" && previous.phase != claim.Status.Phase {
		claimPhaseGauge.DeleteLabelValues(claim.Namespace, claim.Name, string(previous.phase))
	}
	if claim.Status.Phase != "" {
		claimPhaseGauge.WithLabelValues(claim.Namespace, claim.Name, string(claim.Status.Phase)).Set(1)
	}

	if previous.rolloutState != "" && previous.rolloutState != claim.Status.Rollout.State {
		claimRolloutStateGauge.DeleteLabelValues(claim.Namespace, claim.Name, string(previous.rolloutState))
	}
	if claim.Status.Rollout.State != "" {
		claimRolloutStateGauge.WithLabelValues(claim.Namespace, claim.Name, string(claim.Status.Rollout.State)).Set(1)
	}

	if previous.materializationMode != "" && previous.materializationMode != claim.Status.Materialization.Mode {
		claimMaterializationModeGauge.DeleteLabelValues(claim.Namespace, claim.Name, string(previous.materializationMode))
	}
	if claim.Status.Materialization.Mode != "" {
		claimMaterializationModeGauge.WithLabelValues(claim.Namespace, claim.Name, string(claim.Status.Materialization.Mode)).Set(1)
	}

	for _, condition := range claim.Status.Conditions {
		next.conditions[condition.Type] = condition.Status
		claimConditionGauge.WithLabelValues(claim.Namespace, claim.Name, condition.Type, string(condition.Status)).Set(1)
	}
	for conditionType, status := range previous.conditions {
		if next.conditions[conditionType] == status {
			continue
		}
		claimConditionGauge.DeleteLabelValues(claim.Namespace, claim.Name, conditionType, string(status))
	}

	if previous.summary != nil {
		claimSummaryGauge.DeleteLabelValues(claim.Namespace, claim.Name, string(previous.summary.severity), previous.summary.reason)
	}
	if claim.Status.Summary != nil && claim.Status.Summary.Severity != "" {
		next.summary = &claimSummaryLabels{
			severity: claim.Status.Summary.Severity,
			reason:   claim.Status.Summary.Reason,
		}
		claimSummaryGauge.WithLabelValues(
			claim.Namespace,
			claim.Name,
			string(claim.Status.Summary.Severity),
			claim.Status.Summary.Reason,
		).Set(1)
	}

	if previous.info != nil {
		claimInfoGauge.DeleteLabelValues(
			claim.Namespace,
			claim.Name,
			previous.info.tenant,
			previous.info.serviceOffering,
			previous.info.serviceProfile,
		)
	}
	info := resolveClaimInfo(claim)
	next.info = &info
	claimInfoGauge.WithLabelValues(
		claim.Namespace,
		claim.Name,
		info.tenant,
		info.serviceOffering,
		info.serviceProfile,
	).Set(1)

	if previous.restore != nil {
		claimRestoreStateGauge.DeleteLabelValues(
			claim.Namespace,
			claim.Name,
			previous.restore.restore,
			string(previous.restore.state),
		)
	}
	if claim.Status.Restore != nil && claim.Status.Restore.State != "" {
		restoreName := ""
		if claim.Status.Restore.ExecutionRef != nil {
			restoreName = claim.Status.Restore.ExecutionRef.Name
		} else if claim.Status.Restore.RequestRef != nil {
			restoreName = claim.Status.Restore.RequestRef.Name
		}
		next.restore = &claimRestoreLabels{
			restore: restoreName,
			state:   claim.Status.Restore.State,
		}
		claimRestoreStateGauge.WithLabelValues(
			claim.Namespace,
			claim.Name,
			restoreName,
			string(claim.Status.Restore.State),
		).Set(1)
	}

	claimMetricCache[key] = next
}

// ClearClaim removes all claim metrics for one OpenBaoClusterClaim.
func ClearClaim(namespace, name string) {
	claimMetricsMu.Lock()
	previous := claimMetricCache[metricKey(namespace, name)]
	delete(claimMetricCache, metricKey(namespace, name))
	claimMetricsMu.Unlock()

	for _, phase := range claimPhases() {
		claimPhaseGauge.DeleteLabelValues(namespace, name, string(phase))
	}
	for _, state := range claimRolloutStates() {
		claimRolloutStateGauge.DeleteLabelValues(namespace, name, string(state))
	}
	for _, mode := range claimMaterializationModes() {
		claimMaterializationModeGauge.DeleteLabelValues(namespace, name, string(mode))
	}
	for conditionType, status := range previous.conditions {
		claimConditionGauge.DeleteLabelValues(namespace, name, conditionType, string(status))
	}
	if previous.summary != nil {
		claimSummaryGauge.DeleteLabelValues(namespace, name, string(previous.summary.severity), previous.summary.reason)
	}
	if previous.info != nil {
		claimInfoGauge.DeleteLabelValues(namespace, name, previous.info.tenant, previous.info.serviceOffering, previous.info.serviceProfile)
	}
	if previous.restore != nil {
		claimRestoreStateGauge.DeleteLabelValues(namespace, name, previous.restore.restore, string(previous.restore.state))
	}
}

// SyncClaimUpgradeRequest records the current OpenBaoClusterClaimUpgradeRequest state.
func SyncClaimUpgradeRequest(request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) {
	if request == nil {
		return
	}

	claimMetricsMu.Lock()
	defer claimMetricsMu.Unlock()

	key := metricKey(request.Namespace, request.Name)
	previous := upgradeRequestMetricCache[key]
	if previous.state != "" {
		claimUpgradeRequestStateGauge.DeleteLabelValues(
			request.Namespace,
			request.Name,
			previous.claimName,
			string(previous.state),
			previous.reason,
		)
	}
	if previous.class != "" {
		claimUpgradeRequestClassificationGauge.DeleteLabelValues(
			request.Namespace,
			request.Name,
			previous.claimName,
			string(previous.class),
		)
	}

	next := upgradeRequestMetricSnapshot{
		claimName: request.Spec.ClaimRef.Name,
		state:     request.Status.State,
		reason:    request.Status.Reason,
	}
	if request.Status.State != "" {
		claimUpgradeRequestStateGauge.WithLabelValues(
			request.Namespace,
			request.Name,
			request.Spec.ClaimRef.Name,
			string(request.Status.State),
			request.Status.Reason,
		).Set(1)
	}
	if request.Status.Classification != nil && request.Status.Classification.Class != "" {
		next.class = request.Status.Classification.Class
		claimUpgradeRequestClassificationGauge.WithLabelValues(
			request.Namespace,
			request.Name,
			request.Spec.ClaimRef.Name,
			string(request.Status.Classification.Class),
		).Set(1)
	}

	upgradeRequestMetricCache[key] = next
}

// ClearClaimUpgradeRequest removes all metrics for one OpenBaoClusterClaimUpgradeRequest.
func ClearClaimUpgradeRequest(namespace, name string) {
	claimMetricsMu.Lock()
	previous := upgradeRequestMetricCache[metricKey(namespace, name)]
	delete(upgradeRequestMetricCache, metricKey(namespace, name))
	claimMetricsMu.Unlock()

	for _, state := range claimUpgradeRequestStates() {
		claimUpgradeRequestStateGauge.DeleteLabelValues(namespace, name, previous.claimName, string(state), previous.reason)
	}
	if previous.state != "" {
		claimUpgradeRequestStateGauge.DeleteLabelValues(namespace, name, previous.claimName, string(previous.state), previous.reason)
	}
	for _, class := range claimUpgradeRequestClasses() {
		claimUpgradeRequestClassificationGauge.DeleteLabelValues(namespace, name, previous.claimName, string(class))
	}
}

// SyncClaimBackupRequest records the current OpenBaoClusterClaimBackupRequest state.
func SyncClaimBackupRequest(request *openbaov1alpha1.OpenBaoClusterClaimBackupRequest) {
	if request == nil {
		return
	}

	claimMetricsMu.Lock()
	defer claimMetricsMu.Unlock()

	key := metricKey(request.Namespace, request.Name)
	previous := backupRequestMetricCache[key]
	if previous.state != "" {
		claimBackupRequestStateGauge.DeleteLabelValues(
			request.Namespace,
			request.Name,
			previous.claimName,
			string(previous.state),
			previous.reason,
		)
	}

	next := backupRequestMetricSnapshot{
		claimName: request.Spec.ClaimRef.Name,
		state:     request.Status.State,
		reason:    request.Status.Reason,
	}
	if request.Status.State != "" {
		claimBackupRequestStateGauge.WithLabelValues(
			request.Namespace,
			request.Name,
			request.Spec.ClaimRef.Name,
			string(request.Status.State),
			request.Status.Reason,
		).Set(1)
	}
	backupRequestMetricCache[key] = next
}

// ClearClaimBackupRequest removes all metrics for one OpenBaoClusterClaimBackupRequest.
func ClearClaimBackupRequest(namespace, name string) {
	claimMetricsMu.Lock()
	previous := backupRequestMetricCache[metricKey(namespace, name)]
	delete(backupRequestMetricCache, metricKey(namespace, name))
	claimMetricsMu.Unlock()

	for _, state := range claimBackupRequestStates() {
		claimBackupRequestStateGauge.DeleteLabelValues(namespace, name, previous.claimName, string(state), previous.reason)
	}
	if previous.state != "" {
		claimBackupRequestStateGauge.DeleteLabelValues(namespace, name, previous.claimName, string(previous.state), previous.reason)
	}
}

// SyncClaimRestoreRequest records the current OpenBaoClusterClaimRestoreRequest state.
func SyncClaimRestoreRequest(request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest) {
	if request == nil {
		return
	}

	claimMetricsMu.Lock()
	defer claimMetricsMu.Unlock()

	key := metricKey(request.Namespace, request.Name)
	previous := restoreRequestMetricCache[key]
	if previous.state != "" {
		claimRestoreRequestStateGauge.DeleteLabelValues(
			request.Namespace,
			request.Name,
			previous.claimName,
			string(previous.state),
			previous.reason,
		)
	}

	next := restoreRequestMetricSnapshot{
		claimName: request.Spec.ClaimRef.Name,
		state:     request.Status.State,
		reason:    request.Status.Reason,
	}
	if request.Status.State != "" {
		claimRestoreRequestStateGauge.WithLabelValues(
			request.Namespace,
			request.Name,
			request.Spec.ClaimRef.Name,
			string(request.Status.State),
			request.Status.Reason,
		).Set(1)
	}
	restoreRequestMetricCache[key] = next
}

// ClearClaimRestoreRequest removes all metrics for one OpenBaoClusterClaimRestoreRequest.
func ClearClaimRestoreRequest(namespace, name string) {
	claimMetricsMu.Lock()
	previous := restoreRequestMetricCache[metricKey(namespace, name)]
	delete(restoreRequestMetricCache, metricKey(namespace, name))
	claimMetricsMu.Unlock()

	for _, state := range claimRestoreRequestStates() {
		claimRestoreRequestStateGauge.DeleteLabelValues(namespace, name, previous.claimName, string(state), previous.reason)
	}
	if previous.state != "" {
		claimRestoreRequestStateGauge.DeleteLabelValues(namespace, name, previous.claimName, string(previous.state), previous.reason)
	}
}

func resolveClaimInfo(claim *openbaov1alpha1.OpenBaoClusterClaim) claimInfoLabels {
	info := claimInfoLabels{
		tenant: claim.Spec.TenantRef.Name,
	}
	if claim.Spec.ServiceOfferingRef != nil {
		info.serviceOffering = claim.Spec.ServiceOfferingRef.Name
	}
	info.serviceProfile = claim.Spec.ServiceProfileRef.Name
	if claim.Status.Applied.ServiceOfferingRef != nil {
		info.serviceOffering = claim.Status.Applied.ServiceOfferingRef.Name
	}
	if claim.Status.Applied.ServiceProfileRef != nil {
		info.serviceProfile = claim.Status.Applied.ServiceProfileRef.Name
	}
	return info
}

func metricKey(namespace, name string) string {
	return namespace + "/" + name
}

func claimPhases() []openbaov1alpha1.OpenBaoClusterClaimPhase {
	return []openbaov1alpha1.OpenBaoClusterClaimPhase{
		openbaov1alpha1.OpenBaoClusterClaimPhasePending,
		openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning,
		openbaov1alpha1.OpenBaoClusterClaimPhaseReady,
		openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded,
		openbaov1alpha1.OpenBaoClusterClaimPhaseFailed,
		openbaov1alpha1.OpenBaoClusterClaimPhaseDeleting,
	}
}

func claimRolloutStates() []openbaov1alpha1.OpenBaoClusterClaimRolloutState {
	return []openbaov1alpha1.OpenBaoClusterClaimRolloutState{
		openbaov1alpha1.OpenBaoClusterClaimRolloutStateIdle,
		openbaov1alpha1.OpenBaoClusterClaimRolloutStatePending,
		openbaov1alpha1.OpenBaoClusterClaimRolloutStateRendering,
		openbaov1alpha1.OpenBaoClusterClaimRolloutStateRollingOut,
		openbaov1alpha1.OpenBaoClusterClaimRolloutStateBlocked,
		openbaov1alpha1.OpenBaoClusterClaimRolloutStateFailed,
	}
}

func claimMaterializationModes() []openbaov1alpha1.OpenBaoClusterClaimMaterializationMode {
	return []openbaov1alpha1.OpenBaoClusterClaimMaterializationMode{
		openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
	}
}

func claimUpgradeRequestStates() []openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState {
	return []openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState{
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed,
	}
}

func claimUpgradeRequestClasses() []openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClass {
	return []openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClass{
		openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked,
	}
}

func claimBackupRequestStates() []openbaov1alpha1.OpenBaoClusterClaimBackupRequestState {
	return []openbaov1alpha1.OpenBaoClusterClaimBackupRequestState{
		openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatePending,
		openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateRunning,
		openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateSucceeded,
		openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateBlocked,
		openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed,
	}
}

func claimRestoreRequestStates() []openbaov1alpha1.OpenBaoClusterClaimRestoreRequestState {
	return []openbaov1alpha1.OpenBaoClusterClaimRestoreRequestState{
		openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending,
		openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning,
		openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateSucceeded,
		openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked,
		openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed,
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
