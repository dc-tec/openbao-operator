package openbao

import portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"

const (
	LabelActive      = portopenbao.LabelActive
	LabelInitialized = portopenbao.LabelInitialized
	LabelSealed      = portopenbao.LabelSealed
	LabelVersion     = portopenbao.LabelVersion
)

// ParseBoolLabel parses a boolean-like Kubernetes label value.
func ParseBoolLabel(labels map[string]string, key string) (bool, bool, error) {
	return portopenbao.ParseBoolLabel(labels, key)
}
