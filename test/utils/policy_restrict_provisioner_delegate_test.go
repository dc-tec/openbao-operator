package utils

import (
	"os"
	"testing"
)

// TestRestrictProvisionerDelegatePolicyContainsDangerousVerbGuard provides a
// minimal regression check that the ValidatingAdmissionPolicy used to restrict
// the Provisioner Delegate continues to enforce the dangerous-verb guard that
// was added as a defense-in-depth measure.
//
// The policy itself is evaluated by the Kubernetes API server, so this test
// does not attempt to execute CEL expressions. Instead, it asserts that the
// expected Rule 6 stanza is present in the policy manifest. This helps prevent
// accidental removal during future refactors.
func TestRestrictProvisionerDelegatePolicyContainsDangerousVerbGuard(t *testing.T) {
	const (
		policyPath = "../../config/policy/restrict-provisioner-delegate.yaml"
		required   = "The Provisioner Delegate cannot create Roles granting " +
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

func TestRestrictProvisionerDelegatePolicyContainsSecretsRoleGuards(t *testing.T) {
	const (
		policyPath = "../../config/policy/restrict-provisioner-delegate.yaml"
		required   = "The Provisioner Delegate can only grant Secrets permissions via the dedicated secrets allowlist Roles."
	)

	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("failed to read policy %s: %v", policyPath, err)
	}

	if !containsString(string(data), required) {
		t.Fatalf("policy %s does not contain required secrets allowlist guard message:\n%q", policyPath, required)
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
