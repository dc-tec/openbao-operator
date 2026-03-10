package restore

import (
	corev1 "k8s.io/api/core/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func (m *Manager) emitNormalEvent(restore *openbaov1alpha1.OpenBaoRestore, reason, note string, args ...interface{}) {
	m.emitEvent(restore, corev1.EventTypeNormal, reason, note, args...)
}

func (m *Manager) emitWarningEvent(restore *openbaov1alpha1.OpenBaoRestore, reason, note string, args ...interface{}) {
	m.emitEvent(restore, corev1.EventTypeWarning, reason, note, args...)
}

func (m *Manager) emitEvent(restore *openbaov1alpha1.OpenBaoRestore, eventType, reason, note string, args ...interface{}) {
	if m == nil || m.recorder == nil || restore == nil {
		return
	}
	m.recorder.Eventf(restore, nil, eventType, reason, reason, note, args...)
}
