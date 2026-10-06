package restore

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestRestoreAcknowledgementRetainsUncertainty(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"Abandon", "Resume", "wrong-operation/Abandon", "invalid"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			f := newRestoreRecoveryFixture(t)
			require.NoError(t, f.step(t))
			f.finishAcceptedRestore(t)
			request := f.restore(t)
			request.Annotations = map[string]string{
				constants.AnnotationRestoreAcknowledge: request.Status.Execution.OperationID + "/" + action,
			}
			require.NoError(t, f.base.Update(t.Context(), request))
			err := f.step(t)
			if action == "Resume" {
				require.NoError(t, err, "missing workload Pods must wait without releasing recovery")
			} else {
				require.NoError(t, err)
			}
			cluster, request := f.cluster(t), f.restore(t)
			require.Equal(t, openbaov1alpha1.RestorePhaseUnknown, request.Status.Phase)
			require.Equal(t, 1, f.jobCreates)
			if action == "Abandon" {
				require.True(t, cluster.Spec.Paused)
				require.Empty(t, cluster.Annotations[constants.AnnotationRestoreHold])
				require.Nil(t, cluster.Status.OperationLock)
				require.Equal(t, openbaov1alpha1.RestoreAdministratorDisposition(action), request.Status.AdministratorDisposition)
				require.NotNil(t, request.Status.CompletionTime)
			} else {
				require.Empty(t, request.Status.AdministratorDisposition)
				require.NotEmpty(t, cluster.Annotations[constants.AnnotationRestoreHold])
				f.requireLockHeld(t)
			}
		})
	}
}

func TestRestoreAcknowledgementRecoversLostWriteResponses(t *testing.T) {
	t.Parallel()
	for _, failCluster := range []bool{false, true} {
		t.Run(map[bool]string{false: "request status", true: "target hold"}[failCluster], func(t *testing.T) {
			t.Parallel()
			f := newRestoreRecoveryFixture(t)
			require.NoError(t, f.step(t))
			f.finishAcceptedRestore(t)
			request := f.restore(t)
			request.Annotations = map[string]string{constants.AnnotationRestoreAcknowledge: request.Status.Execution.OperationID + "/Abandon"}
			require.NoError(t, f.base.Update(t.Context(), request))
			lost := errors.New("write applied but response lost")
			f.client = interceptor.NewClient(f.base, interceptor.Funcs{
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					if err := c.Patch(ctx, obj, patch, opts...); err != nil {
						return err
					}
					if _, ok := obj.(*openbaov1alpha1.OpenBaoCluster); ok && failCluster {
						return lost
					}
					return nil
				},
				SubResourcePatch: func(ctx context.Context, c client.Client, subresource string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
					if err := c.SubResource(subresource).Patch(ctx, obj, patch, opts...); err != nil {
						return err
					}
					if _, ok := obj.(*openbaov1alpha1.OpenBaoRestore); ok && !failCluster {
						return lost
					}
					return nil
				},
			})
			require.ErrorIs(t, f.step(t), lost)
			f.client = f.base
			require.NoError(t, f.step(t))
			require.Equal(t, openbaov1alpha1.RestoreAdministratorAbandon, f.restore(t).Status.AdministratorDisposition)
			require.True(t, f.cluster(t).Spec.Paused)
			require.Nil(t, f.cluster(t).Status.OperationLock)
			require.Equal(t, 1, f.jobCreates)
		})
	}
}

func TestRestoreCancellationBeforeCommitReleasesHold(t *testing.T) {
	t.Parallel()
	f := newRestoreRecoveryFixture(t)
	manager := NewManager(f.client, f.scheme, nil, nil, "")
	request, cluster := f.restore(t), f.cluster(t)
	require.NoError(t, manager.holdTarget(t.Context(), request, cluster))
	require.NoError(t, f.base.Delete(t.Context(), request))
	require.NoError(t, f.step(t))
	require.Empty(t, f.cluster(t).Annotations[constants.AnnotationRestoreHold])
	require.Nil(t, f.cluster(t).Status.OperationLock)
	require.Zero(t, f.jobCreates)
}

func TestResumeWaitsForOrphanedExecutor(t *testing.T) {
	f := newRestoreRecoveryFixture(t)
	require.NoError(t, f.step(t))
	job := f.job(t)
	require.NoError(t, f.base.Delete(t.Context(), job))
	require.NoError(t, f.step(t))
	r := f.restore(t)
	require.Equal(t, openbaov1alpha1.RestorePhaseUnknown, r.Status.Phase)
	pod := stoppedRestoreExecutor()
	pod.Namespace = r.Namespace
	pod.OwnerReferences[0].Name = job.Name
	pod.OwnerReferences[0].UID = job.UID
	pod.Status.Phase = corev1.PodRunning
	pod.Status.ContainerStatuses = nil
	require.NoError(t, f.base.Create(t.Context(), pod))
	r.Annotations = map[string]string{constants.AnnotationRestoreAcknowledge: string(r.UID) + "/Resume"}
	require.NoError(t, f.base.Update(t.Context(), r))
	require.NoError(t, f.step(t))
	require.Contains(t, f.restore(t).Status.Message, "report termination")
	f.requireLockHeld(t)
}
