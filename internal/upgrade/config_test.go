package upgrade

import (
	"strings"
	"testing"
	"time"
)

func TestExecutorConfig_Validate_BlueGreenRepairConsensus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        ExecutorConfig
		wantErr    bool
		errContain string
	}{
		{
			name: "valid",
			cfg: ExecutorConfig{
				ClusterNamespace: "default",
				ClusterName:      "example",
				ClusterReplicas:  3,
				Action:           ExecutorActionBlueGreenRepairConsensus,
				JWTAuthRole:      "upgrade",
				JWTToken:         "token",
				TLSCACert:        []byte("ca"),
				BlueRevision:     "blue",
				GreenRevision:    "green",
				SyncThreshold:    100,
				Timeout:          10 * time.Second,
			},
		},
		{
			name: "requires blue revision",
			cfg: ExecutorConfig{
				ClusterNamespace: "default",
				ClusterName:      "example",
				ClusterReplicas:  3,
				Action:           ExecutorActionBlueGreenRepairConsensus,
				JWTAuthRole:      "upgrade",
				JWTToken:         "token",
				TLSCACert:        []byte("ca"),
				GreenRevision:    "green",
				SyncThreshold:    100,
				Timeout:          10 * time.Second,
			},
			wantErr:    true,
			errContain: "blue revision is required",
		},
		{
			name: "requires green revision",
			cfg: ExecutorConfig{
				ClusterNamespace: "default",
				ClusterName:      "example",
				ClusterReplicas:  3,
				Action:           ExecutorActionBlueGreenRepairConsensus,
				JWTAuthRole:      "upgrade",
				JWTToken:         "token",
				TLSCACert:        []byte("ca"),
				BlueRevision:     "blue",
				SyncThreshold:    100,
				Timeout:          10 * time.Second,
			},
			wantErr:    true,
			errContain: "green revision is required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Validate() err=nil, want error")
				}
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Fatalf("Validate() err=%q, want contains %q", err.Error(), tt.errContain)
				}
				return
			}

			if err != nil {
				t.Fatalf("Validate() err=%v, want nil", err)
			}
		})
	}
}
