package config

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/constants"
	"github.com/zclconf/go-cty/cty"
)

const (
	configPluginDirectoryPath = "/openbao/plugins"
	configUnsealKeyPath       = "file:///etc/bao/unseal/key"
	configUnsealKeyID         = "operator-generated-v1"
	configMaxRequestDuration  = "90s"
	configNodeIDTemplate      = "${HOSTNAME}"

	jwtPolicyHealthStepDownSnapshot = `path "sys/health" { capabilities = ["read"] }
path "sys/step-down" { capabilities = ["update"] }
path "sys/storage/raft/snapshot" { capabilities = ["read"] }`
)

// OperatorBootstrapConfig holds configuration for operator bootstrap.
type OperatorBootstrapConfig struct {
	OIDCIssuerURL string
	JWTKeysPEM    []string
	OperatorNS    string
	OperatorSA    string
}

// InfrastructureDetails captures the pieces of topology information required to
// render a complete config.hcl file.
type InfrastructureDetails struct {
	HeadlessServiceName string
	Namespace           string
	APIPort             int
	ClusterPort         int
}

// RenderHCL renders a complete OpenBao configuration using the provided cluster
// specification and infrastructure details.
//
// The generated configuration:
//   - Always includes operator-owned listener "tcp" and storage "raft" stanzas.
//   - Includes seal stanza based on spec.unseal (defaults to "static" if omitted).
//   - Uses a Kubernetes go-discover-based retry_join block for dynamic cluster membership.
//   - Merges user-provided spec.config entries as simple string attributes, excluding
//     protected keys that are operator-managed.
//   - Renders audit devices from spec.audit (if configured).
//   - Renders plugins from spec.plugins (if configured).
//   - Renders telemetry configuration from spec.telemetry (if configured).
func RenderHCL(cluster *openbaov1alpha1.OpenBaoCluster, infra InfrastructureDetails) ([]byte, error) {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	headlessSvcName := infra.HeadlessServiceName
	if strings.TrimSpace(headlessSvcName) == "" {
		headlessSvcName = cluster.Name
	}

	namespace := infra.Namespace
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("infrastructure namespace is required to render config.hcl")
	}

	// 1. General configuration.
	// UI defaults to true, but can be overridden by user configuration
	uiEnabled := true
	if cluster.Spec.Configuration != nil && cluster.Spec.Configuration.UI != nil {
		uiEnabled = *cluster.Spec.Configuration.UI
	}
	body.SetAttributeValue("ui", cty.BoolVal(uiEnabled))
	body.SetAttributeValue("cluster_name", cty.StringVal(cluster.Name))

	apiAddr := fmt.Sprintf("https://${HOSTNAME}.%s.%s.svc:%d", headlessSvcName, namespace, infra.APIPort)
	clusterAddr := fmt.Sprintf("https://${HOSTNAME}.%s.%s.svc:%d", headlessSvcName, namespace, infra.ClusterPort)

	body.SetAttributeValue("api_addr", cty.StringVal(apiAddr))
	body.SetAttributeValue("cluster_addr", cty.StringVal(clusterAddr))
	body.SetAttributeValue("plugin_directory", cty.StringVal(configPluginDirectoryPath))

	// 2. Listener stanza (operator-owned).
	listenerBlock := body.AppendNewBlock("listener", []string{"tcp"})
	listenerBody := listenerBlock.Body()
	// Enforce operator defaults (immutable)
	listenerBody.SetAttributeValue("address", cty.StringVal(fmt.Sprintf("0.0.0.0:%d", constants.PortAPI)))
	listenerBody.SetAttributeValue("cluster_address", cty.StringVal(fmt.Sprintf("0.0.0.0:%d", constants.PortCluster)))
	listenerBody.SetAttributeValue("tls_disable", cty.NumberIntVal(0))
	listenerBody.SetAttributeValue("max_request_duration", cty.StringVal(configMaxRequestDuration))

	// Apply user configuration (if present)
	renderListenerConfiguration(listenerBody, cluster.Spec.Configuration)

	// Render TLS configuration based on mode
	if err := renderTLSConfiguration(listenerBody, cluster.Spec.TLS, cluster.Spec.Configuration); err != nil {
		return nil, err
	}

	// 3. Seal stanza (operator-owned, but type is configurable).
	if err := renderSealStanza(body, cluster); err != nil {
		return nil, fmt.Errorf("failed to render seal stanza: %w", err)
	}

	// 4. Storage stanza (Raft) with auto_join for dynamic cluster membership.
	storageBlock := body.AppendNewBlock("storage", []string{"raft"})
	storageBody := storageBlock.Body()
	// Enforce operator defaults (immutable)
	storageBody.SetAttributeValue("path", cty.StringVal(constants.PathData))
	storageBody.SetAttributeValue("node_id", cty.StringVal(configNodeIDTemplate))

	// Apply user configuration (if present)
	if cluster.Spec.Configuration != nil && cluster.Spec.Configuration.Raft != nil {
		if cluster.Spec.Configuration.Raft.PerformanceMultiplier != nil {
			storageBody.SetAttributeValue("performance_multiplier", cty.NumberIntVal(int64(*cluster.Spec.Configuration.Raft.PerformanceMultiplier)))
		}
	}

	// Use Kubernetes go-discover for dynamic cluster membership. This allows pods
	// to discover and join the Raft cluster based on labels, without requiring
	// a static list of peer addresses.
	autoJoinExpr := fmt.Sprintf(
		`provider=k8s namespace=%s label_selector="%s=%s"`,
		namespace,
		constants.LabelOpenBaoCluster,
		cluster.Name,
	)

	retryJoinBlock := storageBody.AppendNewBlock("retry_join", nil)
	retryJoinBody := retryJoinBlock.Body()

	retryJoinBody.SetAttributeValue("auto_join", cty.StringVal(autoJoinExpr))
	// Use a common DNS name for TLS validation during auto-join.
	// Even though go-discover returns IP addresses, this allows TLS validation
	// to succeed using the common DNS SAN that's included in all certificates.
	// This avoids certificate validation failures when new pods join the cluster.
	commonTLSName := fmt.Sprintf("openbao-cluster-%s.local", cluster.Name)
	retryJoinBody.SetAttributeValue("leader_tls_servername", cty.StringVal(commonTLSName))

	// TLS certificate file paths are only needed for OperatorManaged and External modes.
	// In ACME mode, certificates are stored in-memory (or cached) and OpenBao handles
	// cluster TLS automatically using the same ACME-obtained certificates.
	// See: https://openbao.org/docs/rfcs/acme-tls-listeners/
	if cluster.Spec.TLS.Mode != openbaov1alpha1.TLSModeACME {
		retryJoinBody.SetAttributeValue("leader_ca_cert_file", cty.StringVal(constants.PathTLSCACert))
		retryJoinBody.SetAttributeValue("leader_client_cert_file", cty.StringVal(constants.PathTLSServerCert))
		retryJoinBody.SetAttributeValue("leader_client_key_file", cty.StringVal(constants.PathTLSServerKey))
	}

	// 5. Kubernetes service registration (operator-owned).
	// This causes OpenBao to label its own Pod with state such as:
	// - openbao-active (leader)
	// - openbao-initialized
	// - openbao-sealed
	// - openbao-version
	//
	// The operator consumes these labels to reduce reliance on direct OpenBao API access.
	body.AppendNewBlock("service_registration", []string{"kubernetes"})

	// 6. Apply additional user configuration from structured fields.
	renderUserConfiguration(body, cluster.Spec.Configuration)

	// 7. Render audit devices if configured
	if err := renderAuditDevices(body, cluster.Spec.Audit); err != nil {
		return nil, fmt.Errorf("failed to render audit devices: %w", err)
	}

	// 8. Render plugins if configured
	renderPlugins(body, cluster.Spec.Plugins)

	// 9. Render telemetry if configured
	renderTelemetry(body, cluster.Spec.Telemetry)

	// Note: Self-initialization stanzas are rendered separately via RenderSelfInitHCL
	// and stored in a separate ConfigMap that is only mounted for pod-0.

	return file.Bytes(), nil
}

// RenderOperatorBootstrapHCL renders the operator bootstrap initialize block.
// This block configures JWT auth in OpenBao for operator authentication.
func RenderOperatorBootstrapHCL(config OperatorBootstrapConfig) ([]byte, error) {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	if strings.TrimSpace(config.OIDCIssuerURL) == "" {
		return nil, fmt.Errorf("OIDC issuer URL is required to render operator bootstrap")
	}
	if len(config.JWTKeysPEM) == 0 {
		return nil, fmt.Errorf("at least one JWT public key is required to render operator bootstrap")
	}
	if strings.TrimSpace(config.OperatorNS) == "" {
		return nil, fmt.Errorf("operator namespace is required to render operator bootstrap")
	}
	if strings.TrimSpace(config.OperatorSA) == "" {
		return nil, fmt.Errorf("operator service account name is required to render operator bootstrap")
	}

	// Create initialize block
	initBlock := body.AppendNewBlock("initialize", []string{"operator-bootstrap"})
	initBody := initBlock.Body()

	// 1. Enable JWT Auth
	enableReq := initBody.AppendNewBlock("request", []string{"enable-jwt-auth"})
	enableReqBody := enableReq.Body()
	enableReqBody.SetAttributeValue("operation", cty.StringVal("update"))
	enableReqBody.SetAttributeValue("path", cty.StringVal("sys/auth/jwt"))
	enableReqData := enableReqBody.AppendNewBlock("data", nil)
	enableReqData.Body().SetAttributeValue("type", cty.StringVal("jwt"))
	enableReqData.Body().SetAttributeValue("description", cty.StringVal("Auth method for OpenBao Operator"))

	// 2. Configure OIDC
	configReq := initBody.AppendNewBlock("request", []string{"config-jwt-auth"})
	configReqBody := configReq.Body()
	configReqBody.SetAttributeValue("operation", cty.StringVal("update"))
	configReqBody.SetAttributeValue("path", cty.StringVal("auth/jwt/config"))
	configReqData := configReqBody.AppendNewBlock("data", nil)
	configReqData.Body().SetAttributeValue("bound_issuer", cty.StringVal(config.OIDCIssuerURL))
	jwtKeys := make([]cty.Value, 0, len(config.JWTKeysPEM))
	for _, k := range config.JWTKeysPEM {
		jwtKeys = append(jwtKeys, cty.StringVal(k))
	}
	configReqData.Body().SetAttributeValue("jwt_validation_pubkeys", cty.ListVal(jwtKeys))

	// 3. Create Policy
	policyReq := initBody.AppendNewBlock("request", []string{"create-operator-policy"})
	policyReqBody := policyReq.Body()
	policyReqBody.SetAttributeValue("operation", cty.StringVal("update"))
	policyReqBody.SetAttributeValue("path", cty.StringVal("sys/policies/acl/openbao-operator"))
	policyReqData := policyReqBody.AppendNewBlock("data", nil)
	policyReqData.Body().SetAttributeValue("policy", cty.StringVal(jwtPolicyHealthStepDownSnapshot))

	// 4. Bind Role
	roleReq := initBody.AppendNewBlock("request", []string{"create-operator-role"})
	roleReqBody := roleReq.Body()
	roleReqBody.SetAttributeValue("operation", cty.StringVal("update"))
	roleReqBody.SetAttributeValue("path", cty.StringVal("auth/jwt/role/openbao-operator"))
	roleReqData := roleReqBody.AppendNewBlock("data", nil)
	roleReqData.Body().SetAttributeValue("role_type", cty.StringVal("jwt"))
	roleReqData.Body().SetAttributeValue("user_claim", cty.StringVal("sub"))
	roleReqData.Body().SetAttributeValue("bound_audiences", cty.ListVal([]cty.Value{cty.StringVal("openbao-internal")}))

	// Bound claims
	boundClaims := map[string]cty.Value{
		"kubernetes.io/namespace":           cty.StringVal(config.OperatorNS),
		"kubernetes.io/serviceaccount/name": cty.StringVal(config.OperatorSA),
	}
	roleReqData.Body().SetAttributeValue("bound_claims", cty.ObjectVal(boundClaims))
	roleReqData.Body().SetAttributeValue("token_policies", cty.ListVal([]cty.Value{cty.StringVal("openbao-operator")}))
	roleReqData.Body().SetAttributeValue("ttl", cty.StringVal("1h"))

	return file.Bytes(), nil
}

// RenderSelfInitHCL renders only the self-initialization stanzas as a separate
// HCL configuration. This is stored in a separate ConfigMap that is only mounted
// for pod-0, since only the first pod needs to execute initialization requests.
// If bootstrapConfig is provided, it will be merged with user requests.
func RenderSelfInitHCL(cluster *openbaov1alpha1.OpenBaoCluster, bootstrapConfig *OperatorBootstrapConfig) ([]byte, error) {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	// If bootstrap config provided, render it first
	if bootstrapConfig != nil {
		if strings.TrimSpace(bootstrapConfig.OIDCIssuerURL) == "" {
			return nil, fmt.Errorf("OIDC issuer URL is required to render operator bootstrap")
		}
		if len(bootstrapConfig.JWTKeysPEM) == 0 {
			return nil, fmt.Errorf("at least one JWT public key is required to render operator bootstrap")
		}
		if strings.TrimSpace(bootstrapConfig.OperatorNS) == "" {
			return nil, fmt.Errorf("operator namespace is required to render operator bootstrap")
		}
		if strings.TrimSpace(bootstrapConfig.OperatorSA) == "" {
			return nil, fmt.Errorf("operator service account name is required to render operator bootstrap")
		}

		// Render bootstrap blocks directly into the body
		initBlock := body.AppendNewBlock("initialize", []string{"operator-bootstrap"})
		initBody := initBlock.Body()

		// 1. Enable JWT Auth
		enableReq := initBody.AppendNewBlock("request", []string{"enable-jwt-auth"})
		enableReqBody := enableReq.Body()
		enableReqBody.SetAttributeValue("operation", cty.StringVal("update"))
		enableReqBody.SetAttributeValue("path", cty.StringVal("sys/auth/jwt"))
		enableReqData := enableReqBody.AppendNewBlock("data", nil)
		enableReqData.Body().SetAttributeValue("type", cty.StringVal("jwt"))
		enableReqData.Body().SetAttributeValue("description", cty.StringVal("Auth method for OpenBao Operator"))

		// 2. Configure OIDC
		configReq := initBody.AppendNewBlock("request", []string{"config-jwt-auth"})
		configReqBody := configReq.Body()
		configReqBody.SetAttributeValue("operation", cty.StringVal("update"))
		configReqBody.SetAttributeValue("path", cty.StringVal("auth/jwt/config"))
		configReqData := configReqBody.AppendNewBlock("data", nil)
		configReqData.Body().SetAttributeValue("bound_issuer", cty.StringVal(bootstrapConfig.OIDCIssuerURL))
		jwtKeys := make([]cty.Value, 0, len(bootstrapConfig.JWTKeysPEM))
		for _, k := range bootstrapConfig.JWTKeysPEM {
			jwtKeys = append(jwtKeys, cty.StringVal(k))
		}
		configReqData.Body().SetAttributeValue("jwt_validation_pubkeys", cty.ListVal(jwtKeys))

		// 3. Create Policy
		policyReq := initBody.AppendNewBlock("request", []string{"create-operator-policy"})
		policyReqBody := policyReq.Body()
		policyReqBody.SetAttributeValue("operation", cty.StringVal("update"))
		policyReqBody.SetAttributeValue("path", cty.StringVal("sys/policies/acl/openbao-operator"))
		policyReqData := policyReqBody.AppendNewBlock("data", nil)
		policyReqData.Body().SetAttributeValue("policy", cty.StringVal(jwtPolicyHealthStepDownSnapshot))

		// 4. Bind Role
		roleReq := initBody.AppendNewBlock("request", []string{"create-operator-role"})
		roleReqBody := roleReq.Body()
		roleReqBody.SetAttributeValue("operation", cty.StringVal("update"))
		roleReqBody.SetAttributeValue("path", cty.StringVal("auth/jwt/role/openbao-operator"))
		roleReqData := roleReqBody.AppendNewBlock("data", nil)
		roleReqData.Body().SetAttributeValue("role_type", cty.StringVal("jwt"))
		roleReqData.Body().SetAttributeValue("user_claim", cty.StringVal("sub"))
		roleReqData.Body().SetAttributeValue("bound_audiences", cty.ListVal([]cty.Value{cty.StringVal("openbao-internal")}))

		// Bound claims
		boundClaims := map[string]cty.Value{
			"kubernetes.io/namespace":           cty.StringVal(bootstrapConfig.OperatorNS),
			"kubernetes.io/serviceaccount/name": cty.StringVal(bootstrapConfig.OperatorSA),
		}
		roleReqData.Body().SetAttributeValue("bound_claims", cty.ObjectVal(boundClaims))
		roleReqData.Body().SetAttributeValue("token_policies", cty.ListVal([]cty.Value{cty.StringVal("openbao-operator")}))
		roleReqData.Body().SetAttributeValue("ttl", cty.StringVal("1h"))

		// 5. Auto-create backup policy and role if backup is configured with JWT Auth (opt-in)
		// Only creates policies/roles when backup is explicitly configured AND uses JWTAuthRole.
		// If backup is not configured or uses TokenSecretRef instead, nothing is created.
		if cluster.Spec.Backup != nil && cluster.Spec.Backup.JWTAuthRole != "" {
			// Create backup policy
			backupPolicyReq := initBody.AppendNewBlock("request", []string{"create-backup-policy"})
			backupPolicyReqBody := backupPolicyReq.Body()
			backupPolicyReqBody.SetAttributeValue("operation", cty.StringVal("update"))
			backupPolicyReqBody.SetAttributeValue("path", cty.StringVal("sys/policies/acl/backup"))
			backupPolicyReqData := backupPolicyReqBody.AppendNewBlock("data", nil)
			backupPolicy := `path "sys/storage/raft/snapshot" { capabilities = ["read"] }`
			backupPolicyReqData.Body().SetAttributeValue("policy", cty.StringVal(backupPolicy))

			// Create backup JWT role
			backupRoleReq := initBody.AppendNewBlock("request", []string{"create-backup-jwt-role"})
			backupRoleReqBody := backupRoleReq.Body()
			backupRoleReqBody.SetAttributeValue("operation", cty.StringVal("update"))
			backupRoleReqBody.SetAttributeValue("path", cty.StringVal("auth/jwt/role/backup"))
			backupRoleReqData := backupRoleReqBody.AppendNewBlock("data", nil)
			backupRoleReqData.Body().SetAttributeValue("role_type", cty.StringVal("jwt"))
			backupRoleReqData.Body().SetAttributeValue("user_claim", cty.StringVal("sub"))
			backupRoleReqData.Body().SetAttributeValue("bound_audiences", cty.ListVal([]cty.Value{cty.StringVal("openbao-internal")}))

			backupBoundClaims := map[string]cty.Value{
				"kubernetes.io/namespace":           cty.StringVal(cluster.Namespace),
				"kubernetes.io/serviceaccount/name": cty.StringVal(fmt.Sprintf("%s-backup-serviceaccount", cluster.Name)),
			}
			backupRoleReqData.Body().SetAttributeValue("bound_claims", cty.ObjectVal(backupBoundClaims))
			backupRoleReqData.Body().SetAttributeValue("token_policies", cty.ListVal([]cty.Value{cty.StringVal("backup")}))
			backupRoleReqData.Body().SetAttributeValue("ttl", cty.StringVal("1h"))
		}

		// 6. Auto-create upgrade policy and role if upgrade is configured with JWT Auth (opt-in)
		// Only creates policies/roles when upgrade is explicitly configured AND uses JWTAuthRole.
		// If upgrade is not configured or uses TokenSecretRef instead, nothing is created.
		if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.JWTAuthRole != "" {
			// Create upgrade policy
			upgradePolicyReq := initBody.AppendNewBlock("request", []string{"create-upgrade-policy"})
			upgradePolicyReqBody := upgradePolicyReq.Body()
			upgradePolicyReqBody.SetAttributeValue("operation", cty.StringVal("update"))
			upgradePolicyReqBody.SetAttributeValue("path", cty.StringVal("sys/policies/acl/upgrade"))
			upgradePolicyReqData := upgradePolicyReqBody.AppendNewBlock("data", nil)
			upgradePolicyReqData.Body().SetAttributeValue("policy", cty.StringVal(jwtPolicyHealthStepDownSnapshot))

			// Create upgrade JWT role
			upgradeRoleReq := initBody.AppendNewBlock("request", []string{"create-upgrade-jwt-role"})
			upgradeRoleReqBody := upgradeRoleReq.Body()
			upgradeRoleReqBody.SetAttributeValue("operation", cty.StringVal("update"))
			upgradeRoleReqBody.SetAttributeValue("path", cty.StringVal("auth/jwt/role/upgrade"))
			upgradeRoleReqData := upgradeRoleReqBody.AppendNewBlock("data", nil)
			upgradeRoleReqData.Body().SetAttributeValue("role_type", cty.StringVal("jwt"))
			upgradeRoleReqData.Body().SetAttributeValue("user_claim", cty.StringVal("sub"))
			upgradeRoleReqData.Body().SetAttributeValue("bound_audiences", cty.ListVal([]cty.Value{cty.StringVal("openbao-internal")}))

			upgradeBoundClaims := map[string]cty.Value{
				"kubernetes.io/namespace":           cty.StringVal(cluster.Namespace),
				"kubernetes.io/serviceaccount/name": cty.StringVal(fmt.Sprintf("%s-upgrade-serviceaccount", cluster.Name)),
			}
			upgradeRoleReqData.Body().SetAttributeValue("bound_claims", cty.ObjectVal(upgradeBoundClaims))
			upgradeRoleReqData.Body().SetAttributeValue("token_policies", cty.ListVal([]cty.Value{cty.StringVal("upgrade")}))
			upgradeRoleReqData.Body().SetAttributeValue("ttl", cty.StringVal("1h"))
		}
	}

	// Render user self-init requests if enabled
	if cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled {
		if err := renderSelfInitStanzas(body, cluster.Spec.SelfInit.Requests); err != nil {
			return nil, fmt.Errorf("failed to render self-init stanzas: %w", err)
		}
	}

	return file.Bytes(), nil
}

// renderSelfInitStanzas generates HCL initialize stanzas for OpenBao's self-initialization feature.
// Each request is rendered as an initialize block containing a named request block with the specified
// operation, path, and optional data fields. The request name is required by OpenBao's configuration
// schema and is used as the map key when parsing JSON/HCL.
//
// Example output:
//
//	initialize "audit" {
//	  request "enable-audit" {
//	    operation = "update"
//	    path      = "sys/audit/stdout"
//	    data = {
//	      type = "file"
//	      options = {
//	        file_path = "/dev/stdout"
//	        log_raw   = true
//	      }
//	    }
//	  }
//	}
func renderSelfInitStanzas(body *hclwrite.Body, requests []openbaov1alpha1.SelfInitRequest) error {
	for _, req := range requests {
		if strings.TrimSpace(req.Name) == "" {
			continue
		}

		initLabel := req.Name
		requestLabel := fmt.Sprintf("%s-request", req.Name)

		// Create initialize block with the request name as the label.
		// This matches the OpenBao self-init documentation where initialize
		// has a single name label (for example, "audit").
		initBlock := body.AppendNewBlock("initialize", []string{initLabel})
		initBody := initBlock.Body()

		// Create the request block inside initialize with the same name label.
		// OpenBao requires each request block to have a single name, which
		// becomes the key in the "request" map (for example, "enable-audit").
		requestBlock := initBody.AppendNewBlock("request", []string{requestLabel})
		requestBody := requestBlock.Body()

		// Set operation
		requestBody.SetAttributeValue("operation", cty.StringVal(string(req.Operation)))

		// Set path
		requestBody.SetAttributeValue("path", cty.StringVal(req.Path))

		// Set allow_failure if true
		if req.AllowFailure {
			requestBody.SetAttributeValue("allow_failure", cty.BoolVal(true))
		}

		// Set data if provided
		if req.Data != nil && len(req.Data.Raw) > 0 {
			var decoded interface{}
			if err := json.Unmarshal(req.Data.Raw, &decoded); err != nil {
				return fmt.Errorf("failed to decode self-init data for request %q: %w", req.Name, err)
			}

			ctyVal, err := jsonToCty(decoded)
			if err != nil {
				return fmt.Errorf("failed to convert self-init data for request %q to HCL: %w", req.Name, err)
			}

			requestBody.SetAttributeValue("data", ctyVal)
		}
	}

	return nil
}

// jsonToCty converts a decoded JSON value (maps, slices, strings, numbers,
// booleans) into a cty.Value tree suitable for hclwrite. This function uses
// interface{} because encoding/json produces generic map[string]interface{}
// structures; a concrete type is not possible here.
func jsonToCty(v interface{}) (cty.Value, error) {
	switch val := v.(type) {
	case map[string]interface{}:
		if len(val) == 0 {
			return cty.EmptyObjectVal, nil
		}

		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		obj := make(map[string]cty.Value, len(val))
		for _, k := range keys {
			child, err := jsonToCty(val[k])
			if err != nil {
				return cty.NilVal, err
			}
			obj[k] = child
		}

		return cty.ObjectVal(obj), nil
	case []interface{}:
		if len(val) == 0 {
			return cty.EmptyTupleVal, nil
		}

		elems := make([]cty.Value, len(val))
		for i, elem := range val {
			child, err := jsonToCty(elem)
			if err != nil {
				return cty.NilVal, err
			}
			elems[i] = child
		}

		return cty.TupleVal(elems), nil
	case string:
		return cty.StringVal(val), nil
	case bool:
		return cty.BoolVal(val), nil
	case float64:
		// JSON numbers are decoded as float64 by encoding/json.
		// Format as string without scientific notation to ensure HCL compatibility.
		// Check if it's an integer to avoid unnecessary decimal points.
		if val == math.Trunc(val) {
			// Integer value - format without decimal point
			return cty.StringVal(strconv.FormatInt(int64(val), 10)), nil
		}
		// Float value - use 'f' format to avoid scientific notation
		// Use -1 precision to use the smallest number of digits necessary
		return cty.StringVal(strconv.FormatFloat(val, 'f', -1, 64)), nil
	case nil:
		// Represent null as an explicit null value; HCL will render this as
		// "null".
		return cty.NullVal(cty.DynamicPseudoType), nil
	default:
		return cty.NilVal, fmt.Errorf("unsupported JSON value type %T in self-init data", v)
	}
}

// renderAuditDevices renders audit device blocks.
// See: https://openbao.org/docs/configuration/audit/
func renderAuditDevices(body *hclwrite.Body, devices []openbaov1alpha1.AuditDevice) error {
	for _, device := range devices {
		if device.Type == "" || device.Path == "" {
			continue
		}

		// Create audit block: audit "type" "path" { ... }
		auditBlock := body.AppendNewBlock("audit", []string{device.Type, device.Path})
		auditBody := auditBlock.Body()

		// Set description if provided
		if device.Description != "" {
			auditBody.SetAttributeValue("description", cty.StringVal(device.Description))
		}

		// Set options if provided
		if device.Options != nil && len(device.Options.Raw) > 0 {
			var decoded interface{}
			if err := json.Unmarshal(device.Options.Raw, &decoded); err != nil {
				return fmt.Errorf("failed to decode audit device options: %w", err)
			}

			ctyVal, err := jsonToCty(decoded)
			if err != nil {
				return fmt.Errorf("failed to convert audit device options to HCL: %w", err)
			}

			auditBody.SetAttributeValue("options", ctyVal)
		}
	}

	return nil
}

// renderPlugins renders plugin blocks.
// See: https://openbao.org/docs/configuration/plugins/
func renderPlugins(body *hclwrite.Body, plugins []openbaov1alpha1.Plugin) {
	for _, plugin := range plugins {
		if plugin.Type == "" || plugin.Name == "" {
			continue
		}

		// Create plugin block: plugin "type" "name" { ... }
		pluginBlock := body.AppendNewBlock("plugin", []string{plugin.Type, plugin.Name})
		pluginBody := pluginBlock.Body()

		// Set image or command (mutually exclusive)
		if plugin.Image != "" {
			pluginBody.SetAttributeValue("image", cty.StringVal(plugin.Image))
		} else if plugin.Command != "" {
			pluginBody.SetAttributeValue("command", cty.StringVal(plugin.Command))
		}

		// Set required fields
		pluginBody.SetAttributeValue("version", cty.StringVal(plugin.Version))
		pluginBody.SetAttributeValue("binary_name", cty.StringVal(plugin.BinaryName))
		pluginBody.SetAttributeValue("sha256sum", cty.StringVal(plugin.SHA256Sum))

		// Set optional args
		if len(plugin.Args) > 0 {
			args := make([]cty.Value, len(plugin.Args))
			for i, arg := range plugin.Args {
				args[i] = cty.StringVal(arg)
			}
			pluginBody.SetAttributeValue("args", cty.TupleVal(args))
		}

		// Set optional env
		if len(plugin.Env) > 0 {
			env := make([]cty.Value, len(plugin.Env))
			for i, e := range plugin.Env {
				env[i] = cty.StringVal(e)
			}
			pluginBody.SetAttributeValue("env", cty.TupleVal(env))
		}
	}
}

// renderTelemetry renders telemetry block.
// See: https://openbao.org/docs/configuration/telemetry/
func renderTelemetry(body *hclwrite.Body, telemetry *openbaov1alpha1.TelemetryConfig) {
	if telemetry == nil {
		return
	}

	// Create telemetry block: telemetry { ... }
	telemetryBlock := body.AppendNewBlock("telemetry", nil)
	telemetryBody := telemetryBlock.Body()

	// Common options
	if telemetry.UsageGaugePeriod != "" {
		telemetryBody.SetAttributeValue("usage_gauge_period", cty.StringVal(telemetry.UsageGaugePeriod))
	}

	if telemetry.MaximumGaugeCardinality != nil {
		telemetryBody.SetAttributeValue("maximum_gauge_cardinality", cty.NumberIntVal(int64(*telemetry.MaximumGaugeCardinality)))
	}

	if telemetry.DisableHostname {
		telemetryBody.SetAttributeValue("disable_hostname", cty.BoolVal(true))
	}

	if telemetry.EnableHostnameLabel {
		telemetryBody.SetAttributeValue("enable_hostname_label", cty.BoolVal(true))
	}

	if telemetry.MetricsPrefix != "" {
		telemetryBody.SetAttributeValue("metrics_prefix", cty.StringVal(telemetry.MetricsPrefix))
	}

	if telemetry.LeaseMetricsEpsilon != "" {
		telemetryBody.SetAttributeValue("lease_metrics_epsilon", cty.StringVal(telemetry.LeaseMetricsEpsilon))
	}

	// Prometheus options
	if telemetry.PrometheusRetentionTime != "" {
		telemetryBody.SetAttributeValue("prometheus_retention_time", cty.StringVal(telemetry.PrometheusRetentionTime))
	}

	// Statsite options
	if telemetry.StatsiteAddress != "" {
		telemetryBody.SetAttributeValue("statsite_address", cty.StringVal(telemetry.StatsiteAddress))
	}

	// StatsD options
	if telemetry.StatsdAddress != "" {
		telemetryBody.SetAttributeValue("statsd_address", cty.StringVal(telemetry.StatsdAddress))
	}

	// DogStatsD options
	if telemetry.DogStatsdAddress != "" {
		telemetryBody.SetAttributeValue("dogstatsd_address", cty.StringVal(telemetry.DogStatsdAddress))
	}

	if len(telemetry.DogStatsdTags) > 0 {
		tags := make([]cty.Value, len(telemetry.DogStatsdTags))
		for i, tag := range telemetry.DogStatsdTags {
			tags[i] = cty.StringVal(tag)
		}
		telemetryBody.SetAttributeValue("dogstatsd_tags", cty.TupleVal(tags))
	}

	// Circonus options
	if telemetry.CirconusAPIKey != "" {
		telemetryBody.SetAttributeValue("circonus_api_key", cty.StringVal(telemetry.CirconusAPIKey))
	}

	if telemetry.CirconusAPIApp != "" {
		telemetryBody.SetAttributeValue("circonus_api_app", cty.StringVal(telemetry.CirconusAPIApp))
	}

	if telemetry.CirconusAPIURL != "" {
		telemetryBody.SetAttributeValue("circonus_api_url", cty.StringVal(telemetry.CirconusAPIURL))
	}

	if telemetry.CirconusSubmissionInterval != "" {
		telemetryBody.SetAttributeValue("circonus_submission_interval", cty.StringVal(telemetry.CirconusSubmissionInterval))
	}

	if telemetry.CirconusCheckID != "" {
		telemetryBody.SetAttributeValue("circonus_check_id", cty.StringVal(telemetry.CirconusCheckID))
	}

	if telemetry.CirconusCheckForceMetricActivation != "" {
		telemetryBody.SetAttributeValue("circonus_check_force_metric_activation", cty.StringVal(telemetry.CirconusCheckForceMetricActivation))
	}

	if telemetry.CirconusCheckInstanceID != "" {
		telemetryBody.SetAttributeValue("circonus_check_instance_id", cty.StringVal(telemetry.CirconusCheckInstanceID))
	}

	if telemetry.CirconusCheckSearchTag != "" {
		telemetryBody.SetAttributeValue("circonus_check_search_tag", cty.StringVal(telemetry.CirconusCheckSearchTag))
	}

	if telemetry.CirconusCheckDisplayName != "" {
		telemetryBody.SetAttributeValue("circonus_check_display_name", cty.StringVal(telemetry.CirconusCheckDisplayName))
	}

	if telemetry.CirconusCheckTags != "" {
		telemetryBody.SetAttributeValue("circonus_check_tags", cty.StringVal(telemetry.CirconusCheckTags))
	}

	if telemetry.CirconusBrokerID != "" {
		telemetryBody.SetAttributeValue("circonus_broker_id", cty.StringVal(telemetry.CirconusBrokerID))
	}

	if telemetry.CirconusBrokerSelectTag != "" {
		telemetryBody.SetAttributeValue("circonus_broker_select_tag", cty.StringVal(telemetry.CirconusBrokerSelectTag))
	}

	// Stackdriver options
	if telemetry.StackdriverProjectID != "" {
		telemetryBody.SetAttributeValue("stackdriver_project_id", cty.StringVal(telemetry.StackdriverProjectID))
	}

	if telemetry.StackdriverLocation != "" {
		telemetryBody.SetAttributeValue("stackdriver_location", cty.StringVal(telemetry.StackdriverLocation))
	}

	if telemetry.StackdriverNamespace != "" {
		telemetryBody.SetAttributeValue("stackdriver_namespace", cty.StringVal(telemetry.StackdriverNamespace))
	}

	if telemetry.StackdriverDebugLogs {
		telemetryBody.SetAttributeValue("stackdriver_debug_logs", cty.BoolVal(true))
	}
}

// renderListenerConfiguration renders user-provided listener configuration.
func renderListenerConfiguration(listenerBody *hclwrite.Body, config *openbaov1alpha1.OpenBaoConfiguration) {
	if config == nil || config.Listener == nil {
		return
	}
	if config.Listener.ProxyProtocolBehavior != "" {
		listenerBody.SetAttributeValue("proxy_protocol_behavior", cty.StringVal(config.Listener.ProxyProtocolBehavior))
	}
	// Note: TLSDisable is typically managed by the operator based on spec.tls.enabled,
	// but we allow it to be overridden if explicitly set
	if config.Listener.TLSDisable != nil {
		if *config.Listener.TLSDisable {
			listenerBody.SetAttributeValue("tls_disable", cty.NumberIntVal(1))
		} else {
			listenerBody.SetAttributeValue("tls_disable", cty.NumberIntVal(0))
		}
	}
}

// renderTLSConfiguration renders TLS configuration based on the TLS mode.
func renderTLSConfiguration(listenerBody *hclwrite.Body, tlsSpec openbaov1alpha1.TLSConfig, config *openbaov1alpha1.OpenBaoConfiguration) error {
	if tlsSpec.Mode == openbaov1alpha1.TLSModeACME {
		// ACME mode: OpenBao manages certificates via native ACME client
		// Certificates are stored in-memory (or cached per tls_acme_cache_path).
		// See: https://openbao.org/docs/rfcs/acme-tls-listeners/
		if tlsSpec.ACME == nil {
			return fmt.Errorf("ACME configuration is required when tls.mode is ACME")
		}
		acmeConfig := tlsSpec.ACME

		// ACME parameters are set directly on the listener, not in a nested block
		// See: https://openbao.org/docs/configuration/listener/tcp/#acme-parameters
		listenerBody.SetAttributeValue("tls_acme_ca_directory", cty.StringVal(acmeConfig.DirectoryURL))
		// tls_acme_domains is a list of domains. Use exactly what is provided; do not inject localhost,
		// to avoid ACME challenges against loopback.
		acmeDomains := []cty.Value{cty.StringVal(acmeConfig.Domain)}
		listenerBody.SetAttributeValue("tls_acme_domains", cty.ListVal(acmeDomains))
		if acmeConfig.Email != "" {
			listenerBody.SetAttributeValue("tls_acme_email", cty.StringVal(acmeConfig.Email))
		}
		// ACME CA root (if specified in configuration)
		if config != nil && config.ACMECARoot != "" {
			listenerBody.SetAttributeValue("tls_acme_ca_root", cty.StringVal(config.ACMECARoot))
		}
	} else {
		// OperatorManaged or External mode: use file-based TLS certificates
		listenerBody.SetAttributeValue("tls_cert_file", cty.StringVal(constants.PathTLSServerCert))
		listenerBody.SetAttributeValue("tls_key_file", cty.StringVal(constants.PathTLSServerKey))
		listenerBody.SetAttributeValue("tls_client_ca_file", cty.StringVal(constants.PathTLSCACert))
	}
	return nil
}

// renderUserConfiguration renders additional user configuration from structured fields.
func renderUserConfiguration(body *hclwrite.Body, config *openbaov1alpha1.OpenBaoConfiguration) {
	if config == nil {
		return
	}

	// Log level
	if config.LogLevel != "" {
		body.SetAttributeValue("log_level", cty.StringVal(config.LogLevel))
	}

	// Logging configuration
	if config.Logging != nil {
		if config.Logging.Format != "" {
			body.SetAttributeValue("log_format", cty.StringVal(config.Logging.Format))
		}
		if config.Logging.File != "" {
			body.SetAttributeValue("log_file", cty.StringVal(config.Logging.File))
		}
		if config.Logging.RotateDuration != "" {
			body.SetAttributeValue("log_rotate_duration", cty.StringVal(config.Logging.RotateDuration))
		}
		if config.Logging.RotateBytes != nil {
			body.SetAttributeValue("log_rotate_bytes", cty.NumberIntVal(*config.Logging.RotateBytes))
		}
		if config.Logging.RotateMaxFiles != nil {
			body.SetAttributeValue("log_rotate_max_files", cty.NumberIntVal(int64(*config.Logging.RotateMaxFiles)))
		}
		if config.Logging.PIDFile != "" {
			body.SetAttributeValue("pid_file", cty.StringVal(config.Logging.PIDFile))
		}
	}

	// Plugin configuration
	if config.Plugin != nil {
		if config.Plugin.FileUID != nil {
			body.SetAttributeValue("plugin_file_uid", cty.NumberIntVal(*config.Plugin.FileUID))
		}
		if config.Plugin.FilePermissions != "" {
			body.SetAttributeValue("plugin_file_permissions", cty.StringVal(config.Plugin.FilePermissions))
		}
		if config.Plugin.AutoDownload != nil {
			body.SetAttributeValue("plugin_auto_download", cty.BoolVal(*config.Plugin.AutoDownload))
		}
		if config.Plugin.AutoRegister != nil {
			body.SetAttributeValue("plugin_auto_register", cty.BoolVal(*config.Plugin.AutoRegister))
		}
		if config.Plugin.DownloadBehavior != "" {
			body.SetAttributeValue("plugin_download_behavior", cty.StringVal(config.Plugin.DownloadBehavior))
		}
	}

	// Lease/TTL configuration
	if config.DefaultLeaseTTL != "" {
		body.SetAttributeValue("default_lease_ttl", cty.StringVal(config.DefaultLeaseTTL))
	}
	if config.MaxLeaseTTL != "" {
		body.SetAttributeValue("max_lease_ttl", cty.StringVal(config.MaxLeaseTTL))
	}

	// Cache configuration
	if config.CacheSize != nil {
		body.SetAttributeValue("cache_size", cty.NumberIntVal(*config.CacheSize))
	}
	if config.DisableCache != nil {
		body.SetAttributeValue("disable_cache", cty.BoolVal(*config.DisableCache))
	}

	// Advanced/experimental features
	if config.DetectDeadlocks != nil {
		body.SetAttributeValue("detect_deadlocks", cty.BoolVal(*config.DetectDeadlocks))
	}
	if config.RawStorageEndpoint != nil {
		body.SetAttributeValue("raw_storage_endpoint", cty.BoolVal(*config.RawStorageEndpoint))
	}
	if config.IntrospectionEndpoint != nil {
		body.SetAttributeValue("introspection_endpoint", cty.BoolVal(*config.IntrospectionEndpoint))
	}
	if config.DisableStandbyReads != nil {
		body.SetAttributeValue("disable_standby_reads", cty.BoolVal(*config.DisableStandbyReads))
	}
	if config.ImpreciseLeaseRoleTracking != nil {
		body.SetAttributeValue("imprecise_lease_role_tracking", cty.BoolVal(*config.ImpreciseLeaseRoleTracking))
	}
	if config.UnsafeAllowAPIAuditCreation != nil {
		body.SetAttributeValue("unsafe_allow_api_audit_creation", cty.BoolVal(*config.UnsafeAllowAPIAuditCreation))
	}
	if config.AllowAuditLogPrefixing != nil {
		body.SetAttributeValue("allow_audit_log_prefixing", cty.BoolVal(*config.AllowAuditLogPrefixing))
	}
	if config.EnableResponseHeaderHostname != nil {
		body.SetAttributeValue("enable_response_header_hostname", cty.BoolVal(*config.EnableResponseHeaderHostname))
	}
	if config.EnableResponseHeaderRaftNodeID != nil {
		body.SetAttributeValue("enable_response_header_raft_node_id", cty.BoolVal(*config.EnableResponseHeaderRaftNodeID))
	}
}

// renderSealStanza renders the seal block based on the cluster's unseal configuration.
// If spec.unseal is nil or type is "static" (or empty), renders the default static seal.
// Otherwise, renders the specified seal type with options from structured fields.
//
//nolint:unparam // Kept as error-returning for future seal validation and to keep call sites consistent.
func renderSealStanza(body *hclwrite.Body, cluster *openbaov1alpha1.OpenBaoCluster) error {
	unsealType := "static"
	if cluster.Spec.Unseal != nil && cluster.Spec.Unseal.Type != "" {
		unsealType = cluster.Spec.Unseal.Type
	}

	sealBlock := body.AppendNewBlock("seal", []string{unsealType})
	sealBody := sealBlock.Body()

	if unsealType == "static" {
		// Render static seal configuration (operator-managed defaults if not specified)
		currentKey := configUnsealKeyPath
		currentKeyID := configUnsealKeyID
		if cluster.Spec.Unseal != nil && cluster.Spec.Unseal.Static != nil {
			if cluster.Spec.Unseal.Static.CurrentKey != "" {
				currentKey = cluster.Spec.Unseal.Static.CurrentKey
			}
			if cluster.Spec.Unseal.Static.CurrentKeyID != "" {
				currentKeyID = cluster.Spec.Unseal.Static.CurrentKeyID
			}
		}
		sealBody.SetAttributeValue("current_key", cty.StringVal(currentKey))
		sealBody.SetAttributeValue("current_key_id", cty.StringVal(currentKeyID))
	} else if cluster.Spec.Unseal != nil {
		// Render structured seal configuration based on type
		switch unsealType {
		case "transit":
			if cluster.Spec.Unseal.Transit != nil {
				renderTransitSeal(sealBody, cluster.Spec.Unseal.Transit)
			}
		case "awskms":
			if cluster.Spec.Unseal.AWSKMS != nil {
				renderAWSKMSSeal(sealBody, cluster.Spec.Unseal.AWSKMS)
			}
		case "azurekeyvault":
			if cluster.Spec.Unseal.AzureKeyVault != nil {
				renderAzureKeyVaultSeal(sealBody, cluster.Spec.Unseal.AzureKeyVault)
			}
		case "gcpckms":
			if cluster.Spec.Unseal.GCPCloudKMS != nil {
				renderGCPCloudKMSSeal(sealBody, cluster.Spec.Unseal.GCPCloudKMS)
			}
		case "kmip":
			if cluster.Spec.Unseal.KMIP != nil {
				renderKMIPSeal(sealBody, cluster.Spec.Unseal.KMIP)
			}
		case "ocikms":
			if cluster.Spec.Unseal.OCIKMS != nil {
				renderOCIKMSSeal(sealBody, cluster.Spec.Unseal.OCIKMS)
			}
		case "pkcs11":
			if cluster.Spec.Unseal.PKCS11 != nil {
				renderPKCS11Seal(sealBody, cluster.Spec.Unseal.PKCS11)
			}
		}
	}

	return nil
}

// renderTransitSeal renders Transit seal configuration.
func renderTransitSeal(sealBody *hclwrite.Body, config *openbaov1alpha1.TransitSealConfig) {
	sealBody.SetAttributeValue("address", cty.StringVal(config.Address))
	if config.Token != "" {
		sealBody.SetAttributeValue("token", cty.StringVal(config.Token))
	}
	sealBody.SetAttributeValue("key_name", cty.StringVal(config.KeyName))
	sealBody.SetAttributeValue("mount_path", cty.StringVal(config.MountPath))
	if config.Namespace != "" {
		sealBody.SetAttributeValue("namespace", cty.StringVal(config.Namespace))
	}
	if config.DisableRenewal != nil {
		sealBody.SetAttributeValue("disable_renewal", cty.StringVal(fmt.Sprintf("%t", *config.DisableRenewal)))
	}
	if config.TLSCACert != "" {
		sealBody.SetAttributeValue("tls_ca_cert", cty.StringVal(config.TLSCACert))
	}
	if config.TLSClientCert != "" {
		sealBody.SetAttributeValue("tls_client_cert", cty.StringVal(config.TLSClientCert))
	}
	if config.TLSClientKey != "" {
		sealBody.SetAttributeValue("tls_client_key", cty.StringVal(config.TLSClientKey))
	}
	if config.TLSServerName != "" {
		sealBody.SetAttributeValue("tls_server_name", cty.StringVal(config.TLSServerName))
	}
	if config.TLSSkipVerify != nil {
		sealBody.SetAttributeValue("tls_skip_verify", cty.StringVal(fmt.Sprintf("%t", *config.TLSSkipVerify)))
	}
}

// renderAWSKMSSeal renders AWS KMS seal configuration.
func renderAWSKMSSeal(sealBody *hclwrite.Body, config *openbaov1alpha1.AWSKMSSealConfig) {
	sealBody.SetAttributeValue("region", cty.StringVal(config.Region))
	sealBody.SetAttributeValue("kms_key_id", cty.StringVal(config.KMSKeyID))
	if config.Endpoint != "" {
		sealBody.SetAttributeValue("endpoint", cty.StringVal(config.Endpoint))
	}
	if config.AccessKey != "" {
		sealBody.SetAttributeValue("access_key", cty.StringVal(config.AccessKey))
	}
	if config.SecretKey != "" {
		sealBody.SetAttributeValue("secret_key", cty.StringVal(config.SecretKey))
	}
	if config.SessionToken != "" {
		sealBody.SetAttributeValue("session_token", cty.StringVal(config.SessionToken))
	}
}

// renderAzureKeyVaultSeal renders Azure Key Vault seal configuration.
func renderAzureKeyVaultSeal(sealBody *hclwrite.Body, config *openbaov1alpha1.AzureKeyVaultSealConfig) {
	sealBody.SetAttributeValue("vault_name", cty.StringVal(config.VaultName))
	sealBody.SetAttributeValue("key_name", cty.StringVal(config.KeyName))
	if config.TenantID != "" {
		sealBody.SetAttributeValue("tenant_id", cty.StringVal(config.TenantID))
	}
	if config.ClientID != "" {
		sealBody.SetAttributeValue("client_id", cty.StringVal(config.ClientID))
	}
	if config.ClientSecret != "" {
		sealBody.SetAttributeValue("client_secret", cty.StringVal(config.ClientSecret))
	}
	if config.Resource != "" {
		sealBody.SetAttributeValue("resource", cty.StringVal(config.Resource))
	}
	if config.Environment != "" {
		sealBody.SetAttributeValue("environment", cty.StringVal(config.Environment))
	}
}

// renderGCPCloudKMSSeal renders GCP Cloud KMS seal configuration.
func renderGCPCloudKMSSeal(sealBody *hclwrite.Body, config *openbaov1alpha1.GCPCloudKMSSealConfig) {
	sealBody.SetAttributeValue("project", cty.StringVal(config.Project))
	sealBody.SetAttributeValue("region", cty.StringVal(config.Region))
	sealBody.SetAttributeValue("key_ring", cty.StringVal(config.KeyRing))
	sealBody.SetAttributeValue("crypto_key", cty.StringVal(config.CryptoKey))
	if config.Credentials != "" {
		sealBody.SetAttributeValue("credentials", cty.StringVal(config.Credentials))
	}
}

// renderKMIPSeal renders KMIP seal configuration.
func renderKMIPSeal(sealBody *hclwrite.Body, config *openbaov1alpha1.KMIPSealConfig) {
	sealBody.SetAttributeValue("address", cty.StringVal(config.Address))
	if config.Certificate != "" {
		sealBody.SetAttributeValue("certificate", cty.StringVal(config.Certificate))
	}
	if config.Key != "" {
		sealBody.SetAttributeValue("key", cty.StringVal(config.Key))
	}
	if config.CACert != "" {
		sealBody.SetAttributeValue("ca_cert", cty.StringVal(config.CACert))
	}
	if config.TLSServerName != "" {
		sealBody.SetAttributeValue("tls_server_name", cty.StringVal(config.TLSServerName))
	}
	if config.TLSSkipVerify != nil {
		sealBody.SetAttributeValue("tls_skip_verify", cty.StringVal(fmt.Sprintf("%t", *config.TLSSkipVerify)))
	}
}

// renderOCIKMSSeal renders OCI KMS seal configuration.
func renderOCIKMSSeal(sealBody *hclwrite.Body, config *openbaov1alpha1.OCIKMSSealConfig) {
	sealBody.SetAttributeValue("key_id", cty.StringVal(config.KeyID))
	sealBody.SetAttributeValue("crypto_endpoint", cty.StringVal(config.CryptoEndpoint))
	sealBody.SetAttributeValue("management_endpoint", cty.StringVal(config.ManagementEndpoint))
	if config.AuthType != "" {
		sealBody.SetAttributeValue("auth_type", cty.StringVal(config.AuthType))
	}
	if config.CompartmentID != "" {
		sealBody.SetAttributeValue("compartment_id", cty.StringVal(config.CompartmentID))
	}
}

// renderPKCS11Seal renders PKCS#11 seal configuration.
func renderPKCS11Seal(sealBody *hclwrite.Body, config *openbaov1alpha1.PKCS11SealConfig) {
	sealBody.SetAttributeValue("lib", cty.StringVal(config.Lib))
	if config.Slot != "" {
		sealBody.SetAttributeValue("slot", cty.StringVal(config.Slot))
	}
	if config.PIN != "" {
		sealBody.SetAttributeValue("pin", cty.StringVal(config.PIN))
	}
	sealBody.SetAttributeValue("key_label", cty.StringVal(config.KeyLabel))
	if config.HMACKeyLabel != "" {
		sealBody.SetAttributeValue("hmac_key_label", cty.StringVal(config.HMACKeyLabel))
	}
	if config.GenerateKey != nil {
		sealBody.SetAttributeValue("generate_key", cty.StringVal(fmt.Sprintf("%t", *config.GenerateKey)))
	}
	if config.RSAEncryptLocal != nil {
		sealBody.SetAttributeValue("rsa_encrypt_local", cty.StringVal(fmt.Sprintf("%t", *config.RSAEncryptLocal)))
	}
	if config.RSAOAEPHash != "" {
		sealBody.SetAttributeValue("rsa_oaep_hash", cty.StringVal(config.RSAOAEPHash))
	}
}
