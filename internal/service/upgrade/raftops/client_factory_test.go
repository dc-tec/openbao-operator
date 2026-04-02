package raftops

import (
	"strings"
	"testing"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestDefaultOpenBaoClientFactory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  portopenbao.ClientConfig
		wantErr string
	}{
		{
			name:    "missing base url",
			config:  portopenbao.ClientConfig{},
			wantErr: "baseURL is required",
		},
		{
			name: "valid configuration",
			config: portopenbao.ClientConfig{
				BaseURL: "https://openbao.example.svc:8200",
				Token:   "token",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := DefaultOpenBaoClientFactory(tt.config)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("DefaultOpenBaoClientFactory() error=nil, want contains %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("DefaultOpenBaoClientFactory() error=%q, want contains %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("DefaultOpenBaoClientFactory() unexpected error: %v", err)
			}
			if client == nil {
				t.Fatalf("DefaultOpenBaoClientFactory() returned nil client")
			}
		})
	}
}
