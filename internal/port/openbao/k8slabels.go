package openbao

import internalopenbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"

const (
	// LabelActive is set by OpenBao's Kubernetes service registration.
	LabelActive = internalopenbao.LabelActive
	// LabelInitialized is set by OpenBao's Kubernetes service registration.
	LabelInitialized = internalopenbao.LabelInitialized
	// LabelSealed is set by OpenBao's Kubernetes service registration.
	LabelSealed = internalopenbao.LabelSealed
	// LabelVersion is set by OpenBao's Kubernetes service registration.
	LabelVersion = internalopenbao.LabelVersion
)

// ParseBoolLabel parses a boolean-like Kubernetes label value.
func ParseBoolLabel(labels map[string]string, key string) (bool, bool, error) {
	return internalopenbao.ParseBoolLabel(labels, key)
}
