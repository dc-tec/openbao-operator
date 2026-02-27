package openbaocluster

import openbaolabels "github.com/dc-tec/openbao-operator/internal/openbao"

const (
	// OpenBaoLabelActive indicates this pod is the active (leader) node.
	OpenBaoLabelActive = openbaolabels.LabelActive
	// OpenBaoLabelInitialized indicates OpenBao has been initialized.
	OpenBaoLabelInitialized = openbaolabels.LabelInitialized
	// OpenBaoLabelSealed indicates OpenBao is sealed.
	OpenBaoLabelSealed = openbaolabels.LabelSealed
	// OpenBaoLabelVersion reports the running OpenBao version label.
	OpenBaoLabelVersion = openbaolabels.LabelVersion
)

// ParseOpenBaoBoolLabel parses a boolean OpenBao pod label value.
func ParseOpenBaoBoolLabel(labels map[string]string, key string) (bool, bool, error) {
	return openbaolabels.ParseBoolLabel(labels, key)
}
