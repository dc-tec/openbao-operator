package semver

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
		wantErr bool
	}{
		{name: "release", version: "2.5.0", want: "2.5.0"},
		{name: "v prefix", version: "v2.5.0", want: "2.5.0"},
		{name: "prerelease and build", version: "2.5.0-rc.1+build.7", want: "2.5.0-rc.1+build.7"},
		{name: "missing patch", version: "2.5", wantErr: true},
		{name: "leading zero segment", version: "2.05.0", wantErr: true},
		{name: "empty", version: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.version)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.String() != tt.want {
				t.Fatalf("Parse(%q).String() = %q, want %q", tt.version, got.String(), tt.want)
			}
		})
	}
}

func TestCompareUsesSemVerPrecedence(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "numeric prerelease identifiers", a: "2.5.0-rc.10", b: "2.5.0-rc.2", want: 1},
		{name: "release after prerelease", a: "2.5.0", b: "2.5.0-rc.1", want: 1},
		{name: "build metadata ignored", a: "2.5.0+build.1", b: "2.5.0+build.2", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := Parse(tt.a)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.a, err)
			}
			b, err := Parse(tt.b)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.b, err)
			}
			if got := a.Compare(b); got != tt.want {
				t.Fatalf("%q Compare %q = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestAtLeast(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
		wantErr bool
	}{
		{name: "older", version: "2.4.4", want: false},
		{name: "target", version: "2.5.0", want: true},
		{name: "newer", version: "2.6.0", want: true},
		{name: "target prerelease", version: "2.5.0-rc.1", want: false},
		{name: "target with metadata", version: "2.5.0+build.1", want: true},
		{name: "invalid", version: "not-a-version", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AtLeast(tt.version, 2, 5, 0)
			if (err != nil) != tt.wantErr {
				t.Fatalf("AtLeast(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Fatalf("AtLeast(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
