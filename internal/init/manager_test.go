package init

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestReconcileSelfInitUsesPodReadiness(t *testing.T) {
	tests := []struct {
		name            string
		podReady        bool
		wantInitialized bool
		wantSelfInit    bool
	}{
		{
			name:            "pod not ready does not mark initialized",
			podReady:        false,
			wantInitialized: false,
			wantSelfInit:    false,
		},
		{
			name:            "pod ready marks initialized",
			podReady:        true,
			wantInitialized: true,
			wantSelfInit:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
					},
				},
			}

			readyStatus := corev1.ConditionFalse
			if tt.podReady {
				readyStatus = corev1.ConditionTrue
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-0",
					Namespace: "default",
					Labels: map[string]string{
						constants.LabelAppInstance:  "cluster",
						constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
						constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: constants.ContainerBao,
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{
									StartedAt: metav1.Now(),
								},
							},
							Started: ptrTo(true),
						},
					},
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: readyStatus,
						},
					},
				},
			}

			clientset := kubernetesfake.NewSimpleClientset(pod)
			manager := &Manager{
				config:    &rest.Config{},
				clientset: clientset,
			}

			if _, err := manager.Reconcile(context.Background(), logr.Discard(), cluster); err != nil {
				t.Fatalf("Reconcile() error = %v, want no error", err)
			}

			if cluster.Status.Initialized != tt.wantInitialized {
				t.Fatalf("Status.Initialized = %t, want %t", cluster.Status.Initialized, tt.wantInitialized)
			}

			if cluster.Status.SelfInitialized != tt.wantSelfInit {
				t.Fatalf("Status.SelfInitialized = %t, want %t", cluster.Status.SelfInitialized, tt.wantSelfInit)
			}
		})
	}
}

func ptrTo(v bool) *bool {
	return &v
}

func TestReconcileIgnoresServiceLabelsWhenSelfInitDisabled(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "default",
		},
	}

	started := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-0",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelAppInstance:  "cluster",
				constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
				"openbao-initialized":       "true",
				"openbao-sealed":            "false",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: constants.ContainerBao,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: metav1.Now(),
						},
					},
					Started: &started,
				},
			},
		},
	}

	clientset := kubernetesfake.NewSimpleClientset(pod)
	manager := &Manager{
		config:    &rest.Config{},
		clientset: clientset,
	}

	if _, err := manager.Reconcile(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v, want no error", err)
	}

	if cluster.Status.Initialized {
		t.Fatalf("Status.Initialized = %t, want false because operator must still capture root token", cluster.Status.Initialized)
	}

	if cluster.Status.SelfInitialized {
		t.Fatalf("Status.SelfInitialized = %t, want false when self-init disabled", cluster.Status.SelfInitialized)
	}
}

func TestStoreRootTokenCreatesOrUpdatesSecret(t *testing.T) {
	tests := []struct {
		name           string
		existingSecret *corev1.Secret
		rootToken      string
		wantCreated    bool
		wantUpdated    bool
	}{
		{
			name:        "creates new Secret when none exists",
			rootToken:   "s.roottoken",
			wantCreated: true,
		},
		{
			name: "updates existing Secret with new token",
			existingSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-root-token",
					Namespace: "default",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					rootTokenSecretKey: []byte("old-token"),
				},
			},
			rootToken:   "s.newtoken",
			wantUpdated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := kubernetesfake.NewSimpleClientset()
			manager := &Manager{
				config:    &rest.Config{},
				clientset: clientset,
			}

			cluster := &openbaov1alpha1.OpenBaoCluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "openbao.org/v1alpha1",
					Kind:       "OpenBaoCluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster",
					Namespace: "default",
					UID:       types.UID("test-uid-12345"),
				},
			}

			if tt.existingSecret != nil {
				if _, err := clientset.CoreV1().Secrets(tt.existingSecret.Namespace).Create(context.Background(), tt.existingSecret, metav1.CreateOptions{}); err != nil {
					t.Fatalf("failed to seed existing Secret: %v", err)
				}
			}

			if err := manager.storeRootToken(context.Background(), logr.Discard(), cluster, tt.rootToken); err != nil {
				t.Fatalf("storeRootToken() error = %v", err)
			}

			secret, err := clientset.CoreV1().Secrets("default").Get(context.Background(), "cluster-root-token", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("expected root token Secret to exist: %v", err)
			}

			got := string(secret.Data[rootTokenSecretKey])
			if got != tt.rootToken {
				t.Fatalf("root token in Secret = %q, want %q", got, tt.rootToken)
			}

			// Verify OwnerReference is set
			if len(secret.OwnerReferences) == 0 {
				t.Fatalf("expected OwnerReference to be set on root token Secret")
			}
			ownerRef := secret.OwnerReferences[0]
			if ownerRef.UID != cluster.UID {
				t.Errorf("expected OwnerReference UID %s, got %s", cluster.UID, ownerRef.UID)
			}
			if ownerRef.Kind != "OpenBaoCluster" {
				t.Errorf("expected OwnerReference Kind 'OpenBaoCluster', got %s", ownerRef.Kind)
			}
		})
	}
}
