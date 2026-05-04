package requestwatch

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestMapper(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reader := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(
			&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
				ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "backup-1"},
				Spec: openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{
					ClaimRef: openbaov1alpha1.LocalReference{Name: "payments-bao"},
				},
			},
			&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
				ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "backup-other"},
				Spec: openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{
					ClaimRef: openbaov1alpha1.LocalReference{Name: "other-bao"},
				},
			},
		).
		Build()

	mapper := backupRequestMapper(reader)
	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "payments-bao"},
	}
	requests := mapper.FromClaim()(ctx, claim)
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].NamespacedName != (client.ObjectKey{Namespace: "payments", Name: "backup-1"}) {
		t.Fatalf("request key = %#v, want payments/backup-1", requests[0].NamespacedName)
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{
		Namespace: "tenant-payments",
		Name:      "payments-bao",
		Labels: map[string]string{
			constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
			constants.LabelOpenBaoClaimNamespace: "payments",
			constants.LabelOpenBaoClaimName:      "payments-bao",
		},
	}}
	requests = mapper.FromClaimManagedCluster()(ctx, cluster)
	if len(requests) != 1 {
		t.Fatalf("managed cluster request count = %d, want 1", len(requests))
	}
	if requests[0].NamespacedName != (client.ObjectKey{Namespace: "payments", Name: "backup-1"}) {
		t.Fatalf("managed cluster request key = %#v, want payments/backup-1", requests[0].NamespacedName)
	}
}

func TestSyncMetrics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	key := client.ObjectKey{Namespace: "payments", Name: "backup-1"}
	reader := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
			ObjectMeta: metav1.ObjectMeta{Namespace: key.Namespace, Name: key.Name},
		}).
		Build()

	var synced client.ObjectKey
	var cleared client.ObjectKey
	SyncMetrics(
		ctx,
		key,
		reader,
		nil,
		func() *openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
			return &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{}
		},
		func(request *openbaov1alpha1.OpenBaoClusterClaimBackupRequest) {
			synced = client.ObjectKeyFromObject(request)
		},
		func(namespace, name string) {
			cleared = client.ObjectKey{Namespace: namespace, Name: name}
		},
	)
	if synced != key {
		t.Fatalf("synced key = %#v, want %#v", synced, key)
	}
	if cleared != (client.ObjectKey{}) {
		t.Fatalf("cleared key = %#v, want empty", cleared)
	}

	missingKey := client.ObjectKey{Namespace: "payments", Name: "missing"}
	SyncMetrics(
		ctx,
		missingKey,
		reader,
		nil,
		func() *openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
			return &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{}
		},
		func(*openbaov1alpha1.OpenBaoClusterClaimBackupRequest) {
			t.Fatal("sync called for missing request")
		},
		func(namespace, name string) {
			cleared = client.ObjectKey{Namespace: namespace, Name: name}
		},
	)
	if cleared != missingKey {
		t.Fatalf("cleared key = %#v, want %#v", cleared, missingKey)
	}
}

func backupRequestMapper(reader client.Reader) Mapper[
	*openbaov1alpha1.OpenBaoClusterClaimBackupRequest,
	*openbaov1alpha1.OpenBaoClusterClaimBackupRequestList,
] {
	return Mapper[
		*openbaov1alpha1.OpenBaoClusterClaimBackupRequest,
		*openbaov1alpha1.OpenBaoClusterClaimBackupRequestList,
	]{
		Reader: reader,
		NewList: func() *openbaov1alpha1.OpenBaoClusterClaimBackupRequestList {
			return &openbaov1alpha1.OpenBaoClusterClaimBackupRequestList{}
		},
		Items: func(list *openbaov1alpha1.OpenBaoClusterClaimBackupRequestList) []*openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
			return ObjectPointers(list.Items)
		},
		ClaimName: func(request *openbaov1alpha1.OpenBaoClusterClaimBackupRequest) string {
			return request.Spec.ClaimRef.Name
		},
	}
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	return scheme
}
