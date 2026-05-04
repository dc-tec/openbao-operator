package backuprequest

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

func TestMapClaimToBackupRequests(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newTestScheme(t)
	reconciler := &OpenBaoClusterClaimBackupRequestReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "backup-1"},
					Spec:       openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{ClaimRef: openbaov1alpha1.LocalReference{Name: "payments-bao"}},
				},
				&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "backup-2"},
					Spec:       openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{ClaimRef: openbaov1alpha1.LocalReference{Name: "payments-bao"}},
				},
				&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "backup-other"},
					Spec:       openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{ClaimRef: openbaov1alpha1.LocalReference{Name: "other-bao"}},
				},
			).
			Build(),
		Scheme: scheme,
	}

	claim := &openbaov1alpha1.OpenBaoClusterClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "payments-bao"}}
	requests := reconciler.requestMapper().FromClaim()(ctx, claim)
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	if requests[0].NamespacedName != (client.ObjectKey{Namespace: "payments", Name: "backup-1"}) {
		t.Fatalf("first request key = %#v, want payments/backup-1", requests[0].NamespacedName)
	}
	if requests[1].NamespacedName != (client.ObjectKey{Namespace: "payments", Name: "backup-2"}) {
		t.Fatalf("second request key = %#v, want payments/backup-2", requests[1].NamespacedName)
	}
}

func TestMapClaimManagedClusterToBackupRequests(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newTestScheme(t)
	reconciler := &OpenBaoClusterClaimBackupRequestReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "backup-1"},
					Spec:       openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{ClaimRef: openbaov1alpha1.LocalReference{Name: "payments-bao"}},
				},
			).
			Build(),
		Scheme: scheme,
	}

	t.Run("maps claim managed cluster by ownership labels", func(t *testing.T) {
		t.Parallel()

		cluster := &openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{
			Namespace: "tenant-payments",
			Name:      "payments-bao",
			Labels: map[string]string{
				constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
				constants.LabelOpenBaoClaimNamespace: "payments",
				constants.LabelOpenBaoClaimName:      "payments-bao",
			},
		}}

		requests := reconciler.requestMapper().FromClaimManagedCluster()(ctx, cluster)
		if len(requests) != 1 {
			t.Fatalf("request count = %d, want 1", len(requests))
		}
		if requests[0].NamespacedName != (client.ObjectKey{Namespace: "payments", Name: "backup-1"}) {
			t.Fatalf("request key = %#v, want payments/backup-1", requests[0].NamespacedName)
		}
	})

	t.Run("ignores directly managed cluster", func(t *testing.T) {
		t.Parallel()

		cluster := &openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			constants.LabelOpenBaoOwnershipMode: constants.LabelValueOpenBaoOwnershipDirectManaged,
		}}}
		if requests := reconciler.requestMapper().FromClaimManagedCluster()(ctx, cluster); len(requests) != 0 {
			t.Fatalf("request count = %d, want 0", len(requests))
		}
	})
}

func TestReconcile(t *testing.T) {
	t.Parallel()

	t.Run("requires app reconciler", func(t *testing.T) {
		t.Parallel()

		reconciler := &OpenBaoClusterClaimBackupRequestReconciler{}
		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{})
		if err == nil {
			t.Fatal("Reconcile() error = nil, want configuration error")
		}
		if err.Error() != "openbaoclusterclaimbackuprequest app reconciler is not configured" {
			t.Fatalf("Reconcile() error = %q", err.Error())
		}
	})

	t.Run("delegates to app reconciler", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("boom")
		app := &stubAppReconciler{result: recon.Result{RequeueAfter: 5}, err: wantErr}
		reconciler := &OpenBaoClusterClaimBackupRequestReconciler{AppReconciler: app}
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "payments", Name: "backup-1"}}

		result, err := reconciler.Reconcile(context.Background(), req)
		if !errors.Is(err, wantErr) {
			t.Fatalf("Reconcile() error = %v, want %v", err, wantErr)
		}
		if result.RequeueAfter != app.result.RequeueAfter {
			t.Fatalf("Reconcile() requeueAfter = %v, want %v", result.RequeueAfter, app.result.RequeueAfter)
		}
		if app.key != req.NamespacedName {
			t.Fatalf("app reconciler key = %#v, want %#v", app.key, req.NamespacedName)
		}
	})
}

type stubAppReconciler struct {
	key    types.NamespacedName
	result recon.Result
	err    error
}

func (s *stubAppReconciler) Reconcile(_ context.Context, key types.NamespacedName, _ logr.Logger) (recon.Result, error) {
	s.key = key
	return s.result, s.err
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	return scheme
}
