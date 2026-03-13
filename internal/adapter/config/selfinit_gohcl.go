package config

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
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
		req := buildInitializeRequestBlock(reqEnableJWTAuth, opUpdate, pathSysAuthJWT, false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTAuthEnableData{
			Type:        authMethodJWT,
			Description: authDesc,
		}, "data"))
		initBody.AppendBlock(req)
	}

	// 2. Configure OIDC
	{
		req := buildInitializeRequestBlock(reqConfigJWTAuth, opUpdate, pathAuthJWTConfig, false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTConfigData{
			BoundIssuer:          config.OIDCIssuerURL,
			JWTValidationPubkeys: config.JWTKeysPEM,
		}, "data"))
		initBody.AppendBlock(req)
	}

	// 3. Create Policy
	{
		req := buildInitializeRequestBlock(reqCreateOperatorPolicy, opUpdate, fmt.Sprintf("%s%s", pathSysPoliciesACLPrefix, authPolicyNameOperator), false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: jwtPolicyHealthStepDownAutopilot}, "data"))
		initBody.AppendBlock(req)
	}

	// 4. Bind Role
	{
		subject := fmt.Sprintf("system:serviceaccount:%s:%s", config.OperatorNS, config.OperatorSA)
		req := buildInitializeRequestBlock(reqCreateOperatorRole, opUpdate, fmt.Sprintf("%s%s", pathAuthJWTRolePrefix, authRoleNameOperator), false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTRoleData{
			RoleType:       authMethodJWT,
			UserClaim:      "sub",
			BoundAudiences: jwtAudiences,
			BoundSubject:   &subject,
			TokenPolicies:  []string{authPolicyNameOperator},
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
	bootstrapEnabled := portauth.OperatorJWTBootstrapEnabled(cluster)

	// 1. Enable JWT Auth
	{
		req := buildInitializeRequestBlock(reqEnableJWTAuth, opUpdate, pathSysAuthJWT, false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTAuthEnableData{
			Type:        authMethodJWT,
			Description: authDesc,
		}, "data"))
		initBody.AppendBlock(req)
	}

	// 2. Configure OIDC
	{
		req := buildInitializeRequestBlock(reqConfigJWTAuth, opUpdate, pathAuthJWTConfig, false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTConfigData{
			BoundIssuer:          config.OIDCIssuerURL,
			JWTValidationPubkeys: config.JWTKeysPEM,
		}, "data"))
		initBody.AppendBlock(req)
	}

	// 3. Create Policy
	{
		req := buildInitializeRequestBlock(reqCreateOperatorPolicy, opUpdate, fmt.Sprintf("%s%s", pathSysPoliciesACLPrefix, authPolicyNameOperator), false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: jwtPolicyHealthStepDownAutopilot}, "data"))
		initBody.AppendBlock(req)
	}

	// 4. Bind Role (+ policies mirror to match existing golden)
	{
		subject := fmt.Sprintf("system:serviceaccount:%s:%s", config.OperatorNS, config.OperatorSA)
		policies := []string{authPolicyNameOperator}
		req := buildInitializeRequestBlock(reqCreateOperatorRole, opUpdate, fmt.Sprintf("%s%s", pathAuthJWTRolePrefix, authRoleNameOperator), false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTRoleData{
			RoleType:       authMethodJWT,
			UserClaim:      "sub",
			BoundAudiences: jwtAudiences,
			BoundSubject:   &subject,
			TokenPolicies:  []string{authPolicyNameOperator},
			Policies:       &policies,
			TTL:            "1h",
		}, "data"))
		initBody.AppendBlock(req)
	}

	// 5. Auto-create backup policy and role if backup is configured (and OIDC enabled)
	if cluster.Spec.Backup != nil {
		roleName := portauth.EffectiveJWTRole(cluster.Spec.Backup.JWTAuthRole, bootstrapEnabled, authRoleNameBackup)
		// Only create if we have a role name (either explicit or defaulted)
		if roleName != "" {
			{
				req := buildInitializeRequestBlock(reqCreateBackupPolicy, opUpdate, fmt.Sprintf("%s%s", pathSysPoliciesACLPrefix, authPolicyNameBackup), false)
				req.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: `path "sys/storage/raft/snapshot" { capabilities = ["read"] }`}, "data"))
				initBody.AppendBlock(req)
			}
			{
				subject := fmt.Sprintf("system:serviceaccount:%s:%s-backup-serviceaccount", cluster.Namespace, cluster.Name)
				policies := []string{authPolicyNameBackup}
				req := buildInitializeRequestBlock(reqCreateBackupRole, opUpdate, fmt.Sprintf("%s%s", pathAuthJWTRolePrefix, roleName), false)
				req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTRoleData{
					RoleType:       authMethodJWT,
					UserClaim:      "sub",
					BoundAudiences: jwtAudiences,
					BoundSubject:   &subject,
					TokenPolicies:  []string{authPolicyNameBackup},
					Policies:       &policies,
					TTL:            "1h",
				}, "data"))
				initBody.AppendBlock(req)
			}
		}
	}

	// 6. Auto-create upgrade policy and role if OIDC is enabled (or if upgrade is explicitly configured)
	// Upgrade is auto-created when OIDC is enabled to support upgrade operations
	roleName := ""
	if cluster.Spec.Upgrade != nil {
		roleName = cluster.Spec.Upgrade.JWTAuthRole
	}
	roleName = portauth.EffectiveJWTRole(roleName, bootstrapEnabled, authRoleNameUpgrade)
	if roleName != "" {
		{
			req := buildInitializeRequestBlock(reqCreateUpgradePolicy, opUpdate, fmt.Sprintf("%s%s", pathSysPoliciesACLPrefix, authPolicyNameUpgrade), false)
			req.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: upgradePolicyForCluster(cluster)}, "data"))
			initBody.AppendBlock(req)
		}
		{
			subject := fmt.Sprintf("system:serviceaccount:%s:%s-upgrade-serviceaccount", cluster.Namespace, cluster.Name)
			policies := []string{authPolicyNameUpgrade}
			req := buildInitializeRequestBlock(reqCreateUpgradeRole, opUpdate, fmt.Sprintf("%s%s", pathAuthJWTRolePrefix, roleName), false)
			req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTRoleData{
				RoleType:       authMethodJWT,
				UserClaim:      "sub",
				BoundAudiences: jwtAudiences,
				BoundSubject:   &subject,
				TokenPolicies:  []string{authPolicyNameUpgrade},
				Policies:       &policies,
				TTL:            "1h",
			}, "data"))
			initBody.AppendBlock(req)
		}
	}

	// 7. Auto-create restore policy and role if OIDC is enabled (or if restore is explicitly configured)
	// Restore is auto-created when OIDC is enabled to support disaster recovery scenarios
	restoreRoleName := ""
	if cluster.Spec.Restore != nil {
		restoreRoleName = strings.TrimSpace(cluster.Spec.Restore.JWTAuthRole)
	}
	restoreRoleName = portauth.EffectiveJWTRole(restoreRoleName, bootstrapEnabled, authRoleNameRestore)
	if restoreRoleName != "" {
		{
			req := buildInitializeRequestBlock(reqCreateRestorePolicy, opUpdate, fmt.Sprintf("%s%s", pathSysPoliciesACLPrefix, authPolicyNameRestore), false)
			req.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: `path "sys/storage/raft/snapshot-force" { capabilities = ["update"] }`}, "data"))
			initBody.AppendBlock(req)
		}
		{
			subject := fmt.Sprintf("system:serviceaccount:%s:%s-restore-serviceaccount", cluster.Namespace, cluster.Name)
			policies := []string{authPolicyNameRestore}
			req := buildInitializeRequestBlock(reqCreateRestoreRole, opUpdate, fmt.Sprintf("%s%s", pathAuthJWTRolePrefix, restoreRoleName), false)
			req.Body().AppendBlock(gohcl.EncodeAsBlock(hclJWTRoleData{
				RoleType:       authMethodJWT,
				UserClaim:      "sub",
				BoundAudiences: jwtAudiences,
				BoundSubject:   &subject,
				TokenPolicies:  []string{authPolicyNameRestore},
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
		audience = portauth.TokenAudienceOpenBaoInternal
	}
	return []string{audience}
}
