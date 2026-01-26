package restore

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/security"
)

func TestRestoreHandleValidating_HardenedRequiresEgressRules(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			Namespace:       "default",
			ResourceVersion: "1", // Set initial ResourceVersion for fake client SSA compatibility
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileHardened,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized: true,
		},
	}

	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "restore",
			Namespace:       "default",
			ResourceVersion: "1", // Set initial ResourceVersion for fake client SSA compatibility
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test",
			Source: openbaov1alpha1.RestoreSource{
				Key: "snap",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "https://example.com",
					Bucket:   "b",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoRestoreStatus{
			Phase: openbaov1alpha1.RestorePhaseValidating,
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, restore).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).
		WithReturnManagedFields().
		Build()

	mgr := NewManager(k8sClient, scheme, nil, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	_, err := mgr.handleValidating(context.Background(), logr.Discard(), restore)
	require.NoError(t, err)

	updated := &openbaov1alpha1.OpenBaoRestore{}
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Name: "restore", Namespace: "default"}, updated))
	require.Equal(t, openbaov1alpha1.RestorePhaseFailed, updated.Status.Phase)
	require.Contains(t, updated.Status.Message, "spec.network.egressRules")
}
