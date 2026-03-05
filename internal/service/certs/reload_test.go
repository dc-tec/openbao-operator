package certs

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

const testCertHash = "test-hash-123"

func newTestClientset(pods ...*corev1.Pod) kubernetes.Interface {
	objects := []runtime.Object{}
	for _, pod := range pods {
		objects = append(objects, pod)
	}
	return fake.NewClientset(objects...)
}

//nolint:unparam // Keeping parameters makes tests easier to expand later.
func newTestCluster(name, namespace string) *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
		},
	}
}

func TestNewKubernetesReloadSignaler(t *testing.T) {
	clientset := newTestClientset()
	signaler := NewKubernetesReloadSignaler(clientset)

	if signaler == nil {
		t.Fatal("NewKubernetesReloadSignaler() returned nil")
	}

	if signaler.clientset != clientset {
		t.Error("NewKubernetesReloadSignaler() clientset not set correctly")
	}
}

func TestSignalReload_NoPods(t *testing.T) {
	clientset := newTestClientset()
	signaler := NewKubernetesReloadSignaler(clientset)
	logger := logr.Discard()

	cluster := newTestCluster("test-cluster", "default")
	ctx := context.Background()

	err := signaler.SignalReload(ctx, logger, cluster, "test-hash")
	if err != nil {
		t.Fatalf("SignalReload() with no pods should not error, got: %v", err)
	}
}

func TestSignalReload_AnnotatesReadyPods(t *testing.T) {
	readyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-0",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelAppInstance:  "test-cluster",
				constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	clientset := newTestClientset(readyPod)
	signaler := NewKubernetesReloadSignaler(clientset)
	logger := logr.Discard()

	cluster := newTestCluster("test-cluster", "default")
	ctx := context.Background()
	certHash := testCertHash

	err := signaler.SignalReload(ctx, logger, cluster, certHash)
	if err != nil {
		t.Fatalf("SignalReload() error = %v", err)
	}

	// Verify pod was annotated
	updatedPod, err := clientset.CoreV1().Pods("default").Get(ctx, "test-cluster-0", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get pod: %v", err)
	}

	if updatedPod.Annotations[tlsCertHashAnnotation] != certHash {
		t.Errorf("Pod annotation = %v, want %v", updatedPod.Annotations[tlsCertHashAnnotation], certHash)
	}
}

func TestSignalReload_SkipsNonReadyPods(t *testing.T) {
	notReadyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-0",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelAppInstance:  "test-cluster",
				constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	clientset := newTestClientset(notReadyPod)
	signaler := NewKubernetesReloadSignaler(clientset)
	logger := logr.Discard()

	cluster := newTestCluster("test-cluster", "default")
	ctx := context.Background()
	certHash := testCertHash

	err := signaler.SignalReload(ctx, logger, cluster, certHash)
	if err != nil {
		t.Fatalf("SignalReload() error = %v", err)
	}

	// Verify pod was NOT annotated
	updatedPod, err := clientset.CoreV1().Pods("default").Get(ctx, "test-cluster-0", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get pod: %v", err)
	}

	if updatedPod.Annotations != nil && updatedPod.Annotations[tlsCertHashAnnotation] == certHash {
		t.Error("Non-ready pod should not be annotated")
	}
}

func TestSignalReload_SkipsPodsWithCurrentHash(t *testing.T) {
	podWithHash := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-0",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelAppInstance:  "test-cluster",
				constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
			Annotations: map[string]string{
				tlsCertHashAnnotation: testCertHash,
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	clientset := newTestClientset(podWithHash)
	signaler := NewKubernetesReloadSignaler(clientset)
	logger := logr.Discard()

	cluster := newTestCluster("test-cluster", "default")
	ctx := context.Background()
	certHash := testCertHash // Same hash

	err := signaler.SignalReload(ctx, logger, cluster, certHash)
	if err != nil {
		t.Fatalf("SignalReload() error = %v", err)
	}

	// Pod should still have the same annotation (no update needed)
	updatedPod, err := clientset.CoreV1().Pods("default").Get(ctx, "test-cluster-0", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get pod: %v", err)
	}

	if updatedPod.Annotations[tlsCertHashAnnotation] != certHash {
		t.Errorf("Pod annotation should remain unchanged, got %v, want %v", updatedPod.Annotations[tlsCertHashAnnotation], certHash)
	}
}

func TestSignalReload_UpdatesMultiplePods(t *testing.T) {
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-0",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelAppInstance:  "test-cluster",
				constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-1",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelAppInstance:  "test-cluster",
				constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	clientset := newTestClientset(pod1, pod2)
	signaler := NewKubernetesReloadSignaler(clientset)
	logger := logr.Discard()

	cluster := newTestCluster("test-cluster", "default")
	ctx := context.Background()
	certHash := testCertHash

	err := signaler.SignalReload(ctx, logger, cluster, certHash)
	if err != nil {
		t.Fatalf("SignalReload() error = %v", err)
	}

	// Verify both pods were annotated
	for _, podName := range []string{"test-cluster-0", "test-cluster-1"} {
		updatedPod, err := clientset.CoreV1().Pods("default").Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("failed to get pod %s: %v", podName, err)
		}

		if updatedPod.Annotations[tlsCertHashAnnotation] != certHash {
			t.Errorf("Pod %s annotation = %v, want %v", podName, updatedPod.Annotations[tlsCertHashAnnotation], certHash)
		}
	}
}

func TestIsPodReady(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "pod with Ready condition true",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			want: true,
		},
		{
			name: "pod with Ready condition false",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "pod with Ready condition unknown",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionUnknown,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "pod without Ready condition",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "pod with no conditions",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPodReady(tt.pod)
			if got != tt.want {
				t.Errorf("isPodReady() = %v, want %v", got, tt.want)
			}
		})
	}
}
