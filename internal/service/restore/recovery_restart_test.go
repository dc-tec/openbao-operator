package restore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	api "github.com/kubebao/openbao-operator/api/v1alpha1"
	"github.com/kubebao/openbao-operator/internal/platform/constants"
	portopenbao "github.com/kubebao/openbao-operator/internal/port/openbao"
)

type restartClient struct {
	membership portopenbao.RaftConfigurationResponse
	stepDowns  int
	err        error
}

func (c *restartClient) ReadRaftConfiguration(context.Context) (*portopenbao.RaftConfigurationResponse, error) {
	return &c.membership, c.err
}

func (c *restartClient) StepDownLeader(context.Context) error {
	c.stepDowns++
	c.membership.Config.Servers[0].Leader = false
	c.membership.Config.Servers[1].Leader = true
	return c.err
}

func restartFixture(t *testing.T) (*restoreRecoveryFixture, *restartClient, []corev1.Pod) {
	t.Helper()
	f := newRestoreRecoveryFixture(t)
	require.NoError(t, f.step(t))
	f.finishAcceptedRestore(t)
	cluster := f.cluster(t)
	cluster.Spec.TLS.Mode = api.TLSModeACME
	cluster.Spec.ReadReplicas = &api.ReadReplicaConfig{Replicas: 1}
	require.NoError(t, f.base.Update(t.Context(), cluster))
	request := f.restore(t)
	request.Annotations = map[string]string{constants.AnnotationRestoreAcknowledge: string(request.UID) + "/Resume"}
	require.NoError(t, f.base.Update(t.Context(), request))
	bao := &restartClient{}
	var pods []corev1.Pod
	for _, name := range []string{cluster.Name, cluster.Name + "-read"} {
		replicas := cluster.Spec.Replicas
		if name != cluster.Name {
			replicas = 1
		}
		sts := &appsv1.StatefulSet{}
		err := f.base.Get(t.Context(), client.ObjectKey{Namespace: cluster.Namespace, Name: name}, sts)
		if apierrors.IsNotFound(err) {
			sts = managedVoterStatefulSetForCluster(&appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cluster.Namespace},
				Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
			}, cluster)
			require.NoError(t, f.base.Create(t.Context(), sts))
		} else {
			require.NoError(t, err)
		}
		sts.UID = types.UID(name + "-sts")
		sts.Spec.UpdateStrategy.Type = appsv1.OnDeleteStatefulSetStrategyType
		require.NoError(t, f.base.Update(t.Context(), sts))
		for i := int32(0); i < replicas; i++ {
			name := fmt.Sprintf("%s-%d", name, i)
			pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Namespace: cluster.Namespace, Name: name, UID: types.UID(name + "-original"),
				OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "StatefulSet", Name: sts.Name, UID: sts.UID, Controller: ptr.To(true)}},
			}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
			require.NoError(t, f.base.Create(t.Context(), &pod))
			pods = append(pods, pod)
			bao.membership.Config.Servers = append(bao.membership.Config.Servers, portopenbao.RaftServer{
				NodeID: name, Voter: sts.Name == cluster.Name, Leader: name == cluster.Name+"-0",
			})
		}
	}
	return f, bao, pods
}

func restartManager(f *restoreRecoveryFixture, bao *restartClient) *Manager {
	m := NewManager(f.client, f.scheme, nil, nil, "").WithReader(f.base)
	m.recoveryClientFor = func(context.Context, *api.OpenBaoCluster, string) (RecoveryClient, error) { return bao, nil }
	m.readHealth = func(_ context.Context, config portopenbao.ClientConfig) (*portopenbao.HealthStatus, error) {
		leader := ""
		for _, peer := range bao.membership.Config.Servers {
			if peer.Leader {
				leader = peer.NodeID
			}
		}
		return &portopenbao.HealthStatus{Initialized: true, Standby: !strings.Contains(config.BaseURL, "://"+leader+".")}, nil
	}
	return m
}

func TestResumeRestartsOnceAcrossLostResponses(t *testing.T) {
	for _, lostAt := range []string{"intent", "delete", "completion", "release", "disposition"} {
		t.Run(lostAt, func(t *testing.T) {
			f, bao, originals := restartFixture(t)
			lost := false
			deletes := map[string]int{}
			f.client = interceptor.NewClient(f.base, interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					pod, ok := obj.(*corev1.Pod)
					require.True(t, ok)
					require.NotNil(t, f.restore(t).Status.Restart, "intent must precede deletion")
					require.Equal(t, "true", pod.Annotations[constants.AnnotationMaintenance])
					require.NotEmpty(t, f.cluster(t).Annotations[constants.AnnotationRestoreHold])
					f.requireLockHeld(t)
					deletes[pod.Name]++
					require.Equal(t, 1, deletes[pod.Name], "replacement must never be restarted again")
					if err := c.Delete(ctx, obj, opts...); err != nil {
						return err
					}
					if lostAt == "delete" && !lost {
						lost = true
						return errors.New("response lost")
					}
					return nil
				},
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					if err := c.Patch(ctx, obj, patch, opts...); err != nil {
						return err
					}
					if _, ok := obj.(*api.OpenBaoCluster); ok && lostAt == "release" && !lost {
						lost = true
						return errors.New("response lost")
					}
					return nil
				},
				SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
					if err := c.SubResource(sub).Patch(ctx, obj, patch, opts...); err != nil {
						return err
					}
					if r, ok := obj.(*api.OpenBaoRestore); ok && !lost && r.Status.Restart != nil &&
						((lostAt == "intent" && r.Status.Restart.CompletedAt == nil) ||
							(lostAt == "completion" && r.Status.Restart.CompletedAt != nil) ||
							(lostAt == "disposition" && r.Status.AdministratorDisposition != "")) {
						lost = true
						return errors.New("response lost")
					}
					return nil
				},
			})
			for range 20 {
				request := f.restore(t)
				_, err := restartManager(f, bao).Reconcile(t.Context(), logr.Discard(), request)
				if err != nil {
					require.ErrorContains(t, err, "response lost")
				}
				// Model StatefulSet replacement, leaving each new Pod unready for one poll.
				for i := range originals {
					pod := &corev1.Pod{}
					err := f.base.Get(t.Context(), client.ObjectKeyFromObject(&originals[i]), pod)
					if apierrors.IsNotFound(err) {
						pod = originals[i].DeepCopy()
						pod.ResourceVersion = ""
						pod.UID += "-replacement"
						pod.Status.Conditions[0].Status = corev1.ConditionFalse
						require.NoError(t, f.base.Create(t.Context(), pod))
					} else {
						require.NoError(t, err)
						pod.Status.Conditions[0].Status = corev1.ConditionTrue
						require.NoError(t, f.base.Status().Update(t.Context(), pod))
					}
				}
				if f.restore(t).Status.AdministratorDisposition != "" {
					break
				}
			}
			request := f.restore(t)
			require.True(t, lost)
			require.Len(t, deletes, len(originals))
			require.Equal(t, 1, bao.stepDowns)
			require.Equal(t, api.RestoreAdministratorResume, request.Status.AdministratorDisposition)
			require.Equal(t, api.RestorePhaseUnknown, request.Status.Phase)
			require.NotNil(t, request.Status.Restart.CompletedAt)
			require.Empty(t, f.cluster(t).Annotations[constants.AnnotationRestoreHold])
			require.Nil(t, f.cluster(t).Status.OperationLock)
			require.Equal(t, 1, f.jobCreates)
		})
	}
}

func TestResumeCanAbandonDuringRestart(t *testing.T) {
	for _, released := range []bool{false, true} {
		t.Run(fmt.Sprintf("holdReleased=%t", released), func(t *testing.T) {
			f, bao, _ := restartFixture(t)
			m := restartManager(f, bao)
			_, err := m.Reconcile(t.Context(), logr.Discard(), f.restore(t))
			require.NoError(t, err)
			request := f.restore(t)
			require.NotNil(t, request.Status.Restart)
			if released {
				// Model a crash after releasing the target but before disposition is stored.
				request.Status.Restart.CompletedAt = ptr.To(metav1.Now())
				require.NoError(t, f.base.Status().Update(t.Context(), request))
				cluster := f.cluster(t)
				delete(cluster.Annotations, constants.AnnotationRestoreHold)
				require.NoError(t, f.base.Update(t.Context(), cluster))
			}
			request.Annotations[constants.AnnotationRestoreAcknowledge] = string(request.UID) + "/Abandon"
			require.NoError(t, f.base.Update(t.Context(), request))
			_, err = m.Reconcile(t.Context(), logr.Discard(), request)
			require.NoError(t, err)
			require.Equal(t, api.RestoreAdministratorAbandon, f.restore(t).Status.AdministratorDisposition)
			require.True(t, f.cluster(t).Spec.Paused)
		})
	}
}

func TestResumePreservesHoldWhenRecoveryChanges(t *testing.T) {
	for _, change := range []string{"trust", "membership", "statefulset", "target identity", "lock"} {
		t.Run(change, func(t *testing.T) {
			f, bao, originals := restartFixture(t)
			m := restartManager(f, bao)
			_, err := m.Reconcile(t.Context(), logr.Discard(), f.restore(t))
			require.NoError(t, err)
			switch change {
			case "trust":
				bao.err = errors.New("operator JWT rejected")
			case "membership":
				bao.membership.Config.Servers = bao.membership.Config.Servers[:1]
			case "statefulset":
				sts := &appsv1.StatefulSet{}
				require.NoError(t, f.base.Get(t.Context(), client.ObjectKey{Namespace: originals[0].Namespace, Name: originals[0].OwnerReferences[0].Name}, sts))
				sts.UID = "new-statefulset"
				require.NoError(t, f.base.Update(t.Context(), sts))
			default:
				cluster := f.cluster(t)
				if change == "target identity" {
					cluster.UID = "new-target"
				} else {
					cluster.Status.OperationLock.Holder = "other"
				}
				if change == "target identity" {
					require.NoError(t, f.base.Update(t.Context(), cluster))
				} else {
					require.NoError(t, f.base.Status().Update(t.Context(), cluster))
				}
			}
			result, err := m.Reconcile(t.Context(), logr.Discard(), f.restore(t))
			require.Empty(t, f.restore(t).Status.AdministratorDisposition)
			require.NotEmpty(t, f.cluster(t).Annotations[constants.AnnotationRestoreHold])
			if change == "statefulset" || change == "lock" {
				// Operator-defined blockers stay visible and keep the regular poll.
				require.NoError(t, err)
				require.Equal(t, restoreRequeueJobCheck, result.RequeueAfter)
				condition := meta.FindStatusCondition(f.restore(t).Status.Conditions, constants.RestoreRecoveryReleasedConditionType)
				require.NotNil(t, condition)
				require.Equal(t, ReasonRecoveryBlocked, condition.Reason)
				require.NotContains(t, condition.Message, "inspect the controller error")
			}
			for _, original := range originals {
				pod := &corev1.Pod{}
				require.NoError(t, f.base.Get(t.Context(), client.ObjectKeyFromObject(&original), pod))
				require.Equal(t, original.UID, pod.UID)
			}
		})
	}
}

func TestResumeReportsBlockedPodAndSanitizesProviderErrors(t *testing.T) {
	for _, state := range []string{"unready pod", "operator access"} {
		t.Run(state, func(t *testing.T) {
			f, bao, pods := restartFixture(t)
			m := restartManager(f, bao)
			if state == "unready pod" {
				pod := pods[1].DeepCopy()
				pod.Status.Conditions = nil
				require.NoError(t, f.base.Status().Update(t.Context(), pod))
			} else {
				bao.err = errors.New("provider-response-secret-token")
			}
			_, _ = m.Reconcile(t.Context(), logr.Discard(), f.restore(t))
			request := f.restore(t)
			condition := meta.FindStatusCondition(request.Status.Conditions, constants.RestoreRecoveryReleasedConditionType)
			require.NotNil(t, condition)
			require.Equal(t, metav1.ConditionFalse, condition.Status)
			require.NotContains(t, request.Status.Message, "provider-response-secret-token")
			if state == "unready pod" {
				require.Equal(t, ReasonRecoveryWaitingForPod, condition.Reason)
				require.Contains(t, request.Status.Message, pods[1].Name)
			} else {
				require.Equal(t, ReasonRecoveryAccessUnavailable, condition.Reason)
			}
			f.requireLockHeld(t)
		})
	}
}

func TestRetainedResumeWaitsForManagedTemplate(t *testing.T) {
	f, bao, originals := restartFixture(t)
	cluster := f.cluster(t)
	cluster.Spec.Replicas = 1
	cluster.Spec.ReadReplicas = nil
	request := f.restore(t)
	cluster.Annotations[constants.AnnotationRestoreOrigin] = string(request.UID)
	require.NoError(t, f.base.Update(t.Context(), cluster))
	sts := &appsv1.StatefulSet{}
	require.NoError(t, f.base.Get(t.Context(), client.ObjectKeyFromObject(cluster), sts))
	sts.Spec.Replicas = ptr.To(int32(1))
	require.NoError(t, f.base.Update(t.Context(), sts))
	bao.membership.Config.Servers = bao.membership.Config.Servers[:1]
	request.Status.Target = &api.RestoreTargetStatus{UID: cluster.UID}
	require.NoError(t, f.base.Status().Update(t.Context(), request))
	m := restartManager(f, bao)
	calls := 0
	preparationError := errors.New("template patch failed")
	m.WithRetainedTargetPreparer(func(_ context.Context, target *api.OpenBaoCluster, uid types.UID) (string, error) {
		calls++
		require.Equal(t, sts.UID, uid)
		require.Equal(t, string(request.UID), target.Annotations[constants.AnnotationRestoreHold])
		require.NotNil(t, f.restore(t).Status.Restart, "durable authenticated intent must precede template preparation")
		return "managed-config", preparationError
	})
	done, err := m.restartAcknowledgedTarget(t.Context(), request, cluster)
	require.NoError(t, err)
	require.False(t, done)
	require.Zero(t, calls)
	request = f.restore(t)
	done, err = m.restartAcknowledgedTarget(t.Context(), request, cluster)
	require.ErrorIs(t, err, preparationError)
	require.False(t, done)
	require.NoError(t, f.base.Get(t.Context(), client.ObjectKeyFromObject(&originals[0]), &corev1.Pod{}))
	preparationError = nil
	// Model a replacement that started during a partial handoff and cannot become
	// Ready on the old template. It still needs the managed-template restart.
	pod := originals[0].DeepCopy()
	require.NoError(t, f.base.Delete(t.Context(), pod))
	pod.ResourceVersion = ""
	pod.UID = "old-template-replacement"
	pod.Status.Conditions[0].Status = corev1.ConditionFalse
	require.NoError(t, f.base.Create(t.Context(), pod))
	done, err = m.restartAcknowledgedTarget(t.Context(), request, cluster)
	require.ErrorContains(t, err, "Restarting Pod")
	require.False(t, done)
	require.True(t, apierrors.IsNotFound(f.base.Get(t.Context(), client.ObjectKeyFromObject(pod), &corev1.Pod{})))
	pod.ResourceVersion = ""
	pod.UID = "managed-replacement"
	pod.Annotations = map[string]string{constants.AnnotationConfigHash: "managed-config"}
	require.NoError(t, f.base.Create(t.Context(), pod))
	done, err = m.restartAcknowledgedTarget(t.Context(), request, cluster)
	require.ErrorContains(t, err, "become Ready")
	require.False(t, done)
	require.Nil(t, f.restore(t).Status.Restart.CompletedAt)
	pod.Status.Conditions[0].Status = corev1.ConditionTrue
	require.NoError(t, f.base.Status().Update(t.Context(), pod))
	done, err = m.restartAcknowledgedTarget(t.Context(), request, cluster)
	require.NoError(t, err)
	require.False(t, done)
	require.NotNil(t, f.restore(t).Status.Restart.CompletedAt)
	done, err = m.restartAcknowledgedTarget(t.Context(), f.restore(t), cluster)
	require.NoError(t, err)
	require.True(t, done)
}
