package openbao

// ClientFactory creates OpenBao API clients for a given configuration.
type ClientFactory func(config ClientConfig) (ClusterActions, error)
