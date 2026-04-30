package rolling

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/internal/port/backup"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
)

func newManagerWithClientFactory(
	c client.Client,
	scheme *runtime.Scheme,
	backupRuntime backup.PreUpgradeSnapshotRuntime,
	factory raftops.OpenBaoClientFactory,
	operatorImageVerifier imageverify.Verifier,
) *Manager {
	if factory == nil {
		factory = raftops.DefaultOpenBaoClientFactory
	}
	return &Manager{
		client:                c,
		reader:                c,
		scheme:                scheme,
		backupRuntime:         backupRuntime,
		clientFactory:         factory,
		clientConfig:          portopenbao.ClientConfig{},
		operatorImageVerifier: operatorImageVerifier,
	}
}
