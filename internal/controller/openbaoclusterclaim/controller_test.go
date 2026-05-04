package openbaoclusterclaim

import (
	"context"
	"errors"
	"testing"

	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestReconcile(t *testing.T) {
	t.Parallel()

	t.Run("requires app reconciler", func(t *testing.T) {
		t.Parallel()

		reconciler := &OpenBaoClusterClaimReconciler{}
		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{})
		if err == nil {
			t.Fatal("Reconcile() error = nil, want configuration error")
		}
		if err.Error() != "openbaoclusterclaim app reconciler is not configured" {
			t.Fatalf("Reconcile() error = %q", err.Error())
		}
	})

	t.Run("delegates to app reconciler", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("boom")
		app := &stubAppReconciler{
			result: recon.Result{RequeueAfter: 5},
			err:    wantErr,
		}
		reconciler := &OpenBaoClusterClaimReconciler{AppReconciler: app}
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "payments", Name: "payments-bao"}}

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

func (s *stubAppReconciler) Reconcile(
	_ context.Context,
	key types.NamespacedName,
	_ logr.Logger,
) (recon.Result, error) {
	s.key = key
	return s.result, s.err
}
