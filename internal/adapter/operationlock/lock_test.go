package operationlock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func newTestCluster() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
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
}

func newTestClient(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
}

func getCluster(t *testing.T, ctx context.Context, c client.Client) *openbaov1alpha1.OpenBaoCluster {
	t.Helper()

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: "c1", Namespace: "ns1"}, cluster))
	return cluster
}

func TestAcquireRelease(t *testing.T) {
	ctx := context.Background()
	cluster := newTestCluster()
	c := newTestClient(t, cluster)

	err := Acquire(ctx, c, cluster, AcquireOptions{
		Holder:    "controller/upgrade",
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Message:   "starting",
	})
	require.NoError(t, err)
	require.NotNil(t, cluster.Status.OperationLock)
	require.Equal(t, openbaov1alpha1.ClusterOperationUpgrade, cluster.Status.OperationLock.Operation)
	require.Equal(t, "controller/upgrade", cluster.Status.OperationLock.Holder)
	require.Equal(t, "starting", cluster.Status.OperationLock.Message)
	require.NotNil(t, cluster.Status.OperationLock.AcquiredAt)
	require.NotNil(t, cluster.Status.OperationLock.RenewedAt)

	updated := getCluster(t, ctx, c)
	require.NotNil(t, updated.Status.OperationLock)
	require.Equal(t, openbaov1alpha1.ClusterOperationUpgrade, updated.Status.OperationLock.Operation)
	require.Equal(t, "starting", updated.Status.OperationLock.Message)

	err = Acquire(ctx, c, cluster, AcquireOptions{
		Holder:    "controller/upgrade",
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Message:   "renew",
	})
	require.NoError(t, err)
	require.Equal(t, "renew", cluster.Status.OperationLock.Message)
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

	updated = getCluster(t, ctx, c)
	require.Nil(t, updated.Status.OperationLock)
}

func TestAcquireForce(t *testing.T) {
	ctx := context.Background()
	cluster := newTestCluster()
	cluster.Status.OperationLock = &openbaov1alpha1.OperationLockStatus{
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Holder:    "controller/upgrade",
		Message:   "in progress",
	}
	c := newTestClient(t, cluster)

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
	require.Equal(t, "override", cluster.Status.OperationLock.Message)
	require.NotNil(t, cluster.Status.OperationLock.AcquiredAt)
	require.NotNil(t, cluster.Status.OperationLock.RenewedAt)
}

func TestAcquireRejectsInvalidInputs(t *testing.T) {
	ctx := context.Background()
	cluster := newTestCluster()
	c := newTestClient(t, cluster)

	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		opts    AcquireOptions
		wantErr string
	}{
		{
			name:    "nil cluster",
			cluster: nil,
			opts: AcquireOptions{
				Holder:    "controller/upgrade",
				Operation: openbaov1alpha1.ClusterOperationUpgrade,
			},
			wantErr: "cluster is required",
		},
		{
			name:    "missing holder",
			cluster: cluster.DeepCopy(),
			opts: AcquireOptions{
				Operation: openbaov1alpha1.ClusterOperationUpgrade,
			},
			wantErr: "holder is required",
		},
		{
			name:    "missing operation",
			cluster: cluster.DeepCopy(),
			opts: AcquireOptions{
				Holder: "controller/upgrade",
			},
			wantErr: "operation is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Acquire(ctx, c, tt.cluster, tt.opts)
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestReleaseRejectsInvalidInputs(t *testing.T) {
	ctx := context.Background()
	cluster := newTestCluster()
	c := newTestClient(t, cluster)

	tests := []struct {
		name      string
		cluster   *openbaov1alpha1.OpenBaoCluster
		holder    string
		operation openbaov1alpha1.ClusterOperation
		wantErr   string
	}{
		{
			name:      "nil cluster",
			cluster:   nil,
			holder:    "controller/upgrade",
			operation: openbaov1alpha1.ClusterOperationUpgrade,
			wantErr:   "cluster is required",
		},
		{
			name:      "missing holder",
			cluster:   cluster.DeepCopy(),
			operation: openbaov1alpha1.ClusterOperationUpgrade,
			wantErr:   "holder is required",
		},
		{
			name:    "missing operation",
			cluster: cluster.DeepCopy(),
			holder:  "controller/upgrade",
			wantErr: "operation is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Release(ctx, c, tt.cluster, tt.holder, tt.operation)
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestAcquireOnlyRenewsExactMatch(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name             string
		currentOperation openbaov1alpha1.ClusterOperation
		requestOperation openbaov1alpha1.ClusterOperation
		currentHolder    string
		requestHolder    string
	}{
		{
			name:             "same holder different operation current sorts after request",
			currentOperation: openbaov1alpha1.ClusterOperationUpgrade,
			requestOperation: openbaov1alpha1.ClusterOperationBackup,
			currentHolder:    "controller/shared",
			requestHolder:    "controller/shared",
		},
		{
			name:             "same holder different operation current sorts before request",
			currentOperation: openbaov1alpha1.ClusterOperationBackup,
			requestOperation: openbaov1alpha1.ClusterOperationUpgrade,
			currentHolder:    "controller/shared",
			requestHolder:    "controller/shared",
		},
		{
			name:             "same operation different holder current sorts after request",
			currentOperation: openbaov1alpha1.ClusterOperationUpgrade,
			requestOperation: openbaov1alpha1.ClusterOperationUpgrade,
			currentHolder:    "holder-z",
			requestHolder:    "holder-a",
		},
		{
			name:             "same operation different holder current sorts before request",
			currentOperation: openbaov1alpha1.ClusterOperationUpgrade,
			requestOperation: openbaov1alpha1.ClusterOperationUpgrade,
			currentHolder:    "holder-a",
			requestHolder:    "holder-z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newTestCluster()
			cluster.Status.OperationLock = &openbaov1alpha1.OperationLockStatus{
				Operation: tt.currentOperation,
				Holder:    tt.currentHolder,
				Message:   "already held",
			}
			c := newTestClient(t, cluster)

			err := Acquire(ctx, c, cluster, AcquireOptions{
				Holder:    tt.requestHolder,
				Operation: tt.requestOperation,
				Message:   "should not renew",
			})
			require.Error(t, err)
			require.ErrorIs(t, err, ErrLockHeld)

			var heldErr *HeldError
			require.ErrorAs(t, err, &heldErr)
			require.Equal(t, tt.currentOperation, heldErr.Operation)
			require.Equal(t, tt.currentHolder, heldErr.Holder)
			require.Equal(t, "already held", heldErr.Message)

			updated := getCluster(t, ctx, c)
			require.Equal(t, tt.currentOperation, updated.Status.OperationLock.Operation)
			require.Equal(t, tt.currentHolder, updated.Status.OperationLock.Holder)
			require.Equal(t, "already held", updated.Status.OperationLock.Message)
		})
	}
}

func TestAcquireRenewsAndBackfillsAcquiredAt(t *testing.T) {
	ctx := context.Background()
	cluster := newTestCluster()
	previousRenewedAt := metav1.NewTime(time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC))
	cluster.Status.OperationLock = &openbaov1alpha1.OperationLockStatus{
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Holder:    "controller/upgrade",
		Message:   "stale",
		RenewedAt: &previousRenewedAt,
	}
	c := newTestClient(t, cluster)

	err := Acquire(ctx, c, cluster, AcquireOptions{
		Holder:    "controller/upgrade",
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Message:   "renewed",
	})
	require.NoError(t, err)
	require.NotNil(t, cluster.Status.OperationLock.AcquiredAt)
	require.NotNil(t, cluster.Status.OperationLock.RenewedAt)
	require.Equal(t, "renewed", cluster.Status.OperationLock.Message)
	require.True(t, cluster.Status.OperationLock.RenewedAt.After(previousRenewedAt.Time))

	updated := getCluster(t, ctx, c)
	require.NotNil(t, updated.Status.OperationLock.AcquiredAt)
	require.Equal(t, "renewed", updated.Status.OperationLock.Message)
	require.True(t, updated.Status.OperationLock.RenewedAt.After(previousRenewedAt.Time))
}

func TestAcquireReturnsPatchStatusError(t *testing.T) {
	ctx := context.Background()
	cluster := newTestCluster()

	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				if subResourceName == "status" {
					return errors.New("patch failed")
				}
				return c.SubResource(subResourceName).Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	err := Acquire(ctx, c, cluster, AcquireOptions{
		Holder:    "controller/upgrade",
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Message:   "starting",
	})
	require.EqualError(t, err, "failed to patch operation lock status: patch failed")
	require.Nil(t, cluster.Status.OperationLock)
}

func TestReleaseOnlySucceedsForExactMatch(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name             string
		currentOperation openbaov1alpha1.ClusterOperation
		requestOperation openbaov1alpha1.ClusterOperation
		currentHolder    string
		requestHolder    string
	}{
		{
			name:             "same holder different operation current sorts after request",
			currentOperation: openbaov1alpha1.ClusterOperationUpgrade,
			requestOperation: openbaov1alpha1.ClusterOperationBackup,
			currentHolder:    "controller/shared",
			requestHolder:    "controller/shared",
		},
		{
			name:             "same holder different operation current sorts before request",
			currentOperation: openbaov1alpha1.ClusterOperationBackup,
			requestOperation: openbaov1alpha1.ClusterOperationUpgrade,
			currentHolder:    "controller/shared",
			requestHolder:    "controller/shared",
		},
		{
			name:             "same operation different holder current sorts after request",
			currentOperation: openbaov1alpha1.ClusterOperationUpgrade,
			requestOperation: openbaov1alpha1.ClusterOperationUpgrade,
			currentHolder:    "holder-z",
			requestHolder:    "holder-a",
		},
		{
			name:             "same operation different holder current sorts before request",
			currentOperation: openbaov1alpha1.ClusterOperationUpgrade,
			requestOperation: openbaov1alpha1.ClusterOperationUpgrade,
			currentHolder:    "holder-a",
			requestHolder:    "holder-z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newTestCluster()
			cluster.Status.OperationLock = &openbaov1alpha1.OperationLockStatus{
				Operation: tt.currentOperation,
				Holder:    tt.currentHolder,
				Message:   "already held",
			}
			c := newTestClient(t, cluster)

			err := Release(ctx, c, cluster, tt.requestHolder, tt.requestOperation)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrLockHeld)

			var heldErr *HeldError
			require.ErrorAs(t, err, &heldErr)
			require.Equal(t, tt.currentOperation, heldErr.Operation)
			require.Equal(t, tt.currentHolder, heldErr.Holder)
			require.Equal(t, "already held", heldErr.Message)

			updated := getCluster(t, ctx, c)
			require.NotNil(t, updated.Status.OperationLock)
			require.Equal(t, tt.currentOperation, updated.Status.OperationLock.Operation)
			require.Equal(t, tt.currentHolder, updated.Status.OperationLock.Holder)
		})
	}
}

func TestReleaseWithoutLockIsNoOp(t *testing.T) {
	ctx := context.Background()
	cluster := newTestCluster()
	c := newTestClient(t, cluster)

	err := Release(ctx, c, cluster, "controller/upgrade", openbaov1alpha1.ClusterOperationUpgrade)
	require.NoError(t, err)
	require.Nil(t, cluster.Status.OperationLock)

	updated := getCluster(t, ctx, c)
	require.Nil(t, updated.Status.OperationLock)
}

func TestHeldErrorImplementsWrappedError(t *testing.T) {
	err := &HeldError{
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Holder:    "controller/upgrade",
		Message:   "still running",
	}

	require.Equal(
		t,
		"operation lock is held by another operation: operation=\"Upgrade\" holder=\"controller/upgrade\" message=\"still running\"",
		err.Error(),
	)
	require.True(t, errors.Is(err, ErrLockHeld))
}
