package config

import (
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

// OperatorPolicy is a built-in ACL policy with a fixed name and contents.
type OperatorPolicy struct {
	Name   string `json:"name"`
	Policy string `json:"policy"`
}

// OperatorPolicies returns the policies generated during JWT bootstrap.
// Configurable JWT role names never become policy-write targets.
func OperatorPolicies(cluster *openbaov1alpha1.OpenBaoCluster) []OperatorPolicy {
	policies := []OperatorPolicy{
		{Name: portauth.PolicyNameOperator, Policy: jwtPolicyHealthStepDownAutopilot},
		{Name: portauth.PolicyNameUpgrade, Policy: upgradePolicyForCluster(cluster)},
		{Name: portauth.PolicyNameRestore, Policy: jwtPolicyRestoreSnapshot},
	}
	if cluster.Spec.Backup != nil {
		policies = append(policies, OperatorPolicy{Name: portauth.PolicyNameBackup, Policy: jwtPolicyBackupSnapshot})
	}
	return policies
}

// OperatorPolicyApproval renders an ACL that permits only the exact policy
// contents for this cluster. Runtime reconciliation must never write this ACL.
func OperatorPolicyApproval(cluster *openbaov1alpha1.OpenBaoCluster) string {
	file := hclwrite.NewEmptyFile()
	for _, policy := range OperatorPolicies(cluster) {
		body := file.Body().AppendNewBlock("path", []string{pathSysPoliciesACLPrefix + policy.Name}).Body()
		body.SetAttributeValue("capabilities", cty.ListVal([]cty.Value{cty.StringVal("update")}))
		body.SetAttributeValue("required_parameters", cty.ListVal([]cty.Value{cty.StringVal("policy")}))
		body.SetAttributeValue("allowed_parameters", cty.ObjectVal(map[string]cty.Value{
			"policy": cty.ListVal([]cty.Value{cty.StringVal(policy.Policy)}),
		}))
	}
	return string(file.Bytes())
}
