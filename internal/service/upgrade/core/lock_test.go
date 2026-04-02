package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/operationlock"
	"github.com/go-logr/logr"
)

func TestIsOperationLockHeld(t *testing.T) {
	t.Parallel()

	heldErr := &operationlock.HeldError{
		Operation: openbaov1alpha1.ClusterOperationBackup,
		Holder:    "controller/backup",
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "held error", err: heldErr, want: true},
		{name: "wrapped held error", err: errors.Join(errors.New("wrapper"), heldErr), want: true},
		{name: "unrelated error", err: errors.New("boom"), want: false},
		{name: "nil error", err: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsOperationLockHeld(tt.err); got != tt.want {
				t.Fatalf("IsOperationLockHeld()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestAcquireAndReleaseUpgradeOperationLock(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error: %v", err)
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openbao",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.0",
			Image:    "openbao/openbao:2.4.0",
			Replicas: 3,
			TLS:      openbaov1alpha1.TLSConfig{Mode: openbaov1alpha1.TLSModeOperatorManaged},
			Storage:  openbaov1alpha1.StorageConfig{Size: "10Gi"},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()

	if err := AcquireUpgradeOperationLock(context.Background(), k8sClient, cluster, "starting upgrade"); err != nil {
		t.Fatalf("AcquireUpgradeOperationLock() unexpected error: %v", err)
	}
	if cluster.Status.OperationLock == nil {
		t.Fatalf("AcquireUpgradeOperationLock() did not set cluster lock status")
	}
	if cluster.Status.OperationLock.Holder != UpgradeOperationLockHolder {
		t.Fatalf("lock holder=%q, want %q", cluster.Status.OperationLock.Holder, UpgradeOperationLockHolder)
	}

	if err := ReleaseUpgradeLockIfHeld(context.Background(), k8sClient, logr.Discard(), cluster); err != nil {
		t.Fatalf("ReleaseUpgradeLockIfHeld() unexpected error: %v", err)
	}
	if cluster.Status.OperationLock != nil {
		t.Fatalf("ReleaseUpgradeLockIfHeld() did not clear in-memory lock")
	}

	stored := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, stored); err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if stored.Status.OperationLock != nil {
		t.Fatalf("stored cluster lock status = %+v, want nil", stored.Status.OperationLock)
	}
}

func TestAcquireUpgradeLockBlocked(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error: %v", err)
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openbao",
			Namespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			OperationLock: &openbaov1alpha1.OperationLockStatus{
				Operation: openbaov1alpha1.ClusterOperationBackup,
				Holder:    "controller/backup",
				Message:   "backup running",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		Build()

	result, err := AcquireUpgradeLock(context.Background(), k8sClient, logr.Discard(), cluster, "starting upgrade")
	if err != nil {
		t.Fatalf("AcquireUpgradeLock() unexpected error: %v", err)
	}
	if !result.Blocked {
		t.Fatal("AcquireUpgradeLock() Blocked=false, want true")
	}
	if !IsOperationLockHeld(result.LockErr) {
		t.Fatalf("AcquireUpgradeLock() lockErr=%v, want held lock error", result.LockErr)
	}
}

func TestReleaseUpgradeLockIfHeldIgnoresForeignLock(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error: %v", err)
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openbao",
			Namespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			OperationLock: &openbaov1alpha1.OperationLockStatus{
				Operation: openbaov1alpha1.ClusterOperationBackup,
				Holder:    "controller/backup",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		Build()

	if err := ReleaseUpgradeLockIfHeld(context.Background(), k8sClient, logr.Discard(), cluster); err != nil {
		t.Fatalf("ReleaseUpgradeLockIfHeld() unexpected error: %v", err)
	}
	if cluster.Status.OperationLock == nil {
		t.Fatal("foreign lock was cleared unexpectedly")
	}
}

func TestUpgradeOperationLockRequiresCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		invoke  func() error
		wantErr string
	}{
		{
			name: "acquire nil cluster",
			invoke: func() error {
				_, err := AcquireUpgradeLock(context.Background(), nil, logr.Discard(), nil, "message")
				return err
			},
			wantErr: "cluster is required",
		},
		{
			name: "release nil cluster",
			invoke: func() error {
				return ReleaseUpgradeLockIfHeld(context.Background(), nil, logr.Discard(), nil)
			},
			wantErr: "cluster is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.invoke()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error=%q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestReleaseUpgradeLockOnErrorIfHeld(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error: %v", err)
	}

	t.Run("releases when requested", func(t *testing.T) {
		t.Parallel()

		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openbao",
				Namespace: "default",
			},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				OperationLock: &openbaov1alpha1.OperationLockStatus{
					Operation: openbaov1alpha1.ClusterOperationUpgrade,
					Holder:    UpgradeOperationLockHolder,
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
			WithObjects(cluster).
			Build()

		cause := errors.New("validation failed")
		err := ReleaseUpgradeLockOnErrorIfHeld(context.Background(), k8sClient, logr.Discard(), cluster, true, cause, "failed to release")
		if !errors.Is(err, cause) {
			t.Fatalf("error=%v, want joined cause %v", err, cause)
		}
		if cluster.Status.OperationLock != nil {
			t.Fatalf("operation lock = %+v, want nil", cluster.Status.OperationLock)
		}
	})

	t.Run("skips release when not requested", func(t *testing.T) {
		t.Parallel()

		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openbao",
				Namespace: "default",
			},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				OperationLock: &openbaov1alpha1.OperationLockStatus{
					Operation: openbaov1alpha1.ClusterOperationUpgrade,
					Holder:    UpgradeOperationLockHolder,
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
			WithObjects(cluster).
			Build()

		cause := errors.New("still active")
		err := ReleaseUpgradeLockOnErrorIfHeld(context.Background(), k8sClient, logr.Discard(), cluster, false, cause, "failed to release")
		if !errors.Is(err, cause) {
			t.Fatalf("error=%v, want cause %v", err, cause)
		}
		if cluster.Status.OperationLock == nil {
			t.Fatal("expected lock to remain held")
		}
	})
}
