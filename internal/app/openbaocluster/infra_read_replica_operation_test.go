package openbaocluster

import (
	"testing"

	"github.com/stretchr/testify/require"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
)

func TestShouldStageSteadyReadReplicasDown(t *testing.T) {
	t.Run("restore lock forces read pool drain", func(t *testing.T) {
		cluster := &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
			},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				OperationLock: &openbaov1alpha1.OperationLockStatus{
					Operation: openbaov1alpha1.ClusterOperationRestore,
					Holder:    "openbaorestore/example",
				},
			},
		}

		require.True(t, shouldStageSteadyReadReplicasDown(cluster))
	})

	t.Run("bluegreen phase forces read pool drain", func(t *testing.T) {
		cluster := &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
			},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				BlueGreen: &openbaov1alpha1.BlueGreenStatus{Phase: openbaov1alpha1.PhaseSyncing},
			},
		}

		require.True(t, shouldStageSteadyReadReplicasDown(cluster))
	})

	t.Run("bluegreen upgrade lock forces read pool drain before phase advances", func(t *testing.T) {
		cluster := &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Upgrade:      &openbaov1alpha1.UpgradeConfig{Strategy: openbaov1alpha1.UpdateStrategyBlueGreen},
				ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
			},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				OperationLock: &openbaov1alpha1.OperationLockStatus{
					Operation: openbaov1alpha1.ClusterOperationUpgrade,
					Holder:    "upgrade",
				},
			},
		}

		require.True(t, shouldStageSteadyReadReplicasDown(cluster))
	})

	t.Run("rolling upgrade lock does not force read pool drain", func(t *testing.T) {
		cluster := &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Upgrade:      &openbaov1alpha1.UpgradeConfig{Strategy: openbaov1alpha1.UpdateStrategyRollingUpdate},
				ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
			},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				OperationLock: &openbaov1alpha1.OperationLockStatus{
					Operation: openbaov1alpha1.ClusterOperationUpgrade,
					Holder:    "upgrade",
				},
			},
		}

		require.False(t, shouldStageSteadyReadReplicasDown(cluster))
	})

	t.Run("bluegreen restore phase allows steady read pool to come back", func(t *testing.T) {
		cluster := &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Upgrade:      &openbaov1alpha1.UpgradeConfig{Strategy: openbaov1alpha1.UpdateStrategyBlueGreen},
				ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
			},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				BlueGreen: &openbaov1alpha1.BlueGreenStatus{
					Phase: openbaov1alpha1.PhaseRestoringReadReplicas,
				},
				OperationLock: &openbaov1alpha1.OperationLockStatus{
					Operation: openbaov1alpha1.ClusterOperationUpgrade,
					Holder:    "upgrade",
				},
			},
		}

		require.False(t, shouldStageSteadyReadReplicasDown(cluster))
	})
}

func TestApplyOperationalReadReplicaStageDown(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			OperationLock: &openbaov1alpha1.OperationLockStatus{
				Operation: openbaov1alpha1.ClusterOperationRestore,
				Holder:    "openbaorestore/example",
			},
		},
	}
	spec := workloadsvc.StatefulSetSpec{
		Pool:     constants.LabelValueOpenBaoWorkloadPoolReadReplica,
		Replicas: 2,
	}

	changed := applyOperationalReadReplicaStageDown(cluster, &spec)

	require.True(t, changed)
	require.Zero(t, spec.Replicas)
}
