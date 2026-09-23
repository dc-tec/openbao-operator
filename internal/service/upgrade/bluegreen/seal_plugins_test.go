package bluegreen

import (
	"context"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
)

func TestReconcileBlocksRemovedSealBeforeUpgrade(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name, cluster.Namespace = "seal-upgrade", "default"
	cluster.Spec.Version = "2.7.0"
	cluster.Spec.Replicas = 3
	cluster.Spec.Image = "openbao/openbao:2.7.0"
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{Type: "awskms"}
	cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{Strategy: openbaov1alpha1.UpdateStrategyBlueGreen}
	cluster.Status.Initialized = true
	cluster.Status.CurrentVersion = "2.6.3"
	// No Kubernetes client is installed: the guard must run before any API mutation.
	manager := &Manager{}
	_, err := manager.Reconcile(context.Background(), logr.Discard(), cluster)
	require.ErrorContains(t, err, "install the external KMS plugin before upgrading")
	require.ErrorIs(t, err, operatorerrors.ErrPermanentConfig)
	require.Nil(t, cluster.Status.Upgrade)
	require.Nil(t, cluster.Status.OperationLock)
}
