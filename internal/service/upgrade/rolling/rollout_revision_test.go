package rolling

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestTargetPodAlreadyRolledOutRequiresObservedTemplate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		observed    int64
		podImage    string
		terminating bool
		want        bool
	}{
		{name: "unobserved template revision", observed: 6, podImage: "openbao:old"},
		{name: "matching revision with wrong image", observed: 7, podImage: "openbao:old"},
		{name: "terminating target", observed: 7, podImage: "openbao:new", terminating: true},
		{name: "observed matching target", observed: 7, podImage: "openbao:new", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, appsv1.AddToScheme(scheme))
			require.NoError(t, corev1.AddToScheme(scheme))
			cluster := &openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "ns"}}
			sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: cluster.Name, Namespace: cluster.Namespace, Generation: 7},
				Spec:   appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: constants.ContainerBao, Image: "openbao:new"}}}}},
				Status: appsv1.StatefulSetStatus{ObservedGeneration: tc.observed, UpdateRevision: "same-revision"}}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "example-2", Namespace: cluster.Namespace,
				Labels: map[string]string{appsv1.StatefulSetRevisionLabel: "same-revision"}},
				Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: constants.ContainerBao, Image: tc.podImage}}},
				Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
			if tc.terminating {
				now := metav1.Now()
				pod.DeletionTimestamp = &now
				pod.Finalizers = []string{"test.example/hold"}
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts, pod).Build()
			m := &Manager{client: c, reader: c}
			done, err := m.targetPodAlreadyRolledOut(t.Context(), logr.Discard(), cluster, rolloutTargetPod{Name: pod.Name})
			require.NoError(t, err)
			require.Equal(t, tc.want, done)
			require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(pod), &corev1.Pod{}), "observation must not delete a Pod")
		})
	}
}
