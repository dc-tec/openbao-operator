package statusapply

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestMutateAndPatchOpenBaoClusterOperationLockStatus_PropagatesMutatorError(t *testing.T) {
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
	_, err := MutateAndPatchOpenBaoClusterOperationLockStatusWithReader(
		context.Background(),
		nil,
		k8sClient,
		types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
		func(obj *openbaov1alpha1.OpenBaoCluster) error {
			return wantErr
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("MutateAndPatchOpenBaoClusterOperationLockStatusWithReader() error = %v, want %v", err, wantErr)
	}
}

func TestMutateAndPatchOpenBaoClusterOperationLockStatus_UsesOptimisticLockPatch(t *testing.T) {
	t.Parallel()

	acquiredAt := metav1.Now()
	stored := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "operationlock-patch",
			Namespace:       "default",
			ResourceVersion: "17",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Phase:         openbaov1alpha1.ClusterPhaseRunning,
			ReadyReplicas: 3,
			OperationLock: &openbaov1alpha1.OperationLockStatus{
				Operation:  openbaov1alpha1.ClusterOperationBackup,
				Holder:     "openbaocluster/backup",
				Message:    "backup in progress",
				AcquiredAt: &acquiredAt,
				RenewedAt:  &acquiredAt,
			},
		},
	}

	var patchPayload map[string]json.RawMessage
	k8sClient := fake.NewClientBuilder().
		WithScheme(newOpenBaoClusterStatusTestScheme(t)).
		WithStatusSubresource(stored).
		WithObjects(stored.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(
				ctx context.Context,
				c client.Client,
				subResource string,
				obj client.Object,
				patch client.Patch,
				opts ...client.SubResourcePatchOption,
			) error {
				if subResource != "status" {
					t.Fatalf("subResource = %q, want status", subResource)
				}
				if patch.Type() != types.MergePatchType {
					t.Fatalf("patch type = %q, want %q", patch.Type(), types.MergePatchType)
				}
				payload, err := patch.Data(obj)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(payload, &patchPayload); err != nil {
					return err
				}
				return c.Status().Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	_, err := MutateAndPatchOpenBaoClusterOperationLockStatusWithReader(
		context.Background(),
		nil,
		k8sClient,
		types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace},
		func(obj *openbaov1alpha1.OpenBaoCluster) error {
			obj.Status.OperationLock = nil
			return nil
		},
	)
	if err != nil {
		t.Fatalf("MutateAndPatchOpenBaoClusterOperationLockStatusWithReader() error = %v", err)
	}

	if len(patchPayload) != 2 {
		t.Fatalf("patch top-level fields = %#v, want only metadata and status", patchPayload)
	}
	var metadataPayload map[string]json.RawMessage
	if err := json.Unmarshal(patchPayload["metadata"], &metadataPayload); err != nil {
		t.Fatalf("unmarshal metadata patch: %v", err)
	}
	if len(metadataPayload) != 1 || string(metadataPayload["resourceVersion"]) != `"17"` {
		t.Fatalf("metadata patch = %#v, want only resourceVersion 17", metadataPayload)
	}
	var statusPayload map[string]json.RawMessage
	if err := json.Unmarshal(patchPayload["status"], &statusPayload); err != nil {
		t.Fatalf("unmarshal status patch: %v", err)
	}
	if len(statusPayload) != 1 || string(statusPayload["operationLock"]) != "null" {
		t.Fatalf("status patch = %#v, want only operationLock null", statusPayload)
	}

	got := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(stored), got); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status.OperationLock != nil {
		t.Fatalf("stored operation lock = %+v, want nil", got.Status.OperationLock)
	}
	if got.Status.Phase != stored.Status.Phase || got.Status.ReadyReplicas != stored.Status.ReadyReplicas {
		t.Fatalf("unowned status fields changed: got=%+v want=%+v", got.Status, stored.Status)
	}
}
