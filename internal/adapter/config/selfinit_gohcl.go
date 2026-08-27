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
	BoundIssuer          string    `hcl:"bound_issuer"`
	OIDCDiscoveryURL     *string   `hcl:"oidc_discovery_url,optional"`
	OIDCDiscoveryCAPEM   *string   `hcl:"oidc_discovery_ca_pem,optional"`
	JWKSURL              *string   `hcl:"jwks_url,optional"`
	JWKSCAPEM            *string   `hcl:"jwks_ca_pem,optional"`
	JWTValidationPubkeys *[]string `hcl:"jwt_validation_pubkeys,optional"`
}

type hclPolicyData struct {
	Policy string `hcl:"policy"`
}

type hclInitialRecoveryKeysData struct {
	SecretShares    int32    `hcl:"secret_shares"`
	SecretThreshold int32    `hcl:"secret_threshold"`
	Backup          bool     `hcl:"backup"`
	PGPKeys         []string `hcl:"pgp_keys"`
}

type hclJWTRoleData struct {
	RoleType             string             `hcl:"role_type"`
	UserClaim            string             `hcl:"user_claim"`
	BoundAudiences       []string           `hcl:"bound_audiences"`
	BoundClaims          *map[string]string `hcl:"bound_claims,optional"`
	BoundSubject         *string            `hcl:"bound_subject,optional"`
	TokenPolicies        []string           `hcl:"token_policies"`
	Policies             *[]string          `hcl:"policies"`
	TTL                  string             `hcl:"ttl"`
	TokenTTL             string             `hcl:"token_ttl"`
	TokenMaxTTL          string             `hcl:"token_max_ttl"`
	TokenNoDefaultPolicy bool               `hcl:"token_no_default_policy"`
	ClockSkewLeeway      string             `hcl:"clock_skew_leeway"`
	ExpirationLeeway     string             `hcl:"expiration_leeway"`
	NotBeforeLeeway      string             `hcl:"not_before_leeway"`
}

const (
	operatorJWTTokenTTL = "1h"
	operatorJWTLeeway   = "30s"
)

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

func buildSelfInitInitialRecoveryKeysBlock(config *openbaov1alpha1.InitialRecoveryKeysConfig) *hclwrite.Block {
	initBlock := buildInitializeBlock("initial-recovery-keys")
	req := buildInitializeRequestBlock(reqInitialRecoveryKeys, opUpdate, pathSysRotateRecoveryInit, false)

	pgpKeys := make([]string, 0, len(config.Recipients))
	for _, recipient := range config.Recipients {
		pgpKeys = append(pgpKeys, strings.TrimSpace(recipient.PGPPublicKey))
	}

	req.Body().AppendBlock(gohcl.EncodeAsBlock(hclInitialRecoveryKeysData{
		SecretShares:    config.Shares,
		SecretThreshold: config.Threshold,
		Backup:          true,
		PGPKeys:         pgpKeys,
	}, "data"))
	initBlock.Body().AppendBlock(req)
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
		req.Body().AppendBlock(gohcl.EncodeAsBlock(jwtConfigData(config), "data"))
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
		req := buildInitializeRequestBlock(reqCreateOperatorRole, opUpdate, fmt.Sprintf("%s%s", pathAuthJWTRolePrefix, authRoleNameOperator), false)
		req.Body().AppendBlock(gohcl.EncodeAsBlock(operatorJWTRoleData(subject, authPolicyNameOperator, jwtAudiences), "data"))
		initBody.AppendBlock(req)
	}

	// 5. Auto-create backup policy and role if backup is configured (and OIDC enabled)
	if cluster.Spec.Backup != nil {
		roleName := portauth.EffectiveJWTRole(cluster.Spec.Backup.JWTAuthRole, bootstrapEnabled, authRoleNameBackup)
		// Only create if we have a role name (either explicit or defaulted)
		if roleName != "" {
			{
				req := buildInitializeRequestBlock(reqCreateBackupPolicy, opUpdate, fmt.Sprintf("%s%s", pathSysPoliciesACLPrefix, authPolicyNameBackup), false)
				req.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: jwtPolicyBackupSnapshot}, "data"))
				initBody.AppendBlock(req)
			}
			{
				subject := fmt.Sprintf("system:serviceaccount:%s:%s-backup-serviceaccount", cluster.Namespace, cluster.Name)
				req := buildInitializeRequestBlock(reqCreateBackupRole, opUpdate, fmt.Sprintf("%s%s", pathAuthJWTRolePrefix, roleName), false)
				req.Body().AppendBlock(gohcl.EncodeAsBlock(operatorJWTRoleData(subject, authPolicyNameBackup, jwtAudiences), "data"))
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
			req := buildInitializeRequestBlock(reqCreateUpgradeRole, opUpdate, fmt.Sprintf("%s%s", pathAuthJWTRolePrefix, roleName), false)
			req.Body().AppendBlock(gohcl.EncodeAsBlock(operatorJWTRoleData(subject, authPolicyNameUpgrade, jwtAudiences), "data"))
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
			req.Body().AppendBlock(gohcl.EncodeAsBlock(hclPolicyData{Policy: jwtPolicyRestoreSnapshot}, "data"))
			initBody.AppendBlock(req)
		}
		{
			subject := fmt.Sprintf("system:serviceaccount:%s:%s-restore-serviceaccount", cluster.Namespace, cluster.Name)
			req := buildInitializeRequestBlock(reqCreateRestoreRole, opUpdate, fmt.Sprintf("%s%s", pathAuthJWTRolePrefix, restoreRoleName), false)
			req.Body().AppendBlock(gohcl.EncodeAsBlock(operatorJWTRoleData(subject, authPolicyNameRestore, jwtAudiences), "data"))
			initBody.AppendBlock(req)
		}
	}

	return initBlock
}

func operatorJWTRoleData(subject, policy string, audiences []string) hclJWTRoleData {
	policies := []string{policy}
	return hclJWTRoleData{
		RoleType:             authMethodJWT,
		UserClaim:            "sub",
		BoundAudiences:       audiences,
		BoundSubject:         &subject,
		TokenPolicies:        []string{policy},
		Policies:             &policies,
		TTL:                  operatorJWTTokenTTL,
		TokenTTL:             operatorJWTTokenTTL,
		TokenMaxTTL:          operatorJWTTokenTTL,
		TokenNoDefaultPolicy: true,
		ClockSkewLeeway:      operatorJWTLeeway,
		ExpirationLeeway:     operatorJWTLeeway,
		NotBeforeLeeway:      operatorJWTLeeway,
	}
}

func jwtAuthAudiences(config OperatorBootstrapConfig) []string {
	audience := strings.TrimSpace(config.JWTAuthAudience)
	if audience == "" {
		audience = portauth.TokenAudienceOpenBaoInternal
	}
	return []string{audience}
}

func jwtConfigData(config OperatorBootstrapConfig) hclJWTConfigData {
	data := hclJWTConfigData{
		BoundIssuer: config.OIDCIssuerURL,
	}

	if discoveryURL := strings.TrimSpace(config.OIDCDiscoveryURL); discoveryURL != "" && (shouldUseDynamicOIDCDiscovery(config) || len(config.JWTKeysPEM) == 0) {
		data.OIDCDiscoveryURL = &discoveryURL
		if discoveryCAPEM := strings.TrimSpace(config.OIDCDiscoveryCAPEM); discoveryCAPEM != "" {
			data.OIDCDiscoveryCAPEM = &discoveryCAPEM
		}
		return data
	}

	if jwksURL := strings.TrimSpace(config.OIDCJWKSURL); jwksURL != "" && (shouldUseDynamicJWKS(config) || len(config.JWTKeysPEM) == 0) {
		data.JWKSURL = &jwksURL
		if jwksCAPEM := strings.TrimSpace(config.OIDCJWKSCAPEM); jwksCAPEM != "" {
			data.JWKSCAPEM = &jwksCAPEM
		}
		return data
	}

	if len(config.JWTKeysPEM) > 0 {
		keys := append([]string(nil), config.JWTKeysPEM...)
		data.JWTValidationPubkeys = &keys
	}

	return data
}
