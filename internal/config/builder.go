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
	"github.com/zclconf/go-cty/cty"
)

const (
	configPluginDirectoryPath = "/openbao/plugins"
	configStoragePath         = "/bao/data"
	configTLSCACertPath       = "/etc/bao/tls/ca.crt"
	configTLSCertPath         = "/etc/bao/tls/tls.crt"
	configTLSKeyPath          = "/etc/bao/tls/tls.key"
	configUnsealKeyPath       = "file:///etc/bao/unseal/key"
	configUnsealKeyID         = "operator-generated-v1"
	configListenerAddress     = "[::]:8200"
	configListenerClusterAddr = "[::]:8201"
	configMaxRequestDuration  = "90s"
	configNodeIDTemplate      = "${HOSTNAME}"
	autoJoinLabelKey          = "openbao.org/cluster"
)

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
	body.SetAttributeValue("ui", cty.BoolVal(true))
	body.SetAttributeValue("cluster_name", cty.StringVal(cluster.Name))

	apiAddr := fmt.Sprintf("https://${HOSTNAME}.%s.%s.svc:%d", headlessSvcName, namespace, infra.APIPort)
	clusterAddr := fmt.Sprintf("https://${HOSTNAME}.%s.%s.svc:%d", headlessSvcName, namespace, infra.ClusterPort)

	body.SetAttributeValue("api_addr", cty.StringVal(apiAddr))
	body.SetAttributeValue("cluster_addr", cty.StringVal(clusterAddr))
	body.SetAttributeValue("plugin_directory", cty.StringVal(configPluginDirectoryPath))

	// 2. Listener stanza (operator-owned).
	listenerBlock := body.AppendNewBlock("listener", []string{"tcp"})
	listenerBody := listenerBlock.Body()
	listenerBody.SetAttributeValue("address", cty.StringVal(configListenerAddress))
	listenerBody.SetAttributeValue("cluster_address", cty.StringVal(configListenerClusterAddr))
	listenerBody.SetAttributeValue("tls_disable", cty.NumberIntVal(0))
	listenerBody.SetAttributeValue("tls_cert_file", cty.StringVal(configTLSCertPath))
	listenerBody.SetAttributeValue("tls_key_file", cty.StringVal(configTLSKeyPath))
	listenerBody.SetAttributeValue("tls_client_ca_file", cty.StringVal(configTLSCACertPath))
	listenerBody.SetAttributeValue("max_request_duration", cty.StringVal(configMaxRequestDuration))

	// 3. Seal stanza (operator-owned, but type is configurable).
	if err := renderSealStanza(body, cluster); err != nil {
		return nil, fmt.Errorf("failed to render seal stanza: %w", err)
	}

	// 4. Storage stanza (Raft) with auto_join for dynamic cluster membership.
	storageBlock := body.AppendNewBlock("storage", []string{"raft"})
	storageBody := storageBlock.Body()
	storageBody.SetAttributeValue("path", cty.StringVal(configStoragePath))
	storageBody.SetAttributeValue("node_id", cty.StringVal(configNodeIDTemplate))

	// Use Kubernetes go-discover for dynamic cluster membership. This allows pods
	// to discover and join the Raft cluster based on labels, without requiring
	// a static list of peer addresses.
	autoJoinExpr := fmt.Sprintf(
		`provider=k8s namespace=%s label_selector="%s=%s"`,
		namespace,
		autoJoinLabelKey,
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
	retryJoinBody.SetAttributeValue("leader_ca_cert_file", cty.StringVal(configTLSCACertPath))
	retryJoinBody.SetAttributeValue("leader_client_cert_file", cty.StringVal(configTLSCertPath))
	retryJoinBody.SetAttributeValue("leader_client_key_file", cty.StringVal(configTLSKeyPath))

	// 5. User-defined overrides as simple string attributes.
	// Only keys that pass webhook validation (allowlist) will reach here.
	// The webhook ensures only valid OpenBao configuration parameters are allowed.
	if len(cluster.Spec.Config) > 0 {
		keys := make([]string, 0, len(cluster.Spec.Config))
		for k := range cluster.Spec.Config {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			value := strings.TrimSpace(cluster.Spec.Config[key])
			if value == "" {
				continue
			}

			// All keys here have already been validated by the webhook allowlist,
			// so we can safely add them to the configuration.
			body.SetAttributeValue(key, cty.StringVal(value))
		}
	}

	// 6. Render audit devices if configured
	if err := renderAuditDevices(body, cluster.Spec.Audit); err != nil {
		return nil, fmt.Errorf("failed to render audit devices: %w", err)
	}

	// 7. Render plugins if configured
	renderPlugins(body, cluster.Spec.Plugins)

	// 8. Render telemetry if configured
	renderTelemetry(body, cluster.Spec.Telemetry)

	// Note: Self-initialization stanzas are rendered separately via RenderSelfInitHCL
	// and stored in a separate ConfigMap that is only mounted for pod-0.

	return file.Bytes(), nil
}

// RenderSelfInitHCL renders only the self-initialization stanzas as a separate
// HCL configuration. This is stored in a separate ConfigMap that is only mounted
// for pod-0, since only the first pod needs to execute initialization requests.
func RenderSelfInitHCL(cluster *openbaov1alpha1.OpenBaoCluster) ([]byte, error) {
	if cluster.Spec.SelfInit == nil || !cluster.Spec.SelfInit.Enabled {
		// Return empty config if self-init is not enabled
		return []byte{}, nil
	}

	file := hclwrite.NewEmptyFile()
	body := file.Body()

	if err := renderSelfInitStanzas(body, cluster.Spec.SelfInit.Requests); err != nil {
		return nil, fmt.Errorf("failed to render self-init stanzas: %w", err)
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

// renderSealStanza renders the seal block based on the cluster's unseal configuration.
// If spec.unseal is nil or type is "static" (or empty), renders the default static seal.
// Otherwise, renders the specified seal type with options from spec.unseal.options.
func renderSealStanza(body *hclwrite.Body, cluster *openbaov1alpha1.OpenBaoCluster) error {
	unsealType := "static"
	if cluster.Spec.Unseal != nil && cluster.Spec.Unseal.Type != "" {
		unsealType = cluster.Spec.Unseal.Type
	}

	sealBlock := body.AppendNewBlock("seal", []string{unsealType})
	sealBody := sealBlock.Body()

	if unsealType == "static" {
		// Default static seal configuration (operator-managed)
		sealBody.SetAttributeValue("current_key", cty.StringVal(configUnsealKeyPath))
		sealBody.SetAttributeValue("current_key_id", cty.StringVal(configUnsealKeyID))
	} else {
		// External KMS seal - render options from spec.unseal.options
		if cluster.Spec.Unseal != nil && len(cluster.Spec.Unseal.Options) > 0 {
			// Sort keys for deterministic output
			keys := make([]string, 0, len(cluster.Spec.Unseal.Options))
			for k := range cluster.Spec.Unseal.Options {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, key := range keys {
				value := strings.TrimSpace(cluster.Spec.Unseal.Options[key])
				if value == "" {
					continue
				}
				sealBody.SetAttributeValue(key, cty.StringVal(value))
			}
		}
	}

	return nil
}
