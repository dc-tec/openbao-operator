package utils

import (
	"os/exec"
	"strings"
)

// IsOpenShiftCluster returns true if the current cluster serves the OpenShift security API group.
// This is a lightweight capability check suitable for E2E test gating.
func IsOpenShiftCluster() bool {
	// #nosec G204 -- Test utility, command and arguments are controlled
	cmd := exec.Command("kubectl", "get", "--raw", "/apis/security.openshift.io")
	out, err := Run(cmd)
	if err != nil {
		return false
	}
	return strings.Contains(out, "security.openshift.io")
}
