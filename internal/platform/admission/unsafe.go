package admission

import (
	"os"
	"strings"
)

// UnsafeAdmissionDisabled reports whether admission policy enforcement is intentionally disabled.
//
// When true, the operator should treat admission policies as "unsafe mode" and avoid fail-closed
// behavior that would otherwise prevent startup/reconciliation.
//
// WARNING: Enabling this materially weakens the security posture of multi-tenant installs.
func UnsafeAdmissionDisabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("OPENBAO_UNSAFE_ADMISSION_DISABLED")), "true")
}
