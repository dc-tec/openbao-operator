//go:build integration

package integration

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	"github.com/dc-tec/openbao-operator/internal/service/workload"
)

func TestWorkloadPreservesLegacyRestoreRevisionAcrossOperatorUpgrade(t *testing.T) {
	namespace := newTestNamespace(t)
	cluster := createMinimalCluster(t, namespace, "legacy-restore")
	cluster.Status.Initialized = true
	cluster.Status.Restore = &api.ClusterRestoreStatus{UID: "legacy-restore-uid"}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: resourceidentity.TLSServerSecretName(cluster)}}
	require.NoError(t, k8sClient.Create(ctx, secret))
	manager := workload.NewManager(newControllerClient(t), k8sScheme, constants.PlatformKubernetes)
	for _, pool := range []string{constants.LabelValueOpenBaoWorkloadPoolVoter, constants.LabelValueOpenBaoWorkloadPoolReadReplica} {
		t.Run(pool, func(t *testing.T) {
			spec := workload.StatefulSetSpec{Name: cluster.Name + "-" + pool, Pool: pool, Replicas: 1}
			// Seed the legacy annotation with the operator's SSA field manager, even
			// if the new builder stops deriving it from status during a future change.
			cluster.Spec.PodMetadata = &api.PodMetadataConfig{Annotations: map[string]string{
				constants.AnnotationRestoreRevision: cluster.Status.Restore.UID,
			}}
			require.NoError(t, manager.EnsureStatefulSet(ctx, logr.Discard(), cluster, "unchanged-config", spec))
			before := &appsv1.StatefulSet{}
			key := client.ObjectKey{Namespace: namespace, Name: spec.Name}
			require.NoError(t, k8sClient.Get(ctx, key, before))
			cluster.Spec.PodMetadata = nil
			require.NoError(t, manager.EnsureStatefulSet(ctx, logr.Discard(), cluster, "unchanged-config", spec))
			after := &appsv1.StatefulSet{}
			require.NoError(t, k8sClient.Get(ctx, key, after))
			require.Equal(t, before.Spec.Template, after.Spec.Template)
			require.Equal(t, before.Generation, after.Generation)
			require.Equal(t, cluster.Status.Restore.UID, after.Spec.Template.Annotations[constants.AnnotationRestoreRevision])
		})
	}
}
