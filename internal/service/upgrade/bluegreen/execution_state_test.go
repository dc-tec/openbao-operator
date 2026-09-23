package bluegreen

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func executionTestStatefulSet(cluster *openbaov1alpha1.OpenBaoCluster, revision string, replicas int32, image string) *appsv1.StatefulSet {
	name := cluster.Name
	if revision != "" {
		name += "-" + revision
	}
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cluster.Namespace},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: constants.ContainerBao, Image: image},
			}}},
		},
	}
}

func TestEnsureExecutionStateRecoversLegacyWorkloads(t *testing.T) {
	for _, phase := range []openbaov1alpha1.BlueGreenPhase{
		openbaov1alpha1.PhasePromoting, openbaov1alpha1.PhaseCleanup, openbaov1alpha1.PhaseRestoringReadReplicas,
	} {
		t.Run(string(phase), func(t *testing.T) {
			cluster := newPhaseMachineCluster()
			cluster.Status.BlueGreen.Phase = phase
			blue := executionTestStatefulSet(cluster, "blue", 3, "openbao:2.4.4")
			green := executionTestStatefulSet(cluster, deploymentNameSuffix, 3, "openbao:2.5.0")
			if phase == openbaov1alpha1.PhaseRestoringReadReplicas {
				cluster.Status.BlueGreen.BlueRevision = deploymentNameSuffix
				cluster.Status.BlueGreen.GreenRevision = ""
			}
			cluster.Spec.Replicas = 7
			cluster.Spec.Image = "openbao:2.6.2"
			manager := &Manager{client: fake.NewClientBuilder().WithScheme(newBlueGreenTestScheme(t)).WithObjects(blue, green).Build()}
			require.NoError(t, manager.ensureExecutionState(t.Context(), cluster))
			require.EqualValues(t, 3, cluster.Status.BlueGreen.BlueReplicas)
			require.EqualValues(t, 3, cluster.Status.BlueGreen.GreenReplicas)
			require.Equal(t, "openbao:2.5.0", cluster.Status.BlueGreen.GreenImage)
			cluster.Spec.Replicas = 9
			require.NoError(t, manager.ensureExecutionState(t.Context(), cluster))
			require.EqualValues(t, 3, cluster.Status.BlueGreen.BlueReplicas)
		})
	}
}

func TestEnsureExecutionStateDoesNotRecoverDriftedTargetFromSpec(t *testing.T) {
	cluster := newPhaseMachineCluster()
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseCleanup
	manager := &Manager{client: fake.NewClientBuilder().WithScheme(newBlueGreenTestScheme(t)).Build()}
	require.ErrorContains(t, manager.ensureExecutionState(t.Context(), cluster), "recover Green target")
	require.Zero(t, cluster.Status.BlueGreen.GreenReplicas)
}
