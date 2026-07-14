package bluegreen

import (
	"errors"
	"testing"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func TestValidateVersionCompatibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		target  string
		wantErr bool
	}{
		{name: "known incompatible transition", current: "2.5.5", target: "2.6.0", wantErr: true},
		{name: "v-prefixed target", current: "v2.4.4", target: "v2.6.0", wantErr: true},
		{name: "prerelease target remains incompatible", current: "2.5.5", target: "2.6.0-rc1", wantErr: true},
		{name: "future target must be explicitly qualified", current: "2.5.5", target: "2.6.1", wantErr: true},
		{name: "same forwarding generation", current: "2.6.0-beta20260622", target: "2.6.0"},
		{name: "compatible older transition", current: "2.4.4", target: "2.5.5"},
		{name: "invalid current version is handled elsewhere", current: "unknown", target: "2.6.0"},
		{name: "invalid target version is handled elsewhere", current: "2.5.5", target: "latest"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateVersionCompatibility(tt.current, tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatal("validateVersionCompatibility() error = nil, want error")
				}
				if !errors.Is(err, operatorerrors.ErrPermanentConfig) {
					t.Fatalf("validateVersionCompatibility() error = %v, want permanent config error", err)
				}
				reason, ok := operatorerrors.Reason(err)
				if !ok || reason != upgrade.ReasonBlueGreenVersionIncompatible {
					t.Fatalf("validateVersionCompatibility() reason = %q, %v, want %q", reason, ok, upgrade.ReasonBlueGreenVersionIncompatible)
				}
				return
			}

			if err != nil {
				t.Fatalf("validateVersionCompatibility() error = %v, want nil", err)
			}
		})
	}
}
