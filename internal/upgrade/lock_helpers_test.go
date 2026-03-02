package upgrade

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
	"github.com/dc-tec/openbao-operator/internal/operationlock"
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
		{
			name: "held error",
			err:  heldErr,
			want: true,
		},
		{
			name: "wrapped held error",
			err:  errors.Join(errors.New("wrapper"), heldErr),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("boom"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
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

func TestIsUpgradeOperationLockHeldByUs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		lock *openbaov1alpha1.OperationLockStatus
		want bool
	}{
		{
			name: "nil lock",
			lock: nil,
			want: false,
		},
		{
			name: "matching lock",
			lock: &openbaov1alpha1.OperationLockStatus{
				Operation: openbaov1alpha1.ClusterOperationUpgrade,
				Holder:    UpgradeOperationLockHolder,
			},
			want: true,
		},
		{
			name: "different holder",
			lock: &openbaov1alpha1.OperationLockStatus{
				Operation: openbaov1alpha1.ClusterOperationUpgrade,
				Holder:    "controller/other",
			},
			want: false,
		},
		{
			name: "different operation",
			lock: &openbaov1alpha1.OperationLockStatus{
				Operation: openbaov1alpha1.ClusterOperationBackup,
				Holder:    UpgradeOperationLockHolder,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsUpgradeOperationLockHeldByUs(tt.lock); got != tt.want {
				t.Fatalf("IsUpgradeOperationLockHeldByUs()=%v, want %v", got, tt.want)
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
	if cluster.Status.OperationLock.Operation != openbaov1alpha1.ClusterOperationUpgrade {
		t.Fatalf("lock operation=%q, want %q", cluster.Status.OperationLock.Operation, openbaov1alpha1.ClusterOperationUpgrade)
	}
	if cluster.Status.OperationLock.Message != "starting upgrade" {
		t.Fatalf("lock message=%q, want %q", cluster.Status.OperationLock.Message, "starting upgrade")
	}

	stored := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, stored); err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if stored.Status.OperationLock == nil {
		t.Fatalf("stored cluster lock status is nil")
	}

	if err := ReleaseUpgradeOperationLock(context.Background(), k8sClient, cluster); err != nil {
		t.Fatalf("ReleaseUpgradeOperationLock() unexpected error: %v", err)
	}
	if cluster.Status.OperationLock != nil {
		t.Fatalf("ReleaseUpgradeOperationLock() did not clear lock status")
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
				return AcquireUpgradeOperationLock(context.Background(), nil, nil, "message")
			},
			wantErr: "cluster is required",
		},
		{
			name: "release nil cluster",
			invoke: func() error {
				return ReleaseUpgradeOperationLock(context.Background(), nil, nil)
			},
			wantErr: "cluster is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.invoke()
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error=%q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}
