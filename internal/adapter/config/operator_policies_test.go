package config

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/require"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

func TestOperatorPolicyApproval(t *testing.T) {
	for _, strategy := range []openbaov1alpha1.UpdateStrategyType{"RollingUpdate", "BlueGreen"} {
		t.Run(string(strategy), func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{}
			cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{Strategy: strategy, JWTAuthRole: "custom-upgrade-role"}
			cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{JWTAuthRole: "custom-backup-role"}
			file, diagnostics := hclsyntax.ParseConfig([]byte(OperatorPolicyApproval(cluster)), "approval.hcl", hcl.InitialPos)
			require.False(t, diagnostics.HasErrors(), diagnostics.Error())
			var approval struct {
				Paths []struct {
					Name         string              `hcl:"name,label"`
					Capabilities []string            `hcl:"capabilities"`
					Required     []string            `hcl:"required_parameters,optional"`
					Allowed      map[string][]string `hcl:"allowed_parameters"`
				} `hcl:"path,block"`
			}
			diagnostics = gohcl.DecodeBody(file.Body, nil, &approval)
			require.False(t, diagnostics.HasErrors(), diagnostics.Error())
			require.Len(t, approval.Paths, 4)
			for i, policy := range OperatorPolicies(cluster) {
				rule := approval.Paths[i]
				require.Equal(t, "sys/policies/acl/"+policy.Name, rule.Name)
				require.NotEqual(t, portauth.PolicyNameApproval, policy.Name)
				require.NotContains(t, rule.Name, "custom-")
				require.Equal(t, []string{"read", "update"}, rule.Capabilities)
				require.Empty(t, rule.Required, "parameterless policy reads must be allowed")
				require.Equal(t, map[string][]string{"policy": {policy.Policy}}, rule.Allowed)
			}
		})
	}
}

func TestPolicyApprovalBootstrapIsOptIn(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Spec.ReconcilePolicies = true
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: true, OIDC: &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true}}
	for _, enabled := range []bool{false, true} {
		cluster.Spec.ReconcilePolicies = enabled
		block := buildSelfInitBootstrapInitializeBlock(cluster, OperatorBootstrapConfig{OperatorNS: "operator", OperatorSA: "controller"})
		approvalRequests := 0
		for _, request := range block.Body().Blocks() {
			if request.Labels()[0] == "create-policy-approval" {
				approvalRequests++
			}
			for _, data := range request.Body().Blocks() {
				if attr := data.Body().GetAttribute("token_policies"); attr != nil {
					policies := string(attr.Expr().BuildTokens(nil).Bytes())
					if enabled && request.Labels()[0] == reqCreateOperatorRole {
						require.Contains(t, policies, portauth.PolicyNameApproval)
					} else {
						require.NotContains(t, policies, portauth.PolicyNameApproval)
					}
				}
			}
		}
		if enabled {
			require.Equal(t, 1, approvalRequests)
		} else {
			require.Zero(t, approvalRequests)
		}
	}
}
