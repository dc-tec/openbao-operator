package main

import "testing"

func TestNewApproverOptInValidation(t *testing.T) {
	for _, scenario := range []string{
		"new approver", "existing approver", "required approver",
		"default approver", "additional rule", "changed rule",
	} {
		t.Run(scenario, func(t *testing.T) {
			old := schemaNode{
				CRD: "openbaoclusters.openbao.org", Kind: "OpenBaoCluster", Version: "v1alpha1",
				Path: "spec", Type: "object",
			}
			current := old
			current.CEL = []celRule{{
				Rule: "!has(self.selfInit) || !has(self.selfInit.oidc) || !has(self.selfInit.oidc.policyApproverRef) || " +
					"(self.selfInit.enabled && self.selfInit.oidc.enabled && has(self.reconcilePolicies) && self.reconcilePolicies)",
				Message: "policyApproverRef requires selfInit, oidc, and reconcilePolicies enabled",
			}}
			approver, reconciliation := old, old
			approver.Path = "spec.selfInit.oidc.policyApproverRef"
			reconciliation.Path, reconciliation.Type = "spec.reconcilePolicies", "boolean"
			oldNodes := []schemaNode{old, reconciliation}
			switch scenario {
			case "existing approver":
				oldNodes = append(oldNodes, approver)
			case "required approver":
				approver.Required = true
			case "default approver":
				approver.Default = "{}"
			case "additional rule":
				current.CEL = append(current.CEL, celRule{Rule: "self.enabled"})
			case "changed rule":
				oldNodes[0].CEL = []celRule{{Rule: "self.enabled"}}
				current.CEL = append(current.CEL, celRule{Rule: "!self.enabled"})
			}
			changes := compareSnapshots(oldNodes, []schemaNode{current, approver, reconciliation})
			wantBlocking := scenario != "new approver"
			if hasBlockingChanges(changes) != wantBlocking {
				t.Fatalf("unexpected compatibility decision: %#v", changes)
			}
		})
	}
}
