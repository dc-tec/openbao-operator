package statusapply

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestApplyOpenBaoClusterOperationLockStatus_PersistsLockPlane(t *testing.T) {
	t.Parallel()

	acquiredAt := metav1.Now()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "operationlock-plane",
			Namespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			OperationLock: &openbaov1alpha1.OperationLockStatus{
				Operation:  openbaov1alpha1.ClusterOperationUpgrade,
				Holder:     "openbao-adminops-controller/upgrade",
				Message:    "upgrade in progress",
				AcquiredAt: &acquiredAt,
				RenewedAt:  &acquiredAt,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(newOpenBaoClusterStatusTestScheme(t)).
		WithStatusSubresource(cluster).
		WithObjects(cluster.DeepCopy()).
		Build()

	if err := ApplyOpenBaoClusterOperationLockStatus(context.Background(), k8sClient, cluster, OpenBaoClusterOperationLockStatusApplyOptions{}); err != nil {
		t.Fatalf("ApplyOpenBaoClusterOperationLockStatus() error = %v", err)
	}

	stored := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), stored); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if stored.Status.OperationLock == nil {
		t.Fatal("stored operationLock = nil, want populated")
	}
	if stored.Status.OperationLock.Operation != cluster.Status.OperationLock.Operation {
		t.Fatalf("stored operation = %q, want %q", stored.Status.OperationLock.Operation, cluster.Status.OperationLock.Operation)
	}
	if stored.Status.OperationLock.Holder != cluster.Status.OperationLock.Holder {
		t.Fatalf("stored holder = %q, want %q", stored.Status.OperationLock.Holder, cluster.Status.OperationLock.Holder)
	}
	if stored.Status.OperationLock.Message != cluster.Status.OperationLock.Message {
		t.Fatalf("stored message = %q, want %q", stored.Status.OperationLock.Message, cluster.Status.OperationLock.Message)
	}
	if stored.Status.OperationLock.AcquiredAt == nil || stored.Status.OperationLock.RenewedAt == nil {
		t.Fatalf("stored timestamps = %#v, want acquired/renewed timestamps set", stored.Status.OperationLock)
	}
}

func TestApplyOpenBaoClusterOperationLockStatus_ApplyOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		force     bool
		wantForce bool
	}{
		{
			name:      "without force ownership",
			force:     false,
			wantForce: false,
		},
		{
			name:      "with force ownership",
			force:     true,
			wantForce: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "operationlock-options",
					Namespace: "default",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					OperationLock: &openbaov1alpha1.OperationLockStatus{
						Operation: openbaov1alpha1.ClusterOperationUpgrade,
						Holder:    "owner/upgrade",
					},
				},
			}

			var capturedOptions client.SubResourceApplyOptions
			var subResourceName string

			k8sClient := fake.NewClientBuilder().
				WithScheme(newOpenBaoClusterStatusTestScheme(t)).
				WithStatusSubresource(cluster).
				WithObjects(cluster.DeepCopy()).
				WithInterceptorFuncs(interceptor.Funcs{
					SubResourceApply: func(ctx context.Context, c client.Client, subResource string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
						subResourceName = subResource
						capturedOptions = *(&client.SubResourceApplyOptions{}).ApplyOpts(opts)
						return c.Status().Apply(ctx, obj, opts...)
					},
				}).
				Build()

			err := ApplyOpenBaoClusterOperationLockStatus(context.Background(), k8sClient, cluster, OpenBaoClusterOperationLockStatusApplyOptions{
				ForceOwnership: tt.force,
			})
			if err != nil {
				t.Fatalf("ApplyOpenBaoClusterOperationLockStatus() error = %v", err)
			}

			if subResourceName != "status" {
				t.Fatalf("subResourceName = %q, want status", subResourceName)
			}
			if capturedOptions.FieldManager != constants.FieldOwnerOperationLockStatus {
				t.Fatalf("FieldManager = %q, want %q", capturedOptions.FieldManager, constants.FieldOwnerOperationLockStatus)
			}

			force := capturedOptions.Force != nil && *capturedOptions.Force
			if force != tt.wantForce {
				t.Fatalf("Force = %v, want %v", force, tt.wantForce)
			}
		})
	}
}

func TestMutateAndApplyOpenBaoClusterOperationLockStatus_PropagatesMutatorError(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "operationlock-mutate-error",
			Namespace: "default",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(newOpenBaoClusterStatusTestScheme(t)).
		WithStatusSubresource(cluster).
		WithObjects(cluster.DeepCopy()).
		Build()

	wantErr := errors.New("boom")
	_, err := MutateAndApplyOpenBaoClusterOperationLockStatusWithReader(
		context.Background(),
		nil,
		k8sClient,
		types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
		func(obj *openbaov1alpha1.OpenBaoCluster) error {
			return wantErr
		},
		OpenBaoClusterOperationLockStatusApplyOptions{},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("MutateAndApplyOpenBaoClusterOperationLockStatusWithReader() error = %v, want %v", err, wantErr)
	}
}

func TestMutateAndApplyOpenBaoClusterOperationLockStatus_ClearTakesOwnershipThenOmitsField(t *testing.T) {
	t.Parallel()

	acquiredAt := metav1.Now()
	stored := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "operationlock-clear",
			Namespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			OperationLock: &openbaov1alpha1.OperationLockStatus{
				Operation:  openbaov1alpha1.ClusterOperationBackup,
				Holder:     "openbaocluster/backup",
				Message:    "backup in progress",
				AcquiredAt: &acquiredAt,
				RenewedAt:  &acquiredAt,
			},
		},
	}

	var applyPayloads []string
	k8sClient := fake.NewClientBuilder().
		WithScheme(newOpenBaoClusterStatusTestScheme(t)).
		WithStatusSubresource(stored).
		WithObjects(stored.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceApply: func(ctx context.Context, c client.Client, subResource string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
				if subResource == "status" {
					payload, err := json.Marshal(obj)
					if err != nil {
						return err
					}
					applyPayloads = append(applyPayloads, string(payload))
				}
				return c.Status().Apply(ctx, obj, opts...)
			},
		}).
		WithReturnManagedFields().
		Build()

	_, err := MutateAndApplyOpenBaoClusterOperationLockStatusWithReader(
		context.Background(),
		nil,
		k8sClient,
		types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace},
		func(obj *openbaov1alpha1.OpenBaoCluster) error {
			obj.Status.OperationLock = nil
			return nil
		},
		OpenBaoClusterOperationLockStatusApplyOptions{},
	)
	if err != nil {
		t.Fatalf("MutateAndApplyOpenBaoClusterOperationLockStatusWithReader() error = %v", err)
	}

	if len(applyPayloads) != 2 {
		t.Fatalf("apply payloads = %#v, want 2 status apply calls (take ownership, then clear)", applyPayloads)
	}
	if !strings.Contains(applyPayloads[0], `"operationLock":{"`) {
		t.Fatalf("ownership payload missing operationLock object: %s", applyPayloads[0])
	}
	if strings.Contains(applyPayloads[1], `"operationLock":{"`) || !strings.Contains(applyPayloads[1], `"operationLock":null`) {
		t.Fatalf("clear payload should explicitly null operationLock after ownership takeover: %s", applyPayloads[1])
	}
}
