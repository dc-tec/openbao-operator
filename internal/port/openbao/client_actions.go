package openbao

import internalopenbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"

// ClusterActions exposes OpenBao cluster operations through a stable port contract.
type ClusterActions = internalopenbao.ClusterActions

// NewClient creates an OpenBao API client.
func NewClient(cfg ClientConfig) (ClusterActions, error) {
	return internalopenbao.NewClient(cfg)
}
