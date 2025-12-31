package controller

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestReconcileMetrics_NoPanic(t *testing.T) {
	m := NewReconcileMetrics("ns", "name", "ctrl")

	// These calls should not panic and will register/update metrics for the
	// given label set.
	m.ObserveDuration(0.5)
	m.ObserveDuration(1.0)
	m.IncrementError("Error")
}

func TestClusterMetrics_NoPanic(t *testing.T) {
	m := NewClusterMetrics("ns", "name")

	m.SetReadyReplicas(3)
	m.SetPhase(openbaov1alpha1.ClusterPhaseInitializing)
	m.SetPhase(openbaov1alpha1.ClusterPhaseRunning)
	m.Clear()
}
