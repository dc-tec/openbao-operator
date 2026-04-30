package bluegreen

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/internal/port/backup"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	"github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/port/workload"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
)

func newManagerWithClientFactory(
	c client.Client,
	scheme *runtime.Scheme,
	workloadRuntime workload.BlueGreenRuntime,
	backupRuntime backup.PreUpgradeSnapshotRuntime,
	clientFactory raftops.OpenBaoClientFactory,
	clientConfig openbao.ClientConfig,
	imageVerifier imageverify.Verifier,
	operatorImageVerifier imageverify.Verifier,
	platform string,
	recorder ...events.EventRecorder,
) *Manager {
	mgr := NewManager(c, scheme, workloadRuntime, backupRuntime, clientConfig, imageVerifier, operatorImageVerifier, platform, recorder...)
	if clientFactory != nil {
		mgr.clientFactory = clientFactory
	}
	mgr.clusterOps = newOpenBaoClusterOps(c, mgr.clientFactory)
	return mgr
}
