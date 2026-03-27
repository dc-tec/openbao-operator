package config

import (
	"fmt"
	"path"
	"strings"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
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
	TLSACMECachePath            *string   `hcl:"tls_acme_cache_path"`
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

	Endpoint     string  `hcl:"endpoint"`
	KMSKeyID     string  `hcl:"kms_key_id"`
	ClientCert   *string `hcl:"client_cert"`
	ClientKey    *string `hcl:"client_key"`
	CACert       *string `hcl:"ca_cert"`
	ServerName   *string `hcl:"server_name"`
	Timeout      *int32  `hcl:"timeout"`
	EncryptAlg   *string `hcl:"encrypt_alg"`
	TLS12Ciphers *string `hcl:"tls12_ciphers"`
	Disabled     *string `hcl:"disabled"`
}

type hclSealOCIKMS struct {
	Type string `hcl:"type,label"`

	KeyID              string  `hcl:"key_id"`
	CryptoEndpoint     string  `hcl:"crypto_endpoint"`
	ManagementEndpoint string  `hcl:"management_endpoint"`
	AuthTypeAPIKey     *bool   `hcl:"auth_type_api_key"`
	Disabled           *string `hcl:"disabled"`
}

type hclSealPKCS11 struct {
	Type string `hcl:"type,label"`

	Lib                       string  `hcl:"lib"`
	Slot                      *string `hcl:"slot"`
	TokenLabel                *string `hcl:"token_label"`
	PIN                       *string `hcl:"pin"`
	KeyLabel                  string  `hcl:"key_label"`
	KeyID                     *string `hcl:"key_id"`
	Mechanism                 *string `hcl:"mechanism"`
	DisableSoftwareEncryption *string `hcl:"disable_software_encryption"`
	Disabled                  *string `hcl:"disabled"`
	RSAOAEPHash               *string `hcl:"rsa_oaep_hash"`
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
		Address:            fmt.Sprintf("[::]:%d", openBaoAPIPort),
		ClusterAddress:     fmt.Sprintf("[::]:%d", openBaoClusterPort),
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
		if strings.TrimSpace(cluster.Spec.TLS.ACME.Domain) != "" && len(cluster.Spec.TLS.ACME.Domains) > 0 {
			return nil, fmt.Errorf("tls.acme.domain and tls.acme.domains are mutually exclusive; use only one")
		}
		listener.TLSACMECADir = stringPtr(cluster.Spec.TLS.ACME.DirectoryURL)
		domains := portopenbao.ComputeACMEDomains(cluster)
		listener.TLSACMEDomains = &domains
		listener.TLSACMEEmail = stringPtr(cluster.Spec.TLS.ACME.Email)
		listener.TLSACMECachePath = stringPtr(portopenbao.ACMESharedCachePath(cluster))
		listener.TLSACMEDisableHTTPChallenge = boolPtrValue(true)
		if cluster.Spec.Configuration != nil {
			listener.TLSACMECARoot = stringPtr(cluster.Spec.Configuration.ACMECARoot)
		}
	} else {
		listener.TLSCertFile = stringPtr(openBaoPathTLSServerCert)
		listener.TLSKeyFile = stringPtr(openBaoPathTLSServerKey)
		listener.TLSClientCAFile = stringPtr(openBaoPathTLSCACert)
	}

	return gohcl.EncodeAsBlock(listener, "listener"), nil
}

func buildStorageBlock(cluster *openbaov1alpha1.OpenBaoCluster, infra InfrastructureDetails) *hclwrite.Block {
	storageAttrs := hclStorageRaft{
		Type:   "raft",
		Path:   openBaoPathData,
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
			openBaoLabelCluster,
			cluster.Name,
			openBaoLabelRevision,
			infra.TargetRevisionForJoin,
		)
		storageAttrs.RetryJoinAsNonVoter = boolPtrValue(true)
		storageAttrs.ElectionTimeout = stringPtr("30s")
	} else {
		autoJoinExpr = fmt.Sprintf(
			`provider=k8s namespace=%s label_selector="%s=%s"`,
			infra.Namespace,
			openBaoLabelCluster,
			cluster.Name,
		)
	}

	retryJoinAttrs := hclRetryJoin{
		AutoJoin:            autoJoinExpr,
		LeaderTLSServerName: portopenbao.ComputeTLSServerName(cluster),
	}

	if cluster.Spec.TLS.Mode != openbaov1alpha1.TLSModeACME {
		retryJoinAttrs.LeaderCACertFile = stringPtr(openBaoPathTLSCACert)
		retryJoinAttrs.LeaderClientCertFile = stringPtr(openBaoPathTLSServerCert)
		retryJoinAttrs.LeaderClientKeyFile = stringPtr(openBaoPathTLSServerKey)
	} else if cluster.Spec.Configuration != nil && strings.TrimSpace(cluster.Spec.Configuration.ACMECARoot) != "" {
		// When using a private ACME CA (like infra-bao PKI), the issued leaf certificate is not
		// trusted by system roots. If tls_acme_ca_root is configured (to trust the ACME directory
		// server), we expect the PKI CA to be available alongside it as "pki-ca.crt" (same volume),
		// and use it for retry_join leader certificate verification.
		acmeCARootDir := path.Dir(strings.TrimSpace(cluster.Spec.Configuration.ACMECARoot))
		retryJoinAttrs.LeaderCACertFile = stringPtr(path.Join(acmeCARootDir, "pki-ca.crt"))
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
