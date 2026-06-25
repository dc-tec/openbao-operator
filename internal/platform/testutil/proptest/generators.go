package proptest

import "pgregory.net/rapid"

const identifierPattern = `[A-Za-z0-9][A-Za-z0-9._/-]{0,63}`

// Identifier generates short operator-style identifiers for property tests.
func Identifier() *rapid.Generator[string] {
	return rapid.StringMatching(identifierPattern)
}

func OptionalIdentifier() *rapid.Generator[string] {
	return rapid.OneOf(rapid.Just(""), Identifier())
}

func DifferentIdentifier(t *rapid.T, label string, other string) string {
	return Identifier().Filter(func(candidate string) bool {
		return candidate != other
	}).Draw(t, label)
}
