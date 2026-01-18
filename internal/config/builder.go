package config

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

const (
	configPluginDirectoryPath = "/openbao/plugins"
	configUnsealKeyPath       = "file:///etc/bao/unseal/key"
	configUnsealKeyID         = "operator-generated-v1"
	configMaxRequestDuration  = "90s"
	configNodeIDTemplate      = "${HOSTNAME}"

	jwtPolicyHealthStepDownSnapshot = `path "sys/health" { capabilities = ["read"] }
path "sys/step-down" { capabilities = ["sudo", "update"] }
path "sys/storage/raft/snapshot" { capabilities = ["read"] }
path "sys/storage/raft/autopilot/configuration" { capabilities = ["read", "update"] }`

	jwtPolicyUpgradeRolling = `path "sys/health" { capabilities = ["read"] }
path "sys/step-down" { capabilities = ["sudo", "update"] }
path "sys/storage/raft/snapshot" { capabilities = ["read"] }
path "sys/storage/raft/autopilot/state" { capabilities = ["read"] }`

	jwtPolicyUpgradeBlueGreen = `path "sys/health" { capabilities = ["read"] }
path "sys/step-down" { capabilities = ["sudo", "update"] }
path "sys/storage/raft/snapshot" { capabilities = ["read"] }
path "sys/storage/raft/autopilot/state" { capabilities = ["read"] }
path "sys/storage/raft/join" { capabilities = ["update"] }
path "sys/storage/raft/configuration" { capabilities = ["read", "update"] }
path "sys/storage/raft/remove-peer" { capabilities = ["update"] }
path "sys/storage/raft/promote" { capabilities = ["update"] }
path "sys/storage/raft/demote" { capabilities = ["update"] }`
)

// OperatorBootstrapConfig holds configuration for operator bootstrap.
type OperatorBootstrapConfig struct {
	OIDCIssuerURL   string
	JWTKeysPEM      []string
	OperatorNS      string
	OperatorSA      string
	JWTAuthAudience string
}

// InfrastructureDetails captures the pieces of topology information required to
// render a complete config.hcl file.
type InfrastructureDetails struct {
	HeadlessServiceName string
	Namespace           string
	APIPort             int
	ClusterPort         int
	// TargetRevisionForJoin is an optional revision identifier for blue/green deployments.
	// When set, the retry_join label selector will include this revision to ensure
	// Green pods only discover Blue pods (not each other).
	TargetRevisionForJoin string
}

// RenderHCL renders a complete OpenBao configuration using the provided cluster
// specification and infrastructure details.
//
// The generated configuration:
//   - Always includes operator-owned listener "tcp" and storage "raft" stanzas.
//   - Includes seal stanza based on spec.unseal (defaults to "static" if omitted).
//   - Uses a Kubernetes go-discover-based retry_join block for dynamic cluster membership.
//   - Renders user-tunable configuration from typed fields under spec.configuration.
//   - Renders audit devices from spec.audit (if configured).
//   - Renders plugins from spec.plugins (if configured).
//   - Renders telemetry configuration from spec.telemetry (if configured).
func RenderHCL(cluster *openbaov1alpha1.OpenBaoCluster, infra InfrastructureDetails) ([]byte, error) {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	infra, err := validateInfrastructureDetails(cluster, infra)
	if err != nil {
		return nil, err
	}

	if err := validateConfigVersionCompatibility(cluster); err != nil {
		return nil, err
	}

	// General configuration. UI defaults to true, but can be overridden by user configuration.
	uiEnabled := true
	if cluster.Spec.Configuration != nil && cluster.Spec.Configuration.UI != nil {
		uiEnabled = *cluster.Spec.Configuration.UI
	}

	apiAddr := fmt.Sprintf("https://${HOSTNAME}.%s.%s.svc:%d", infra.HeadlessServiceName, infra.Namespace, infra.APIPort)
	clusterAddr := fmt.Sprintf("https://${HOSTNAME}.%s.%s.svc:%d", infra.HeadlessServiceName, infra.Namespace, infra.ClusterPort)

	gohcl.EncodeIntoBody(hclCoreAttributes{
		UI:              uiEnabled,
		ClusterName:     cluster.Name,
		APIAddr:         apiAddr,
		ClusterAddr:     clusterAddr,
		PluginDirectory: configPluginDirectoryPath,
	}, body)

	listenerBlock, err := buildListenerBlock(cluster)
	if err != nil {
		return nil, err
	}
	body.AppendBlock(listenerBlock)

	sealBlock, err := buildSealBlock(cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to render seal stanza: %w", err)
	}
	body.AppendBlock(sealBlock)

	body.AppendBlock(buildStorageBlock(cluster, infra))

	body.AppendNewBlock("service_registration", []string{"kubernetes"})

	if tokens := buildUserConfigTokens(cluster.Spec.Configuration); len(tokens) > 0 {
		body.AppendUnstructuredTokens(tokens)
	}

	// 8. Render audit devices if configured
	auditBlocks, err := buildAuditDeviceBlocks(cluster.Spec.Audit)
	if err != nil {
		return nil, fmt.Errorf("failed to render audit devices: %w", err)
	}
	for _, block := range auditBlocks {
		body.AppendBlock(block)
	}

	// 9. Render plugins if configured
	for _, block := range buildPluginBlocks(cluster.Spec.Plugins) {
		body.AppendBlock(block)
	}

	// 10. Render telemetry if configured
	if telemetryBlock := buildTelemetryBlock(cluster.Spec.Telemetry); telemetryBlock != nil {
		body.AppendBlock(telemetryBlock)
	}

	// Note: Self-initialization stanzas are rendered separately via RenderSelfInitHCL
	// and stored in a separate ConfigMap that is only mounted for pod-0.

	return file.Bytes(), nil
}

func validateConfigVersionCompatibility(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	if cluster.Spec.Configuration == nil || cluster.Spec.Configuration.Plugin == nil {
		return nil
	}

	plugin := cluster.Spec.Configuration.Plugin
	if plugin.AutoDownload == nil && plugin.AutoRegister == nil && strings.TrimSpace(plugin.DownloadBehavior) == "" {
		return nil
	}

	ok, err := openBaoVersionAtLeast(cluster.Spec.Version, 2, 5, 0)
	if err != nil {
		return fmt.Errorf("failed to validate config version compatibility: %w", err)
	}
	if ok {
		return nil
	}

	return fmt.Errorf("spec.configuration.plugin.{autoDownload,autoRegister,downloadBehavior} requires OpenBao >= 2.5.0 (spec.version=%q)", cluster.Spec.Version)
}

func openBaoVersionAtLeast(version string, wantMajor, wantMinor, wantPatch int) (bool, error) {
	parsed, err := parseSemVer(version)
	if err != nil {
		return false, err
	}

	if parsed.major != wantMajor {
		return parsed.major > wantMajor, nil
	}
	if parsed.minor != wantMinor {
		return parsed.minor > wantMinor, nil
	}
	return parsed.patch >= wantPatch, nil
}

type semVer struct {
	major int
	minor int
	patch int
}

func parseSemVer(version string) (semVer, error) {
	if strings.TrimSpace(version) == "" {
		return semVer{}, fmt.Errorf("version string is empty")
	}

	trimmed := strings.TrimPrefix(strings.TrimSpace(version), "v")

	// Strip build metadata and prerelease suffixes.
	if idx := strings.IndexAny(trimmed, "+-"); idx != -1 {
		trimmed = trimmed[:idx]
	}

	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return semVer{}, fmt.Errorf("invalid version format %q: expected MAJOR.MINOR.PATCH", version)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semVer{}, fmt.Errorf("invalid major version %q: %w", parts[0], err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semVer{}, fmt.Errorf("invalid minor version %q: %w", parts[1], err)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semVer{}, fmt.Errorf("invalid patch version %q: %w", parts[2], err)
	}

	return semVer{major: major, minor: minor, patch: patch}, nil
}

func upgradePolicyForCluster(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen {
		return jwtPolicyUpgradeBlueGreen
	}
	return jwtPolicyUpgradeRolling
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

	body.AppendBlock(buildOperatorBootstrapInitializeBlock(config))

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

		body.AppendBlock(buildSelfInitBootstrapInitializeBlock(cluster, *bootstrapConfig))
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

		initBlock := buildInitializeBlock(initLabel)
		initBody := initBlock.Body()
		requestBlock := buildInitializeRequestBlock(requestLabel, string(req.Operation), req.Path, req.AllowFailure)
		requestBody := requestBlock.Body()

		// Set data if provided
		// Use structured types for supported paths; Data only for unsupported paths
		dataVal := cty.NilVal

		if structuredVal, handled, err := resolveSelfInitRequestStructuredData(req); err != nil {
			return err
		} else if handled {
			dataVal = structuredVal
		} else if req.Data != nil && len(req.Data.Raw) > 0 {
			// For paths without structured types, use raw Data JSON
			var decoded interface{}
			if err := json.Unmarshal(req.Data.Raw, &decoded); err != nil {
				return fmt.Errorf("failed to decode self-init data for request %q: %w", req.Name, err)
			}

			ctyVal, err := jsonToCty(decoded)
			if err != nil {
				return fmt.Errorf("failed to convert self-init data for request %q to HCL: %w", req.Name, err)
			}

			dataVal = ctyVal
		}

		if dataVal != cty.NilVal {
			if err := setSelfInitRequestData(requestBody, dataVal); err != nil {
				return fmt.Errorf("failed to render self-init request data for request %q: %w", req.Name, err)
			}
		}

		initBody.AppendBlock(requestBlock)
		body.AppendBlock(initBlock)
	}

	return nil
}

//nolint:unparam // Error return maintained for API consistency and future extensibility
func setSelfInitRequestData(requestBody *hclwrite.Body, dataVal cty.Value) error {
	if dataVal == cty.NilVal {
		return nil
	}

	// Prefer "data { ... }" blocks which match OpenBao self-init docs and the
	// operator-bootstrap output. Some endpoints appear to ignore "data = { ... }".
	if dataVal.Type().IsObjectType() || dataVal.Type().IsMapType() {
		dataBlock := requestBody.AppendNewBlock("data", nil)
		dataBody := dataBlock.Body()

		dataMap := dataVal.AsValueMap()
		keys := make([]string, 0, len(dataMap))
		for k := range dataMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			dataBody.SetAttributeValue(k, dataMap[k])
		}
		return nil
	}

	requestBody.SetAttributeValue("data", dataVal)
	return nil
}

// buildAuditDeviceData builds the API request data for an audit device from structured config.
func buildAuditDeviceData(device *openbaov1alpha1.SelfInitAuditDevice) (cty.Value, error) {
	if device == nil {
		return cty.NilVal, fmt.Errorf("audit device config is nil")
	}

	// Build the data object using HCLWrite-style patterns
	// Build options based on device type
	var optionsMap map[string]cty.Value

	switch device.Type {
	case "file":
		if device.FileOptions == nil {
			return cty.NilVal, fmt.Errorf("fileOptions is required for file audit device")
		}
		optionsMap = make(map[string]cty.Value)
		optionsMap["file_path"] = cty.StringVal(device.FileOptions.FilePath)
		if device.FileOptions.Mode != "" {
			optionsMap["mode"] = cty.StringVal(device.FileOptions.Mode)
		}
	case "http":
		if device.HTTPOptions == nil {
			return cty.NilVal, fmt.Errorf("httpOptions is required for http audit device")
		}
		optionsMap = make(map[string]cty.Value)
		optionsMap["uri"] = cty.StringVal(device.HTTPOptions.URI)
		if device.HTTPOptions.Headers != nil && len(device.HTTPOptions.Headers.Raw) > 0 {
			var decoded interface{}
			if err := json.Unmarshal(device.HTTPOptions.Headers.Raw, &decoded); err != nil {
				return cty.NilVal, fmt.Errorf("failed to unmarshal headers: %w", err)
			}
			headersVal, err := jsonToCty(decoded)
			if err != nil {
				return cty.NilVal, fmt.Errorf("failed to convert headers to HCL: %w", err)
			}
			optionsMap["headers"] = headersVal
		}
	case "syslog":
		if device.SyslogOptions != nil {
			optionsMap = make(map[string]cty.Value)
			if device.SyslogOptions.Facility != "" {
				optionsMap["facility"] = cty.StringVal(device.SyslogOptions.Facility)
			}
			if device.SyslogOptions.Tag != "" {
				optionsMap["tag"] = cty.StringVal(device.SyslogOptions.Tag)
			}
		}
	case "socket":
		if device.SocketOptions != nil {
			optionsMap = make(map[string]cty.Value)
			if device.SocketOptions.Address != "" {
				optionsMap["address"] = cty.StringVal(device.SocketOptions.Address)
			}
			if device.SocketOptions.SocketType != "" {
				optionsMap["socket_type"] = cty.StringVal(device.SocketOptions.SocketType)
			}
			if device.SocketOptions.WriteTimeout != "" {
				optionsMap["write_timeout"] = cty.StringVal(device.SocketOptions.WriteTimeout)
			}
		}
	default:
		return cty.NilVal, fmt.Errorf("unsupported audit device type: %s", device.Type)
	}

	// Build the final data object
	dataMap := make(map[string]cty.Value)
	dataMap["type"] = cty.StringVal(device.Type)
	if device.Description != "" {
		dataMap["description"] = cty.StringVal(device.Description)
	}
	if len(optionsMap) > 0 {
		dataMap["options"] = cty.ObjectVal(optionsMap)
	}

	return cty.ObjectVal(dataMap), nil
}

// buildAuthMethodData builds the API request data for an auth method from structured config.
func buildAuthMethodData(authMethod *openbaov1alpha1.SelfInitAuthMethod) (cty.Value, error) {
	if authMethod == nil {
		return cty.NilVal, fmt.Errorf("auth method config is nil")
	}

	dataMap := make(map[string]cty.Value)
	dataMap["type"] = cty.StringVal(authMethod.Type)

	if authMethod.Description != "" {
		dataMap["description"] = cty.StringVal(authMethod.Description)
	}

	if len(authMethod.Config) > 0 {
		configMap := make(map[string]cty.Value)
		for k, v := range authMethod.Config {
			configMap[k] = cty.StringVal(v)
		}
		dataMap["config"] = cty.ObjectVal(configMap)
	}

	return cty.ObjectVal(dataMap), nil
}

// buildSecretEngineData builds the API request data for a secret engine from structured config.
func buildSecretEngineData(secretEngine *openbaov1alpha1.SelfInitSecretEngine) (cty.Value, error) {
	if secretEngine == nil {
		return cty.NilVal, fmt.Errorf("secret engine config is nil")
	}

	dataMap := make(map[string]cty.Value)
	dataMap["type"] = cty.StringVal(secretEngine.Type)

	if secretEngine.Description != "" {
		dataMap["description"] = cty.StringVal(secretEngine.Description)
	}

	if len(secretEngine.Options) > 0 {
		optionsMap := make(map[string]cty.Value)
		for k, v := range secretEngine.Options {
			optionsMap[k] = cty.StringVal(v)
		}
		dataMap["options"] = cty.ObjectVal(optionsMap)
	}

	return cty.ObjectVal(dataMap), nil
}

// buildPolicyData builds the API request data for a policy from structured config.
func buildPolicyData(policy *openbaov1alpha1.SelfInitPolicy) (cty.Value, error) {
	if policy == nil {
		return cty.NilVal, fmt.Errorf("policy config is nil")
	}

	if policy.Policy == "" {
		return cty.NilVal, fmt.Errorf("policy content is required")
	}

	dataMap := make(map[string]cty.Value)
	dataMap["policy"] = cty.StringVal(policy.Policy)

	return cty.ObjectVal(dataMap), nil
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
			if val[k] == nil {
				// OpenBao's HCL parsing does not accept the HCL "null" literal.
				// Treat JSON null values as "unset" and omit them from the rendered output.
				continue
			}

			child, err := jsonToCty(val[k])
			if err != nil {
				return cty.NilVal, err
			}
			if child != cty.NilVal {
				obj[k] = child
			}
		}

		return cty.ObjectVal(obj), nil
	case []interface{}:
		if len(val) == 0 {
			return cty.EmptyTupleVal, nil
		}

		elems := make([]cty.Value, 0, len(val))
		for _, elem := range val {
			if elem == nil {
				// See map case above: omit JSON null elements.
				continue
			}
			child, err := jsonToCty(elem)
			if err != nil {
				return cty.NilVal, err
			}
			if child != cty.NilVal {
				elems = append(elems, child)
			}
		}

		if len(elems) == 0 {
			return cty.EmptyTupleVal, nil
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
		// OpenBao's HCL parsing does not accept the HCL "null" literal.
		// Treat JSON null values as "unset" and omit them from the rendered output.
		return cty.NilVal, nil
	default:
		return cty.NilVal, fmt.Errorf("unsupported JSON value type %T in self-init data", v)
	}
}
