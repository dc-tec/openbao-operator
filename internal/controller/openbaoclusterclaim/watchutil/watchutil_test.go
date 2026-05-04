package watchutil

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

func TestRequestForClaimManagedLabels(t *testing.T) {
	t.Parallel()

	requests := RequestForClaimManagedLabels(map[string]string{
		constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
		constants.LabelOpenBaoClaimNamespace: "payments",
		constants.LabelOpenBaoClaimName:      "payments-bao",
	})
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].NamespacedName != (client.ObjectKey{Namespace: "payments", Name: "payments-bao"}) {
		t.Fatalf("request key = %#v, want payments/payments-bao", requests[0].NamespacedName)
	}

	requests = RequestForClaimManagedLabels(map[string]string{
		constants.LabelOpenBaoOwnershipMode: constants.LabelValueOpenBaoOwnershipDirectManaged,
	})
	if len(requests) != 0 {
		t.Fatalf("direct managed request count = %d, want 0", len(requests))
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

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	return scheme
}
