package constants

import (
	"fmt"
	"strings"
)

// ParseBoolLabel parses a boolean-like Kubernetes label value.
// It returns (value, present, error). If the label key is not present or blank,
// present=false and error=nil.
func ParseBoolLabel(labels map[string]string, key string) (bool, bool, error) {
	if labels == nil {
		return false, false, nil
	}

	raw, ok := labels[key]
	if !ok {
		return false, false, nil
	}

	value := strings.TrimSpace(raw)
	if value == "" {
		return false, false, nil
	}

	switch strings.ToLower(value) {
	case "true":
		return true, true, nil
	case "false":
		return false, true, nil
	default:
		return false, true, fmt.Errorf("invalid boolean label value %q for %q (expected true/false)", value, key)
	}
}
