package upgrade

import (
	"errors"
	"testing"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

func TestValidateImageRefMatchesVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		version    string
		imageRef   string
		wantErr    bool
		wantReason string
	}{
		{
			name:     "empty image is allowed",
			version:  "2.5.0",
			imageRef: "",
		},
		{
			name:     "matching semver tag is allowed",
			version:  "2.5.0",
			imageRef: "registry.example.com/openbao/openbao:2.5.0",
		},
		{
			name:     "matching v prefixed semver tag is allowed",
			version:  "2.5.0",
			imageRef: "registry.example.com/openbao/openbao:v2.5.0",
		},
		{
			name:     "digest pin is allowed",
			version:  "2.5.0",
			imageRef: "registry.example.com/openbao/openbao@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		{
			name:     "custom non semver tag is allowed",
			version:  "2.5.0",
			imageRef: "registry.example.com/openbao/openbao:fips-latest",
		},
		{
			name:       "mismatched semver tag is rejected",
			version:    "2.5.0",
			imageRef:   "registry.example.com/openbao/openbao:2.4.4",
			wantErr:    true,
			wantReason: ReasonImageVersionMismatch,
		},
		{
			name:       "invalid image ref is rejected",
			version:    "2.5.0",
			imageRef:   "registry.example.com/openbao/openbao:",
			wantErr:    true,
			wantReason: ReasonImageVersionMismatch,
		},
		{
			name:       "invalid version is rejected when image tag is semver",
			version:    "latest",
			imageRef:   "registry.example.com/openbao/openbao:2.5.0",
			wantErr:    true,
			wantReason: ReasonInvalidVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateImageRefMatchesVersion(tt.version, tt.imageRef)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateImageRefMatchesVersion(%q, %q) error = %v, wantErr %v", tt.version, tt.imageRef, err, tt.wantErr)
			}
			if !tt.wantErr {
				return
			}
			if !errors.Is(err, operatorerrors.ErrPermanentConfig) {
				t.Fatalf("expected permanent config error, got %v", err)
			}
			reason, ok := operatorerrors.Reason(err)
			if !ok {
				t.Fatalf("expected reasoned error, got %v", err)
			}
			if reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}
