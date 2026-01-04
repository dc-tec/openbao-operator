package infra

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestInferActiveRevisionFromPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	readyPod := func(name, ns, rev string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels: map[string]string{
					constants.LabelAppName:         constants.LabelValueAppNameOpenBao,
					constants.LabelAppInstance:     "test",
					constants.LabelAppManagedBy:    constants.LabelValueAppManagedByOpenBaoOperator,
					constants.LabelOpenBaoCluster:  "test",
					constants.LabelOpenBaoRevision: rev,
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		}
	}

	notReadyPod := func(name, ns, rev string) *corev1.Pod {
		p := readyPod(name, ns, rev)
		p.Status.Conditions[0].Status = corev1.ConditionFalse
		return p
	}

	tests := []struct {
		name  string
		pods  []runtime.Object
		want  string
		wantE bool
	}{
		{
			name: "returns empty when no revision-labeled pods exist",
			pods: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-rev",
						Namespace: "default",
						Labels: map[string]string{
							constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
							constants.LabelAppInstance:    "test",
							constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
							constants.LabelOpenBaoCluster: "test",
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
			},
			want: "",
		},
		{
			name: "picks revision with most ready pods",
			pods: []runtime.Object{
				readyPod("blue-0", "default", "blue"),
				readyPod("blue-1", "default", "blue"),
				notReadyPod("green-0", "default", "green"),
			},
			want: "blue",
		},
		{
			name: "ties broken by total pods then lexicographically",
			pods: []runtime.Object{
				readyPod("a-0", "default", "a"),
				notReadyPod("a-1", "default", "a"),
				readyPod("b-0", "default", "b"),
				notReadyPod("b-1", "default", "b"),
			},
			want: "a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			}
			objects := append([]runtime.Object{cluster}, tt.pods...)
			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()

			got, err := InferActiveRevisionFromPods(context.Background(), c, cluster)
			if tt.wantE && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantE && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
