package restore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	api "github.com/kubebao/openbao-operator/api/v1alpha1"
	"github.com/kubebao/openbao-operator/internal/platform/constants"
)

func TestDisposableCleanupResumesWithoutApplicationHold(t *testing.T) {
	for _, checkpoint := range []string{"expired", "hold released", "cleanup complete", "failure already reported"} {
		t.Run(checkpoint, func(t *testing.T) {
			c, request := freshTargetFixture(t)
			request.Status.Phase = api.RestorePhaseUnknown
			request.Status.SubmissionClaim = &api.RestoreSubmissionClaim{ClaimedAt: metav1.NewTime(time.Now().Add(-time.Hour))}
			request.Status.Target = &api.RestoreTargetStatus{UID: "target", DataPVCUID: "data", BootstrapClusterID: "bootstrap", ReservedAt: metav1.Now()}
			cluster, pvc := freshTargetObjects(request)
			switch checkpoint {
			case "hold released":
				request.Status.Target.Cleanup = api.RestoreTargetCleanupPending
				delete(cluster.Annotations, constants.AnnotationRestoreHold)
			case "cleanup complete":
				request.Status.Target.Cleanup = api.RestoreTargetCleanupComplete
			case "failure already reported":
				request.Status.Phase = api.RestorePhaseFailed
				request.Status.Message = "Original staging failure"
			}
			if checkpoint != "cleanup complete" {
				require.NoError(t, c.Create(t.Context(), cluster))
				require.NoError(t, c.Create(t.Context(), pvc))
			}
			require.NoError(t, c.Status().Update(t.Context(), request))
			for range 9 {
				require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(request), request))
				_, err := NewManager(c, c.Scheme(), nil, nil, "").Reconcile(t.Context(), logr.Discard(), request)
				require.NoError(t, err)
			}
			require.Equal(t, api.RestoreTargetCleanupComplete, request.Status.Target.Cleanup)
			require.Equal(t, api.RestorePhaseFailed, request.Status.Phase)
			require.True(t, apierrors.IsNotFound(c.Get(t.Context(), client.ObjectKeyFromObject(cluster), cluster)))
			require.True(t, apierrors.IsNotFound(c.Get(t.Context(), client.ObjectKeyFromObject(pvc), pvc)))
			if checkpoint == "failure already reported" {
				require.Equal(t, "Original staging failure", request.Status.Message)
			}
		})
	}
}

func TestCleanupCollisionAndAdministratorAbandon(t *testing.T) {
	for _, bound := range []bool{false, true} {
		t.Run(map[bool]string{false: "creation collision", true: "replacement"}[bound], func(t *testing.T) {
			c, request := freshTargetFixture(t)
			request.Status.Phase = api.RestorePhaseFailed
			request.Status.Target = &api.RestoreTargetStatus{ReservedAt: metav1.Now(), Cleanup: api.RestoreTargetCleanupPending}
			if bound {
				request.Status.Target.UID = "original"
			}
			require.NoError(t, c.Status().Update(t.Context(), request))
			foreign := &api.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{Namespace: request.Namespace, Name: request.Spec.Cluster, UID: "foreign"}}
			require.NoError(t, c.Create(t.Context(), foreign))
			m := NewManager(c, c.Scheme(), nil, nil, "")
			_, err := m.Reconcile(t.Context(), logr.Discard(), request)
			require.NoError(t, err)
			if bound {
				require.Equal(t, api.RestoreTargetCleanupFailed, request.Status.Target.Cleanup)
				request.Annotations = map[string]string{constants.AnnotationRestoreAcknowledge: string(request.UID) + "/Abandon"}
				require.NoError(t, c.Update(t.Context(), request))
				_, err = m.Reconcile(t.Context(), logr.Discard(), request)
				require.NoError(t, err)
				require.Equal(t, api.RestoreAdministratorAbandon, request.Status.AdministratorDisposition)
			} else {
				require.Equal(t, api.RestoreTargetCleanupComplete, request.Status.Target.Cleanup)
			}
			require.NoError(t, c.Delete(t.Context(), request))
			require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(request), request))
			_, err = m.Reconcile(t.Context(), logr.Discard(), request)
			require.NoError(t, err)
			require.True(t, apierrors.IsNotFound(c.Get(t.Context(), client.ObjectKeyFromObject(request), request)))
			require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(foreign), foreign))
			require.False(t, foreign.Spec.Paused)
		})
	}
}

func TestRejectedTargetCreationDoesNotOwnExistingPVC(t *testing.T) {
	c, request := freshTargetFixture(t)
	request.Status.Phase = api.RestorePhaseFailed
	request.Status.Target = &api.RestoreTargetStatus{ReservedAt: metav1.Now(), Cleanup: api.RestoreTargetCleanupPending}
	require.NoError(t, c.Status().Update(t.Context(), request))
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Namespace: request.Namespace, Name: targetPVCName(request), UID: "foreign-data",
	}}
	require.NoError(t, c.Create(t.Context(), pvc))

	_, err := NewManager(c, c.Scheme(), nil, nil, "").Reconcile(t.Context(), logr.Discard(), request)
	require.NoError(t, err)
	require.Equal(t, api.RestoreTargetCleanupComplete, request.Status.Target.Cleanup)
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(pvc), pvc))
	require.Nil(t, pvc.DeletionTimestamp)
}

func TestCleanupCompletionSurvivesLostStatusResponse(t *testing.T) {
	c, request := freshTargetFixture(t)
	request.Status.Phase = api.RestorePhaseUnknown
	request.Status.Target = &api.RestoreTargetStatus{Cleanup: api.RestoreTargetCleanupPending, ReservedAt: metav1.Now()}
	require.NoError(t, c.Status().Update(t.Context(), request))
	lost := errors.New("cleanup status response lost")
	wrapped := interceptor.NewClient(c, interceptor.Funcs{SubResourcePatch: func(ctx context.Context, c client.Client, subresource string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
		if err := c.SubResource(subresource).Patch(ctx, obj, patch, opts...); err != nil {
			return err
		}
		return lost
	}})
	_, err := NewManager(wrapped, c.Scheme(), nil, nil, "").cleanupTarget(t.Context(), logr.Discard(), request)
	require.ErrorIs(t, err, lost)
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(request), request))
	_, err = NewManager(c, c.Scheme(), nil, nil, "").Reconcile(t.Context(), logr.Discard(), request)
	require.NoError(t, err)
	require.Equal(t, api.RestoreTargetCleanupComplete, request.Status.Target.Cleanup)
	require.Equal(t, api.RestorePhaseFailed, request.Status.Phase)
}

func TestCancellationBindsUnrecordedTargetPVC(t *testing.T) {
	const unownedLaterPVC = "unowned later PVC"
	for _, state := range []string{"target present", "late owned PVC", "foreign PVC", unownedLaterPVC, existingDataPVC} {
		t.Run(state, func(t *testing.T) {
			c, request := freshTargetFixture(t)
			request.Finalizers = []string{api.OpenBaoRestoreFinalizer}
			require.NoError(t, c.Update(t.Context(), request))
			cluster, pvc := freshTargetObjects(request)
			if state == "target present" || state == unownedLaterPVC || state == existingDataPVC {
				require.NoError(t, c.Create(t.Context(), cluster))
			}
			if state == "foreign PVC" || state == unownedLaterPVC {
				pvc.Annotations = nil
			}
			if state == existingDataPVC {
				pvc.CreationTimestamp = metav1.NewTime(cluster.CreationTimestamp.Add(-time.Minute))
			}
			require.NoError(t, c.Create(t.Context(), pvc))
			request.Status.Target = &api.RestoreTargetStatus{UID: cluster.UID, ReservedAt: metav1.Now()}
			require.NoError(t, c.Status().Update(t.Context(), request))
			require.NoError(t, c.Delete(t.Context(), request))
			for range 10 {
				err := c.Get(t.Context(), client.ObjectKeyFromObject(request), request)
				if apierrors.IsNotFound(err) {
					break
				}
				require.NoError(t, err)
				_, err = NewManager(c, c.Scheme(), nil, nil, "").Reconcile(t.Context(), logr.Discard(), request)
				require.NoError(t, err)
			}
			if state == "foreign PVC" || state == unownedLaterPVC || state == existingDataPVC {
				require.Equal(t, api.RestoreTargetCleanupFailed, request.Status.Target.Cleanup)
				require.Empty(t, request.Status.Target.DataPVCUID)
				require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(pvc), pvc))
				require.True(t, apierrors.IsNotFound(c.Get(t.Context(), client.ObjectKeyFromObject(cluster), cluster)), "refusing a PVC must not retain the disposable cluster")
				return
			}
			require.True(t, apierrors.IsNotFound(c.Get(t.Context(), client.ObjectKeyFromObject(request), request)))
			require.True(t, apierrors.IsNotFound(c.Get(t.Context(), client.ObjectKeyFromObject(cluster), cluster)))
			require.True(t, apierrors.IsNotFound(c.Get(t.Context(), client.ObjectKeyFromObject(pvc), pvc)))
		})
	}
}
