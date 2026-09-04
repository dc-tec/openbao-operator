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

// ScaleDownRuntime exposes authenticated Raft membership operations used to
// stage safe cluster scale-downs one replica at a time.
type ScaleDownRuntime interface {
	PrepareScaleDown(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, statefulSetName string, currentReplicas, desiredReplicas int32) error
}

// ReadReplicaScaleDownRuntime exposes authenticated non-voter membership
// operations used to stage safe steady-state read-replica scale-downs.
type ReadReplicaScaleDownRuntime interface {
	PrepareReadReplicaScaleDown(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, statefulSetName string, currentReplicas, desiredReplicas int32) error
}
