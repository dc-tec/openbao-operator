package config

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

type hclInitialize struct {
	Name string `hcl:"name,label"`
}

type hclInitializeRequest struct {
	Name string `hcl:"name,label"`

	Operation string `hcl:"operation"`
	Path      string `hcl:"path"`

	AllowFailure *bool `hcl:"allow_failure"`
}

type hclJWTAuthEnableData struct {
	Type        string `hcl:"type"`
	Description string `hcl:"description"`
}

type hclJWTConfigData struct {
	BoundIssuer          string   `hcl:"bound_issuer"`
	JWTValidationPubkeys []string `hcl:"jwt_validation_pubkeys"`
}

type hclPolicyData struct {
	Policy string `hcl:"policy"`
}

type hclJWTRoleData struct {
	RoleType       string             `hcl:"role_type"`
	UserClaim      string             `hcl:"user_claim"`
	BoundAudiences []string           `hcl:"bound_audiences"`
	BoundClaims    *map[string]string `hcl:"bound_claims,optional"`
	BoundSubject   *string            `hcl:"bound_subject,optional"`
	TokenPolicies  []string           `hcl:"token_policies"`
	Policies       *[]string          `hcl:"policies"`
	TTL            string             `hcl:"ttl"`
}

func buildInitializeBlock(label string) *hclwrite.Block {
	return gohcl.EncodeAsBlock(hclInitialize{Name: label}, "initialize")
}

func buildInitializeRequestBlock(label, operation, path string, allowFailure bool) *hclwrite.Block {
	req := hclInitializeRequest{
		Name:      label,
		Operation: operation,
		Path:      path,
	}
	if allowFailure {
		req.AllowFailure = boolPtrValue(true)
	}
	return gohcl.EncodeAsBlock(req, "request")
}

func buildOperatorBootstrapInitializeBlock(config OperatorBootstrapConfig) *hclwrite.Block {
	initBlock := buildInitializeBlock("operator-bootstrap")
	initBody := initBlock.Body()
	jwtAudiences := jwtAuthAudiences(config)

	// 1. Enable JWT Auth
	{
		req := buildInitializeRequestBlock("enable-jwt-auth", "update", "sys/auth/jwt", false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTAuthEnableData{
			Type:        "jwt",
			Description: "Auth method for OpenBao Operator",
		}, "data"))
		initBody.AppendBlock(req)
	}

	// 2. Configure OIDC
	{
		req := buildInitializeRequestBlock("config-jwt-auth", "update", "auth/jwt/config", false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTConfigData{
			BoundIssuer:          config.OIDCIssuerURL,
			JWTValidationPubkeys: config.JWTKeysPEM,
		}, "data"))
		initBody.AppendBlock(req)
	}

	// 3. Create Policy
	{
		req := buildInitializeRequestBlock("create-operator-policy", "update", "sys/policies/acl/openbao-operator", false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: jwtPolicyHealthStepDownSnapshot}, "data"))
		initBody.AppendBlock(req)
	}

	// 4. Bind Role
	{
		subject := fmt.Sprintf("system:serviceaccount:%s:%s", config.OperatorNS, config.OperatorSA)
		req := buildInitializeRequestBlock("create-operator-role", "update", "auth/jwt/role/openbao-operator", false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTRoleData{
			RoleType:       "jwt",
			UserClaim:      "sub",
			BoundAudiences: jwtAudiences,
			BoundSubject:   &subject,
			TokenPolicies:  []string{"openbao-operator"},
			TTL:            "1h",
		}, "data"))
		initBody.AppendBlock(req)
	}

	return initBlock
}

func buildSelfInitBootstrapInitializeBlock(cluster *openbaov1alpha1.OpenBaoCluster, config OperatorBootstrapConfig) *hclwrite.Block {
	initBlock := buildInitializeBlock("operator-bootstrap")
	initBody := initBlock.Body()
	jwtAudiences := jwtAuthAudiences(config)

	// 1. Enable JWT Auth
	{
		req := buildInitializeRequestBlock("enable-jwt-auth", "update", "sys/auth/jwt", false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTAuthEnableData{
			Type:        "jwt",
			Description: "Auth method for OpenBao Operator",
		}, "data"))
		initBody.AppendBlock(req)
	}

	// 2. Configure OIDC
	{
		req := buildInitializeRequestBlock("config-jwt-auth", "update", "auth/jwt/config", false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTConfigData{
			BoundIssuer:          config.OIDCIssuerURL,
			JWTValidationPubkeys: config.JWTKeysPEM,
		}, "data"))
		initBody.AppendBlock(req)
	}

	// 3. Create Policy
	{
		req := buildInitializeRequestBlock("create-operator-policy", "update", "sys/policies/acl/openbao-operator", false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: jwtPolicyHealthStepDownSnapshot}, "data"))
		initBody.AppendBlock(req)
	}

	// 4. Bind Role (+ policies mirror to match existing golden)
	{
		subject := fmt.Sprintf("system:serviceaccount:%s:%s", config.OperatorNS, config.OperatorSA)
		policies := []string{"openbao-operator"}
		req := buildInitializeRequestBlock("create-operator-role", "update", "auth/jwt/role/openbao-operator", false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTRoleData{
			RoleType:       "jwt",
			UserClaim:      "sub",
			BoundAudiences: jwtAudiences,
			BoundSubject:   &subject,
			TokenPolicies:  []string{"openbao-operator"},
			Policies:       &policies,
			TTL:            "1h",
		}, "data"))
		initBody.AppendBlock(req)
	}

	// 5. Auto-create backup policy and role if backup is configured with JWT Auth (opt-in)
	if cluster.Spec.Backup != nil && cluster.Spec.Backup.JWTAuthRole != "" {
		{
			req := buildInitializeRequestBlock("create-backup-policy", "update", "sys/policies/acl/backup", false)
			req.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: `path "sys/storage/raft/snapshot" { capabilities = ["read"] }`}, "data"))
			initBody.AppendBlock(req)
		}
		{
			subject := fmt.Sprintf("system:serviceaccount:%s:%s-backup-serviceaccount", cluster.Namespace, cluster.Name)
			policies := []string{"backup"}
			req := buildInitializeRequestBlock("create-backup-jwt-role", "update", "auth/jwt/role/backup", false)
			req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTRoleData{
				RoleType:       "jwt",
				UserClaim:      "sub",
				BoundAudiences: jwtAudiences,
				BoundSubject:   &subject,
				TokenPolicies:  []string{"backup"},
				Policies:       &policies,
				TTL:            "1h",
			}, "data"))
			initBody.AppendBlock(req)
		}
	}

	// 6. Auto-create upgrade policy and role if upgrade is configured with JWT Auth (opt-in)
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.JWTAuthRole != "" {
		{
			req := buildInitializeRequestBlock("create-upgrade-policy", "update", "sys/policies/acl/upgrade", false)
			req.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: upgradePolicyForCluster(cluster)}, "data"))
			initBody.AppendBlock(req)
		}
		{
			subject := fmt.Sprintf("system:serviceaccount:%s:%s-upgrade-serviceaccount", cluster.Namespace, cluster.Name)
			policies := []string{"upgrade"}
			req := buildInitializeRequestBlock("create-upgrade-jwt-role", "update", "auth/jwt/role/upgrade", false)
			req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTRoleData{
				RoleType:       "jwt",
				UserClaim:      "sub",
				BoundAudiences: jwtAudiences,
				BoundSubject:   &subject,
				TokenPolicies:  []string{"upgrade"},
				Policies:       &policies,
				TTL:            "1h",
			}, "data"))
			initBody.AppendBlock(req)
		}
	}

	// 7. Auto-create restore policy and role if restore JWT Auth is configured (opt-in via bootstrap)
	if cluster.Spec.Restore != nil && strings.TrimSpace(cluster.Spec.Restore.JWTAuthRole) != "" {
		{
			req := buildInitializeRequestBlock("create-restore-policy", "update", "sys/policies/acl/restore", false)
			req.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: `path "sys/storage/raft/snapshot-force" { capabilities = ["update"] }`}, "data"))
			initBody.AppendBlock(req)
		}
		{
			roleName := strings.TrimSpace(cluster.Spec.Restore.JWTAuthRole)
			subject := fmt.Sprintf("system:serviceaccount:%s:%s", cluster.Namespace, cluster.Name+constants.SuffixRestoreServiceAccount)
			policies := []string{"restore"}
			req := buildInitializeRequestBlock("create-restore-jwt-role", "update", fmt.Sprintf("auth/jwt/role/%s", roleName), false)
			req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTRoleData{
				RoleType:       "jwt",
				UserClaim:      "sub",
				BoundAudiences: jwtAudiences,
				BoundSubject:   &subject,
				TokenPolicies:  []string{"restore"},
				Policies:       &policies,
				TTL:            "1h",
			}, "data"))
			initBody.AppendBlock(req)
		}
	}

	return initBlock
}

func jwtAuthAudiences(config OperatorBootstrapConfig) []string {
	audience := strings.TrimSpace(config.JWTAuthAudience)
	if audience == "" {
		audience = constants.TokenAudienceOpenBaoInternal
	}
	return []string{audience}
}
