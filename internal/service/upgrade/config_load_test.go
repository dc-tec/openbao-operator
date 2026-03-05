package upgrade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

var executorConfigEnvKeys = []string{
	constants.EnvClusterNamespace,
	constants.EnvClusterName,
	constants.EnvClusterReplicas,
	constants.EnvUpgradeAction,
	constants.EnvUpgradeJWTAuthRole,
	constants.EnvJWTTokenPath,
	constants.EnvTLSCAPath,
	constants.EnvUpgradeBlueRevision,
	constants.EnvUpgradeGreenRevision,
	constants.EnvUpgradeSyncThreshold,
	constants.EnvUpgradeTimeout,
	constants.EnvClientQPS,
	constants.EnvClientBurst,
	constants.EnvClientCircuitBreakerFailureThreshold,
	constants.EnvClientCircuitBreakerOpenDuration,
}

func TestLoadExecutorConfig(t *testing.T) {
	tmpDir := t.TempDir()
	jwtPath := writeUpgradeTestFile(t, tmpDir, "upgrade.jwt", "  my-token  \n")
	caPath := writeUpgradeTestFile(t, tmpDir, "ca.crt", "test-ca-data\n")

	baseEnv := map[string]string{
		constants.EnvClusterNamespace:                     "default",
		constants.EnvClusterName:                          "openbao",
		constants.EnvClusterReplicas:                      "3",
		constants.EnvUpgradeAction:                        string(ExecutorActionBlueGreenRepairConsensus),
		constants.EnvUpgradeJWTAuthRole:                   "upgrade-role",
		constants.EnvJWTTokenPath:                         jwtPath,
		constants.EnvTLSCAPath:                            caPath,
		constants.EnvUpgradeBlueRevision:                  "rev-blue",
		constants.EnvUpgradeGreenRevision:                 "rev-green",
		constants.EnvUpgradeSyncThreshold:                 "100",
		constants.EnvUpgradeTimeout:                       "10m",
		constants.EnvClientQPS:                            "2.5",
		constants.EnvClientBurst:                          "10",
		constants.EnvClientCircuitBreakerFailureThreshold: "7",
		constants.EnvClientCircuitBreakerOpenDuration:     "45s",
	}

	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantErr string
		assert  func(*testing.T, *ExecutorConfig)
	}{
		{
			name: "missing namespace",
			mutate: func(env map[string]string) {
				env[constants.EnvClusterNamespace] = "  "
			},
			wantErr: constants.EnvClusterNamespace + " environment variable is required",
		},
		{
			name: "missing cluster name",
			mutate: func(env map[string]string) {
				env[constants.EnvClusterName] = ""
			},
			wantErr: constants.EnvClusterName + " environment variable is required",
		},
		{
			name: "invalid replicas",
			mutate: func(env map[string]string) {
				env[constants.EnvClusterReplicas] = "three"
			},
			wantErr: "invalid " + constants.EnvClusterReplicas,
		},
		{
			name: "missing action",
			mutate: func(env map[string]string) {
				env[constants.EnvUpgradeAction] = ""
			},
			wantErr: constants.EnvUpgradeAction + " environment variable is required",
		},
		{
			name: "missing jwt auth role",
			mutate: func(env map[string]string) {
				env[constants.EnvUpgradeJWTAuthRole] = ""
			},
			wantErr: constants.EnvUpgradeJWTAuthRole + " environment variable is required",
		},
		{
			name: "missing jwt token file",
			mutate: func(env map[string]string) {
				env[constants.EnvJWTTokenPath] = filepath.Join(tmpDir, "missing.jwt")
			},
			wantErr: "failed to read JWT token",
		},
		{
			name: "missing ca file",
			mutate: func(env map[string]string) {
				env[constants.EnvTLSCAPath] = filepath.Join(tmpDir, "missing-ca.crt")
			},
			wantErr: "failed to read TLS CA certificate",
		},
		{
			name: "invalid sync threshold",
			mutate: func(env map[string]string) {
				env[constants.EnvUpgradeSyncThreshold] = "bad"
			},
			wantErr: "invalid " + constants.EnvUpgradeSyncThreshold,
		},
		{
			name: "invalid timeout",
			mutate: func(env map[string]string) {
				env[constants.EnvUpgradeTimeout] = "forever"
			},
			wantErr: "invalid " + constants.EnvUpgradeTimeout,
		},
		{
			name: "invalid client qps",
			mutate: func(env map[string]string) {
				env[constants.EnvClientQPS] = "qps"
			},
			wantErr: "invalid " + constants.EnvClientQPS,
		},
		{
			name: "invalid client burst",
			mutate: func(env map[string]string) {
				env[constants.EnvClientBurst] = "burst"
			},
			wantErr: "invalid " + constants.EnvClientBurst,
		},
		{
			name: "invalid circuit breaker threshold",
			mutate: func(env map[string]string) {
				env[constants.EnvClientCircuitBreakerFailureThreshold] = "oops"
			},
			wantErr: "invalid " + constants.EnvClientCircuitBreakerFailureThreshold,
		},
		{
			name: "invalid circuit breaker duration",
			mutate: func(env map[string]string) {
				env[constants.EnvClientCircuitBreakerOpenDuration] = "not-a-duration"
			},
			wantErr: "invalid " + constants.EnvClientCircuitBreakerOpenDuration,
		},
		{
			name: "loads defaults when optional values omitted",
			mutate: func(env map[string]string) {
				delete(env, constants.EnvUpgradeSyncThreshold)
				delete(env, constants.EnvUpgradeTimeout)
				delete(env, constants.EnvClientQPS)
				delete(env, constants.EnvClientBurst)
				delete(env, constants.EnvClientCircuitBreakerFailureThreshold)
				delete(env, constants.EnvClientCircuitBreakerOpenDuration)
			},
			assert: func(t *testing.T, cfg *ExecutorConfig) {
				t.Helper()
				if cfg.SyncThreshold != 100 {
					t.Fatalf("SyncThreshold=%d, want 100", cfg.SyncThreshold)
				}
				if cfg.Timeout != 10*time.Minute {
					t.Fatalf("Timeout=%s, want 10m0s", cfg.Timeout)
				}
				if cfg.ClientQPS != 0 {
					t.Fatalf("ClientQPS=%f, want 0", cfg.ClientQPS)
				}
				if cfg.ClientBurst != 0 {
					t.Fatalf("ClientBurst=%d, want 0", cfg.ClientBurst)
				}
				if cfg.ClientCircuitBreakerFailureThreshold != 0 {
					t.Fatalf("ClientCircuitBreakerFailureThreshold=%d, want 0", cfg.ClientCircuitBreakerFailureThreshold)
				}
				if cfg.ClientCircuitBreakerOpenDuration != 0 {
					t.Fatalf("ClientCircuitBreakerOpenDuration=%s, want 0", cfg.ClientCircuitBreakerOpenDuration)
				}
				if cfg.JWTToken != "my-token" {
					t.Fatalf("JWTToken=%q, want %q", cfg.JWTToken, "my-token")
				}
				if string(cfg.TLSCACert) != "test-ca-data\n" {
					t.Fatalf("TLSCACert=%q, want %q", string(cfg.TLSCACert), "test-ca-data\n")
				}
			},
		},
		{
			name: "loads and trims optional values",
			mutate: func(env map[string]string) {
				env[constants.EnvClusterNamespace] = "  team-a  "
				env[constants.EnvClusterName] = "  demo  "
				env[constants.EnvUpgradeBlueRevision] = "  blue-v2  "
				env[constants.EnvUpgradeGreenRevision] = "  green-v2  "
				env[constants.EnvUpgradeSyncThreshold] = " 250 "
				env[constants.EnvUpgradeTimeout] = " 2m30s "
				env[constants.EnvClientQPS] = " 3.25 "
				env[constants.EnvClientBurst] = " 15 "
				env[constants.EnvClientCircuitBreakerFailureThreshold] = " 9 "
				env[constants.EnvClientCircuitBreakerOpenDuration] = " 1m "
			},
			assert: func(t *testing.T, cfg *ExecutorConfig) {
				t.Helper()
				if cfg.ClusterNamespace != "team-a" {
					t.Fatalf("ClusterNamespace=%q, want %q", cfg.ClusterNamespace, "team-a")
				}
				if cfg.ClusterName != "demo" {
					t.Fatalf("ClusterName=%q, want %q", cfg.ClusterName, "demo")
				}
				if cfg.BlueRevision != "blue-v2" {
					t.Fatalf("BlueRevision=%q, want %q", cfg.BlueRevision, "blue-v2")
				}
				if cfg.GreenRevision != "green-v2" {
					t.Fatalf("GreenRevision=%q, want %q", cfg.GreenRevision, "green-v2")
				}
				if cfg.SyncThreshold != 250 {
					t.Fatalf("SyncThreshold=%d, want 250", cfg.SyncThreshold)
				}
				if cfg.Timeout != 2*time.Minute+30*time.Second {
					t.Fatalf("Timeout=%s, want 2m30s", cfg.Timeout)
				}
				if cfg.ClientQPS != 3.25 {
					t.Fatalf("ClientQPS=%f, want 3.25", cfg.ClientQPS)
				}
				if cfg.ClientBurst != 15 {
					t.Fatalf("ClientBurst=%d, want 15", cfg.ClientBurst)
				}
				if cfg.ClientCircuitBreakerFailureThreshold != 9 {
					t.Fatalf("ClientCircuitBreakerFailureThreshold=%d, want 9", cfg.ClientCircuitBreakerFailureThreshold)
				}
				if cfg.ClientCircuitBreakerOpenDuration != time.Minute {
					t.Fatalf("ClientCircuitBreakerOpenDuration=%s, want 1m0s", cfg.ClientCircuitBreakerOpenDuration)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := copyUpgradeEnv(baseEnv)
			if tt.mutate != nil {
				tt.mutate(env)
			}

			setUpgradeEnv(t, env)

			cfg, err := LoadExecutorConfig()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("LoadExecutorConfig() error=nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadExecutorConfig() error=%q, want contains %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadExecutorConfig() unexpected error: %v", err)
			}

			if tt.assert != nil {
				tt.assert(t, cfg)
			}
		})
	}
}

func setUpgradeEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, key := range executorConfigEnvKeys {
		t.Setenv(key, "")
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
}

func copyUpgradeEnv(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func writeUpgradeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write file %q: %v", path, err)
	}
	return path
}
