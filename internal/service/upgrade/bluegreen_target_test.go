package upgrade

import (
	"testing"

	"github.com/stretchr/testify/require"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestExecutorJobPinsBothReplicaPopulations(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{Replicas: 7},
		Status: openbaov1alpha1.OpenBaoClusterStatus{BlueGreen: &openbaov1alpha1.BlueGreenStatus{
			Phase: openbaov1alpha1.PhaseRollingBack, BlueReplicas: 3, GreenReplicas: 5,
		}},
	}
	variables := buildUpgradeExecutorEnv(cluster, ExecutorActionBlueGreenRepairConsensus, "upgrade", "blue", "green", portopenbao.ClientConfig{}, portopenbao.TrustBundleSource{})
	env := map[string]string{}
	for _, variable := range variables {
		env[variable.Name] = variable.Value
	}
	require.Equal(t, "5", env[constants.EnvClusterReplicas])
	require.Equal(t, "3", env[constants.EnvUpgradeBlueReplicas])
}

func TestRollingExecutorIgnoresCompletedBlueGreenPopulation(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{Replicas: 7},
		Status: openbaov1alpha1.OpenBaoClusterStatus{BlueGreen: &openbaov1alpha1.BlueGreenStatus{
			Phase: openbaov1alpha1.PhaseIdle, BlueReplicas: 3,
		}},
	}
	variables := buildUpgradeExecutorEnv(cluster, ExecutorActionRollingStepDownLeader, "upgrade", "blue", "", portopenbao.ClientConfig{}, portopenbao.TrustBundleSource{})
	env := map[string]string{}
	for _, variable := range variables {
		env[variable.Name] = variable.Value
	}
	require.Equal(t, "7", env[constants.EnvClusterReplicas])
	require.NotContains(t, env, constants.EnvUpgradeBlueReplicas)
}
