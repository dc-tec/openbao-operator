package upgrade

import "testing"

func FuzzParseVersionAndCompareVersions(f *testing.F) {
	f.Add("2.4.0", "2.4.1")
	f.Add("v2.4.0-rc1", "2.4.0")
	f.Add("", "2.4")

	f.Fuzz(func(t *testing.T, from, to string) {
		fromVer, fromErr := ParseVersion(from)
		if fromErr == nil {
			if err := ValidateVersion(from); err != nil {
				t.Fatalf("ValidateVersion(%q) error = %v after ParseVersion succeeded", from, err)
			}
			roundTrip, err := ParseVersion(fromVer.String())
			if err != nil {
				t.Fatalf("round-trip ParseVersion(%q) error = %v", fromVer.String(), err)
			}
			if roundTrip.String() != fromVer.String() {
				t.Fatalf("version round-trip mismatch: got %q want %q", roundTrip.String(), fromVer.String())
			}
		}

		toVer, toErr := ParseVersion(to)
		if toErr == nil {
			if err := ValidateVersion(to); err != nil {
				t.Fatalf("ValidateVersion(%q) error = %v after ParseVersion succeeded", to, err)
			}
			roundTrip, err := ParseVersion(toVer.String())
			if err != nil {
				t.Fatalf("round-trip ParseVersion(%q) error = %v", toVer.String(), err)
			}
			if roundTrip.String() != toVer.String() {
				t.Fatalf("version round-trip mismatch: got %q want %q", roundTrip.String(), toVer.String())
			}
		}

		change, err := CompareVersions(from, to)
		if fromErr != nil || toErr != nil {
			if err == nil {
				t.Fatalf("expected CompareVersions(%q, %q) to fail when parsing fails", from, to)
			}
			return
		}
		if err != nil {
			t.Fatalf("CompareVersions(%q, %q) error = %v", from, to, err)
		}

		isDowngrade := IsDowngrade(from, to)
		switch change {
		case VersionChangeNone:
			if isDowngrade {
				t.Fatalf("identical versions cannot be a downgrade")
			}
		case VersionChangeDowngrade:
			if !isDowngrade {
				t.Fatalf("downgrade classification inconsistent")
			}
		case VersionChangePatch, VersionChangeMinor, VersionChangeMajor:
			if isDowngrade {
				t.Fatalf("upgrade classification inconsistent for %v", change)
			}
		default:
			t.Fatalf("unexpected version change %q", change)
		}

		_ = IsSkipMinorUpgrade(from, to)
	})
}
