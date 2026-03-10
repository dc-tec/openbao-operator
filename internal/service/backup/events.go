package backup

import (
	corev1 "k8s.io/api/core/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func (m *Manager) emitPreconditionEvent(cluster *openbaov1alpha1.OpenBaoCluster, preconditionErr *backupPreconditionError) {
	if preconditionErr == nil {
		return
	}
	m.emitEvent(cluster, preconditionErr.eventType, preconditionErr.reason, "%s", preconditionErr.message)
}

func (m *Manager) emitNormalEvent(cluster *openbaov1alpha1.OpenBaoCluster, reason, note string, args ...interface{}) {
	m.emitEvent(cluster, corev1.EventTypeNormal, reason, note, args...)
}

func (m *Manager) emitWarningEvent(cluster *openbaov1alpha1.OpenBaoCluster, reason, note string, args ...interface{}) {
	m.emitEvent(cluster, corev1.EventTypeWarning, reason, note, args...)
}

func (m *Manager) emitEvent(cluster *openbaov1alpha1.OpenBaoCluster, eventType, reason, note string, args ...interface{}) {
	if m == nil || m.recorder == nil || cluster == nil {
		return
	}
	m.recorder.Eventf(cluster, nil, eventType, reason, reason, note, args...)
}
