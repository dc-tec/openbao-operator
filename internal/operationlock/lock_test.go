package operationlock

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

func TestAcquireRelease(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	ctx := context.Background()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "c1",
			Namespace: "ns1",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.0",
			Image:    "openbao/openbao:2.4.0",
			Replicas: 3,
			TLS:      openbaov1alpha1.TLSConfig{Mode: openbaov1alpha1.TLSModeOperatorManaged},
			Storage:  openbaov1alpha1.StorageConfig{Size: "10Gi"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		Build()

	err := Acquire(ctx, c, cluster, AcquireOptions{
		Holder:    "controller/upgrade",
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Message:   "starting",
	})
	require.NoError(t, err)
	require.NotNil(t, cluster.Status.OperationLock)
	require.Equal(t, openbaov1alpha1.ClusterOperationUpgrade, cluster.Status.OperationLock.Operation)
	require.Equal(t, "controller/upgrade", cluster.Status.OperationLock.Holder)

	updated := &openbaov1alpha1.OpenBaoCluster{}
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: "c1", Namespace: "ns1"}, updated))
	require.NotNil(t, updated.Status.OperationLock)
	require.Equal(t, openbaov1alpha1.ClusterOperationUpgrade, updated.Status.OperationLock.Operation)

	err = Acquire(ctx, c, cluster, AcquireOptions{
		Holder:    "controller/upgrade",
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Message:   "renew",
	})
	require.NoError(t, err)
	require.NotNil(t, cluster.Status.OperationLock.RenewedAt)

	err = Acquire(ctx, c, cluster, AcquireOptions{
		Holder:    "controller/backup",
		Operation: openbaov1alpha1.ClusterOperationBackup,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrLockHeld))

	err = Release(ctx, c, cluster, "controller/backup", openbaov1alpha1.ClusterOperationBackup)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrLockHeld))

	err = Release(ctx, c, cluster, "controller/upgrade", openbaov1alpha1.ClusterOperationUpgrade)
	require.NoError(t, err)
	require.Nil(t, cluster.Status.OperationLock)

	updated = &openbaov1alpha1.OpenBaoCluster{}
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: "c1", Namespace: "ns1"}, updated))
	require.Nil(t, updated.Status.OperationLock)
}

func TestAcquireForce(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	ctx := context.Background()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "c1",
			Namespace: "ns1",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.0",
			Image:    "openbao/openbao:2.4.0",
			Replicas: 3,
			TLS:      openbaov1alpha1.TLSConfig{Mode: openbaov1alpha1.TLSModeOperatorManaged},
			Storage:  openbaov1alpha1.StorageConfig{Size: "10Gi"},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			OperationLock: &openbaov1alpha1.OperationLockStatus{
				Operation: openbaov1alpha1.ClusterOperationUpgrade,
				Holder:    "controller/upgrade",
				Message:   "in progress",
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		Build()

	err := Acquire(ctx, c, cluster, AcquireOptions{
		Holder:    "restore/req-1",
		Operation: openbaov1alpha1.ClusterOperationRestore,
		Message:   "override",
		Force:     true,
	})
	require.NoError(t, err)
	require.NotNil(t, cluster.Status.OperationLock)
	require.Equal(t, openbaov1alpha1.ClusterOperationRestore, cluster.Status.OperationLock.Operation)
	require.Equal(t, "restore/req-1", cluster.Status.OperationLock.Holder)
}
