package openbao

import "testing"

func TestNormalizeJWTAuthStrategy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty defaults inline", input: "", want: JWTAuthStrategyInline},
		{name: "trims and lowercases inline", input: " INLINE ", want: JWTAuthStrategyInline},
		{name: "standard", input: JWTAuthStrategyStandard, want: JWTAuthStrategyStandard},
		{name: "invalid", input: "legacy", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NormalizeJWTAuthStrategy(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeJWTAuthStrategy() error=nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeJWTAuthStrategy() error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeJWTAuthStrategy()=%q, want %q", got, tt.want)
			}
		})
	}
}
