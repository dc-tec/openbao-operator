package openbaocluster

import (
	"fmt"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func fuzzProfile(seed uint8) openbaov1alpha1.Profile {
	switch seed % 3 {
	case 0:
		return ""
	case 1:
		return openbaov1alpha1.ProfileDevelopment
	default:
		return openbaov1alpha1.ProfileHardened
	}
}

func fuzzUnsealType(seed uint8) string {
	switch seed % 4 {
	case 0:
		return ""
	case 1:
		return unsealTypeStatic
	case 2:
		return "awskms"
	default:
		return "transit"
	}
}

func sanitizeClusterToken(input, fallback string) string {
	var builder strings.Builder
	for _, value := range strings.ToLower(input) {
		switch {
		case value >= 'a' && value <= 'z':
			builder.WriteRune(value)
		case value >= '0' && value <= '9':
			builder.WriteRune(value)
		case value == '-':
			builder.WriteRune(value)
		}
		if builder.Len() >= 32 {
			break
		}
	}
	out := strings.Trim(builder.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}

func sanitizeMessage(input, fallback string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return fallback
	}
	if len(trimmed) > 120 {
		return fmt.Sprintf("%s...", trimmed[:117])
	}
	return trimmed
}
