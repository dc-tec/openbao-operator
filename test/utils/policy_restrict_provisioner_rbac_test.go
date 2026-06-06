package utils

import (
	"os"
	"testing"
)

// TestRestrictProvisionerRBACPolicyContainsDangerousVerbGuard provides a
// minimal regression check that the ValidatingAdmissionPolicy used to restrict
// the Provisioner continues to enforce the dangerous-verb guard that
// was added as a defense-in-depth measure.
//
// The policy itself is evaluated by the Kubernetes API server, so this test
// does not attempt to execute CEL expressions. Instead, it asserts that the
// expected Rule 6 stanza is present in the policy manifest. This helps prevent
// accidental removal during future refactors.
func TestRestrictProvisionerRBACPolicyContainsDangerousVerbGuard(t *testing.T) {
	const (
		policyPath = "../../config/policy/openbao-restrict-provisioner-rbac.yaml"
		required   = "The Provisioner cannot create Roles granting " +
			"'impersonate', 'bind', 'escalate', or wildcard permissions."
	)

	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("failed to read policy %s: %v", policyPath, err)
	}

	if !containsString(string(data), required) {
		t.Fatalf("policy %s does not contain required dangerous-verb guard message:\n%q", policyPath, required)
	}
}

func TestRestrictProvisionerRBACPolicyContainsSecretsRoleGuards(t *testing.T) {
	const (
		policyPath = "../../config/policy/openbao-restrict-provisioner-rbac.yaml"
		required   = "The Provisioner can only grant Secrets permissions via the dedicated secrets allowlist Roles."
	)

	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("failed to read policy %s: %v", policyPath, err)
	}

	if !containsString(string(data), required) {
		t.Fatalf("policy %s does not contain required secrets allowlist guard message:\n%q", policyPath, required)
	}
}

func TestRestrictProvisionerRBACPolicyAllowsTenantServiceMonitorRule(t *testing.T) {
	const policyPath = "../../config/policy/openbao-restrict-provisioner-rbac.yaml"

	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("failed to read policy %s: %v", policyPath, err)
	}

	policy := string(data)
	required := []string{
		"rule.apiGroups[0] == 'monitoring.coreos.com'",
		"rule.resources[0] == 'servicemonitors'",
		"rule.verbs.all(v, v in ['create', 'delete', 'get', 'patch'])",
	}
	for _, needle := range required {
		if !containsString(policy, needle) {
			t.Fatalf("policy %s does not allow the tenant ServiceMonitor rule; missing %q", policyPath, needle)
		}
	}
	if containsString(policy, "prometheusrules") {
		t.Fatalf("policy %s unexpectedly allows tenant PrometheusRule management", policyPath)
	}
}

func TestRestrictProvisionerRBACPolicyAllowsControllerDelegationRule(t *testing.T) {
	const policyPath = "../../config/policy/openbao-restrict-provisioner-rbac.yaml"

	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("failed to read policy %s: %v", policyPath, err)
	}

	policy := string(data)
	required := []string{
		"rule.resources[0] == 'openbaoclusters'",
		"rule.verbs.all(v, v in ['restore', 'usecloudidentities', 'usecustomexecutables', 'useimagetrustroots'])",
	}
	for _, needle := range required {
		if !containsString(policy, needle) {
			t.Fatalf("policy %s does not allow the controller delegation rule; missing %q", policyPath, needle)
		}
	}
}

// containsString performs a simple substring check without introducing extra
// dependencies. Kept private and local to avoid over-abstracting.
func containsString(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

// indexOf returns the index of the first occurrence of needle in haystack, or
// -1 if not found.
func indexOf(haystack, needle string) int {
outer:
	for i := 0; i <= len(haystack)-len(needle); i++ {
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
