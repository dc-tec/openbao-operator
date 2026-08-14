//go:build integration
// +build integration

package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
)

func TestOperationLockConcurrentStaleAcquisitionsSerialize(t *testing.T) {
	namespace := newTestNamespace(t)
	cluster := newMinimalClusterObj(namespace, "operation-lock-concurrent")
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}

	key := types.NamespacedName{Namespace: namespace, Name: cluster.Name}
	staleClusters := make([]*openbaov1alpha1.OpenBaoCluster, 2)
	for i := range staleClusters {
		staleClusters[i] = &openbaov1alpha1.OpenBaoCluster{}
		if err := k8sClient.Get(ctx, key, staleClusters[i]); err != nil {
			t.Fatalf("get stale OpenBaoCluster copy %d: %v", i, err)
		}
	}

	barrierReader := newOperationLockReadBarrier(k8sClient, 2)
	locks := []opslifecycle.OperationLock{
		{Operation: openbaov1alpha1.ClusterOperationBackup, Holder: "test/backup"},
		{Operation: openbaov1alpha1.ClusterOperationRestore, Holder: "test/restore"},
	}
	errs := make([]error, len(locks))

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i := range locks {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errs[index] = opslifecycle.AcquireWithReader(
				testCtx,
				barrierReader,
				k8sClient,
				staleClusters[index],
				locks[index],
				opslifecycle.AcquireOptions{Message: "concurrent acquisition"},
			)
		}(i)
	}
	wg.Wait()

	successes := 0
	heldErrors := 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case opslifecycle.IsLockHeld(err):
			heldErrors++
		default:
			t.Fatalf("concurrent acquisition returned unexpected error: %v", err)
		}
	}
	if successes != 1 || heldErrors != 1 {
		t.Fatalf("concurrent acquisition results: successes=%d heldErrors=%d errors=%v", successes, heldErrors, errs)
	}

	stored := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(ctx, key, stored); err != nil {
		t.Fatalf("get OpenBaoCluster after concurrent acquisition: %v", err)
	}
	if stored.Status.OperationLock == nil {
		t.Fatal("stored operation lock is nil after concurrent acquisition")
	}

	winnerFound := false
	for i := range locks {
		if errs[i] == nil {
			winnerFound = locks[i].IsHeldBy(stored.Status.OperationLock)
			break
		}
	}
	if !winnerFound {
		t.Fatalf("stored operation lock does not match the successful acquisition: %+v", stored.Status.OperationLock)
	}
}

func TestOperationLockStaleReleaseDoesNotClearForcedReplacement(t *testing.T) {
	namespace := newTestNamespace(t)
	cluster := newMinimalClusterObj(namespace, "operation-lock-stale-release")
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}

	key := types.NamespacedName{Namespace: namespace, Name: cluster.Name}
	owner := opslifecycle.OperationLock{
		Operation: openbaov1alpha1.ClusterOperationBackup,
		Holder:    "test/backup",
	}
	replacement := opslifecycle.OperationLock{
		Operation: openbaov1alpha1.ClusterOperationRestore,
		Holder:    "test/restore",
	}

	if err := opslifecycle.AcquireWithReader(
		ctx,
		k8sClient,
		k8sClient,
		cluster,
		owner,
		opslifecycle.AcquireOptions{Message: "backup"},
	); err != nil {
		t.Fatalf("acquire original operation lock: %v", err)
	}

	staleOwner := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(ctx, key, staleOwner); err != nil {
		t.Fatalf("get stale owner copy: %v", err)
	}
	forceCaller := staleOwner.DeepCopy()
	if err := opslifecycle.AcquireWithReader(
		ctx,
		k8sClient,
		k8sClient,
		forceCaller,
		replacement,
		opslifecycle.AcquireOptions{Message: "forced restore", Force: true},
	); err != nil {
		t.Fatalf("force replacement operation lock: %v", err)
	}

	err := opslifecycle.ReleaseWithReader(ctx, k8sClient, k8sClient, staleOwner, owner)
	if !opslifecycle.IsLockHeld(err) {
		t.Fatalf("stale release error = %v, want held-lock error", err)
	}

	stored := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(ctx, key, stored); err != nil {
		t.Fatalf("get OpenBaoCluster after stale release: %v", err)
	}
	if !replacement.IsHeldBy(stored.Status.OperationLock) {
		t.Fatalf("stored operation lock = %+v, want forced replacement", stored.Status.OperationLock)
	}
}

func TestOperationLockNormalAcquireRenewRelease(t *testing.T) {
	namespace := newTestNamespace(t)
	cluster := newMinimalClusterObj(namespace, "operation-lock-lifecycle")
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}

	key := types.NamespacedName{Namespace: namespace, Name: cluster.Name}
	lock := opslifecycle.OperationLock{
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Holder:    "test/upgrade",
	}
	if err := opslifecycle.AcquireWithReader(
		ctx,
		k8sClient,
		k8sClient,
		cluster,
		lock,
		opslifecycle.AcquireOptions{Message: "acquire"},
	); err != nil {
		t.Fatalf("acquire operation lock: %v", err)
	}

	acquired := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(ctx, key, acquired); err != nil {
		t.Fatalf("get acquired operation lock: %v", err)
	}
	if !lock.IsHeldBy(acquired.Status.OperationLock) {
		t.Fatalf("acquired operation lock = %+v, want %+v", acquired.Status.OperationLock, lock)
	}
	if acquired.Status.OperationLock.AcquiredAt == nil || acquired.Status.OperationLock.RenewedAt == nil {
		t.Fatalf("acquired operation lock timestamps are incomplete: %+v", acquired.Status.OperationLock)
	}
	originalAcquiredAt := acquired.Status.OperationLock.AcquiredAt.DeepCopy()
	originalRenewedAt := acquired.Status.OperationLock.RenewedAt.DeepCopy()

	if err := opslifecycle.AcquireWithReader(
		ctx,
		k8sClient,
		k8sClient,
		acquired,
		lock,
		opslifecycle.AcquireOptions{Message: "renew"},
	); err != nil {
		t.Fatalf("renew operation lock: %v", err)
	}

	renewed := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(ctx, key, renewed); err != nil {
		t.Fatalf("get renewed operation lock: %v", err)
	}
	if renewed.Status.OperationLock == nil {
		t.Fatal("renewed operation lock is nil")
	}
	if renewed.Status.OperationLock.Message != "renew" {
		t.Fatalf("renewed message = %q, want %q", renewed.Status.OperationLock.Message, "renew")
	}
	if !renewed.Status.OperationLock.AcquiredAt.Equal(originalAcquiredAt) {
		t.Fatalf(
			"renewed acquiredAt = %v, want %v",
			renewed.Status.OperationLock.AcquiredAt,
			originalAcquiredAt,
		)
	}
	if renewed.Status.OperationLock.RenewedAt.Before(originalRenewedAt) {
		t.Fatalf(
			"renewed renewedAt = %v, want time at or after %v",
			renewed.Status.OperationLock.RenewedAt,
			originalRenewedAt,
		)
	}

	if err := opslifecycle.ReleaseWithReader(ctx, k8sClient, k8sClient, renewed, lock); err != nil {
		t.Fatalf("release operation lock: %v", err)
	}

	released := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(ctx, key, released); err != nil {
		t.Fatalf("get released operation lock: %v", err)
	}
	if released.Status.OperationLock != nil {
		t.Fatalf("released operation lock = %+v, want nil", released.Status.OperationLock)
	}
}

type operationLockReadBarrier struct {
	client.Reader

	mu      sync.Mutex
	target  int
	arrived int
	release chan struct{}
}

func newOperationLockReadBarrier(reader client.Reader, target int) *operationLockReadBarrier {
	return &operationLockReadBarrier{
		Reader:  reader,
		target:  target,
		release: make(chan struct{}),
	}
}

func (r *operationLockReadBarrier) Get(
	ctx context.Context,
	key client.ObjectKey,
	obj client.Object,
	opts ...client.GetOption,
) error {
	if err := r.Reader.Get(ctx, key, obj, opts...); err != nil {
		return err
	}

	r.mu.Lock()
	if r.arrived >= r.target {
		r.mu.Unlock()
		return nil
	}
	r.arrived++
	if r.arrived == r.target {
		close(r.release)
	}
	release := r.release
	r.mu.Unlock()

	select {
	case <-release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
