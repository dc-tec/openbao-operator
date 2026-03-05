package initmanager

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

// Manager handles OpenBao cluster initialization.
type Manager interface {
	Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error)
}

// AutopilotRuntime exposes day-2 autopilot reconciliation capabilities used by workload orchestration.
type AutopilotRuntime interface {
	ReconcileAutopilotConfig(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error
}

// AutopilotProvider allows init managers to optionally expose an autopilot runtime.
type AutopilotProvider interface {
	AutopilotRuntime() AutopilotRuntime
}
