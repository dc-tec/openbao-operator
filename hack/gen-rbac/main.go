package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dc-tec/openbao-operator/internal/provisioner"
	rbacv1 "k8s.io/api/rbac/v1"
)

const outputPath = "config/rbac/provisioner_delegate_clusterrole.yaml"

const header = `---
# [SECURITY NOTICE]
# This ClusterRole defines the MAXIMUM permissions that can be delegated to a Tenant.
# It is bound to the 'delegate' ServiceAccount, which is never used directly.
# The Provisioner impersonates this account ONLY to create scoped Roles in Tenant namespaces.
#
# Trivy/Scanner Alerts regarding Secret/Role access here are FALSE POSITIVES relative to
# the architecture: these permissions are required to satisfy Kubernetes RBAC escalation
# checks when generating tenant-scoped policies.
#
# Architecture Context:
# - Delegation: The Provisioner uses 'impersonate' to create Roles for tenants.
# - K8s RBAC Rule: A user (or ServiceAccount) cannot create a Role granting permissions
#   they do not possess themselves.
# - Conclusion: The Delegate *must* hold the union of all permissions that *any* tenant
#   might need. Removing "Secrets" access here would prevent the operator from allowing
#   Tenants to manage their own secrets.
#
# Template ClusterRole that defines exactly what a tenant should be allowed to do.
# This is bound to the delegate ServiceAccount, which the Provisioner impersonates
# when creating tenant Roles/RoleBindings. The API server enforces that the delegate
# can only create Roles with permissions it itself possesses.
`

func main() {
	rules, err := desiredRules()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	content := renderClusterRoleYAML(rules)
	// #nosec G306 -- writes non-sensitive YAML intended to be committed to the repo.
	if err := os.WriteFile(filepath.Clean(outputPath), []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: write %s: %v\n", outputPath, err)
		os.Exit(1)
	}
}

func desiredRules() ([]rbacv1.PolicyRule, error) {
	// Namespace value is irrelevant for rules. Use a stable placeholder.
	const namespace = "tenant-placeholder"

	tenantRole := provisioner.GenerateTenantRole(namespace)
	tenantSecretsReaderRole := provisioner.GenerateTenantSecretsReaderRole(namespace, nil)
	tenantSecretsWriterRole := provisioner.GenerateTenantSecretsWriterRole(namespace, nil)

	rbacRule, ok := findRBACManagementRule(tenantRole.Rules)
	if !ok {
		return nil, fmt.Errorf(
			"failed to locate tenant RBAC management rule (roles/rolebindings) in GenerateTenantRole output",
		)
	}

	rules := make([]rbacv1.PolicyRule, 0,
		len(tenantRole.Rules)+len(tenantSecretsReaderRole.Rules)+len(tenantSecretsWriterRole.Rules)+1,
	)
	rules = append(rules, rbacRule)

	for _, rule := range tenantRole.Rules {
		if policyRuleEqual(rule, rbacRule) {
			continue
		}
		rules = append(rules, rule)
	}

	for _, rule := range tenantSecretsReaderRole.Rules {
		if anyRuleCovers(rules, rule) {
			continue
		}
		rules = append(rules, rule)
	}

	for _, rule := range tenantSecretsWriterRole.Rules {
		if anyRuleCovers(rules, rule) {
			continue
		}
		rules = append(rules, rule)
	}

	// For the delegate template, we need to include maximum secrets permissions
	// even when no specific secret names are provided. This allows the delegate
	// to grant any secrets permissions that tenants might need.
	// Note: GenerateTenantSecretsWriterRole returns empty rules when secretNames is nil,
	// so we need to explicitly add the maximum permissions here.
	if len(tenantSecretsWriterRole.Rules) == 0 {
		// Add maximum secrets write permissions (create, delete, get, patch, update)
		// without resourceNames restrictions for the delegate template.
		maxSecretsWriteRule := rbacv1.PolicyRule{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"create", "delete", "get", "patch", "update"},
		}
		if !anyRuleCovers(rules, maxSecretsWriteRule) {
			rules = append(rules, maxSecretsWriteRule)
		}
	}

	return rules, nil
}

func findRBACManagementRule(rules []rbacv1.PolicyRule) (rbacv1.PolicyRule, bool) {
	for _, rule := range rules {
		if len(rule.APIGroups) != 1 || rule.APIGroups[0] != "rbac.authorization.k8s.io" {
			continue
		}
		if !stringSliceContainsAll(rule.Resources, []string{"roles", "rolebindings"}) {
			continue
		}
		if !stringSliceContainsAll(rule.Verbs, []string{"create", "update", "patch", "delete"}) {
			continue
		}
		return rule, true
	}
	return rbacv1.PolicyRule{}, false
}

func anyRuleCovers(existing []rbacv1.PolicyRule, candidate rbacv1.PolicyRule) bool {
	for _, rule := range existing {
		if policyRuleCovers(rule, candidate) {
			return true
		}
	}
	return false
}

func policyRuleCovers(a, b rbacv1.PolicyRule) bool {
	// We only generate resource rules here; if either side uses NonResourceURLs,
	// keep it strict to avoid accidental broadening.
	if len(a.NonResourceURLs) > 0 || len(b.NonResourceURLs) > 0 {
		return policyRuleEqual(a, b)
	}
	if !stringSetCovers(a.APIGroups, b.APIGroups) {
		return false
	}
	if !stringSetCovers(a.Resources, b.Resources) {
		return false
	}
	if !stringSetCovers(a.Verbs, b.Verbs) {
		return false
	}
	if !stringSetCovers(a.ResourceNames, b.ResourceNames) {
		return false
	}
	return true
}

func stringSetCovers(have, need []string) bool {
	if len(need) == 0 {
		return true
	}
	if stringSliceContains(have, "*") {
		return true
	}
	return stringSliceContainsAll(have, need)
}

func policyRuleEqual(a, b rbacv1.PolicyRule) bool {
	return stringSliceEqual(a.APIGroups, b.APIGroups) &&
		stringSliceEqual(a.Resources, b.Resources) &&
		stringSliceEqual(a.Verbs, b.Verbs) &&
		stringSliceEqual(a.ResourceNames, b.ResourceNames) &&
		stringSliceEqual(a.NonResourceURLs, b.NonResourceURLs)
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringSliceContainsAll(haystack, needles []string) bool {
	for _, needle := range needles {
		if !stringSliceContains(haystack, needle) {
			return false
		}
	}
	return true
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func renderClusterRoleYAML(rules []rbacv1.PolicyRule) string {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("apiVersion: rbac.authorization.k8s.io/v1\n")
	b.WriteString("kind: ClusterRole\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: openbao-operator-tenant-template\n")
	b.WriteString("  labels:\n")
	b.WriteString("    app.kubernetes.io/name: openbao-operator\n")
	b.WriteString("    app.kubernetes.io/component: provisioner\n")
	b.WriteString("    app.kubernetes.io/managed-by: kustomize\n")
	b.WriteString("rules:\n")

	for _, rule := range rules {
		writePolicyRule(&b, rule)
	}

	return b.String()
}

func writePolicyRule(b *strings.Builder, rule rbacv1.PolicyRule) {
	b.WriteString("  - apiGroups:\n")
	for _, group := range rule.APIGroups {
		b.WriteString("      - ")
		b.WriteString(yamlScalar(group))
		b.WriteString("\n")
	}

	b.WriteString("    resources:\n")
	for _, resource := range rule.Resources {
		b.WriteString("      - ")
		b.WriteString(yamlScalar(resource))
		b.WriteString("\n")
	}

	b.WriteString("    verbs:\n")
	for _, verb := range rule.Verbs {
		b.WriteString("      - ")
		b.WriteString(yamlScalar(verb))
		b.WriteString("\n")
	}

	if len(rule.ResourceNames) > 0 {
		b.WriteString("    resourceNames:\n")
		for _, name := range rule.ResourceNames {
			b.WriteString("      - ")
			b.WriteString(yamlScalar(name))
			b.WriteString("\n")
		}
	}

	if len(rule.NonResourceURLs) > 0 {
		b.WriteString("    nonResourceURLs:\n")
		for _, url := range rule.NonResourceURLs {
			b.WriteString("      - ")
			b.WriteString(yamlScalar(url))
			b.WriteString("\n")
		}
	}
}

func yamlScalar(value string) string {
	if value == "" {
		return `""`
	}
	return value
}
