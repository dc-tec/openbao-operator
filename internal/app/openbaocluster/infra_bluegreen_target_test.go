package openbaocluster

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestComputeStatefulSetSpecPinsBlueReplicaPopulation(t *testing.T) {
	for _, phase := range []openbaov1alpha1.BlueGreenPhase{
		openbaov1alpha1.PhasePromoting, openbaov1alpha1.PhaseRollingBack,
		openbaov1alpha1.PhaseRestoringReadReplicas, openbaov1alpha1.PhaseIdle,
	} {
		t.Run(string(phase), func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Replicas: 7, Upgrade: &openbaov1alpha1.UpgradeConfig{Strategy: openbaov1alpha1.UpdateStrategyBlueGreen},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{BlueGreen: &openbaov1alpha1.BlueGreenStatus{
					Phase: phase, BlueRevision: "blue", BlueReplicas: 3,
				}},
			}
			spec := (&infraReconciler{}).computeStatefulSetSpec(logr.Discard(), cluster, "openbao:2.5.0", "init")
			wantReplicas := int32(3)
			if phase == openbaov1alpha1.PhaseIdle {
				wantReplicas = 7
			}
			require.Equal(t, wantReplicas, spec.Replicas)
			require.False(t, spec.SkipReconciliation)
		})
	}
}
