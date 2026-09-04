package controller

import (
	"context"
	"fmt"

	"github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/adapter/raft"
)

type raftClientFactoryProvider struct {
	clientManager *openbao.ClientManager
}

type raftClientFactoryAdapter struct {
	factory *openbao.ClientFactory
}

func (p raftClientFactoryProvider) FactoryFor(
	clusterKey string,
	caCert []byte,
	tlsServerName string,
) raft.ClientFactory {
	if p.clientManager == nil {
		return nil
	}

	factory := p.clientManager.FactoryFor(clusterKey, caCert, tlsServerName)
	if factory == nil {
		return nil
	}

	return raftClientFactoryAdapter{factory: factory}
}

func (a raftClientFactoryAdapter) NewWithJWT(
	ctx context.Context,
	baseURL string,
	role string,
	jwtToken string,
) (raft.Client, error) {
	if a.factory == nil {
		return nil, fmt.Errorf("OpenBao client factory is required")
	}

	return a.factory.NewWithJWT(ctx, baseURL, role, jwtToken)
}

func (a raftClientFactoryAdapter) NewWithToken(baseURL, token string) (raft.Client, error) {
	if a.factory == nil {
		return nil, fmt.Errorf("OpenBao client factory is required")
	}

	return a.factory.NewWithToken(baseURL, token)
}
