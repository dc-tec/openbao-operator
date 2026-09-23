//go:build integration

package integration

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/bluegreen"
)

func TestBlueGreenTargetSurvivesSpecDriftAndManagerRestart(t *testing.T) {
	namespace := newTestNamespace(t)
	cluster := newMinimalClusterObj(namespace, "pinned-target")
	cluster.Spec.Replicas = 3
	cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{
		Strategy: openbaov1alpha1.UpdateStrategyBlueGreen, Image: "openbao-upgrade:dev", JWTAuthRole: "upgrade",
	}
	require.NoError(t, k8sClient.Create(ctx, cluster))
	createTLSSecret(t, namespace, cluster.Name)
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
		status.CurrentVersion = testPreviousOpenBaoVersion
		status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
			Phase: openbaov1alpha1.PhasePromoting, BlueRevision: "blue", GreenRevision: "green",
			BlueReplicas: 3, GreenReplicas: 3, GreenImage: cluster.Spec.Image, GreenVersion: cluster.Spec.Version,
		}
	})
	originalImage := cluster.Spec.Image
	key := client.ObjectKeyFromObject(cluster)
	require.NoError(t, k8sClient.Get(ctx, key, cluster))
	cluster.Spec.Replicas = 7
	require.NoError(t, k8sClient.Update(ctx, cluster))

	// A fresh manager must use persisted inputs, rather than its current spec or prior memory.
	newManager := func() *bluegreen.Manager {
		return bluegreen.NewManager(k8sClient, k8sScheme, nil, nil, portopenbao.ClientConfig{}, nil, nil, "")
	}
	require.NoError(t, k8sClient.Get(ctx, key, cluster))
	_, err := newManager().Reconcile(ctx, logr.Discard(), cluster)
	require.NoError(t, err)
	require.Equal(t, openbaov1alpha1.PhaseRollingBack, cluster.Status.BlueGreen.Phase)
	require.NoError(t, k8sClient.Status().Update(ctx, cluster))
	require.NoError(t, k8sClient.Get(ctx, key, cluster))
	require.Equal(t, originalImage, cluster.Status.BlueGreen.GreenImage)
	_, err = newManager().Reconcile(ctx, logr.Discard(), cluster)
	require.NoError(t, err)
	job := findUpgradeJobByAction(t, namespace, string(bluegreen.ActionRepairConsensus))
	env := envVarMap(job.Spec.Template.Spec.Containers[0].Env)
	require.Equal(t, "3", env[constants.EnvClusterReplicas])
	require.Equal(t, "3", env[constants.EnvUpgradeBlueReplicas])
	require.EqualValues(t, 7, cluster.Spec.Replicas)
}
