package upgrade

import openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"

// OpenBaoClientFactory creates OpenBao API clients for connecting to cluster pods.
// This is primarily used for testing to inject mock clients.
type OpenBaoClientFactory func(config openbaoapi.ClientConfig) (openbaoapi.ClusterActions, error)

// DefaultOpenBaoClientFactory is the default OpenBao client factory used by upgrade managers.
func DefaultOpenBaoClientFactory(config openbaoapi.ClientConfig) (openbaoapi.ClusterActions, error) {
	return openbaoapi.NewClient(config)
}
