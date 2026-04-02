package raftops

import (
	openbaoapi "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// OpenBaoClientFactory creates OpenBao API clients for connecting to cluster pods.
// This is primarily used for testing to inject mock clients.
type OpenBaoClientFactory func(config portopenbao.ClientConfig) (portopenbao.ClusterActions, error)

// DefaultOpenBaoClientFactory is the default OpenBao client factory used by upgrade managers.
func DefaultOpenBaoClientFactory(config portopenbao.ClientConfig) (portopenbao.ClusterActions, error) {
	return openbaoapi.NewClient(config)
}
