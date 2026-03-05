package opslifecycle

import (
	"github.com/go-logr/logr"

	"github.com/dc-tec/openbao-operator/internal/logging"
)

// LogPhaseTransition emits a consistent audit event for phase transitions.
func LogPhaseTransition(logger logr.Logger, eventType, phaseFrom, phaseTo string, fields map[string]string) {
	if phaseFrom == phaseTo {
		return
	}
	logging.LogAuditEvent(logger, eventType, phaseTransitionFields(phaseFrom, phaseTo, fields))
}

func phaseTransitionFields(phaseFrom, phaseTo string, fields map[string]string) map[string]string {
	size := len(fields) + 2
	out := make(map[string]string, size)
	for k, v := range fields {
		out[k] = v
	}
	out["phase_from"] = phaseFrom
	out["phase_to"] = phaseTo
	return out
}
