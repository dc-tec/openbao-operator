package openbao

import portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"

const (
	DefaultConnectionTimeout = portopenbao.DefaultConnectionTimeout
	DefaultRequestTimeout    = portopenbao.DefaultRequestTimeout
	DefaultSnapshotTimeout   = portopenbao.DefaultSnapshotTimeout
)

type ClientConfig = portopenbao.ClientConfig
type AutopilotConfig = portopenbao.AutopilotConfig
type RaftServer = portopenbao.RaftServer
type RaftConfiguration = portopenbao.RaftConfiguration
type RaftConfigurationResponse = portopenbao.RaftConfigurationResponse
