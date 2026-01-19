package infra

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestReconcileMaintenanceAnnotationsForPods(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(openbaov1alpha1.AddToScheme(scheme))

	clusterName := "test-cluster"
	namespace := "default"

	baseCluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Maintenance: &openbaov1alpha1.MaintenanceConfig{
				Enabled: true,
			},
		},
	}

	podLabels := map[string]string{
		constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:    clusterName,
		constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster: clusterName,
	}

	tests := []struct {
		name            string
		cluster         *openbaov1alpha1.OpenBaoCluster
		existingPods    []corev1.Pod
		revision        string
		wantAnnotations map[string]map[string]string // podName -> annotations
		wantErr         bool
	}{
		{
			name:    "nil cluster",
			cluster: nil,
			wantErr: false,
		},
		{
			name:    "maintenance enabled - adds annotation",
			cluster: baseCluster,
			existingPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-0",
						Namespace: namespace,
						Labels:    podLabels,
					},
				},
			},
			wantAnnotations: map[string]map[string]string{
				"pod-0": {constants.AnnotationMaintenance: "true"},
			},
		},
		{
			name: "maintenance disabled - removes annotation",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: namespace},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Maintenance: &openbaov1alpha1.MaintenanceConfig{Enabled: false},
				},
			},
			existingPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-0",
						Namespace: namespace,
						Labels:    podLabels,
						Annotations: map[string]string{
							constants.AnnotationMaintenance: "true",
							"other":                         "value",
						},
					},
				},
			},
			wantAnnotations: map[string]map[string]string{
				"pod-0": {"other": "value"},
			},
		},
		{
			name:    "idempotent - no changes if already set",
			cluster: baseCluster,
			existingPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-0",
						Namespace: namespace,
						Labels:    podLabels,
						Annotations: map[string]string{
							constants.AnnotationMaintenance: "true",
						},
					},
				},
			},
			wantAnnotations: map[string]map[string]string{
				"pod-0": {constants.AnnotationMaintenance: "true"},
			},
		},
			{
				name:    "skips deleted pods",
				cluster: baseCluster,
				existingPods: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "pod-0",
							Namespace:         namespace,
							Labels:            podLabels,
							DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
							// controller-runtime fake client refuses objects with DeletionTimestamp but no finalizers.
							Finalizers: []string{"test.openbao.org/finalizer"},
						},
					},
				},
				wantAnnotations: map[string]map[string]string{
					"pod-0": nil, // Should not have annotation added
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var initObjs []client.Object
			for i := range tt.existingPods {
				initObjs = append(initObjs, &tt.existingPods[i])
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjs...).Build()
			mgr := &Manager{client: c}

			err := mgr.reconcileMaintenanceAnnotationsForPods(context.Background(), logr.Discard(), tt.cluster, tt.revision)
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcileMaintenanceAnnotationsForPods() error = %v, wantErr %v", err, tt.wantErr)
			}

			for podName, wantAnn := range tt.wantAnnotations {
				var pod corev1.Pod
				err := c.Get(context.Background(), types.NamespacedName{Name: podName, Namespace: namespace}, &pod)
				if err != nil {
					t.Fatalf("failed to get pod %s: %v", podName, err)
				}

				if wantAnn == nil {
					if val, ok := pod.Annotations[constants.AnnotationMaintenance]; ok {
						t.Errorf("pod %s has unexpected annotation %s=%s", podName, constants.AnnotationMaintenance, val)
					}
				} else {
					for k, v := range wantAnn {
						if got := pod.Annotations[k]; got != v {
							t.Errorf("pod %s annotation[%s] = %q, want %q", podName, k, got, v)
						}
					}
					// Verify specific maintenance annotation absence if not wanted in map
					if _, ok := wantAnn[constants.AnnotationMaintenance]; !ok {
						if _, has := pod.Annotations[constants.AnnotationMaintenance]; has {
							t.Errorf("pod %s has unexpected annotation %s", podName, constants.AnnotationMaintenance)
						}
					}
				}
			}
		})
	}
}
