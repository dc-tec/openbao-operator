package upgrade

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		wantMajor  int
		wantMinor  int
		wantPatch  int
		wantPrerel string
		wantBuild  string
		wantErr    bool
	}{
		{
			name:      "simple version",
			version:   "2.4.0",
			wantMajor: 2,
			wantMinor: 4,
			wantPatch: 0,
			wantErr:   false,
		},
		{
			name:      "with v prefix",
			version:   "v2.4.0",
			wantMajor: 2,
			wantMinor: 4,
			wantPatch: 0,
			wantErr:   false,
		},
		{
			name:       "with prerelease",
			version:    "2.4.0-rc1",
			wantMajor:  2,
			wantMinor:  4,
			wantPatch:  0,
			wantPrerel: "rc1",
			wantErr:    false,
		},
		{
			name:      "with build metadata",
			version:   "2.4.0+build123",
			wantMajor: 2,
			wantMinor: 4,
			wantPatch: 0,
			wantBuild: "build123",
			wantErr:   false,
		},
		{
			name:       "with prerelease and build",
			version:    "2.4.0-beta.1+build.456",
			wantMajor:  2,
			wantMinor:  4,
			wantPatch:  0,
			wantPrerel: "beta.1",
			wantBuild:  "build.456",
			wantErr:    false,
		},
		{
			name:    "empty string",
			version: "",
			wantErr: true,
		},
		{
			name:    "missing patch",
			version: "2.4",
			wantErr: true,
		},
		{
			name:    "too many parts",
			version: "2.4.0.1",
			wantErr: true,
		},
		{
			name:    "non-numeric major",
			version: "a.4.0",
			wantErr: true,
		},
		{
			name:    "non-numeric minor",
			version: "2.b.0",
			wantErr: true,
		},
		{
			name:    "non-numeric patch",
			version: "2.4.c",
			wantErr: true,
		},
		{
			name:    "negative major",
			version: "-1.4.0",
			wantErr: true,
		},
		{
			name:      "zero version",
			version:   "0.0.0",
			wantMajor: 0,
			wantMinor: 0,
			wantPatch: 0,
			wantErr:   false,
		},
		{
			name:      "large version numbers",
			version:   "999.999.999",
			wantMajor: 999,
			wantMinor: 999,
			wantPatch: 999,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVersion(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if got.Major != tt.wantMajor {
				t.Errorf("Major = %d, want %d", got.Major, tt.wantMajor)
			}
			if got.Minor != tt.wantMinor {
				t.Errorf("Minor = %d, want %d", got.Minor, tt.wantMinor)
			}
			if got.Patch != tt.wantPatch {
				t.Errorf("Patch = %d, want %d", got.Patch, tt.wantPatch)
			}
			if got.Prerelease != tt.wantPrerel {
				t.Errorf("Prerelease = %q, want %q", got.Prerelease, tt.wantPrerel)
			}
			if got.Build != tt.wantBuild {
				t.Errorf("Build = %q, want %q", got.Build, tt.wantBuild)
			}
		})
	}
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"valid simple", "2.4.0", false},
		{"valid with v", "v2.4.0", false},
		{"valid with prerelease", "2.4.0-rc1", false},
		{"invalid empty", "", true},
		{"invalid format", "2.4", true},
		{"invalid chars", "2.4.x", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersion(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name       string
		from       string
		to         string
		wantChange VersionChange
		wantErr    bool
	}{
		// Patch upgrades
		{
			name:       "patch upgrade",
			from:       "2.4.0",
			to:         "2.4.1",
			wantChange: VersionChangePatch,
		},
		{
			name:       "patch upgrade multiple",
			from:       "2.4.0",
			to:         "2.4.5",
			wantChange: VersionChangePatch,
		},

		// Minor upgrades
		{
			name:       "minor upgrade",
			from:       "2.4.0",
			to:         "2.5.0",
			wantChange: VersionChangeMinor,
		},
		{
			name:       "minor upgrade with patch diff",
			from:       "2.4.5",
			to:         "2.5.0",
			wantChange: VersionChangeMinor,
		},

		// Major upgrades
		{
			name:       "major upgrade",
			from:       "2.4.0",
			to:         "3.0.0",
			wantChange: VersionChangeMajor,
		},
		{
			name:       "major upgrade from 1 to 2",
			from:       "1.9.9",
			to:         "2.0.0",
			wantChange: VersionChangeMajor,
		},

		// Downgrades
		{
			name:       "patch downgrade",
			from:       "2.4.5",
			to:         "2.4.0",
			wantChange: VersionChangeDowngrade,
		},
		{
			name:       "minor downgrade",
			from:       "2.5.0",
			to:         "2.4.0",
			wantChange: VersionChangeDowngrade,
		},
		{
			name:       "major downgrade",
			from:       "3.0.0",
			to:         "2.4.0",
			wantChange: VersionChangeDowngrade,
		},

		// No change
		{
			name:       "same version",
			from:       "2.4.0",
			to:         "2.4.0",
			wantChange: VersionChangeNone,
		},
		{
			name:       "same version with v prefix",
			from:       "v2.4.0",
			to:         "2.4.0",
			wantChange: VersionChangeNone,
		},

		// Prerelease handling
		{
			name:       "prerelease to release",
			from:       "2.4.0-rc1",
			to:         "2.4.0",
			wantChange: VersionChangePatch,
		},
		{
			name:       "release to prerelease is downgrade",
			from:       "2.4.0",
			to:         "2.4.0-rc1",
			wantChange: VersionChangeDowngrade,
		},
		{
			name:       "prerelease to different prerelease",
			from:       "2.4.0-alpha",
			to:         "2.4.0-beta",
			wantChange: VersionChangePatch,
		},

		// Error cases
		{
			name:    "invalid from version",
			from:    "invalid",
			to:      "2.4.0",
			wantErr: true,
		},
		{
			name:    "invalid to version",
			from:    "2.4.0",
			to:      "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareVersions(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("CompareVersions(%q, %q) error = %v, wantErr %v", tt.from, tt.to, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantChange {
				t.Errorf("CompareVersions(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.wantChange)
			}
		})
	}
}

func TestIsDowngrade(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{"patch downgrade", "2.4.1", "2.4.0", true},
		{"minor downgrade", "2.5.0", "2.4.0", true},
		{"major downgrade", "3.0.0", "2.0.0", true},
		{"patch upgrade", "2.4.0", "2.4.1", false},
		{"same version", "2.4.0", "2.4.0", false},
		{"invalid from", "invalid", "2.4.0", false},
		{"invalid to", "2.4.0", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDowngrade(tt.from, tt.to); got != tt.want {
				t.Errorf("IsDowngrade(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestIsUpgrade(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{"patch upgrade", "2.4.0", "2.4.1", true},
		{"minor upgrade", "2.4.0", "2.5.0", true},
		{"major upgrade", "2.0.0", "3.0.0", true},
		{"downgrade", "2.4.1", "2.4.0", false},
		{"same version", "2.4.0", "2.4.0", false},
		{"invalid from", "invalid", "2.4.0", false},
		{"invalid to", "2.4.0", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUpgrade(tt.from, tt.to); got != tt.want {
				t.Errorf("IsUpgrade(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestIsSkipMinorUpgrade(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{"no skip", "2.4.0", "2.5.0", false},
		{"single skip", "2.4.0", "2.6.0", true},
		{"multiple skips", "2.4.0", "2.8.0", true},
		{"different major", "2.4.0", "3.6.0", false},
		{"downgrade", "2.6.0", "2.4.0", false},
		{"patch only", "2.4.0", "2.4.5", false},
		{"invalid from", "invalid", "2.6.0", false},
		{"invalid to", "2.4.0", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSkipMinorUpgrade(tt.from, tt.to); got != tt.want {
				t.Errorf("IsSkipMinorUpgrade(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestSemVer_String(t *testing.T) {
	tests := []struct {
		name string
		ver  SemVer
		want string
	}{
		{
			name: "simple",
			ver:  SemVer{Major: 2, Minor: 4, Patch: 0},
			want: "2.4.0",
		},
		{
			name: "with prerelease",
			ver:  SemVer{Major: 2, Minor: 4, Patch: 0, Prerelease: "rc1"},
			want: "2.4.0-rc1",
		},
		{
			name: "with build",
			ver:  SemVer{Major: 2, Minor: 4, Patch: 0, Build: "build123"},
			want: "2.4.0+build123",
		},
		{
			name: "with both",
			ver:  SemVer{Major: 2, Minor: 4, Patch: 0, Prerelease: "beta", Build: "123"},
			want: "2.4.0-beta+123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ver.String(); got != tt.want {
				t.Errorf("SemVer.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
