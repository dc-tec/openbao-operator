package semver

import (
	"fmt"
	"strconv"
	"strings"

	masterminds "github.com/Masterminds/semver/v3"
)

const maxInt = uint64(1<<(strconv.IntSize-1) - 1)

// Version wraps a strict semantic version parsed by a dedicated SemVer library.
type Version struct {
	value *masterminds.Version

	major int
	minor int
	patch int
}

// New returns a semantic version from numeric MAJOR.MINOR.PATCH parts.
func New(major, minor, patch int) (*Version, error) {
	if major < 0 {
		return nil, fmt.Errorf("major version cannot be negative: %d", major)
	}
	if minor < 0 {
		return nil, fmt.Errorf("minor version cannot be negative: %d", minor)
	}
	if patch < 0 {
		return nil, fmt.Errorf("patch version cannot be negative: %d", patch)
	}
	return Parse(fmt.Sprintf("%d.%d.%d", major, minor, patch))
}

// Parse parses a strict semantic version string. A leading lowercase "v" is
// accepted for compatibility with image tags and user-facing OpenBao versions.
func Parse(version string) (*Version, error) {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return nil, fmt.Errorf("version string is empty")
	}

	parsed, err := masterminds.StrictNewVersion(strings.TrimPrefix(trimmed, "v"))
	if err != nil {
		return nil, fmt.Errorf("invalid semantic version %q: %w", version, err)
	}

	major, err := versionPartToInt(parsed.Major(), "major")
	if err != nil {
		return nil, err
	}
	minor, err := versionPartToInt(parsed.Minor(), "minor")
	if err != nil {
		return nil, err
	}
	patch, err := versionPartToInt(parsed.Patch(), "patch")
	if err != nil {
		return nil, err
	}

	return &Version{
		value: parsed,
		major: major,
		minor: minor,
		patch: patch,
	}, nil
}

// AtLeast reports whether version is greater than or equal to want.
func AtLeast(version string, wantMajor, wantMinor, wantPatch int) (bool, error) {
	parsed, err := Parse(version)
	if err != nil {
		return false, err
	}
	want, err := New(wantMajor, wantMinor, wantPatch)
	if err != nil {
		return false, err
	}
	return parsed.Compare(want) >= 0, nil
}

// Compare returns -1, 0, or 1 when v is less than, equal to, or greater than
// other according to SemVer precedence. Build metadata does not affect ordering.
func (v *Version) Compare(other *Version) int {
	return v.value.Compare(other.value)
}

// Major returns the major version number.
func (v *Version) Major() int {
	return v.major
}

// Minor returns the minor version number.
func (v *Version) Minor() int {
	return v.minor
}

// Patch returns the patch version number.
func (v *Version) Patch() int {
	return v.patch
}

// Prerelease returns the prerelease portion of the version.
func (v *Version) Prerelease() string {
	return v.value.Prerelease()
}

// Build returns the build metadata portion of the version.
func (v *Version) Build() string {
	return v.value.Metadata()
}

// String returns the canonical semantic version string without a leading "v".
func (v *Version) String() string {
	return v.value.String()
}

func versionPartToInt(part uint64, name string) (int, error) {
	if part > maxInt {
		return 0, fmt.Errorf("%s version %d exceeds supported integer range", name, part)
	}
	return int(part), nil
}
