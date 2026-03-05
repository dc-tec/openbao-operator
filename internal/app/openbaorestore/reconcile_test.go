package openbaorestore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

type fakeRestoreManager struct {
	result       recon.Result
	err          error
	called       bool
	observedName string
}

func (f *fakeRestoreManager) Reconcile(_ context.Context, _ logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (recon.Result, error) {
	f.called = true
	if restore != nil {
		f.observedName = restore.Name
	}
	return f.result, f.err
}

func newRestoreScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return s
}

func newRestoreClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	builder := fake.NewClientBuilder().WithScheme(newRestoreScheme(t))
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

func newRestoreClientWithGetError(t *testing.T, matchName string, err error, objs ...client.Object) client.Client {
	t.Helper()
	builder := fake.NewClientBuilder().WithScheme(newRestoreScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if key.Name == matchName {
				return err
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

func TestReconcileOpenBaoRestore(t *testing.T) {
	t.Parallel()

	req := types.NamespacedName{Namespace: "ns", Name: "restore"}
	restoreObj := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "ns"},
		Spec:       openbaov1alpha1.OpenBaoRestoreSpec{Cluster: "cluster-a"},
	}

	t.Run("nil manager", func(t *testing.T) {
		_, err := ReconcileOpenBaoRestore(context.Background(), newRestoreClient(t), req, logr.Discard(), nil)
		if err == nil || !strings.Contains(err.Error(), "restore manager is required") {
			t.Fatalf("expected manager required error, got %v", err)
		}
	})

	t.Run("resource not found", func(t *testing.T) {
		mgr := &fakeRestoreManager{}
		result, err := ReconcileOpenBaoRestore(context.Background(), newRestoreClient(t), req, logr.Discard(), mgr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != (recon.Result{}) {
			t.Fatalf("result=%v, want zero", result)
		}
		if mgr.called {
			t.Fatalf("manager should not be called for not-found resource")
		}
	})

	t.Run("get failure", func(t *testing.T) {
		mgr := &fakeRestoreManager{}
		reader := newRestoreClientWithGetError(t, "restore", errors.New("boom"), restoreObj)
		_, err := ReconcileOpenBaoRestore(context.Background(), reader, req, logr.Discard(), mgr)
		if err == nil || !strings.Contains(err.Error(), "failed to get OpenBaoRestore") {
			t.Fatalf("expected get error, got %v", err)
		}
		if mgr.called {
			t.Fatalf("manager should not be called when get fails")
		}
	})

	t.Run("manager error is propagated", func(t *testing.T) {
		mgr := &fakeRestoreManager{err: errors.New("reconcile failed")}
		_, err := ReconcileOpenBaoRestore(context.Background(), newRestoreClient(t, restoreObj.DeepCopy()), req, logr.Discard(), mgr)
		if err == nil || !strings.Contains(err.Error(), "reconcile failed") {
			t.Fatalf("expected manager error, got %v", err)
		}
		if !mgr.called || mgr.observedName != "restore" {
			t.Fatalf("expected manager to be called with restore object")
		}
	})

	t.Run("manager result is passed through", func(t *testing.T) {
		mgr := &fakeRestoreManager{result: recon.Result{RequeueAfter: 9 * time.Second}}
		result, err := ReconcileOpenBaoRestore(context.Background(), newRestoreClient(t, restoreObj.DeepCopy()), req, logr.Discard(), mgr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.RequeueAfter != 9*time.Second {
			t.Fatalf("RequeueAfter=%v, want 9s", result.RequeueAfter)
		}
	})

	t.Run("explicit not found error path", func(t *testing.T) {
		mgr := &fakeRestoreManager{}
		reader := newRestoreClientWithGetError(t, "restore", apierrors.NewNotFound(schema.GroupResource{Group: "openbao.org", Resource: "openbaorestores"}, "restore"))
		result, err := ReconcileOpenBaoRestore(context.Background(), reader, req, logr.Discard(), mgr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != (recon.Result{}) {
			t.Fatalf("result=%v, want zero", result)
		}
		if mgr.called {
			t.Fatalf("manager should not be called for not found")
		}
	})
}
