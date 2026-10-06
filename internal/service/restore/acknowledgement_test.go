package restore

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestAppliedRetainedTargetAbandonDoesNotComplete(t *testing.T) {
	for _, recorded := range []bool{false, true} {
		t.Run(map[bool]string{false: "new acknowledgement", true: "recorded disposition"}[recorded], func(t *testing.T) {
			c, request := freshTargetFixture(t)
			request.Spec.TargetLifecycle = openbaov1alpha1.RestoreTargetLifecycleRetain
			request.Annotations = map[string]string{constants.AnnotationRestoreAcknowledge: string(request.UID) + "/Abandon"}
			require.NoError(t, c.Update(t.Context(), request))
			request.Status.Phase = openbaov1alpha1.RestorePhaseUnknown
			appliedAt := metav1.Now()
			request.Status.Target = &openbaov1alpha1.RestoreTargetStatus{UID: "target", AppliedAt: &appliedAt}
			cluster := projectRestoreTarget(request)
			cluster.UID = request.Status.Target.UID
			if recorded {
				request.Status.AdministratorDisposition = openbaov1alpha1.RestoreAdministratorAbandon
				cluster.Spec.Paused = true
				delete(cluster.Annotations, constants.AnnotationRestoreOrigin)
			} else {
				cluster.Annotations[constants.AnnotationRestoreHold] = string(request.UID)
			}
			require.NoError(t, c.Create(t.Context(), cluster))
			require.NoError(t, c.Status().Update(t.Context(), request))

			_, err := NewManager(c, c.Scheme(), nil, nil, "").Reconcile(t.Context(), logr.Discard(), request)
			require.NoError(t, err)
			require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(request), request))
			require.Equal(t, openbaov1alpha1.RestorePhaseUnknown, request.Status.Phase)
			require.Equal(t, openbaov1alpha1.RestoreAdministratorAbandon, request.Status.AdministratorDisposition)
			require.NotNil(t, request.Status.Target.AppliedAt)
			require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(cluster), cluster))
			require.True(t, cluster.Spec.Paused)
			require.Empty(t, cluster.Annotations[constants.AnnotationRestoreHold])
		})
	}
}

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

func TestRetainedFailureBeforeExecutionHandsOffPausedTarget(t *testing.T) {
	c, request := freshTargetFixture(t)
	request.Spec.TargetLifecycle = openbaov1alpha1.RestoreTargetLifecycleRetain
	require.NoError(t, c.Update(t.Context(), request))
	request.Status.Phase = openbaov1alpha1.RestorePhaseFailed
	request.Status.Target = &openbaov1alpha1.RestoreTargetStatus{UID: "target", ReservedAt: metav1.Now()}
	require.NoError(t, c.Status().Update(t.Context(), request))
	cluster := projectRestoreTarget(request)
	cluster.UID = "target"
	require.NoError(t, c.Create(t.Context(), cluster))
	_, err := NewManager(c, c.Scheme(), nil, nil, "").Reconcile(t.Context(), logr.Discard(), request)
	require.NoError(t, err)
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(cluster), cluster))
	require.True(t, cluster.Spec.Paused)
	require.Empty(t, cluster.Annotations[constants.AnnotationRestoreOrigin])
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

func TestUnrecoverableResumeExplainsAbandon(t *testing.T) {
	for _, state := range []string{"cleanup failed", "disposable", "job unbound", "target absent", "target replaced"} {
		t.Run(state, func(t *testing.T) {
			c, request := freshTargetFixture(t)
			request.Spec.TargetLifecycle = openbaov1alpha1.RestoreTargetLifecycleRetain
			if state == "disposable" {
				request.Spec.TargetLifecycle = openbaov1alpha1.RestoreTargetLifecycleDisposable
			}
			request.Annotations = map[string]string{constants.AnnotationRestoreAcknowledge: string(request.UID) + "/Resume"}
			require.NoError(t, c.Update(t.Context(), request))
			request.Status.Phase = openbaov1alpha1.RestorePhaseUnknown
			request.Status.Target = &openbaov1alpha1.RestoreTargetStatus{UID: "original", ReservedAt: metav1.Now()}
			if state == "cleanup failed" {
				request.Status.Target.Cleanup = openbaov1alpha1.RestoreTargetCleanupFailed
			}
			if state == "target replaced" || state == "job unbound" {
				cluster := projectRestoreTarget(request)
				cluster.UID = request.Status.Target.UID
				if state == "target replaced" {
					cluster.UID = "replacement"
				}
				require.NoError(t, c.Create(t.Context(), cluster))
			}
			require.NoError(t, c.Status().Update(t.Context(), request))
			_, err := NewManager(c, c.Scheme(), nil, nil, "").reconcileAcknowledgement(t.Context(), logr.Discard(), request)
			require.NoError(t, err)
			require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(request), request))
			require.Contains(t, request.Status.Message, "use Abandon")
			require.Empty(t, request.Status.AdministratorDisposition)
		})
	}
}
