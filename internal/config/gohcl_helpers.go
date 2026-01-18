package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

type hclCoreAttributes struct {
	UI              bool   `hcl:"ui"`
	ClusterName     string `hcl:"cluster_name"`
	APIAddr         string `hcl:"api_addr"`
	ClusterAddr     string `hcl:"cluster_addr"`
	PluginDirectory string `hcl:"plugin_directory"`
}

type hclListenerTCP struct {
	Type               string `hcl:"type,label"`
	Address            string `hcl:"address"`
	ClusterAddress     string `hcl:"cluster_address"`
	TLSDisable         int    `hcl:"tls_disable"`
	MaxRequestDuration string `hcl:"max_request_duration"`

	ProxyProtocolBehavior *string `hcl:"proxy_protocol_behavior"`

	TLSCertFile                 *string   `hcl:"tls_cert_file"`
	TLSKeyFile                  *string   `hcl:"tls_key_file"`
	TLSClientCAFile             *string   `hcl:"tls_client_ca_file"`
	TLSACMECADir                *string   `hcl:"tls_acme_ca_directory"`
	TLSACMEDomains              *[]string `hcl:"tls_acme_domains"`
	TLSACMEEmail                *string   `hcl:"tls_acme_email"`
	TLSACMEDisableHTTPChallenge *bool     `hcl:"tls_acme_disable_http_challenge"`
	TLSACMECARoot               *string   `hcl:"tls_acme_ca_root"`
}

type hclStorageRaft struct {
	Type                  string `hcl:"type,label"`
	Path                  string `hcl:"path"`
	NodeID                string `hcl:"node_id"`
	PerformanceMultiplier *int32 `hcl:"performance_multiplier"`

	RetryJoinAsNonVoter *bool   `hcl:"retry_join_as_non_voter"`
	ElectionTimeout     *string `hcl:"election_timeout"`
}

type hclRetryJoin struct {
	AutoJoin             string  `hcl:"auto_join"`
	LeaderTLSServerName  string  `hcl:"leader_tls_servername"`
	LeaderCACertFile     *string `hcl:"leader_ca_cert_file"`
	LeaderClientCertFile *string `hcl:"leader_client_cert_file"`
	LeaderClientKeyFile  *string `hcl:"leader_client_key_file"`
}

type hclAuditDevice struct {
	Type string `hcl:"type,label"`
	Path string `hcl:"path,label"`

	Description *string `hcl:"description"`
}

type hclPlugin struct {
	Type string `hcl:"type,label"`
	Name string `hcl:"name,label"`

	Image   *string `hcl:"image"`
	Command *string `hcl:"command"`

	Version    string `hcl:"version"`
	BinaryName string `hcl:"binary_name"`
	SHA256Sum  string `hcl:"sha256sum"`

	Args *[]string `hcl:"args"`
	Env  *[]string `hcl:"env"`
}

type hclTelemetry struct {
	UsageGaugePeriod        *string `hcl:"usage_gauge_period"`
	MaximumGaugeCardinality *int32  `hcl:"maximum_gauge_cardinality"`
	DisableHostname         *bool   `hcl:"disable_hostname"`
	EnableHostnameLabel     *bool   `hcl:"enable_hostname_label"`
	MetricsPrefix           *string `hcl:"metrics_prefix"`
	LeaseMetricsEpsilon     *string `hcl:"lease_metrics_epsilon"`

	PrometheusRetentionTime *string `hcl:"prometheus_retention_time"`

	StatsiteAddress *string `hcl:"statsite_address"`
	StatsdAddress   *string `hcl:"statsd_address"`

	DogStatsdAddress *string   `hcl:"dogstatsd_addr"`
	DogStatsdTags    *[]string `hcl:"dogstatsd_tags"`

	CirconusAPIKey                     *string `hcl:"circonus_api_token"`
	CirconusAPIApp                     *string `hcl:"circonus_api_app"`
	CirconusAPIURL                     *string `hcl:"circonus_api_url"`
	CirconusSubmissionInterval         *string `hcl:"circonus_submission_interval"`
	CirconusCheckID                    *string `hcl:"circonus_check_id"`
	CirconusCheckForceMetricActivation *string `hcl:"circonus_check_force_metric_activation"`
	CirconusCheckInstanceID            *string `hcl:"circonus_check_instance_id"`
	CirconusCheckSearchTag             *string `hcl:"circonus_check_search_tag"`
	CirconusCheckDisplayName           *string `hcl:"circonus_check_display_name"`
	CirconusCheckTags                  *string `hcl:"circonus_check_tags"`
	CirconusBrokerID                   *string `hcl:"circonus_broker_id"`
	CirconusBrokerSelectTag            *string `hcl:"circonus_broker_select_tag"`

	StackdriverProjectID *string `hcl:"stackdriver_project_id"`
	StackdriverLocation  *string `hcl:"stackdriver_location"`
	StackdriverNamespace *string `hcl:"stackdriver_namespace"`
	StackdriverDebugLogs *bool   `hcl:"stackdriver_debug_logs"`
}

type hclUserConfigurationAttributes struct {
	LogLevel *string `hcl:"log_level"`

	LogFormat                      *string `hcl:"log_format"`
	LogFile                        *string `hcl:"log_file"`
	LogRotateDuration              *string `hcl:"log_rotate_duration"`
	LogRotateBytes                 *int64  `hcl:"log_rotate_bytes"`
	LogRotateMaxFiles              *int32  `hcl:"log_rotate_max_files"`
	PIDFile                        *string `hcl:"pid_file"`
	PluginFileUID                  *int64  `hcl:"plugin_file_uid"`
	PluginFilePerms                *string `hcl:"plugin_file_permissions"`
	PluginAutoDownload             *bool   `hcl:"plugin_auto_download"`
	PluginAutoRegister             *bool   `hcl:"plugin_auto_register"`
	PluginDownloadMode             *string `hcl:"plugin_download_behavior"`
	DefaultLeaseTTL                *string `hcl:"default_lease_ttl"`
	MaxLeaseTTL                    *string `hcl:"max_lease_ttl"`
	CacheSize                      *int64  `hcl:"cache_size"`
	DisableCache                   *bool   `hcl:"disable_cache"`
	DetectDeadlocks                *string `hcl:"detect_deadlocks"`
	RawStorageEndpoint             *bool   `hcl:"raw_storage_endpoint"`
	Introspection                  *bool   `hcl:"introspection_endpoint"`
	ImpreciseLeaseRoleTracking     *bool   `hcl:"imprecise_lease_role_tracking"`
	UnsafeAllowAPIAuditCreation    *bool   `hcl:"unsafe_allow_api_audit_creation"`
	AllowAuditLogPrefixing         *bool   `hcl:"allow_audit_log_prefixing"`
	EnableResponseHeaderHostname   *bool   `hcl:"enable_response_header_hostname"`
	EnableResponseHeaderRaftNodeID *bool   `hcl:"enable_response_header_raft_node_id"`
}

type hclSealStatic struct {
	Type         string `hcl:"type,label"`
	CurrentKey   string `hcl:"current_key"`
	CurrentKeyID string `hcl:"current_key_id"`
}

type hclSealTransit struct {
	Type string `hcl:"type,label"`

	Address        string  `hcl:"address"`
	Token          *string `hcl:"token"`
	KeyName        string  `hcl:"key_name"`
	MountPath      string  `hcl:"mount_path"`
	Namespace      *string `hcl:"namespace"`
	DisableRenewal *string `hcl:"disable_renewal"`
	TLSCACert      *string `hcl:"tls_ca_cert"`
	TLSClientCert  *string `hcl:"tls_client_cert"`
	TLSClientKey   *string `hcl:"tls_client_key"`
	TLSServerName  *string `hcl:"tls_server_name"`
	TLSSkipVerify  *string `hcl:"tls_skip_verify"`
}

type hclSealAWSKMS struct {
	Type string `hcl:"type,label"`

	Region       string  `hcl:"region"`
	KMSKeyID     string  `hcl:"kms_key_id"`
	Endpoint     *string `hcl:"endpoint"`
	AccessKey    *string `hcl:"access_key"`
	SecretKey    *string `hcl:"secret_key"`
	SessionToken *string `hcl:"session_token"`
}

type hclSealAzureKeyVault struct {
	Type string `hcl:"type,label"`

	VaultName    string  `hcl:"vault_name"`
	KeyName      string  `hcl:"key_name"`
	TenantID     *string `hcl:"tenant_id"`
	ClientID     *string `hcl:"client_id"`
	ClientSecret *string `hcl:"client_secret"`
	Resource     *string `hcl:"resource"`
	Environment  *string `hcl:"environment"`
}

type hclSealGCPCloudKMS struct {
	Type string `hcl:"type,label"`

	Project     string  `hcl:"project"`
	Region      string  `hcl:"region"`
	KeyRing     string  `hcl:"key_ring"`
	CryptoKey   string  `hcl:"crypto_key"`
	Credentials *string `hcl:"credentials"`
}

type hclSealKMIP struct {
	Type string `hcl:"type,label"`

	Address       string  `hcl:"address"`
	Certificate   *string `hcl:"certificate"`
	Key           *string `hcl:"key"`
	CACert        *string `hcl:"ca_cert"`
	TLSServerName *string `hcl:"tls_server_name"`
	TLSSkipVerify *string `hcl:"tls_skip_verify"`
}

type hclSealOCIKMS struct {
	Type string `hcl:"type,label"`

	KeyID              string  `hcl:"key_id"`
	CryptoEndpoint     string  `hcl:"crypto_endpoint"`
	ManagementEndpoint string  `hcl:"management_endpoint"`
	AuthType           *string `hcl:"auth_type"`
	CompartmentID      *string `hcl:"compartment_id"`
}

type hclSealPKCS11 struct {
	Type string `hcl:"type,label"`

	Lib             string  `hcl:"lib"`
	Slot            *string `hcl:"slot"`
	PIN             *string `hcl:"pin"`
	KeyLabel        string  `hcl:"key_label"`
	HMACKeyLabel    *string `hcl:"hmac_key_label"`
	GenerateKey     *string `hcl:"generate_key"`
	RSAEncryptLocal *string `hcl:"rsa_encrypt_local"`
	RSAOAEPHash     *string `hcl:"rsa_oaep_hash"`
}

func stringPtr(v string) *string {
	if v == "" {
		return nil
	}
	val := v
	return &val
}

func normalizeTrailingNewline(tokens hclwrite.Tokens) hclwrite.Tokens {
	for len(tokens) > 0 && tokens[len(tokens)-1].Type == hclsyntax.TokenNewline {
		tokens = tokens[:len(tokens)-1]
	}
	if len(tokens) == 0 {
		return tokens
	}
	return append(tokens, &hclwrite.Token{
		Type:  hclsyntax.TokenNewline,
		Bytes: []byte("\n"),
	})
}

func boolPtrString(b *bool) *string {
	if b == nil {
		return nil
	}
	val := fmt.Sprintf("%t", *b)
	return &val
}

func boolPtrTrue(v bool) *bool {
	if !v {
		return nil
	}
	return boolPtrValue(true)
}

func buildListenerBlock(cluster *openbaov1alpha1.OpenBaoCluster) (*hclwrite.Block, error) {
	listener := hclListenerTCP{
		Type:               "tcp",
		Address:            fmt.Sprintf("[::]:%d", constants.PortAPI),
		ClusterAddress:     fmt.Sprintf("[::]:%d", constants.PortCluster),
		TLSDisable:         0,
		MaxRequestDuration: configMaxRequestDuration,
	}

	if cluster.Spec.Configuration != nil && cluster.Spec.Configuration.Listener != nil {
		listener.ProxyProtocolBehavior = stringPtr(cluster.Spec.Configuration.Listener.ProxyProtocolBehavior)
		if cluster.Spec.Configuration.Listener.TLSDisable != nil {
			if *cluster.Spec.Configuration.Listener.TLSDisable {
				listener.TLSDisable = 1
			} else {
				listener.TLSDisable = 0
			}
		}
	}

	tlsMode := cluster.Spec.TLS.Mode
	if tlsMode == "" {
		tlsMode = openbaov1alpha1.TLSModeOperatorManaged
	}

	if tlsMode == openbaov1alpha1.TLSModeACME {
		if cluster.Spec.TLS.ACME == nil {
			return nil, fmt.Errorf("ACME configuration is required when tls.mode is ACME")
		}
		listener.TLSACMECADir = stringPtr(cluster.Spec.TLS.ACME.DirectoryURL)
		domains := []string{cluster.Spec.TLS.ACME.Domain}
		listener.TLSACMEDomains = &domains
		listener.TLSACMEEmail = stringPtr(cluster.Spec.TLS.ACME.Email)
		listener.TLSACMEDisableHTTPChallenge = boolPtrValue(true)
		if cluster.Spec.Configuration != nil {
			listener.TLSACMECARoot = stringPtr(cluster.Spec.Configuration.ACMECARoot)
		}
	} else {
		listener.TLSCertFile = stringPtr(constants.PathTLSServerCert)
		listener.TLSKeyFile = stringPtr(constants.PathTLSServerKey)
		listener.TLSClientCAFile = stringPtr(constants.PathTLSCACert)
	}

	return gohcl.EncodeAsBlock(listener, "listener"), nil
}

func buildStorageBlock(cluster *openbaov1alpha1.OpenBaoCluster, infra InfrastructureDetails) *hclwrite.Block {
	storageAttrs := hclStorageRaft{
		Type:   "raft",
		Path:   constants.PathData,
		NodeID: configNodeIDTemplate,
	}

	if cluster.Spec.Configuration != nil && cluster.Spec.Configuration.Raft != nil {
		storageAttrs.PerformanceMultiplier = cluster.Spec.Configuration.Raft.PerformanceMultiplier
	}

	var autoJoinExpr string
	if infra.TargetRevisionForJoin != "" {
		autoJoinExpr = fmt.Sprintf(
			`provider=k8s namespace=%s label_selector="%s=%s,%s=%s"`,
			infra.Namespace,
			constants.LabelOpenBaoCluster,
			cluster.Name,
			constants.LabelOpenBaoRevision,
			infra.TargetRevisionForJoin,
		)
		storageAttrs.RetryJoinAsNonVoter = boolPtrValue(true)
		storageAttrs.ElectionTimeout = stringPtr("30s")
	} else {
		autoJoinExpr = fmt.Sprintf(
			`provider=k8s namespace=%s label_selector="%s=%s"`,
			infra.Namespace,
			constants.LabelOpenBaoCluster,
			cluster.Name,
		)
	}

	retryJoinAttrs := hclRetryJoin{
		AutoJoin:            autoJoinExpr,
		LeaderTLSServerName: fmt.Sprintf("openbao-cluster-%s.local", cluster.Name),
	}

	if cluster.Spec.TLS.Mode != openbaov1alpha1.TLSModeACME {
		retryJoinAttrs.LeaderCACertFile = stringPtr(constants.PathTLSCACert)
		retryJoinAttrs.LeaderClientCertFile = stringPtr(constants.PathTLSServerCert)
		retryJoinAttrs.LeaderClientKeyFile = stringPtr(constants.PathTLSServerKey)
	}

	storageBlock := hclwrite.NewBlock("storage", []string{storageAttrs.Type})
	gohcl.EncodeIntoBody(storageAttrs, storageBlock.Body())

	retryJoinBlock := hclwrite.NewBlock("retry_join", nil)
	gohcl.EncodeIntoBody(retryJoinAttrs, retryJoinBlock.Body())
	storageBlock.Body().AppendBlock(retryJoinBlock)

	return storageBlock
}

func boolPtrValue(v bool) *bool {
	val := v
	return &val
}

func buildAuditDeviceBlocks(devices []openbaov1alpha1.AuditDevice) ([]*hclwrite.Block, error) {
	blocks := make([]*hclwrite.Block, 0, len(devices))
	for _, device := range devices {
		if device.Type == "" || device.Path == "" {
			continue
		}

		block := gohcl.EncodeAsBlock(hclAuditDevice{
			Type:        device.Type,
			Path:        device.Path,
			Description: stringPtr(device.Description),
		}, "audit")

		optionsVal, ok, err := buildAuditOptionsValue(device)
		if err != nil {
			return nil, err
		}
		if ok {
			block.Body().SetAttributeValue("options", optionsVal)
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func buildAuditOptionsValue(device openbaov1alpha1.AuditDevice) (cty.Value, bool, error) {
	var options map[string]cty.Value

	switch device.Type {
	case "file":
		if device.FileOptions != nil {
			options = make(map[string]cty.Value)
			options["file_path"] = cty.StringVal(device.FileOptions.FilePath)
			if device.FileOptions.Mode != "" {
				options["mode"] = cty.StringVal(device.FileOptions.Mode)
			}
		}
	case "http":
		if device.HTTPOptions != nil {
			options = make(map[string]cty.Value)
			options["uri"] = cty.StringVal(device.HTTPOptions.URI)
			if device.HTTPOptions.Headers != nil && len(device.HTTPOptions.Headers.Raw) > 0 {
				var decoded interface{}
				if err := json.Unmarshal(device.HTTPOptions.Headers.Raw, &decoded); err != nil {
					return cty.NilVal, false, fmt.Errorf("failed to decode HTTP audit device headers: %w", err)
				}
				headersVal, err := jsonToCty(decoded)
				if err != nil {
					return cty.NilVal, false, fmt.Errorf("failed to convert HTTP audit device headers to HCL: %w", err)
				}
				options["headers"] = headersVal
			}
		}
	case "syslog":
		if device.SyslogOptions != nil {
			options = make(map[string]cty.Value)
			if device.SyslogOptions.Facility != "" {
				options["facility"] = cty.StringVal(device.SyslogOptions.Facility)
			}
			if device.SyslogOptions.Tag != "" {
				options["tag"] = cty.StringVal(device.SyslogOptions.Tag)
			}
		}
	case "socket":
		if device.SocketOptions != nil {
			options = make(map[string]cty.Value)
			if device.SocketOptions.Address != "" {
				options["address"] = cty.StringVal(device.SocketOptions.Address)
			}
			if device.SocketOptions.SocketType != "" {
				options["socket_type"] = cty.StringVal(device.SocketOptions.SocketType)
			}
			if device.SocketOptions.WriteTimeout != "" {
				options["write_timeout"] = cty.StringVal(device.SocketOptions.WriteTimeout)
			}
		}
	}

	if len(options) > 0 {
		return cty.ObjectVal(options), true, nil
	}

	if device.Options != nil && len(device.Options.Raw) > 0 {
		var decoded any
		if err := json.Unmarshal(device.Options.Raw, &decoded); err != nil {
			return cty.NilVal, false, fmt.Errorf("failed to decode audit device options: %w", err)
		}

		ctyVal, err := jsonToCty(decoded)
		if err != nil {
			return cty.NilVal, false, fmt.Errorf("failed to convert audit device options to HCL: %w", err)
		}

		return ctyVal, true, nil
	}

	return cty.NilVal, false, nil
}

func buildPluginBlocks(plugins []openbaov1alpha1.Plugin) []*hclwrite.Block {
	blocks := make([]*hclwrite.Block, 0, len(plugins))
	for _, plugin := range plugins {
		if plugin.Type == "" || plugin.Name == "" {
			continue
		}

		var imagePtr *string
		var commandPtr *string
		if plugin.Image != "" {
			imagePtr = stringPtr(plugin.Image)
		} else if plugin.Command != "" {
			commandPtr = stringPtr(plugin.Command)
		}

		var argsPtr *[]string
		if len(plugin.Args) > 0 {
			args := append([]string(nil), plugin.Args...)
			argsPtr = &args
		}

		var envPtr *[]string
		if len(plugin.Env) > 0 {
			env := append([]string(nil), plugin.Env...)
			envPtr = &env
		}

		block := gohcl.EncodeAsBlock(hclPlugin{
			Type: plugin.Type,
			Name: plugin.Name,

			Image:   imagePtr,
			Command: commandPtr,

			Version:    plugin.Version,
			BinaryName: plugin.BinaryName,
			SHA256Sum:  plugin.SHA256Sum,

			Args: argsPtr,
			Env:  envPtr,
		}, "plugin")

		blocks = append(blocks, block)
	}
	return blocks
}

func buildTelemetryBlock(telemetry *openbaov1alpha1.TelemetryConfig) *hclwrite.Block {
	if telemetry == nil {
		return nil
	}

	var dogTagsPtr *[]string
	if len(telemetry.DogStatsdTags) > 0 {
		tags := append([]string(nil), telemetry.DogStatsdTags...)
		dogTagsPtr = &tags
	}

	return gohcl.EncodeAsBlock(hclTelemetry{
		UsageGaugePeriod:        stringPtr(telemetry.UsageGaugePeriod),
		MaximumGaugeCardinality: telemetry.MaximumGaugeCardinality,
		DisableHostname:         boolPtrTrue(telemetry.DisableHostname),
		EnableHostnameLabel:     boolPtrTrue(telemetry.EnableHostnameLabel),
		MetricsPrefix:           stringPtr(telemetry.MetricsPrefix),
		LeaseMetricsEpsilon:     stringPtr(telemetry.LeaseMetricsEpsilon),

		PrometheusRetentionTime: stringPtr(telemetry.PrometheusRetentionTime),

		StatsiteAddress: stringPtr(telemetry.StatsiteAddress),
		StatsdAddress:   stringPtr(telemetry.StatsdAddress),

		DogStatsdAddress: stringPtr(telemetry.DogStatsdAddress),
		DogStatsdTags:    dogTagsPtr,

		CirconusAPIKey:                     stringPtr(telemetry.CirconusAPIKey),
		CirconusAPIApp:                     stringPtr(telemetry.CirconusAPIApp),
		CirconusAPIURL:                     stringPtr(telemetry.CirconusAPIURL),
		CirconusSubmissionInterval:         stringPtr(telemetry.CirconusSubmissionInterval),
		CirconusCheckID:                    stringPtr(telemetry.CirconusCheckID),
		CirconusCheckForceMetricActivation: stringPtr(telemetry.CirconusCheckForceMetricActivation),
		CirconusCheckInstanceID:            stringPtr(telemetry.CirconusCheckInstanceID),
		CirconusCheckSearchTag:             stringPtr(telemetry.CirconusCheckSearchTag),
		CirconusCheckDisplayName:           stringPtr(telemetry.CirconusCheckDisplayName),
		CirconusCheckTags:                  stringPtr(telemetry.CirconusCheckTags),
		CirconusBrokerID:                   stringPtr(telemetry.CirconusBrokerID),
		CirconusBrokerSelectTag:            stringPtr(telemetry.CirconusBrokerSelectTag),

		StackdriverProjectID: stringPtr(telemetry.StackdriverProjectID),
		StackdriverLocation:  stringPtr(telemetry.StackdriverLocation),
		StackdriverNamespace: stringPtr(telemetry.StackdriverNamespace),
		StackdriverDebugLogs: boolPtrTrue(telemetry.StackdriverDebugLogs),
	}, "telemetry")
}

func buildSealBlock(cluster *openbaov1alpha1.OpenBaoCluster) (*hclwrite.Block, error) {
	unsealType := "static"
	if cluster.Spec.Unseal != nil && cluster.Spec.Unseal.Type != "" {
		unsealType = cluster.Spec.Unseal.Type
	}

	switch unsealType {
	case "static":
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
		return gohcl.EncodeAsBlock(hclSealStatic{
			Type:         "static",
			CurrentKey:   currentKey,
			CurrentKeyID: currentKeyID,
		}, "seal"), nil
	case "transit":
		if cluster.Spec.Unseal == nil || cluster.Spec.Unseal.Transit == nil {
			return nil, fmt.Errorf("unseal.transit is required when unseal.type is transit")
		}
		cfg := cluster.Spec.Unseal.Transit
		return gohcl.EncodeAsBlock(hclSealTransit{
			Type:           "transit",
			Address:        cfg.Address,
			Token:          stringPtr(cfg.Token),
			KeyName:        cfg.KeyName,
			MountPath:      cfg.MountPath,
			Namespace:      stringPtr(cfg.Namespace),
			DisableRenewal: boolPtrString(cfg.DisableRenewal),
			TLSCACert:      stringPtr(cfg.TLSCACert),
			TLSClientCert:  stringPtr(cfg.TLSClientCert),
			TLSClientKey:   stringPtr(cfg.TLSClientKey),
			TLSServerName:  stringPtr(cfg.TLSServerName),
			TLSSkipVerify:  boolPtrString(cfg.TLSSkipVerify),
		}, "seal"), nil
	case "awskms":
		if cluster.Spec.Unseal == nil || cluster.Spec.Unseal.AWSKMS == nil {
			return nil, fmt.Errorf("unseal.awskms is required when unseal.type is awskms")
		}
		cfg := cluster.Spec.Unseal.AWSKMS
		return gohcl.EncodeAsBlock(hclSealAWSKMS{
			Type:         "awskms",
			Region:       cfg.Region,
			KMSKeyID:     cfg.KMSKeyID,
			Endpoint:     stringPtr(cfg.Endpoint),
			AccessKey:    stringPtr(cfg.AccessKey),
			SecretKey:    stringPtr(cfg.SecretKey),
			SessionToken: stringPtr(cfg.SessionToken),
		}, "seal"), nil
	case "azurekeyvault":
		if cluster.Spec.Unseal == nil || cluster.Spec.Unseal.AzureKeyVault == nil {
			return nil, fmt.Errorf("unseal.azureKeyVault is required when unseal.type is azurekeyvault")
		}
		cfg := cluster.Spec.Unseal.AzureKeyVault
		return gohcl.EncodeAsBlock(hclSealAzureKeyVault{
			Type:         "azurekeyvault",
			VaultName:    cfg.VaultName,
			KeyName:      cfg.KeyName,
			TenantID:     stringPtr(cfg.TenantID),
			ClientID:     stringPtr(cfg.ClientID),
			ClientSecret: stringPtr(cfg.ClientSecret),
			Resource:     stringPtr(cfg.Resource),
			Environment:  stringPtr(cfg.Environment),
		}, "seal"), nil
	case "gcpckms":
		if cluster.Spec.Unseal == nil || cluster.Spec.Unseal.GCPCloudKMS == nil {
			return nil, fmt.Errorf("unseal.gcpCloudKMS is required when unseal.type is gcpckms")
		}
		cfg := cluster.Spec.Unseal.GCPCloudKMS
		return gohcl.EncodeAsBlock(hclSealGCPCloudKMS{
			Type:        "gcpckms",
			Project:     cfg.Project,
			Region:      cfg.Region,
			KeyRing:     cfg.KeyRing,
			CryptoKey:   cfg.CryptoKey,
			Credentials: stringPtr(cfg.Credentials),
		}, "seal"), nil
	case "kmip":
		if cluster.Spec.Unseal == nil || cluster.Spec.Unseal.KMIP == nil {
			return nil, fmt.Errorf("unseal.kmip is required when unseal.type is kmip")
		}
		cfg := cluster.Spec.Unseal.KMIP
		return gohcl.EncodeAsBlock(hclSealKMIP{
			Type:          "kmip",
			Address:       cfg.Address,
			Certificate:   stringPtr(cfg.Certificate),
			Key:           stringPtr(cfg.Key),
			CACert:        stringPtr(cfg.CACert),
			TLSServerName: stringPtr(cfg.TLSServerName),
			TLSSkipVerify: boolPtrString(cfg.TLSSkipVerify),
		}, "seal"), nil
	case "ocikms":
		if cluster.Spec.Unseal == nil || cluster.Spec.Unseal.OCIKMS == nil {
			return nil, fmt.Errorf("unseal.ocikms is required when unseal.type is ocikms")
		}
		cfg := cluster.Spec.Unseal.OCIKMS
		return gohcl.EncodeAsBlock(hclSealOCIKMS{
			Type:               "ocikms",
			KeyID:              cfg.KeyID,
			CryptoEndpoint:     cfg.CryptoEndpoint,
			ManagementEndpoint: cfg.ManagementEndpoint,
			AuthType:           stringPtr(cfg.AuthType),
			CompartmentID:      stringPtr(cfg.CompartmentID),
		}, "seal"), nil
	case "pkcs11":
		if cluster.Spec.Unseal == nil || cluster.Spec.Unseal.PKCS11 == nil {
			return nil, fmt.Errorf("unseal.pkcs11 is required when unseal.type is pkcs11")
		}
		cfg := cluster.Spec.Unseal.PKCS11
		return gohcl.EncodeAsBlock(hclSealPKCS11{
			Type:            "pkcs11",
			Lib:             cfg.Lib,
			Slot:            stringPtr(cfg.Slot),
			PIN:             stringPtr(cfg.PIN),
			KeyLabel:        cfg.KeyLabel,
			HMACKeyLabel:    stringPtr(cfg.HMACKeyLabel),
			GenerateKey:     boolPtrString(cfg.GenerateKey),
			RSAEncryptLocal: boolPtrString(cfg.RSAEncryptLocal),
			RSAOAEPHash:     stringPtr(cfg.RSAOAEPHash),
		}, "seal"), nil
	default:
		return nil, fmt.Errorf("unsupported unseal type %q", unsealType)
	}
}

func buildUserConfigTokens(config *openbaov1alpha1.OpenBaoConfiguration) hclwrite.Tokens {
	if config == nil {
		return nil
	}

	var attrs hclUserConfigurationAttributes
	attrs.LogLevel = stringPtr(config.LogLevel)
	if config.Logging != nil {
		attrs.LogFormat = stringPtr(config.Logging.Format)
		attrs.LogFile = stringPtr(config.Logging.File)
		attrs.LogRotateDuration = stringPtr(config.Logging.RotateDuration)
		attrs.LogRotateBytes = config.Logging.RotateBytes
		attrs.PIDFile = stringPtr(config.Logging.PIDFile)
		if config.Logging.RotateMaxFiles != nil {
			val := *config.Logging.RotateMaxFiles
			attrs.LogRotateMaxFiles = &val
		}
	}
	if config.Plugin != nil {
		attrs.PluginFileUID = config.Plugin.FileUID
		attrs.PluginFilePerms = stringPtr(config.Plugin.FilePermissions)
		attrs.PluginAutoDownload = config.Plugin.AutoDownload
		attrs.PluginAutoRegister = config.Plugin.AutoRegister
		attrs.PluginDownloadMode = stringPtr(config.Plugin.DownloadBehavior)
	}
	attrs.DefaultLeaseTTL = stringPtr(config.DefaultLeaseTTL)
	attrs.MaxLeaseTTL = stringPtr(config.MaxLeaseTTL)
	attrs.CacheSize = config.CacheSize
	attrs.DisableCache = config.DisableCache
	attrs.DetectDeadlocks = boolPtrString(config.DetectDeadlocks)
	attrs.RawStorageEndpoint = config.RawStorageEndpoint
	attrs.Introspection = config.IntrospectionEndpoint
	attrs.ImpreciseLeaseRoleTracking = config.ImpreciseLeaseRoleTracking
	attrs.UnsafeAllowAPIAuditCreation = config.UnsafeAllowAPIAuditCreation
	attrs.AllowAuditLogPrefixing = config.AllowAuditLogPrefixing
	attrs.EnableResponseHeaderHostname = config.EnableResponseHeaderHostname
	attrs.EnableResponseHeaderRaftNodeID = config.EnableResponseHeaderRaftNodeID

	tmpFile := hclwrite.NewEmptyFile()
	gohcl.EncodeIntoBody(attrs, tmpFile.Body())
	return normalizeTrailingNewline(tmpFile.Body().BuildTokens(nil))
}

func validateInfrastructureDetails(cluster *openbaov1alpha1.OpenBaoCluster, infra InfrastructureDetails) (InfrastructureDetails, error) {
	headlessSvcName := infra.HeadlessServiceName
	if strings.TrimSpace(headlessSvcName) == "" {
		headlessSvcName = cluster.Name
	}
	namespace := infra.Namespace
	if strings.TrimSpace(namespace) == "" {
		return InfrastructureDetails{}, fmt.Errorf("infrastructure namespace is required to render config.hcl")
	}

	infra.HeadlessServiceName = headlessSvcName
	infra.Namespace = namespace
	return infra, nil
}
