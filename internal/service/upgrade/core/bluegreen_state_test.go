package core_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

func TestCurrentBlueGreenPhaseDefaultsToIdle(t *testing.T) {
	t.Parallel()

	require.Equal(t, openbaov1alpha1.PhaseIdle, core.CurrentBlueGreenPhase(nil))
	require.Equal(t, openbaov1alpha1.PhaseIdle, core.CurrentBlueGreenPhase(&openbaov1alpha1.OpenBaoCluster{}))
}

func TestBlueGreenUpgradeState(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.5.0",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.0",
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase: openbaov1alpha1.PhaseSyncing,
			},
		},
	}

	active, needed := core.BlueGreenUpgradeState(cluster)
	require.True(t, active)
	require.True(t, needed)
	require.False(t, core.IsBlueGreenRollbackSet(cluster))
}

func TestInitializeBlueGreenManualPromotion(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				BlueGreen: &openbaov1alpha1.BlueGreenConfig{
					AutoPromote: false,
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{},
		},
	}

	require.True(t, core.BlueGreenStartEventPending(cluster))
	core.InitializeBlueGreenManualPromotion(cluster)
	require.True(t, cluster.Status.BlueGreen.ManualPromotionRequired)
}

func TestFinalizeBlueGreenTerminalState(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Image: "openbao:2.5.0",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase:                   openbaov1alpha1.PhaseCleanup,
				BlueRevision:            "blue",
				GreenRevision:           "green",
				StartTime:               &now,
				ManualPromotionRequired: true,
				JobFailureCount:         2,
				LastJobFailure:          "boom",
				RollbackStartTime:       &now,
			},
		},
	}

	core.FinalizeBlueGreenTerminalState(cluster, true)

	require.Equal(t, openbaov1alpha1.PhaseIdle, cluster.Status.BlueGreen.Phase)
	require.Equal(t, "green", cluster.Status.BlueGreen.BlueRevision)
	require.Equal(t, "openbao:2.5.0", cluster.Status.BlueGreen.BlueImage)
	require.Empty(t, cluster.Status.BlueGreen.GreenRevision)
	require.False(t, cluster.Status.BlueGreen.ManualPromotionRequired)
	require.Nil(t, cluster.Status.BlueGreen.StartTime)
	require.Zero(t, cluster.Status.BlueGreen.JobFailureCount)
	require.Empty(t, cluster.Status.BlueGreen.LastJobFailure)
	require.NotNil(t, cluster.Status.BlueGreen.RollbackStartTime)
}
