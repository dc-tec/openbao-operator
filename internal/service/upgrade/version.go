package upgrade

import (
	"fmt"
	"strconv"
	"strings"
)

// VersionChange represents the type of version change detected.
type VersionChange string

const (
	// VersionChangeNone indicates no version change.
	VersionChangeNone VersionChange = "None"
	// VersionChangePatch indicates a patch-level upgrade (e.g., 2.4.0 -> 2.4.1).
	VersionChangePatch VersionChange = "Patch"
	// VersionChangeMinor indicates a minor-level upgrade (e.g., 2.4.0 -> 2.5.0).
	VersionChangeMinor VersionChange = "Minor"
	// VersionChangeMajor indicates a major-level upgrade (e.g., 2.x -> 3.x).
	VersionChangeMajor VersionChange = "Major"
	// VersionChangeDowngrade indicates a version downgrade.
	VersionChangeDowngrade VersionChange = "Downgrade"
)

// SemVer represents a parsed semantic version.
type SemVer struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
}

// String returns the string representation of the version.
func (v SemVer) String() string {
	version := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		version += "-" + v.Prerelease
	}
	if v.Build != "" {
		version += "+" + v.Build
	}
	return version
}

// ParseVersion parses a semantic version string.
// Supports formats:
// - 2.4.0
// - 2.4.0-rc1
// - 2.4.0+build123
// - 2.4.0-rc1+build123
// - v2.4.0 (optional 'v' prefix)
func ParseVersion(version string) (*SemVer, error) {
	if version == "" {
		return nil, fmt.Errorf("version string is empty")
	}

	// Strip optional 'v' prefix
	version = strings.TrimPrefix(version, "v")

	// Extract build metadata (after '+')
	var build string
	if idx := strings.Index(version, "+"); idx != -1 {
		build = version[idx+1:]
		version = version[:idx]
	}

	// Extract prerelease (after '-')
	var prerelease string
	if idx := strings.Index(version, "-"); idx != -1 {
		prerelease = version[idx+1:]
		version = version[:idx]
	}

	// Parse major.minor.patch
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid version format %q: expected MAJOR.MINOR.PATCH", version)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major version %q: %w", parts[0], err)
	}
	if major < 0 {
		return nil, fmt.Errorf("major version cannot be negative: %d", major)
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid minor version %q: %w", parts[1], err)
	}
	if minor < 0 {
		return nil, fmt.Errorf("minor version cannot be negative: %d", minor)
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid patch version %q: %w", parts[2], err)
	}
	if patch < 0 {
		return nil, fmt.Errorf("patch version cannot be negative: %d", patch)
	}

	return &SemVer{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: prerelease,
		Build:      build,
	}, nil
}

// ValidateVersion validates that a version string is a valid semantic version.
func ValidateVersion(version string) error {
	_, err := ParseVersion(version)
	return err
}

// CompareVersions compares two version strings and returns the type of change.
// Returns an error if either version is invalid.
func CompareVersions(from, to string) (VersionChange, error) {
	fromVer, err := ParseVersion(from)
	if err != nil {
		return VersionChangeNone, fmt.Errorf("invalid 'from' version: %w", err)
	}

	toVer, err := ParseVersion(to)
	if err != nil {
		return VersionChangeNone, fmt.Errorf("invalid 'to' version: %w", err)
	}

	// Compare major versions
	if toVer.Major < fromVer.Major {
		return VersionChangeDowngrade, nil
	}
	if toVer.Major > fromVer.Major {
		return VersionChangeMajor, nil
	}

	// Major versions are equal, compare minor versions
	if toVer.Minor < fromVer.Minor {
		return VersionChangeDowngrade, nil
	}
	if toVer.Minor > fromVer.Minor {
		return VersionChangeMinor, nil
	}

	// Major and minor versions are equal, compare patch versions
	if toVer.Patch < fromVer.Patch {
		return VersionChangeDowngrade, nil
	}
	if toVer.Patch > fromVer.Patch {
		return VersionChangePatch, nil
	}

	// Core versions are equal, check prerelease
	// A prerelease version has lower precedence than a release version
	// e.g., 2.4.0-rc1 < 2.4.0
	if fromVer.Prerelease != "" && toVer.Prerelease == "" {
		// Going from prerelease to release is an upgrade
		return VersionChangePatch, nil
	}
	if fromVer.Prerelease == "" && toVer.Prerelease != "" {
		// Going from release to prerelease is a downgrade
		return VersionChangeDowngrade, nil
	}
	if fromVer.Prerelease != toVer.Prerelease {
		// Different prerelease strings, compare lexicographically
		// This is simplified; full semver uses more complex rules
		if toVer.Prerelease < fromVer.Prerelease {
			return VersionChangeDowngrade, nil
		}
		return VersionChangePatch, nil
	}

	// Versions are identical
	return VersionChangeNone, nil
}

// IsDowngrade returns true if changing from 'from' to 'to' would be a downgrade.
func IsDowngrade(from, to string) bool {
	change, err := CompareVersions(from, to)
	if err != nil {
		// If we can't parse versions, be conservative and don't consider it a downgrade
		return false
	}
	return change == VersionChangeDowngrade
}

// IsUpgrade returns true if changing from 'from' to 'to' would be an upgrade.
func IsUpgrade(from, to string) bool {
	change, err := CompareVersions(from, to)
	if err != nil {
		return false
	}
	return change == VersionChangePatch || change == VersionChangeMinor || change == VersionChangeMajor
}

// IsSkipMinorUpgrade returns true if the upgrade skips minor versions.
// For example, 2.4.0 -> 2.6.0 skips 2.5.x.
func IsSkipMinorUpgrade(from, to string) bool {
	fromVer, err := ParseVersion(from)
	if err != nil {
		return false
	}

	toVer, err := ParseVersion(to)
	if err != nil {
		return false
	}

	// Only applies to same-major upgrades
	if fromVer.Major != toVer.Major {
		return false
	}

	// Check if more than one minor version is skipped
	return toVer.Minor-fromVer.Minor > 1
}
