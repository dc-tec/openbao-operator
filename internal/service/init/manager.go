package init

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	initmanagerport "github.com/dc-tec/openbao-operator/internal/port/initmanager"
)

const (
	// openBaoInitTimeout is the timeout for initialization operations
	openBaoInitTimeout = 30 * time.Second
	// selfInitHealthTimeout bounds passive health observation while waiting for
	// OpenBao native self-initialization to complete.
	selfInitHealthTimeout = 10 * time.Second
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
	raftRuntime RaftRuntime
	recorder    events.EventRecorder
}

// RaftRuntime exposes the Raft operations required during cluster
// initialization.
type RaftRuntime interface {
	initmanagerport.AutopilotRuntime
	ConfigureAutopilot(
		ctx context.Context,
		logger logr.Logger,
		cluster *openbaov1alpha1.OpenBaoCluster,
		rootToken string,
	) error
}

// NewManager creates an initialization Manager. The client manager provides
// per-cluster OpenBao client state. The required Raft runtime configures
// Autopilot during operator-managed and self-initialization.
func NewManager(
	config *rest.Config,
	clientset kubernetes.Interface,
	clientMgr *openbao.ClientManager,
	raftOps RaftRuntime,
	recorder ...events.EventRecorder,
) (*Manager, error) {
	if raftOps == nil {
		return nil, fmt.Errorf("raft runtime is required")
	}

	var eventRecorder events.EventRecorder
	if len(recorder) > 0 {
		eventRecorder = recorder[0]
	}
	return &Manager{
		config:      config,
		clientset:   clientset,
		clientMgr:   clientMgr,
		raftRuntime: raftOps,
		recorder:    eventRecorder,
	}, nil
}
