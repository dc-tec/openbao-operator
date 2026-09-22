//go:build integration
// +build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestStatefulSetPartitionOptimisticLockConflict(t *testing.T) {
	namespace := newTestNamespace(t)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "rolling-conflict", Namespace: namespace},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: "rolling-conflict",
			Replicas:    ptr.To(int32(1)),
			Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": "rolling-conflict"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "rolling-conflict"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name: "bao", Image: "openbao/openbao:2.6.2",
				}}},
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type:          appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{Partition: ptr.To(int32(1))},
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, sts))

	stale := &appsv1.StatefulSet{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(sts), stale))
	current := stale.DeepCopy()
	current.Annotations = map[string]string{"example.com/other-writer": "true"}
	require.NoError(t, k8sClient.Update(ctx, current))

	partitionPatch := stale.DeepCopy()
	partitionPatch.Spec.UpdateStrategy.RollingUpdate.Partition = ptr.To(int32(0))
	err := k8sClient.Patch(ctx, partitionPatch, client.MergeFromWithOptions(stale, client.MergeFromWithOptimisticLock{}))
	require.Error(t, err)
	require.True(t, apierrors.IsConflict(err), "expected API server conflict, got %v", err)

	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(sts), current))
	retry := current.DeepCopy()
	retry.Spec.UpdateStrategy.RollingUpdate.Partition = ptr.To(int32(0))
	require.NoError(t, k8sClient.Patch(ctx, retry, client.MergeFromWithOptions(current, client.MergeFromWithOptimisticLock{})))
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(sts), current))
	require.Equal(t, int32(0), *current.Spec.UpdateStrategy.RollingUpdate.Partition)
}
