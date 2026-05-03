package config

import (
	"encoding/json"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	platformsemver "github.com/dc-tec/openbao-operator/internal/platform/semver"
)

func FuzzOpenBaoVersionAtLeast(f *testing.F) {
	seeds := []string{
		"",
		"2.4.4",
		"v2.4.4",
		"2.5.0-rc.1",
		"2.5.0+build.1",
		"999.999.999",
		"not-a-version",
		"1.2",
		"1.2.3.4",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, version string) {
		_, _ = openBaoVersionAtLeast(version, 2, 5, 0)
	})
}

func FuzzJSONToCty(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{}`),
		[]byte(`{"policy":"path \"sys/health\" { capabilities = [\"read\"] }"}`),
		[]byte(`{"headers":{"X-Test":"1"},"enabled":true,"count":2}`),
		[]byte(`["a",1,true,{"nested":"value"}]`),
		[]byte(`null`),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32*1024 {
			t.Skip()
		}
		if !json.Valid(raw) {
			t.Skip()
		}

		var decoded interface{}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Skip()
		}

		_, _ = jsonToCty(decoded)
	})
}

func FuzzRenderSelfInitHCL(f *testing.F) {
	seeds := []struct {
		name         string
		path         string
		operation    string
		allowFailure bool
		data         string
		selfInitOn   bool
		bootstrapOn  bool
		clusterName  string
		clusterNS    string
		clusterVer   string
	}{
		{
			name:         "configure-autopilot",
			path:         "sys/storage/raft/autopilot/configuration",
			operation:    string(openbaov1alpha1.SelfInitOperationUpdate),
			allowFailure: false,
			data:         `{"cleanup_dead_servers":true,"min_quorum":3}`,
			selfInitOn:   true,
			bootstrapOn:  false,
			clusterName:  "demo",
			clusterNS:    "default",
			clusterVer:   "2.5.0",
		},
		{
			name:         "policy",
			path:         "sys/policies/acl/example",
			operation:    string(openbaov1alpha1.SelfInitOperationUpdate),
			allowFailure: true,
			data:         `{"policy":"path \"sys/health\" { capabilities = [\"read\"] }"}`,
			selfInitOn:   true,
			bootstrapOn:  true,
			clusterName:  "bootstrap",
			clusterNS:    "operators",
			clusterVer:   "2.5.0",
		},
		{
			name:         "",
			path:         "",
			operation:    string(openbaov1alpha1.SelfInitOperationCreate),
			allowFailure: false,
			data:         `{}`,
			selfInitOn:   false,
			bootstrapOn:  false,
			clusterName:  "empty",
			clusterNS:    "default",
			clusterVer:   "2.4.4",
		},
	}
	for _, seed := range seeds {
		f.Add(
			seed.name,
			seed.path,
			seed.operation,
			seed.allowFailure,
			seed.data,
			seed.selfInitOn,
			seed.bootstrapOn,
			seed.clusterName,
			seed.clusterNS,
			seed.clusterVer,
		)
	}

	f.Fuzz(func(t *testing.T, name, path, operation string, allowFailure bool, data string, selfInitOn, bootstrapOn bool, clusterName, clusterNS, clusterVer string) {
		if len(name) > 256 || len(path) > 512 || len(operation) > 64 || len(data) > 8*1024 || len(clusterName) > 128 || len(clusterNS) > 128 || len(clusterVer) > 64 {
			t.Skip()
		}
		if strings.Count(data, "{")+strings.Count(data, "[") > 128 {
			t.Skip()
		}

		cluster := newMinimalCluster(sanitizeCRToken(clusterName, "cluster"), sanitizeCRToken(clusterNS, "default"))
		cluster.Spec.Version = sanitizeVersion(clusterVer)
		cluster.Spec.Image = "openbao/openbao:" + cluster.Spec.Version

		if selfInitOn {
			req := openbaov1alpha1.SelfInitRequest{
				Name:         name,
				Path:         path,
				AllowFailure: allowFailure,
			}
			switch operation {
			case string(openbaov1alpha1.SelfInitOperationCreate):
				req.Operation = openbaov1alpha1.SelfInitOperationCreate
			case string(openbaov1alpha1.SelfInitOperationDelete):
				req.Operation = openbaov1alpha1.SelfInitOperationDelete
			default:
				req.Operation = openbaov1alpha1.SelfInitOperationUpdate
			}

			if json.Valid([]byte(data)) {
				req.Data = &apiextensionsv1.JSON{Raw: []byte(data)}
			}

			cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
				Enabled:  true,
				Requests: []openbaov1alpha1.SelfInitRequest{req},
			}
		}

		var bootstrap *OperatorBootstrapConfig
		if bootstrapOn {
			bootstrap = &OperatorBootstrapConfig{
				OIDCIssuerURL:   "https://issuer.example.test",
				JWTKeysPEM:      []string{"-----BEGIN PUBLIC KEY-----\nZmFrZQ==\n-----END PUBLIC KEY-----"},
				OperatorNS:      "openbao-operator-system",
				OperatorSA:      "openbao-operator-controller",
				JWTAuthAudience: "vault",
			}
		}

		_, _ = RenderSelfInitHCL(cluster, bootstrap)
	})
}

func sanitizeCRToken(v, fallback string) string {
	if v == "" {
		return fallback
	}

	out := make([]byte, 0, len(v))
	for i := 0; i < len(v); i++ {
		ch := v[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			out = append(out, ch)
			continue
		}
		if ch >= 'A' && ch <= 'Z' {
			out = append(out, ch+('a'-'A'))
			continue
		}
		out = append(out, '-')
	}
	if len(out) == 0 {
		return fallback
	}
	return string(out)
}

func sanitizeVersion(v string) string {
	if _, err := platformsemver.Parse(v); err == nil {
		return v
	}
	return "2.5.0"
}

func FuzzRenderHCL(f *testing.F) {
	seeds := []struct {
		clusterName   string
		clusterNS     string
		clusterVer    string
		headless      string
		infraNS       string
		apiPort       int
		clusterPort   int
		logLevel      string
		defaultTTL    string
		maxTTL        string
		pluginMode    string
		auditType     string
		auditPath     string
		rawOptions    string
		telemetryAddr string
		enableObs     bool
	}{
		{"demo", "default", "2.5.0", "demo", "default", 8200, 8201, "info", "3600h", "7200h", "direct", "file", "stdout", `{"file_path":"stdout"}`, "127.0.0.1:8125", true},
		{"acme", "operators", "2.4.4", "acme", "operators", 8200, 8201, "debug", "", "", "", "socket", "custom-socket", `{"address":"127.0.0.1:9000"}`, "", false},
		{"broken", "default", "not-a-version", "", "", 0, -1, "", "", "", "", "", "", `{}`, "", false},
	}
	for _, seed := range seeds {
		f.Add(
			seed.clusterName,
			seed.clusterNS,
			seed.clusterVer,
			seed.headless,
			seed.infraNS,
			seed.apiPort,
			seed.clusterPort,
			seed.logLevel,
			seed.defaultTTL,
			seed.maxTTL,
			seed.pluginMode,
			seed.auditType,
			seed.auditPath,
			seed.rawOptions,
			seed.telemetryAddr,
			seed.enableObs,
		)
	}

	f.Fuzz(func(t *testing.T, clusterName, clusterNS, clusterVer, headless, infraNS string, apiPort, clusterPort int, logLevel, defaultTTL, maxTTL, pluginMode, auditType, auditPath, rawOptions, telemetryAddr string, enableObs bool) {
		if len(clusterName) > 128 || len(clusterNS) > 128 || len(clusterVer) > 64 || len(headless) > 128 || len(infraNS) > 128 || len(logLevel) > 64 || len(defaultTTL) > 64 || len(maxTTL) > 64 || len(pluginMode) > 64 || len(auditType) > 64 || len(auditPath) > 128 || len(rawOptions) > 8*1024 || len(telemetryAddr) > 256 {
			t.Skip()
		}
		if apiPort < -1 || apiPort > 65535 || clusterPort < -1 || clusterPort > 65535 {
			t.Skip()
		}

		cluster := newMinimalCluster(sanitizeCRToken(clusterName, "cluster"), sanitizeCRToken(clusterNS, "default"))
		cluster.Spec.Version = sanitizeVersion(clusterVer)
		cluster.Spec.Image = "openbao/openbao:" + cluster.Spec.Version
		cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{
			LogLevel:        logLevel,
			DefaultLeaseTTL: defaultTTL,
			MaxLeaseTTL:     maxTTL,
		}

		if pluginMode != "" {
			autoDownload := true
			autoRegister := false
			cluster.Spec.Configuration.Plugin = &openbaov1alpha1.PluginConfig{
				AutoDownload:     &autoDownload,
				AutoRegister:     &autoRegister,
				DownloadBehavior: pluginMode,
			}
		}

		if auditType != "" && auditPath != "" {
			device := openbaov1alpha1.AuditDevice{
				Type:        auditType,
				Path:        auditPath,
				Description: "fuzzed",
			}
			if json.Valid([]byte(rawOptions)) {
				device.Options = &apiextensionsv1.JSON{Raw: []byte(rawOptions)}
			}
			cluster.Spec.Audit = []openbaov1alpha1.AuditDevice{device}
		}

		if telemetryAddr != "" {
			cluster.Spec.Telemetry = &openbaov1alpha1.TelemetryConfig{
				DogStatsdAddress: telemetryAddr,
				DisableHostname:  true,
			}
		}
		if enableObs {
			cluster.Spec.Observability = &openbaov1alpha1.ObservabilityConfig{
				Metrics: &openbaov1alpha1.MetricsConfig{Enabled: true},
			}
		}

		infra := InfrastructureDetails{
			HeadlessServiceName: sanitizeCRToken(headless, cluster.Name),
			Namespace:           sanitizeCRToken(infraNS, cluster.Namespace),
			APIPort:             apiPort,
			ClusterPort:         clusterPort,
		}

		_, _ = RenderHCL(cluster, infra)
	})
}

func FuzzResolveSelfInitRequestStructuredData(f *testing.F) {
	seeds := []struct {
		name        string
		path        string
		auditType   string
		authType    string
		secretType  string
		policy      string
		headersJSON string
		configKey   string
		configValue string
	}{
		{"audit", "sys/audit/stdout", "file", "", "", "", `{"X-Test":"1"}`, "", ""},
		{"auth", "sys/auth/jwt", "", "jwt", "", "", `{}`, "default_role", "operator"},
		{"mount", "sys/mounts/kv", "", "", "kv", "", `{}`, "version", "2"},
		{"policy", "sys/policies/acl/example", "", "", "", `path "sys/health" { capabilities = ["read"] }`, `{}`, "", ""},
		{"unknown", "sys/unknown/path", "", "", "", "", `{}`, "", ""},
	}
	for _, seed := range seeds {
		f.Add(seed.name, seed.path, seed.auditType, seed.authType, seed.secretType, seed.policy, seed.headersJSON, seed.configKey, seed.configValue)
	}

	f.Fuzz(func(t *testing.T, name, path, auditType, authType, secretType, policy, headersJSON, configKey, configValue string) {
		if len(name) > 128 || len(path) > 256 || len(auditType) > 64 || len(authType) > 64 || len(secretType) > 64 || len(policy) > 8*1024 || len(headersJSON) > 8*1024 || len(configKey) > 128 || len(configValue) > 256 {
			t.Skip()
		}

		req := openbaov1alpha1.SelfInitRequest{
			Name: sanitizeCRToken(name, "req"),
			Path: path,
		}

		if stringsHasPrefix(path, "sys/audit/") {
			req.AuditDevice = &openbaov1alpha1.SelfInitAuditDevice{
				Type:        auditType,
				Description: "fuzzed",
				FileOptions: &openbaov1alpha1.FileAuditOptions{
					FilePath: "stdout",
				},
				HTTPOptions: &openbaov1alpha1.HTTPAuditOptions{
					URI: "https://example.test",
				},
			}
			if json.Valid([]byte(headersJSON)) {
				req.AuditDevice.HTTPOptions.Headers = &apiextensionsv1.JSON{Raw: []byte(headersJSON)}
			}
		}

		if stringsHasPrefix(path, "sys/auth/") {
			req.AuthMethod = &openbaov1alpha1.SelfInitAuthMethod{
				Type: authType,
				Config: map[string]string{
					configKey: configValue,
				},
			}
		}

		if stringsHasPrefix(path, "sys/mounts/") {
			req.SecretEngine = &openbaov1alpha1.SelfInitSecretEngine{
				Type: secretType,
				Options: map[string]string{
					configKey: configValue,
				},
			}
		}

		if stringsHasPrefix(path, "sys/policies/") {
			req.Policy = &openbaov1alpha1.SelfInitPolicy{
				Policy: policy,
			}
		}

		_, _, _ = resolveSelfInitRequestStructuredData(req)
	})
}

func stringsHasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}
