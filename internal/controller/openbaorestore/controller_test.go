package openbaorestore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

func TestOpenBaoRestoreReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	tests := []struct {
		name          string
		restore       *openbaov1alpha1.OpenBaoRestore
		expectedPhase openbaov1alpha1.RestorePhase
	}{
		{
			name: "New restore starts validation",
			restore: &openbaov1alpha1.OpenBaoRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-restore",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoRestoreSpec{
					Cluster: "test-cluster",
				},
				Status: openbaov1alpha1.OpenBaoRestoreStatus{
					Phase: openbaov1alpha1.RestorePhasePending,
				},
			},
			expectedPhase: openbaov1alpha1.RestorePhaseValidating,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).WithObjects(tt.restore).Build()

			r := &OpenBaoRestoreReconciler{
				Client: c,
				Scheme: scheme,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.restore.Name,
					Namespace: tt.restore.Namespace,
				},
			}

			result, err := r.Reconcile(context.Background(), req)
			assert.NoError(t, err)
			assert.True(t, result.Requeue || result.RequeueAfter > 0)

			updated := &openbaov1alpha1.OpenBaoRestore{}
			err = c.Get(context.Background(), req.NamespacedName, updated)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedPhase, updated.Status.Phase)
		})
	}
}

func TestOpenBaoRestoreReconciler_Reconcile_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &OpenBaoRestoreReconciler{
		Client: c,
		Scheme: scheme,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "missing",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}
