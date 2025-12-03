package logging

import "github.com/go-logr/logr"

// LogAuditEvent logs a structured audit event for operator actions.
// Audit events are distinct from regular debug/info logs and are tagged
// with "audit=true" for easy filtering in log aggregation systems.
func LogAuditEvent(logger logr.Logger, eventType string, fields map[string]string) {
	auditLogger := logger.WithValues("audit", "true", "event_type", eventType)
	for key, value := range fields {
		auditLogger = auditLogger.WithValues(key, value)
	}
	auditLogger.Info("Operator audit event")
}
