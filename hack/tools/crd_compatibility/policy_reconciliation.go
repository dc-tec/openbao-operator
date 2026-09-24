package main

// newPolicyOptInGuard recognizes the bootstrap guard for a newly added
// optional approver reference. Tightening an already released field still needs review.
func newPolicyOptInGuard(oldNode, current schemaNode, oldNodes, newNodes map[string]schemaNode) *celRule {
	if current.CRD != "openbaoclusters.openbao.org" || current.Kind != "OpenBaoCluster" ||
		current.Version != "v1alpha1" || current.Path != "spec" {
		return nil
	}
	key := nodeKey(current, "spec.selfInit.oidc.policyApproverRef")
	_, existed := oldNodes[key]
	field, exists := newNodes[key]
	if existed || !exists || field.Type != "object" || field.Required || field.Default != "" {
		return nil
	}
	expected := celRule{
		Rule: "!has(self.selfInit) || !has(self.selfInit.oidc) || !has(self.selfInit.oidc.policyApproverRef) || " +
			"(self.selfInit.enabled && self.selfInit.oidc.enabled && has(self.reconcilePolicies) && self.reconcilePolicies)",
		Message: "policyApproverRef requires selfInit, oidc, and reconcilePolicies enabled",
	}
	oldRules, newRules := celRuleSet(oldNode.CEL), celRuleSet(current.CEL)
	for rule := range celRuleSet([]celRule{expected}) {
		if newRules[rule] && !oldRules[rule] {
			return &expected
		}
	}
	return nil
}
