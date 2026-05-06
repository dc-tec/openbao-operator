package upgrade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func FuzzLoadUpgradeExecutorConfig(f *testing.F) {
	tmpDir := f.TempDir()
	jwtPath := filepath.Join(tmpDir, "upgrade.jwt")
	caPath := filepath.Join(tmpDir, "ca.crt")
	if err := os.WriteFile(jwtPath, []byte("jwt-token"), 0o600); err != nil {
		f.Fatalf("failed to write JWT token file: %v", err)
	}
	if err := os.WriteFile(caPath, []byte("ca-data"), 0o600); err != nil {
		f.Fatalf("failed to write CA file: %v", err)
	}

	f.Add("default", "openbao", "3", string(ExecutorActionRollingStepDownLeader), "upgrade-role", "", "", "100", "10m")
	f.Add("ns", "cluster", "3", string(ExecutorActionBlueGreenRepairConsensus), "role", "blue", "green", "250", "2m")
	f.Add("", "cluster", "x", "", "", "", "", "bad", "forever")

	f.Fuzz(func(t *testing.T, namespace, clusterName, replicas, action, jwtRole, blueRevision, greenRevision, syncThreshold, timeout string) {
		t.Setenv(constants.EnvClusterNamespace, sanitizeUpgradeConfigText(namespace))
		t.Setenv(constants.EnvClusterName, sanitizeUpgradeConfigText(clusterName))
		t.Setenv(constants.EnvClusterReplicas, sanitizeUpgradeConfigText(replicas))
		t.Setenv(constants.EnvUpgradeAction, sanitizeUpgradeConfigText(action))
		t.Setenv(constants.EnvUpgradeJWTAuthRole, sanitizeUpgradeConfigText(jwtRole))
		t.Setenv(constants.EnvJWTTokenPath, jwtPath)
		t.Setenv(constants.EnvTLSCAPath, caPath)
		t.Setenv(constants.EnvUpgradeBlueRevision, sanitizeUpgradeConfigText(blueRevision))
		t.Setenv(constants.EnvUpgradeGreenRevision, sanitizeUpgradeConfigText(greenRevision))
		t.Setenv(constants.EnvUpgradeSyncThreshold, sanitizeUpgradeConfigText(syncThreshold))
		t.Setenv(constants.EnvUpgradeTimeout, sanitizeUpgradeConfigText(timeout))

		cfg, err := LoadExecutorConfig()
		if err == nil {
			if cfg == nil {
				t.Fatalf("successful config load returned nil config")
			}
			if strings.TrimSpace(cfg.ClusterNamespace) == "" || strings.TrimSpace(cfg.ClusterName) == "" {
				t.Fatalf("successful config load requires cluster identity fields")
			}
			if strings.TrimSpace(cfg.JWTAuthRole) == "" {
				t.Fatalf("successful config load requires JWT auth role")
			}
			_ = cfg.Validate()
		}
	})
}

func sanitizeUpgradeConfigText(input string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(input, "\x00", ""))
	if len(trimmed) > 128 {
		return trimmed[:128]
	}
	return trimmed
}
