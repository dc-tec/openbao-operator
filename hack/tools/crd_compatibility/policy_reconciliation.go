package main

// isNewPolicyOptInValidation recognizes only the two guards for the newly
// introduced optional policy-reconciliation field. Neither guard rejects an
// object expressible by the old schema. Once that field exists in a released
// baseline, adding these guards must require review like other CEL tightening.
func isNewPolicyOptInValidation(oldNode, current schemaNode, oldNodes, newNodes map[string]schemaNode) bool {
	if current.CRD != "openbaoclusters.openbao.org" || current.Kind != "OpenBaoCluster" || current.Version != "v1alpha1" {
		return false
	}
	fieldKey := nodeKey(current, "spec.selfInit.oidc.reconcilePolicies")
	if _, existed := oldNodes[fieldKey]; existed {
		return false
	}
	field, exists := newNodes[fieldKey]
	if !exists || field.Type != "boolean" || field.Required || (field.Default != "" && field.Default != "false") {
		return false
	}
	var expected celRule
	switch current.Path {
	case "spec.selfInit":
		expected = celRule{
			Rule:    "!has(self.oidc) || !has(self.oidc.reconcilePolicies) || !self.oidc.reconcilePolicies || self.enabled",
			Message: "policy reconciliation requires selfInit.enabled=true",
		}
	case "spec.selfInit.oidc":
		expected = celRule{
			Rule:    "!has(self.reconcilePolicies) || !self.reconcilePolicies || self.enabled",
			Message: "policy reconciliation requires oidc.enabled=true",
		}
	default:
		return false
	}
	oldRules, newRules := celRuleSet(oldNode.CEL), celRuleSet(current.CEL)
	added, removed := setDifference(newRules, oldRules), setDifference(oldRules, newRules)
	return len(added) == 1 && len(removed) == 0 && celRuleSet([]celRule{expected})[added[0]]
}
