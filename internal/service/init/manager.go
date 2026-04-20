package init

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"

	"github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/adapter/raft"
	initmanagerport "github.com/dc-tec/openbao-operator/internal/port/initmanager"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

const (
	// openBaoInitTimeout is the timeout for initialization operations
	openBaoInitTimeout = 30 * time.Second
	// rootTokenSecretKey is the key used to store the root token in the Secret data.
	rootTokenSecretKey = "token"
	// rootTokenStoreTimeout is the maximum time we will spend trying to persist the root token Secret
	// after receiving it from the init API call. The root token is only returned once; if we cannot
	// persist it within the same reconcile, it is lost.
	rootTokenStoreTimeout = 2 * time.Minute
)

// Manager handles OpenBao cluster initialization.
type Manager struct {
	config      *rest.Config
	clientset   kubernetes.Interface
	clientMgr   *openbao.ClientManager
	raftManager *raft.Manager
	recorder    events.EventRecorder
}

type raftClientFactoryProvider struct {
	clientMgr *openbao.ClientManager
}

type raftClientFactoryAdapter struct {
	factory *openbao.ClientFactory
}

type raftClientAdapter struct {
	client *openbao.Client
}

// NewManager creates a new initialization Manager.
// The clientMgr is used to create OpenBao clients with proper state isolation.
func NewManager(config *rest.Config, clientset kubernetes.Interface, clientMgr *openbao.ClientManager, recorder ...events.EventRecorder) *Manager {
	var eventRecorder events.EventRecorder
	if len(recorder) > 0 {
		eventRecorder = recorder[0]
	}
	return &Manager{
		config:      config,
		clientset:   clientset,
		clientMgr:   clientMgr,
		raftManager: raft.NewManager(clientset, raftClientFactoryProvider{clientMgr: clientMgr}),
		recorder:    eventRecorder,
	}
}

func (p raftClientFactoryProvider) FactoryFor(clusterKey string, caCert []byte) raft.ClientFactory {
	if p.clientMgr == nil {
		return nil
	}

	factory := p.clientMgr.FactoryFor(clusterKey, caCert)
	if factory == nil {
		return nil
	}

	return raftClientFactoryAdapter{factory: factory}
}

func (a raftClientFactoryAdapter) NewWithJWT(ctx context.Context, baseURL, role, jwtToken string) (raft.Client, error) {
	if a.factory == nil {
		return nil, fmt.Errorf("OpenBao client factory is required")
	}

	client, err := a.factory.NewWithJWT(ctx, baseURL, role, jwtToken)
	if err != nil {
		return nil, err
	}

	return raftClientAdapter{client: client}, nil
}

func (a raftClientFactoryAdapter) NewWithToken(baseURL, token string) (raft.Client, error) {
	if a.factory == nil {
		return nil, fmt.Errorf("OpenBao client factory is required")
	}

	client, err := a.factory.NewWithToken(baseURL, token)
	if err != nil {
		return nil, err
	}

	return raftClientAdapter{client: client}, nil
}

func (a raftClientAdapter) ConfigureRaftAutopilot(ctx context.Context, config portopenbao.AutopilotConfig) error {
	if a.client == nil {
		return fmt.Errorf("OpenBao client is required")
	}
	return a.client.ConfigureRaftAutopilot(ctx, config)
}

func (a raftClientAdapter) ReadRaftConfiguration(ctx context.Context) (*portopenbao.RaftConfigurationResponse, error) {
	if a.client == nil {
		return nil, fmt.Errorf("OpenBao client is required")
	}
	return a.client.ReadRaftConfiguration(ctx)
}

func (a raftClientAdapter) ReadRaftAutopilotState(ctx context.Context) (*portopenbao.RaftAutopilotStateResponse, error) {
	if a.client == nil {
		return nil, fmt.Errorf("OpenBao client is required")
	}
	return a.client.ReadRaftAutopilotState(ctx)
}

func (a raftClientAdapter) RemoveRaftPeer(ctx context.Context, serverID string) error {
	if a.client == nil {
		return fmt.Errorf("OpenBao client is required")
	}
	return a.client.RemoveRaftPeer(ctx, serverID)
}

func (a raftClientAdapter) StepDownLeader(ctx context.Context) error {
	if a.client == nil {
		return fmt.Errorf("OpenBao client is required")
	}
	return a.client.StepDownLeader(ctx)
}

// RaftManager returns the Raft Manager for autopilot configuration.
func (m *Manager) RaftManager() *raft.Manager {
	return m.raftManager
}

// AutopilotRuntime returns the optional day-2 autopilot runtime.
func (m *Manager) AutopilotRuntime() initmanagerport.AutopilotRuntime {
	return m.raftManager
}

// ScaleDownRuntime returns the optional day-2 scale-down runtime.
func (m *Manager) ScaleDownRuntime() initmanagerport.ScaleDownRuntime {
	return m.raftManager
}

// MembershipRuntime returns the optional authenticated raft membership reader.
func (m *Manager) MembershipRuntime() initmanagerport.MembershipRuntime {
	return m.raftManager
}

// ReadReplicaScaleDownRuntime returns the optional read-replica scale-down runtime.
func (m *Manager) ReadReplicaScaleDownRuntime() initmanagerport.ReadReplicaScaleDownRuntime {
	return m.raftManager
}

// Clientset returns the Kubernetes clientset.
func (m *Manager) Clientset() kubernetes.Interface {
	return m.clientset
}

// ClientManager returns the OpenBao ClientManager.
func (m *Manager) ClientManager() *openbao.ClientManager {
	return m.clientMgr
}
