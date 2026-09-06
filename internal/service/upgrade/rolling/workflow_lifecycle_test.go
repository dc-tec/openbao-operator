package rolling

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	controllermetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/platform/testutil/openbao"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func TestRollingLifecyclePersistenceOrdering(t *testing.T) {
	const partitionWrite = "partition"
	t.Parallel()
	for _, completing := range []bool{false, true} {
		operation := "start"
		if completing {
			operation = "complete"
		}
		for _, failure := range []string{"none", partitionWrite, "apply", "read-back"} {
			if completing && failure == partitionWrite {
				continue
			}
			t.Run(operation+"/"+failure, func(t *testing.T) {
				t.Parallel()
				cluster, objects := rollingLifecycleObjects(completing)
				cluster.Namespace = operation + "-" + failure
				for _, obj := range objects {
					obj.SetNamespace(cluster.Namespace)
				}
				base := fake.NewClientBuilder().WithScheme(newRollingEventTestScheme(t)).
					WithObjects(append(objects, withoutUpgradeStatus(cluster))...).
					WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).Build()
				if completing {
					seedUpgradeStatus(t, base, cluster)
				}
				recorder := events.NewFakeRecorder(10)
				metricName := "openbao_upgrade_total"
				if completing {
					metricName = "openbao_upgrade_success_total"
				}
				before := lifecycleCounter(t, metricName, cluster.Namespace)
				var writes []string
				applied := false
				writeErr := errors.New("injected lifecycle write failure")
				assertNotReported := func() {
					t.Helper()
					require.Empty(t, recorder.Events, "events must follow successful persistence and read-back")
					require.Equal(t, before, lifecycleCounter(t, metricName, cluster.Namespace))
				}
				c := interceptor.NewClient(base, interceptor.Funcs{
					Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
						writes = append(writes, partitionWrite)
						assertNotReported()
						if failure == partitionWrite {
							return writeErr
						}
						return c.Patch(ctx, obj, patch, opts...)
					},
					SubResourceApply: func(ctx context.Context, c client.Client, subresource string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
						writes = append(writes, "apply")
						assertNotReported()
						if failure == "apply" {
							return writeErr
						}
						err := c.SubResource(subresource).Apply(ctx, obj, opts...)
						applied = err == nil
						return err
					},
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*openbaov1alpha1.OpenBaoCluster); ok && applied {
							writes = append(writes, "read-back")
							assertNotReported()
							if failure == "read-back" {
								return writeErr
							}
						}
						return c.Get(ctx, key, obj, opts...)
					},
				})
				manager := &Manager{
					client: c, reader: c, recorder: recorder, adminOpsMutator: testAdminOpsMutator(c),
					clientFactory: func(portopenbao.ClientConfig) (portopenbao.ClusterActions, error) {
						return &openbaoapi.MockClusterActions{IsHealthyFunc: func(context.Context) (bool, error) { return true, nil }}, nil
					},
				}
				metrics := upgrade.NewMetrics(cluster.Namespace, cluster.Name)
				var err error
				if completing {
					_, err = manager.finalizeConvergedUpgrade(t.Context(), logr.Discard(), cluster, metrics, "rolling")
				} else {
					err = manager.startUpgradeExecutionIfNeeded(t.Context(), logr.Discard(), cluster, metrics, "rolling")
				}
				if failure != "none" {
					require.ErrorIs(t, err, writeErr)
					assertNotReported()
				} else {
					require.NoError(t, err)
					require.Equal(t, before+1, lifecycleCounter(t, metricName, cluster.Namespace))
					reason := upgrade.ReasonUpgradeStarted
					if completing {
						reason = upgrade.ReasonUpgradeComplete
					}
					expectEventContains(t, recorder, reason, "2.5.5", "2.6.2")
					require.Empty(t, recorder.Events)
				}
				wantWrites := []string{partitionWrite}
				if completing {
					wantWrites = nil
				}
				if failure != partitionWrite {
					wantWrites = append(wantWrites, "apply")
					if failure != "apply" {
						wantWrites = append(wantWrites, "read-back")
					}
				}
				require.Equal(t, wantWrites, writes)
				stored := &openbaov1alpha1.OpenBaoCluster{}
				require.NoError(t, base.Get(t.Context(), client.ObjectKeyFromObject(cluster), stored))
				if applied == completing {
					require.Nil(t, stored.Status.Upgrade)
				} else {
					require.NotNil(t, stored.Status.Upgrade)
					require.Equal(t, "2.5.5", stored.Status.Upgrade.FromVersion)
					require.Equal(t, "2.6.2", stored.Status.Upgrade.TargetVersion)
				}
				require.Equal(t, "2.5.5", stored.Status.CurrentVersion, "the status controller owns the observed version")
			})
		}
	}
}

func rollingLifecycleObjects(completing bool) (*openbaov1alpha1.OpenBaoCluster, []client.Object) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "lifecycle-cluster"},
		Spec:       openbaov1alpha1.OpenBaoClusterSpec{Version: "2.6.2", Replicas: 1},
		Status:     openbaov1alpha1.OpenBaoClusterStatus{CurrentVersion: "2.5.5"},
	}
	if completing {
		startedAt := metav1.NewTime(time.Now().Add(-time.Minute))
		cluster.Status.Upgrade = &openbaov1alpha1.UpgradeProgress{
			FromVersion: "2.5.5", TargetVersion: "2.6.2", StartedAt: &startedAt,
		}
	}
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.Name},
		Status:     appsv1.StatefulSetStatus{ReadyReplicas: 1, UpdatedReplicas: 1, CurrentRevision: "updated", UpdateRevision: "updated"},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.Name + "-0", Labels: map[string]string{
			constants.LabelAppInstance: cluster.Name, constants.LabelAppName: constants.LabelValueAppNameOpenBao,
			constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator, appsv1.StatefulSetRevisionLabel: "updated",
		}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	}
	ca := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cluster.Name + constants.SuffixTLSCA}, Data: map[string][]byte{"ca.crt": []byte("test-ca")}}
	return cluster, []client.Object{sts, pod, ca}
}

func lifecycleCounter(t *testing.T, metricName, namespace string) float64 {
	t.Helper()
	families, err := controllermetrics.Registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != metricName {
			continue
		}
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "namespace" && label.GetValue() == namespace {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}
