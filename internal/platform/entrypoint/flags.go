package entrypoint

import (
	"flag"
	"fmt"
	"strings"
	"time"
)

const (
	AdmissionEnforcementFail = "fail"
	AdmissionEnforcementWarn = "warn"
)

// NormalizeAdmissionEnforcement validates and canonicalizes admission enforcement mode.
func NormalizeAdmissionEnforcement(in string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(in))
	if normalized == "" {
		normalized = AdmissionEnforcementFail
	}
	if normalized != AdmissionEnforcementFail && normalized != AdmissionEnforcementWarn {
		return "", fmt.Errorf("invalid admission enforcement mode %q", normalized)
	}
	return normalized, nil
}

// BindManagerFlags binds standard manager networking/election flags.
func BindManagerFlags(fs *flag.FlagSet, metricsAddr, probeAddr *string, enableLeaderElection, secureMetrics *bool) {
	fs.StringVar(metricsAddr, "metrics-bind-address", ":8443", "The address the metrics endpoint binds to.")
	fs.StringVar(probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.BoolVar(enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.BoolVar(secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
}

// BindAdmissionFlags binds admission enforcement flags shared across entrypoints.
func BindAdmissionFlags(fs *flag.FlagSet, admissionEnforcement *string, admissionStartupTimeout *time.Duration) {
	fs.StringVar(admissionEnforcement, "admission-enforcement", AdmissionEnforcementFail,
		"Admission dependency enforcement mode: fail or warn. "+
			"In fail mode the operator refuses to start unless required ValidatingAdmissionPolicies are present and enforced.")
	fs.DurationVar(admissionStartupTimeout, "admission-startup-timeout", 60*time.Second,
		"Maximum time to wait for required admission policies at startup when --admission-enforcement=fail.")
}
