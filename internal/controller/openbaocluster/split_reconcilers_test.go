package openbaocluster

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestOpenBaoClusterAdminOpsReconcilerReconcile_UsesAPIReaderWhenAvailable(t *testing.T) {
	t.Parallel()

	scheme := newOpenBaoClusterTestScheme(t)
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "adminops-api-reader",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Paused: true,
		},
	}

	cachedClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		Build()
	apiReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster.DeepCopy()).
		Build()

	reconciler := &openBaoClusterAdminOpsReconciler{
		parent: &OpenBaoClusterReconciler{
			Client: cachedClient,
			ControllerRuntime: ControllerRuntime{
				APIReader: apiReader,
			},
		},
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}
