package requestworkflow

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
)

const (
	stateBlocked = "Blocked"
	stateFailed  = "Failed"
)

// StateTransitionChanged reports whether a workflow state transition deserves a user-facing event.
func StateTransitionChanged(oldState, oldReason, newState, newReason string) bool {
	return oldState != newState || oldReason != newReason
}

// EventTypeForState returns a warning event type for blocked/failed states and normal otherwise.
func EventTypeForState(state string) string {
	switch state {
	case stateBlocked, stateFailed:
		return corev1.EventTypeWarning
	default:
		return corev1.EventTypeNormal
	}
}

// EventReason returns a low-cardinality event reason for a workflow state.
func EventReason(state, reason, fallback string) string {
	if reason != "" {
		return reason
	}
	if state != "" {
		return fallback + state
	}
	return fallback
}

// EmitEvent records a Kubernetes Event for a workflow object.
func EmitEvent(recorder events.EventRecorder, object runtime.Object, eventType, reason, note string) {
	if recorder == nil || object == nil || reason == "" || note == "" {
		return
	}
	recorder.Eventf(object, nil, eventType, reason, reason, "%s", note)
}
