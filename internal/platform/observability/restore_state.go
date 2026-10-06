package observability

import (
	"github.com/prometheus/client_golang/prometheus"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// ObserveRestoreStates rebuilds the namespace's gauges from cached requests. An
// unresolved execution takes precedence; otherwise the newest request wins.
// Reconciliation order and controller restarts do not change that selection.
func ObserveRestoreStates(namespace string, requests []api.OpenBaoRestore) {
	selected := make(map[string]*api.OpenBaoRestore)
	for i := range requests {
		request := &requests[i]
		if request.Namespace != namespace || request.Spec.Cluster == "" {
			continue
		}

		previous := selected[request.Spec.Cluster]
		if previous == nil || preferRestoreState(request, previous) {
			selected[request.Spec.Cluster] = request
		}
	}

	restoreStateGauge.DeletePartialMatch(prometheus.Labels{"namespace": namespace})
	for cluster, request := range selected {
		NewRestoreMetrics(namespace, cluster).setState(observedRestoreState(request))
	}
}

func preferRestoreState(request, previous *api.OpenBaoRestore) bool {
	requestHeld := unresolvedRestore(request)
	previousHeld := unresolvedRestore(previous)
	if requestHeld != previousHeld {
		return requestHeld
	}
	if request.CreationTimestamp.Equal(&previous.CreationTimestamp) {
		return request.Name > previous.Name
	}
	return request.CreationTimestamp.After(previous.CreationTimestamp.Time)
}

func unresolvedRestore(request *api.OpenBaoRestore) bool {
	return request.Status.AdministratorDisposition == "" &&
		request.Status.Phase == api.RestorePhaseUnknown
}

func observedRestoreState(request *api.OpenBaoRestore) float64 {
	if request.Status.AdministratorDisposition == api.RestoreAdministratorAbandon {
		return 7
	}
	if request.Status.Phase == api.RestorePhaseCompleted {
		return 2
	}
	if request.Status.AdministratorDisposition == api.RestoreAdministratorResume {
		return 6
	}
	if request.Status.Restart != nil {
		return 5
	}
	if unresolvedRestore(request) {
		return 4
	}
	if request.Status.Phase == api.RestorePhaseFailed {
		return 3
	}
	return 1
}
