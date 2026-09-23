//go:build integration
// +build integration

package integration

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/bluegreen"
)

func TestBlueGreenRollbackCleanupWaitsForDataDeletion(t *testing.T) {
	namespace := newTestNamespace(t)
	cluster := newMinimalClusterObj(namespace, "rollback-data")
	cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{
		Strategy: openbaov1alpha1.UpdateStrategyBlueGreen, Image: "openbao-upgrade:dev", JWTAuthRole: "upgrade",
	}
	require.NoError(t, k8sClient.Create(ctx, cluster))
	createTLSSecret(t, namespace, cluster.Name)
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
		status.CurrentVersion = testPreviousOpenBaoVersion
		status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
			BlueReplicas:  cluster.Spec.Replicas,
			GreenReplicas: cluster.Spec.Replicas,
			GreenImage:    cluster.Spec.Image,
			GreenVersion:  cluster.Spec.Version,
			Phase:         openbaov1alpha1.PhaseRollbackCleanup, BlueRevision: "blue",
			GreenRevision: "green", OperationID: "bg-v2-cleanup",
		}
	})
	bluePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.Name + "-blue-0", Namespace: namespace, Labels: map[string]string{
			constants.LabelAppInstance: cluster.Name, constants.LabelAppName: constants.LabelValueAppNameOpenBao,
			constants.LabelOpenBaoRevision: "blue", portopenbao.LabelActive: testTrueString,
		}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "noop", Image: "busybox:1.36"}}},
	}
	require.NoError(t, k8sClient.Create(ctx, bluePod))
	newClaim := func(name string) *corev1.PersistentVolumeClaim {
		claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace,
			Labels:      resourceidentity.Labels(cluster),
			Annotations: map[string]string{constants.AnnotationOpenBaoOwnerUID: string(cluster.UID)},
			Finalizers:  []string{"test.openbao.org/storage-held"},
		}, Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			}}}
		require.NoError(t, k8sClient.Create(ctx, claim))
		return claim
	}
	green := newClaim("data-" + cluster.Name + "-green-0")
	blue := newClaim("data-" + cluster.Name + "-blue-0")
	cache := newClaim(cluster.Name + "-acme-cache")
	mount := bluePod.DeepCopy()
	mount.ResourceVersion, mount.UID = "", ""
	mount.Name, mount.Labels = "foreign-mount", nil
	mount.Finalizers = []string{"test.openbao.org/pod-held"}
	mount.Spec.Volumes = []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: green.Name},
	}}}
	require.NoError(t, k8sClient.Create(ctx, mount))
	require.NoError(t, k8sClient.Delete(ctx, mount, client.GracePeriodSeconds(0)))
	verifier := security.NewImageVerifier(logr.Discard(), k8sClient, nil)
	manager := bluegreen.NewManager(
		k8sClient, k8sScheme, nil, nil, portopenbao.ClientConfig{}, verifier, verifier, "",
	).WithReader(k8sClient)
	reconcile := func() {
		t.Helper()
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster))
		_, err := manager.Reconcile(ctx, logr.Discard(), cluster)
		require.NoError(t, err)
		require.NoError(t, k8sClient.Status().Update(ctx, cluster))
	}
	reconcile()
	job := findUpgradeJobByAction(t, namespace, string(bluegreen.ActionRemoveGreenPeers))
	markJobSucceeded(t, job)
	for range 2 {
		reconcile()
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(green), green))
		require.Nil(t, green.DeletionTimestamp, "a foreign Terminating Pod still references the claim")
		require.Equal(t, openbaov1alpha1.PhaseRollbackCleanup, cluster.Status.BlueGreen.Phase)
	}
	// Envtest has no Pod or storage controllers. Simulate their completion;
	// the runtime must leave these finalizers untouched.
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(mount), mount))
	mount.Finalizers = nil
	require.NoError(t, k8sClient.Update(ctx, mount))
	for range 2 {
		reconcile()
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(green), green))
		require.NotNil(t, green.DeletionTimestamp)
		require.Contains(t, green.Finalizers, "test.openbao.org/storage-held")
		require.Equal(t, openbaov1alpha1.PhaseRollbackCleanup, cluster.Status.BlueGreen.Phase)
	}
	green.Finalizers = nil
	require.NoError(t, k8sClient.Update(ctx, green))
	reconcile()
	require.Equal(t, openbaov1alpha1.PhaseIdle, cluster.Status.BlueGreen.Phase)
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(green), &corev1.PersistentVolumeClaim{})
	require.True(t, apierrors.IsNotFound(err))
	for _, preserved := range []*corev1.PersistentVolumeClaim{blue, cache} {
		got := &corev1.PersistentVolumeClaim{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(preserved), got))
		require.Equal(t, preserved.UID, got.UID)
		require.Nil(t, got.DeletionTimestamp)
	}
}
