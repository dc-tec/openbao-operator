package main

import "testing"

func TestParseUmask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{
			name: "valid 0000",
			raw:  "0000",
			want: 0,
		},
		{
			name: "valid 0077",
			raw:  "0077",
			want: 0o077,
		},
		{
			name: "valid 0777",
			raw:  "0777",
			want: 0o777,
		},
		{
			name:    "invalid non octal",
			raw:     "not-octal",
			wantErr: true,
		},
		{
			name:    "invalid octal digit",
			raw:     "0888",
			wantErr: true,
		},
		{
			name:    "out of range",
			raw:     "1000",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseUmask(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseUmask(%q) expected error, got nil", tt.raw)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseUmask(%q) unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("parseUmask(%q)=%#o, want %#o", tt.raw, got, tt.want)
			}
		})
	}
}
