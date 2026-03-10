package init

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	ReasonInitStarted   = "InitStarted"
	ReasonInitCompleted = "InitCompleted"
	ReasonInitFailed    = "InitFailed"
)

func emitNormalEvent(recorder events.EventRecorder, cluster *openbaov1alpha1.OpenBaoCluster, reason, note string, args ...interface{}) {
	emitEvent(recorder, cluster, corev1.EventTypeNormal, reason, note, args...)
}

func emitWarningEvent(recorder events.EventRecorder, cluster *openbaov1alpha1.OpenBaoCluster, reason, note string, args ...interface{}) {
	emitEvent(recorder, cluster, corev1.EventTypeWarning, reason, note, args...)
}

func emitEvent(recorder events.EventRecorder, cluster *openbaov1alpha1.OpenBaoCluster, eventType, reason, note string, args ...interface{}) {
	if recorder == nil || cluster == nil {
		return
	}
	recorder.Eventf(cluster, nil, eventType, reason, reason, note, args...)
}
