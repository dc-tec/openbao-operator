package logging

import (
	"sort"
	"strings"

	"github.com/go-logr/logr"
)

// LogAuditEvent logs a structured audit event for operator actions.
// Audit events are distinct from regular debug/info logs and are tagged
// with "audit=true" for easy filtering in log aggregation systems.
func LogAuditEvent(logger logr.Logger, eventType string, fields map[string]string) {
	auditLogger := logger.WithValues("audit", "true", "event_type", eventType)
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		prefixedKey := auditFieldKey(key)
		if prefixedKey == "" {
			continue
		}
		auditLogger = auditLogger.WithValues(prefixedKey, fields[key])
	}
	auditLogger.Info("Operator audit event")
}

func auditFieldKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "audit_") {
		return trimmed
	}
	return "audit_" + trimmed
}
