package statusapply

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	persistMe               = "persist-me"
	concurrentTargetVersion = "2.5.0"
	concurrentFromVersion   = "2.4.4"
	concurrentRetryMarker   = "retry-1"
)

type stagedReader struct {
	first client.Reader
	then  client.Reader
	mu    sync.Mutex
	calls int
}

func newConcurrentApplyTestClient(base client.WithWatch, initialReadWriters int32) (client.WithWatch, *atomic.Int32) {
	var conflictCount atomic.Int32
	var initialReadCount atomic.Int32
	initialReadsComplete := make(chan struct{})

	guarded := interceptor.NewClient(base, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if err := c.Get(ctx, key, obj, opts...); err != nil {
				return err
			}

			readNumber := initialReadCount.Add(1)
			if readNumber == initialReadWriters {
				close(initialReadsComplete)
			}
			if readNumber <= initialReadWriters {
				<-initialReadsComplete
			}
			return nil
		},
		SubResourceApply: func(ctx context.Context, c client.Client, subResource string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
			err := c.SubResource(subResource).Apply(ctx, obj, opts...)
			if apierrors.IsConflict(err) {
				conflictCount.Add(1)
			}
			return err
		},
	})
	return guarded, &conflictCount
}

func (r *stagedReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls++
	if r.calls == 1 {
		return r.first.Get(ctx, key, obj, opts...)
	}
	return r.then.Get(ctx, key, obj, opts...)
}

func (r *stagedReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return r.then.List(ctx, list, opts...)
}

func TestMutateAndApplyOpenBaoClusterAdminOpsStatus_UsesLatestStateAndPreservesSiblingFields(t *testing.T) {
	t.Parallel()

	stored := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "adminops-mutate",
			Namespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Backup:          &openbaov1alpha1.BackupStatus{LastFailureReason: persistMe},
			UpgradeRequests: &openbaov1alpha1.UpgradeRequestStatus{LastHandledRetry: "before"},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(newOpenBaoClusterStatusTestScheme(t)).
		WithStatusSubresource(stored).
		WithObjects(stored.DeepCopy()).
		Build()

	// Stale caller state intentionally omits Backup.
	stale := stored.DeepCopy()
	stale.Status.Backup = nil

	key := types.NamespacedName{Name: stale.Name, Namespace: stale.Namespace}
	updated, err := MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader(context.Background(), nil, k8sClient, key, func(obj *openbaov1alpha1.OpenBaoCluster) error {
		obj.Status.BreakGlass = &openbaov1alpha1.BreakGlassStatus{Active: true, Nonce: "nonce-1"}
		return nil
	}, OpenBaoClusterAdminOpsStatusApplyOptions{})
	if err != nil {
		t.Fatalf("MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader() error = %v", err)
	}

	if updated.Status.Backup == nil || updated.Status.Backup.LastFailureReason != persistMe {
		t.Fatalf("updated.Status.Backup = %#v, want preserved backup state", updated.Status.Backup)
	}
	if updated.Status.BreakGlass == nil || !updated.Status.BreakGlass.Active || updated.Status.BreakGlass.Nonce != "nonce-1" {
		t.Fatalf("updated.Status.BreakGlass = %#v, want break-glass state set", updated.Status.BreakGlass)
	}

	reloaded := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(stored), reloaded); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !reflect.DeepEqual(reloaded.Status.Backup, updated.Status.Backup) {
		t.Fatalf("stored backup = %#v, want %#v", reloaded.Status.Backup, updated.Status.Backup)
	}
}

func TestMutateAndApplyOpenBaoClusterAdminOpsStatusWithReader_UsesProvidedReaderForFreshState(t *testing.T) {
	t.Parallel()

	scheme := newOpenBaoClusterStatusTestScheme(t)
	key := types.NamespacedName{Name: "adminops-mutate-reader", Namespace: "default"}

	live := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Backup: &openbaov1alpha1.BackupStatus{
				LastFailureReason: persistMe,
			},
		},
	}
	cached := live.DeepCopy()
	cached.Status.Backup = nil

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cached.DeepCopy()).
		Build()
	reader := &stagedReader{
		first: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
			WithObjects(live.DeepCopy()).
			Build(),
		then: k8sClient,
	}

	updated, err := MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader(context.Background(), reader, k8sClient, key, func(obj *openbaov1alpha1.OpenBaoCluster) error {
		obj.Status.BreakGlass = &openbaov1alpha1.BreakGlassStatus{
			Active: true,
			Nonce:  "nonce-1",
		}
		return nil
	}, OpenBaoClusterAdminOpsStatusApplyOptions{})
	if err != nil {
		t.Fatalf("MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader() error = %v", err)
	}

	if updated.Status.Backup == nil || updated.Status.Backup.LastFailureReason != persistMe {
		t.Fatalf("updated.Status.Backup = %#v, want preserved backup state", updated.Status.Backup)
	}
	if updated.Status.BreakGlass == nil || !updated.Status.BreakGlass.Active || updated.Status.BreakGlass.Nonce != "nonce-1" {
		t.Fatalf("updated.Status.BreakGlass = %#v, want break-glass state set", updated.Status.BreakGlass)
	}

	stored := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), key, stored); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.Status.Backup == nil || stored.Status.Backup.LastFailureReason != "persist-me" {
		t.Fatalf("stored.Status.Backup = %#v, want preserved backup state", stored.Status.Backup)
	}
}

func TestMutateAndApplyOpenBaoClusterAdminOpsStatus_PropagatesMutatorError(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "adminops-mutate-error",
			Namespace: "default",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(newOpenBaoClusterStatusTestScheme(t)).
		WithStatusSubresource(cluster).
		WithObjects(cluster.DeepCopy()).
		Build()

	wantErr := errors.New("boom")
	_, err := MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader(
		context.Background(),
		nil,
		k8sClient,
		types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
		func(obj *openbaov1alpha1.OpenBaoCluster) error {
			return wantErr
		},
		OpenBaoClusterAdminOpsStatusApplyOptions{},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader() error = %v, want %v", err, wantErr)
	}
}

func TestMutateAndApplyOpenBaoClusterAdminOpsStatus_ConcurrentWritersPreserveSiblingFields(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "adminops-concurrent",
			Namespace:       "default",
			ResourceVersion: "1",
		},
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(newOpenBaoClusterStatusTestScheme(t)).
		WithStatusSubresource(cluster).
		WithObjects(cluster.DeepCopy()).
		Build()
	k8sClient, conflictCount := newConcurrentApplyTestClient(baseClient, 3)

	key := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}
	errCh := make(chan error, 64)
	var wg sync.WaitGroup

	wg.Add(3)
	go func() {
		defer wg.Done()
		_, err := MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader(context.Background(), nil, k8sClient, key, func(obj *openbaov1alpha1.OpenBaoCluster) error {
			obj.Status.Backup = &openbaov1alpha1.BackupStatus{
				LastFailureReason: "backup-writer",
			}
			return nil
		}, OpenBaoClusterAdminOpsStatusApplyOptions{})
		if err != nil {
			errCh <- err
		}
	}()

	go func() {
		defer wg.Done()
		_, err := MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader(context.Background(), nil, k8sClient, key, func(obj *openbaov1alpha1.OpenBaoCluster) error {
			obj.Status.Upgrade = &openbaov1alpha1.UpgradeProgress{
				TargetVersion:    concurrentTargetVersion,
				FromVersion:      concurrentFromVersion,
				CurrentPartition: 1,
			}
			obj.Status.UpgradeRequests = &openbaov1alpha1.UpgradeRequestStatus{
				LastHandledRetry: concurrentRetryMarker,
			}
			return nil
		}, OpenBaoClusterAdminOpsStatusApplyOptions{})
		if err != nil {
			errCh <- err
		}
	}()

	go func() {
		defer wg.Done()
		_, err := MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader(context.Background(), nil, k8sClient, key, func(obj *openbaov1alpha1.OpenBaoCluster) error {
			obj.Status.BreakGlass = &openbaov1alpha1.BreakGlassStatus{
				Active: true,
				Nonce:  "nonce-concurrent",
			}
			obj.Status.AdminOps = &openbaov1alpha1.AdminOpsControllerStatus{
				LastError: &openbaov1alpha1.ControllerErrorStatus{
					Reason: "WrapperWrite",
				},
			}
			return nil
		}, OpenBaoClusterAdminOpsStatusApplyOptions{})
		if err != nil {
			errCh <- err
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent mutate+apply failed: %v", err)
	}
	if conflictCount.Load() == 0 {
		t.Fatal("concurrent mutate+apply conflict count = 0, want at least one retry")
	}

	stored := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), stored); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.Status.Backup == nil || stored.Status.Backup.LastFailureReason != "backup-writer" {
		t.Fatalf("stored backup = %#v, want backup writer state", stored.Status.Backup)
	}
	if stored.Status.Upgrade == nil || stored.Status.Upgrade.TargetVersion != concurrentTargetVersion {
		t.Fatalf("stored upgrade = %#v, want rolling writer state", stored.Status.Upgrade)
	}
	if stored.Status.UpgradeRequests == nil || stored.Status.UpgradeRequests.LastHandledRetry != concurrentRetryMarker {
		t.Fatalf("stored upgradeRequests = %#v, want retry marker", stored.Status.UpgradeRequests)
	}
	if stored.Status.BreakGlass == nil || !stored.Status.BreakGlass.Active || stored.Status.BreakGlass.Nonce != "nonce-concurrent" {
		t.Fatalf("stored breakGlass = %#v, want wrapper writer state", stored.Status.BreakGlass)
	}
	if stored.Status.AdminOps == nil || stored.Status.AdminOps.LastError == nil || stored.Status.AdminOps.LastError.Reason != "WrapperWrite" {
		t.Fatalf("stored adminOps = %#v, want wrapper writer adminops state", stored.Status.AdminOps)
	}
}
