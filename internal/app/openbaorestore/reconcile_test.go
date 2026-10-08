package openbaorestore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/kubebao/openbao-operator/api/v1alpha1"
	"github.com/kubebao/openbao-operator/internal/platform/constants"
	recon "github.com/kubebao/openbao-operator/internal/platform/reconcile"
)

type fakeRestoreManager struct {
	result   recon.Result
	err      error
	called   bool
	observed *openbaov1alpha1.OpenBaoRestore
}

func (f *fakeRestoreManager) Reconcile(_ context.Context, _ logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (recon.Result, error) {
	f.called = true
	f.observed = restore
	return f.result, f.err
}

func TestReconcileOpenBaoRestore(t *testing.T) {
	t.Parallel()

	restoreObj := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "ns"},
		Spec:       openbaov1alpha1.OpenBaoRestoreSpec{Cluster: "cluster-a"},
	}

	t.Run("nil manager", func(t *testing.T) {
		_, err := ReconcileOpenBaoRestore(context.Background(), restoreObj, logr.Discard(), nil)
		if err == nil || !strings.Contains(err.Error(), "restore manager is required") {
			t.Fatalf("expected manager required error, got %v", err)
		}
	})

	t.Run("nil restore resource", func(t *testing.T) {
		mgr := &fakeRestoreManager{}
		_, err := ReconcileOpenBaoRestore(context.Background(), nil, logr.Discard(), mgr)
		if err == nil || !strings.Contains(err.Error(), "restore resource is required") {
			t.Fatalf("expected restore resource required error, got %v", err)
		}
		if mgr.called {
			t.Fatalf("manager should not be called without a restore resource")
		}
	})

	t.Run("manager error is propagated", func(t *testing.T) {
		mgr := &fakeRestoreManager{err: errors.New("reconcile failed")}
		_, err := ReconcileOpenBaoRestore(context.Background(), restoreObj, logr.Discard(), mgr)
		if err == nil || !strings.Contains(err.Error(), "reconcile failed") {
			t.Fatalf("expected manager error, got %v", err)
		}
		if !mgr.called || mgr.observed != restoreObj {
			t.Fatalf("expected manager to receive the supplied restore object")
		}
	})

	t.Run("manager result is passed through", func(t *testing.T) {
		mgr := &fakeRestoreManager{result: recon.Result{RequeueAfter: 9 * time.Second}}
		result, err := ReconcileOpenBaoRestore(context.Background(), restoreObj, logr.Discard(), mgr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.RequeueAfter != 9*time.Second {
			t.Fatalf("RequeueAfter=%v, want 9s", result.RequeueAfter)
		}
		if mgr.observed != restoreObj {
			t.Fatalf("expected manager to receive the supplied restore object")
		}
	})
}

func TestResumeRequiresCurrentAdmissionPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, acknowledgement      string
		restart, released, allowed bool
	}{
		{name: "observation", allowed: true},
		{name: "new Resume", acknowledgement: "operation/Resume"},
		{name: "persisted Resume", restart: true},
		{name: "Abandon during restart", acknowledgement: "operation/Abandon", restart: true, allowed: true},
		{name: "released request", restart: true, released: true, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := &openbaov1alpha1.OpenBaoRestore{
				ObjectMeta: metav1.ObjectMeta{UID: "operation", Annotations: map[string]string{constants.AnnotationRestoreAcknowledge: tc.acknowledgement}},
				Status: openbaov1alpha1.OpenBaoRestoreStatus{
					Phase:     openbaov1alpha1.RestorePhaseUnknown,
					Execution: &openbaov1alpha1.RestoreExecutionStatus{Stage: openbaov1alpha1.RestoreExecutionStageUnknown},
				},
			}
			if tc.restart {
				request.Status.Restart = &openbaov1alpha1.RestoreRestartStatus{}
			}
			if tc.released {
				request.Status.AdministratorDisposition = openbaov1alpha1.RestoreAdministratorResume
			}
			if allowed := CanContinueWithoutAdmission(request); allowed != tc.allowed {
				t.Fatalf("CanContinueWithoutAdmission() = %t, want %t", allowed, tc.allowed)
			}
		})
	}
}
