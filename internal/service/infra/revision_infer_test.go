package infra

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
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
		name      string
		setup     func(*openbaov1alpha1.OpenBaoCluster)
		pods      []runtime.Object
		want      string
		wantImage string
		wantE     bool
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
			want:      "",
			wantImage: "",
		},
		{
			name: "picks revision with most ready pods",
			pods: []runtime.Object{
				readyPod("blue-0", "default", "blue"),
				readyPod("blue-1", "default", "blue"),
				notReadyPod("green-0", "default", "green"),
			},
			want:      "blue",
			wantImage: "", // default pod helper doesn't set image, good enough for empty check or we update helper
		},
		{
			name: "ties broken by total pods then lexicographically",
			pods: []runtime.Object{
				readyPod("a-0", "default", "a"),
				notReadyPod("a-1", "default", "a"),
				readyPod("b-0", "default", "b"),
				notReadyPod("b-1", "default", "b"),
			},
			want:      "a",
			wantImage: "",
		},
	}

	// Update helper to set image
	readyPodWithImage := func(name, ns, rev, image string) *corev1.Pod {
		p := readyPod(name, ns, rev)
		p.Spec.Containers = []corev1.Container{{Name: "openbao", Image: image}}
		return p
	}

	// Add a specific test case for image extraction
	tests = append(tests, struct {
		name      string
		setup     func(*openbaov1alpha1.OpenBaoCluster)
		pods      []runtime.Object
		want      string
		wantImage string
		wantE     bool
	}{
		name: "extracts image from selected revision",
		pods: []runtime.Object{
			readyPodWithImage("blue-0", "default", "blue", "openbao:1.0.0"),
		},
		want:      "blue",
		wantImage: "openbao:1.0.0",
		wantE:     false,
	})

	tests = append(tests, struct {
		name      string
		setup     func(*openbaov1alpha1.OpenBaoCluster)
		pods      []runtime.Object
		want      string
		wantImage string
		wantE     bool
	}{
		name: "prefers image matching currentVersion during upgrades",
		setup: func(cluster *openbaov1alpha1.OpenBaoCluster) {
			cluster.Spec.Version = "2.4.4"
			cluster.Spec.Image = "openbao/openbao:2.4.4"
			cluster.Status.CurrentVersion = "2.4.3"
		},
		pods: []runtime.Object{
			readyPodWithImage("pod-new-0", "default", "a", "openbao/openbao:2.4.4"),
			readyPodWithImage("pod-old-0", "default", "b", "openbao/openbao:2.4.3"),
		},
		want:      "b", // would be "a" by lexicographic tie-breaker without currentVersion hint
		wantImage: "openbao/openbao:2.4.3",
		wantE:     false,
	})

	tests = append(tests, struct {
		name      string
		setup     func(*openbaov1alpha1.OpenBaoCluster)
		pods      []runtime.Object
		want      string
		wantImage string
		wantE     bool
	}{
		name: "does not lock onto stale blueImage when currentVersion indicates otherwise",
		setup: func(cluster *openbaov1alpha1.OpenBaoCluster) {
			cluster.Spec.Version = "2.4.4"
			cluster.Spec.Image = "openbao/openbao:2.4.4"
			cluster.Status.CurrentVersion = "2.4.3"
			cluster.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
				BlueImage: "openbao/openbao:2.4.4", // stale/incorrect hint
			}
		},
		pods: []runtime.Object{
			readyPodWithImage("pod-new-0", "default", "a", "openbao/openbao:2.4.4"),
			readyPodWithImage("pod-old-0", "default", "b", "openbao/openbao:2.4.3"),
		},
		want:      "b",
		wantImage: "openbao/openbao:2.4.3",
		wantE:     false,
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			}
			if tt.setup != nil {
				tt.setup(cluster)
			}
			objects := append([]runtime.Object{cluster}, tt.pods...)
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objects...).
				WithReturnManagedFields().
				Build()

			got, gotImage, err := InferActiveRevisionFromPods(context.Background(), c, cluster)
			if tt.wantE && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantE && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected revision %q, got %q", tt.want, got)
			}
			if gotImage != tt.wantImage {
				t.Fatalf("expected image %q, got %q", tt.wantImage, gotImage)
			}
		})
	}
}
