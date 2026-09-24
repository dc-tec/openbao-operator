package config

import (
	"fmt"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

// OperatorPolicyBundleRevision identifies immutable approval bundles shipped in the optional chart.
// Advance it when built-in policy contents change; retain earlier bundles for transition preconditions.
const OperatorPolicyBundleRevision = "v1"

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
		body.SetAttributeValue("capabilities", cty.ListVal([]cty.Value{cty.StringVal("read"), cty.StringVal("update")}))
		// required_parameters would also block parameterless reads. The policy
		// endpoint rejects missing contents; allowed_parameters restricts writes.
		body.SetAttributeValue("allowed_parameters", cty.ObjectVal(map[string]cty.Value{
			"policy": cty.ListVal([]cty.Value{cty.StringVal(policy.Policy)}),
		}))
	}
	return string(file.Bytes())
}

func validatePolicyApprover(cluster *openbaov1alpha1.OpenBaoCluster, bootstrap *OperatorBootstrapConfig) error {
	if cluster.Spec.SelfInit == nil || cluster.Spec.SelfInit.OIDC == nil ||
		cluster.Spec.SelfInit.OIDC.PolicyApproverRef == nil {
		return nil
	}
	ref := cluster.Spec.SelfInit.OIDC.PolicyApproverRef
	if !portauth.PolicyReconciliationEnabled(cluster) || !portauth.OperatorJWTBootstrapEnabled(cluster) ||
		ref.Namespace == "" || ref.Name == "" {
		return fmt.Errorf("policyApproverRef requires policy reconciliation, OIDC bootstrap, and a namespace and ServiceAccount name")
	}
	if ref.Namespace == cluster.Namespace {
		return fmt.Errorf("policyApproverRef must use an administration namespace outside the managed cluster namespace")
	}
	if bootstrap != nil && ref.Namespace == bootstrap.OperatorNS && ref.Name == bootstrap.OperatorSA {
		return fmt.Errorf("policyApproverRef must not reference the controller ServiceAccount")
	}
	return nil
}

func appendPolicyApprover(body *hclwrite.Body, cluster *openbaov1alpha1.OpenBaoCluster) {
	if cluster.Spec.SelfInit == nil || cluster.Spec.SelfInit.OIDC == nil {
		return
	}
	ref := cluster.Spec.SelfInit.OIDC.PolicyApproverRef
	if ref == nil {
		return
	}
	policy := buildInitializeRequestBlock("create-policy-approver", opUpdate,
		pathSysPoliciesACLPrefix+portauth.PolicyApproverName, false)
	policy.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: fmt.Sprintf(
		"path %q {\n  capabilities = [\"read\", \"update\"]\n}\n",
		pathSysPoliciesACLPrefix+portauth.PolicyNameApproval)}, "data"))
	body.AppendBlock(policy)
	role := buildInitializeRequestBlock("bind-policy-approver", opUpdate,
		pathAuthJWTRolePrefix+portauth.PolicyApproverName, false)
	subject, ttl := fmt.Sprintf("system:serviceaccount:%s:%s", ref.Namespace, ref.Name), "5m"
	role.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTRoleData{
		RoleType: authMethodJWT, UserClaim: "sub", BoundSubject: &subject,
		BoundAudiences: []string{portauth.PolicyApproverAudience(cluster)},
		TokenPolicies:  []string{portauth.PolicyApproverName}, TokenNoDefaultPolicy: true,
		TTL: ttl, TokenTTL: ttl, TokenMaxTTL: ttl, TokenExplicitMaxTTL: &ttl,
		ClockSkewLeeway: operatorJWTLeeway, ExpirationLeeway: operatorJWTLeeway, NotBeforeLeeway: operatorJWTLeeway,
	}, "data"))
	body.AppendBlock(role)
}
