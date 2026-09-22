package main

import "testing"

func TestNewPolicyOptInValidation(t *testing.T) {
	rules := map[string]celRule{
		"spec.selfInit": {
			Rule:    "!has(self.oidc) || !has(self.oidc.reconcilePolicies) || !self.oidc.reconcilePolicies || self.enabled",
			Message: "policy reconciliation requires selfInit.enabled=true",
		},
		"spec.selfInit.oidc": {
			Rule:    "!has(self.reconcilePolicies) || !self.reconcilePolicies || self.enabled",
			Message: "policy reconciliation requires oidc.enabled=true",
		},
	}
	for path, rule := range rules {
		for _, scenario := range []string{
			"new opt-in", "existing field", "different rule", "different metadata", "additional rule", "enabled by default",
		} {
			t.Run(path+"/"+scenario, func(t *testing.T) {
				old := schemaNode{
					CRD: "openbaoclusters.openbao.org", Kind: "OpenBaoCluster", Version: "v1alpha1", Path: path, Type: "object",
				}
				current := old
				current.CEL = []celRule{rule}
				field := old
				field.Path, field.Type = "spec.selfInit.oidc.reconcilePolicies", "boolean"
				oldNodes := []schemaNode{old}
				switch scenario {
				case "existing field":
					oldNodes = append(oldNodes, field)
				case "different rule":
					current.CEL[0].Rule = "self.enabled"
				case "different metadata":
					current.CEL[0].OptionalOldSelf = trueValue
				case "additional rule":
					current.CEL = append(current.CEL, celRule{Rule: "self.enabled"})
				case "enabled by default":
					field.Default = trueValue
				}
				changes := compareSnapshots(oldNodes, []schemaNode{current, field})
				if hasBlockingChanges(changes) != (scenario != "new opt-in") {
					t.Fatalf("unexpected compatibility decision: %#v", changes)
				}
			})
		}
	}
}
