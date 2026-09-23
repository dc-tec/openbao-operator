//go:build integration

package integration

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/bluegreen"
)

func TestBlueGreenUpgradeWaitsForBootstrapReplicaCount(t *testing.T) {
	namespace := newTestNamespace(t)
	cluster := newMinimalClusterObj(namespace, "bootstrap-target")
	cluster.Spec.Replicas = 3
	cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{
		Strategy: openbaov1alpha1.UpdateStrategyBlueGreen, Image: "openbao-upgrade:dev", JWTAuthRole: "upgrade",
	}
	require.NoError(t, k8sClient.Create(ctx, cluster))
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
		status.CurrentVersion = testPreviousOpenBaoVersion
		status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
			Phase: openbaov1alpha1.PhaseIdle, BlueRevision: "blue",
		}
	})
	labels := map[string]string{"app": cluster.Name}
	blue := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.Name + "-blue", Namespace: namespace},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To(int32(1)), ServiceName: cluster.Name,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "bao", Image: constants.GetOpenBaoImage(testPreviousOpenBaoVersion)},
				}},
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, blue))
	manager := bluegreen.NewManager(k8sClient, k8sScheme, nil, nil, portopenbao.ClientConfig{}, nil, nil, "")
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster))
	_, err := manager.Reconcile(ctx, logr.Discard(), cluster)
	require.NoError(t, err)
	require.Equal(t, openbaov1alpha1.PhaseIdle, cluster.Status.BlueGreen.Phase)
	require.Zero(t, cluster.Status.BlueGreen.BlueReplicas)
	require.NoError(t, k8sClient.Status().Update(ctx, cluster))

	*blue.Spec.Replicas = cluster.Spec.Replicas
	require.NoError(t, k8sClient.Update(ctx, blue))
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster))
	_, err = manager.Reconcile(ctx, logr.Discard(), cluster)
	require.NoError(t, err)
	require.Equal(t, openbaov1alpha1.PhaseDeployingGreen, cluster.Status.BlueGreen.Phase)
	require.EqualValues(t, 3, cluster.Status.BlueGreen.BlueReplicas)
	require.NoError(t, k8sClient.Status().Update(ctx, cluster))
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster))
	require.EqualValues(t, 3, cluster.Status.BlueGreen.BlueReplicas)
}

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
