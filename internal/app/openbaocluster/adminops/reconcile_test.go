package adminops

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	backupmanager "github.com/dc-tec/openbao-operator/internal/service/backup"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/bluegreen"
	rollingupgrade "github.com/dc-tec/openbao-operator/internal/service/upgrade/rolling"
)

type fakeSubReconciler struct {
	result recon.Result
	err    error
}

func (f fakeSubReconciler) Reconcile(_ context.Context, _ logr.Logger, _ *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	return f.result, f.err
}

type fakeMutatingSubReconciler struct {
	result recon.Result
	err    error
	mutate func(*openbaov1alpha1.OpenBaoCluster)
}

func (f fakeMutatingSubReconciler) Reconcile(_ context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	if f.mutate != nil {
		f.mutate(cluster)
	}
	return f.result, f.err
}

func withAdminOpsReconcilers(t *testing.T, recs ...subReconciler) {
	t.Helper()
	orig := adminOpsReconcilersBuilder
	adminOpsReconcilersBuilder = func(_ Dependencies) []subReconciler {
		return recs
	}
	t.Cleanup(func() {
		adminOpsReconcilersBuilder = orig
	})
}

func TestReconcile_AdminOpsErrorPaths(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name             string
		reconcilerErr    error
		patchErr         error
		wantErrContains  string
		wantRequeueAfter time.Duration
		wantPatchReason  string
	}{
		{
			name:             "transient connection error requeues with delay",
			reconcilerErr:    operatorerrors.WrapTransientConnection(errors.New("dial tcp: connection refused")),
			wantRequeueAfter: 5 * time.Second,
			wantPatchReason:  "adminops-error",
		},
		{
			name:             "transient overloaded error requeues with longer delay",
			reconcilerErr:    operatorerrors.WrapTransientRemoteOverloaded(errors.New("http 429")),
			wantRequeueAfter: 15 * time.Second,
			wantPatchReason:  "adminops-error",
		},
		{
			name:            "patch failure takes precedence",
			reconcilerErr:   operatorerrors.WrapPermanentConfig(errors.New("bad config")),
			patchErr:        errors.New("status patch failed"),
			wantErrContains: "status patch failed",
			wantPatchReason: "adminops-error",
		},
		{
			name:            "permanent error returns original error",
			reconcilerErr:   operatorerrors.WrapPermanentConfig(errors.New("invalid setting")),
			wantErrContains: "invalid setting",
			wantPatchReason: "adminops-error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withAdminOpsReconcilers(t, fakeSubReconciler{err: tt.reconcilerErr})

			var recorded []error
			var patchReasons []string
			patchStatus := func(_ context.Context, _ client.Client, _ logr.Logger, _ *openbaov1alpha1.OpenBaoCluster, _ *openbaov1alpha1.OpenBaoCluster, reason string) error {
				patchReasons = append(patchReasons, reason)
				return tt.patchErr
			}

			cluster := &openbaov1alpha1.OpenBaoCluster{
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					AdminOps: &openbaov1alpha1.AdminOpsControllerStatus{},
				},
			}
			original := cluster.DeepCopy()

			result, err := Reconcile(
				context.Background(),
				logr.Discard(),
				Dependencies{},
				original,
				cluster,
				func(err error) { recorded = append(recorded, err) },
				patchStatus,
				func(err error) *openbaov1alpha1.ControllerErrorStatus {
					return &openbaov1alpha1.ControllerErrorStatus{Reason: "MappedError", Message: err.Error(), At: &now}
				},
			)

			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErrContains, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantRequeueAfter != 0 && result.RequeueAfter != tt.wantRequeueAfter {
				t.Fatalf("RequeueAfter=%v, want %v", result.RequeueAfter, tt.wantRequeueAfter)
			}

			if len(recorded) != 1 {
				t.Fatalf("recorded errors=%d, want 1", len(recorded))
			}

			if cluster.Status.AdminOps == nil || cluster.Status.AdminOps.LastError == nil {
				t.Fatalf("expected LastError to be set")
			}
			if cluster.Status.AdminOps.LastError.Reason != "MappedError" {
				t.Fatalf("LastError.Reason=%q, want MappedError", cluster.Status.AdminOps.LastError.Reason)
			}

			if len(patchReasons) != 1 || patchReasons[0] != tt.wantPatchReason {
				t.Fatalf("patch reasons=%v, want [%s]", patchReasons, tt.wantPatchReason)
			}
		})
	}
}

func TestReconcile_AdminOpsRequeueAndSuccess(t *testing.T) {
	t.Run("subreconciler requeue is propagated", func(t *testing.T) {
		withAdminOpsReconcilers(t, fakeSubReconciler{result: recon.Result{RequeueAfter: 7 * time.Second}})

		var patchReasons []string
		cluster := &openbaov1alpha1.OpenBaoCluster{
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				AdminOps: &openbaov1alpha1.AdminOpsControllerStatus{LastError: &openbaov1alpha1.ControllerErrorStatus{Reason: "old"}},
			},
		}

		result, err := Reconcile(
			context.Background(),
			logr.Discard(),
			Dependencies{},
			cluster.DeepCopy(),
			cluster,
			nil,
			func(_ context.Context, _ client.Client, _ logr.Logger, _ *openbaov1alpha1.OpenBaoCluster, _ *openbaov1alpha1.OpenBaoCluster, reason string) error {
				patchReasons = append(patchReasons, reason)
				return nil
			},
			nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.RequeueAfter != 7*time.Second {
			t.Fatalf("RequeueAfter=%v, want 7s", result.RequeueAfter)
		}
		if len(patchReasons) != 1 || patchReasons[0] != "adminops-requeue" {
			t.Fatalf("patch reasons=%v, want [adminops-requeue]", patchReasons)
		}
		if cluster.Status.AdminOps.LastError == nil || cluster.Status.AdminOps.LastError.Reason != "old" {
			t.Fatalf("expected existing LastError to remain unchanged on requeue path")
		}
	})

	t.Run("success clears last error and patches complete", func(t *testing.T) {
		withAdminOpsReconcilers(t, fakeSubReconciler{})

		cluster := &openbaov1alpha1.OpenBaoCluster{
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				AdminOps: &openbaov1alpha1.AdminOpsControllerStatus{LastError: &openbaov1alpha1.ControllerErrorStatus{Reason: "old"}},
			},
		}
		var patchReasons []string

		result, err := Reconcile(
			context.Background(),
			logr.Discard(),
			Dependencies{},
			cluster.DeepCopy(),
			cluster,
			nil,
			func(_ context.Context, _ client.Client, _ logr.Logger, _ *openbaov1alpha1.OpenBaoCluster, _ *openbaov1alpha1.OpenBaoCluster, reason string) error {
				patchReasons = append(patchReasons, reason)
				return nil
			},
			nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != (recon.Result{}) {
			t.Fatalf("result=%v, want zero result", result)
		}
		if len(patchReasons) != 1 || patchReasons[0] != "adminops-complete" {
			t.Fatalf("patch reasons=%v, want [adminops-complete]", patchReasons)
		}
		if cluster.Status.AdminOps.LastError != nil {
			t.Fatalf("expected LastError to be cleared after success")
		}
	})

	t.Run("success recreates adminops status if a subreconciler refresh clears it", func(t *testing.T) {
		withAdminOpsReconcilers(t, fakeMutatingSubReconciler{
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Status.AdminOps = nil
			},
		})

		cluster := &openbaov1alpha1.OpenBaoCluster{
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				AdminOps: &openbaov1alpha1.AdminOpsControllerStatus{LastError: &openbaov1alpha1.ControllerErrorStatus{Reason: "old"}},
			},
		}
		var patchReasons []string

		result, err := Reconcile(
			context.Background(),
			logr.Discard(),
			Dependencies{},
			cluster.DeepCopy(),
			cluster,
			nil,
			func(_ context.Context, _ client.Client, _ logr.Logger, _ *openbaov1alpha1.OpenBaoCluster, _ *openbaov1alpha1.OpenBaoCluster, reason string) error {
				patchReasons = append(patchReasons, reason)
				return nil
			},
			nil,
		)
		if err != nil {
			t.Fatalf("unexpected error on refreshed success path: %v", err)
		}
		if result != (recon.Result{}) {
			t.Fatalf("result=%v, want zero result", result)
		}
		if cluster.Status.AdminOps == nil {
			t.Fatalf("expected AdminOps status to be recreated")
		}
		if cluster.Status.AdminOps.LastError != nil {
			t.Fatalf("expected recreated AdminOps status to have nil LastError")
		}
		if len(patchReasons) != 1 || patchReasons[0] != "adminops-complete" {
			t.Fatalf("patch reasons=%v, want [adminops-complete]", patchReasons)
		}
	})
}

func TestReconcile_InitializesAdminOpsStatus(t *testing.T) {
	withAdminOpsReconcilers(t, fakeSubReconciler{})

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	_, err := Reconcile(
		context.Background(),
		logr.Discard(),
		Dependencies{},
		cluster.DeepCopy(),
		cluster,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cluster.Status.AdminOps == nil {
		t.Fatalf("expected AdminOps status to be initialized")
	}
}

func assertRecorderInjected(t *testing.T, value any) {
	t.Helper()

	field := reflect.ValueOf(value).Elem().FieldByName("recorder")
	if !field.IsValid() {
		t.Fatal("recorder field not found")
	}
	if field.IsNil() {
		t.Fatal("recorder field is nil")
	}
}

func TestBuildReconcilers_InjectsRecorderIntoManagers(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := events.NewFakeRecorder(10)

	reconcilers := buildReconcilers(Dependencies{
		Client:    k8sClient,
		APIReader: k8sClient,
		Scheme:    scheme,
		Recorder:  recorder,
	})

	if len(reconcilers) != 3 {
		t.Fatalf("len(reconcilers) = %d, want 3", len(reconcilers))
	}

	blueGreenMgr, ok := reconcilers[0].(*bluegreen.Manager)
	if !ok {
		t.Fatalf("reconcilers[0] = %T, want *bluegreen.Manager", reconcilers[0])
	}
	assertRecorderInjected(t, blueGreenMgr)

	rollingMgr, ok := reconcilers[1].(*rollingupgrade.Manager)
	if !ok {
		t.Fatalf("reconcilers[1] = %T, want *rollingupgrade.Manager", reconcilers[1])
	}
	assertRecorderInjected(t, rollingMgr)

	backupMgr, ok := reconcilers[2].(*backupmanager.Manager)
	if !ok {
		t.Fatalf("reconcilers[2] = %T, want *backup.Manager", reconcilers[2])
	}
	assertRecorderInjected(t, backupMgr)
}
