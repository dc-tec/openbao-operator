package config

import (
	"strings"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestBuildStructuredSelfInitDataRejectsUnresolvedRefs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		build   func() error
		wantErr string
	}{
		{
			name: "auth method config ref",
			build: func() error {
				_, err := buildAuthMethodData(&openbaov1alpha1.SelfInitAuthMethod{
					Type: "jwt",
					ConfigFromRef: &openbaov1alpha1.TypedObjectReference{
						Kind: "Secret",
						Name: "oidc-config",
					},
				})
				return err
			},
			wantErr: "configFromRef must be resolved",
		},
		{
			name: "policy content ref",
			build: func() error {
				_, err := buildPolicyData(&openbaov1alpha1.SelfInitPolicy{
					ContentFromRef: &openbaov1alpha1.TypedObjectReference{
						Kind: "Secret",
						Name: "policy-content",
					},
				})
				return err
			},
			wantErr: "contentFromRef must be resolved",
		},
		{
			name: "audit sink ref",
			build: func() error {
				_, err := buildAuditDeviceData(&openbaov1alpha1.SelfInitAuditDevice{
					Type: "http",
					SinkFromRef: &openbaov1alpha1.TypedObjectReference{
						Kind: "Secret",
						Name: "audit-sink",
					},
				})
				return err
			},
			wantErr: "sinkFromRef must be resolved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.build()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
