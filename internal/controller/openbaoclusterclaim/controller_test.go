package openbaoclusterclaim

import (
	"context"
	"errors"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlreconcile "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapTenantToClaims(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newTestScheme(t)
	reconciler := &OpenBaoClusterClaimReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				&openbaov1alpha1.OpenBaoClusterClaim{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "payments",
						Name:      "payments-bao",
					},
					Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
						TenantRef: openbaov1alpha1.LocalReference{Name: "payments"},
					},
				},
				&openbaov1alpha1.OpenBaoClusterClaim{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "payments",
						Name:      "other-bao",
					},
					Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
						TenantRef: openbaov1alpha1.LocalReference{Name: "other"},
					},
				},
			).
			Build(),
		Scheme: scheme,
	}

	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      "payments",
		},
	}

	requests := reconciler.mapTenantToClaims(ctx, tenant)
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].NamespacedName != (client.ObjectKey{Namespace: "payments", Name: "payments-bao"}) {
		t.Fatalf("request key = %#v, want payments/payments-bao", requests[0].NamespacedName)
	}
}

func TestMapLocalClusterToClaim(t *testing.T) {
	t.Parallel()

	reconciler := &OpenBaoClusterClaimReconciler{}
	ctx := context.Background()

	t.Run("maps claim managed cluster by ownership labels", func(t *testing.T) {
		t.Parallel()

		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "openbao-payments",
				Name:      "payments-bao",
				Labels: map[string]string{
					constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
					constants.LabelOpenBaoClaimNamespace: "payments",
					constants.LabelOpenBaoClaimName:      "payments-bao",
				},
			},
		}

		requests := reconciler.mapLocalClusterToClaim(ctx, cluster)
		if len(requests) != 1 {
			t.Fatalf("request count = %d, want 1", len(requests))
		}
		if requests[0].NamespacedName != (client.ObjectKey{Namespace: "payments", Name: "payments-bao"}) {
			t.Fatalf("request key = %#v, want payments/payments-bao", requests[0].NamespacedName)
		}
	})

	t.Run("ignores directly managed cluster", func(t *testing.T) {
		t.Parallel()

		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					constants.LabelOpenBaoOwnershipMode: constants.LabelValueOpenBaoOwnershipDirectManaged,
				},
			},
		}

		requests := reconciler.mapLocalClusterToClaim(ctx, cluster)
		if len(requests) != 0 {
			t.Fatalf("request count = %d, want 0", len(requests))
		}
	})

	t.Run("ignores malformed ownership markers", func(t *testing.T) {
		t.Parallel()

		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					constants.LabelOpenBaoOwnershipMode: constants.LabelValueOpenBaoOwnershipClaimManaged,
				},
			},
		}

		requests := reconciler.mapLocalClusterToClaim(ctx, cluster)
		if len(requests) != 0 {
			t.Fatalf("request count = %d, want 0", len(requests))
		}
	})
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	return scheme
}

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

func TestMapWorkflowRequestToClaim(t *testing.T) {
	t.Parallel()

	reconciler := &OpenBaoClusterClaimReconciler{}
	ctx := context.Background()

	tests := []struct {
		name      string
		valid     client.Object
		malformed client.Object
		mapFn     func(context.Context, client.Object) []ctrlreconcile.Request
	}{
		{
			name: "upgrade request",
			valid: &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "payments",
					Name:      "payments-bao-upgrade-1",
				},
				Spec: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestSpec{
					ClaimRef: openbaov1alpha1.LocalReference{Name: "payments-bao"},
				},
			},
			malformed: &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "payments",
					Name:      "payments-bao-upgrade-1",
				},
			},
			mapFn: reconciler.mapUpgradeRequestToClaim,
		},
		{
			name: "backup request",
			valid: &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "payments",
					Name:      "payments-bao-backup-1",
				},
				Spec: openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{
					ClaimRef: openbaov1alpha1.LocalReference{Name: "payments-bao"},
				},
			},
			malformed: &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "payments",
					Name:      "payments-bao-backup-1",
				},
			},
			mapFn: reconciler.mapBackupRequestToClaim,
		},
		{
			name: "restore request",
			valid: &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "payments",
					Name:      "payments-bao-restore-1",
				},
				Spec: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSpec{
					ClaimRef: openbaov1alpha1.LocalReference{Name: "payments-bao"},
				},
			},
			malformed: &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "payments",
					Name:      "payments-bao-restore-1",
				},
			},
			mapFn: reconciler.mapRestoreRequestToClaim,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			requests := tt.mapFn(ctx, tt.valid)
			if len(requests) != 1 {
				t.Fatalf("request count = %d, want 1", len(requests))
			}
			if requests[0].NamespacedName != (client.ObjectKey{Namespace: "payments", Name: "payments-bao"}) {
				t.Fatalf("request key = %#v, want payments/payments-bao", requests[0].NamespacedName)
			}

			requests = tt.mapFn(ctx, tt.malformed)
			if len(requests) != 0 {
				t.Fatalf("request count = %d, want 0", len(requests))
			}
		})
	}
}

func TestMapRestoreToClaim(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newTestScheme(t)
	reconciler := &OpenBaoClusterClaimReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				&openbaov1alpha1.OpenBaoCluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "payments",
						Name:      "payments-bao-a1b2c3",
						Labels: map[string]string{
							constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
							constants.LabelOpenBaoClaimNamespace: "payments",
							constants.LabelOpenBaoClaimName:      "payments-bao",
						},
					},
				},
			).
			Build(),
		Scheme: scheme,
	}

	t.Run("maps restore through claim-managed cluster labels", func(t *testing.T) {
		t.Parallel()

		restore := &openbaov1alpha1.OpenBaoRestore{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "payments",
				Name:      "restore-1",
			},
			Spec: openbaov1alpha1.OpenBaoRestoreSpec{
				Cluster: "payments-bao-a1b2c3",
				Source: openbaov1alpha1.RestoreSource{
					Key: "snapshots/backup.snap",
					Target: openbaov1alpha1.BackupTarget{
						Endpoint: "https://backup.example.internal",
						Bucket:   "backups",
					},
				},
			},
		}

		requests := reconciler.mapRestoreToClaim(ctx, restore)
		if len(requests) != 1 {
			t.Fatalf("request count = %d, want 1", len(requests))
		}
		if requests[0].NamespacedName != (client.ObjectKey{Namespace: "payments", Name: "payments-bao"}) {
			t.Fatalf("request key = %#v, want payments/payments-bao", requests[0].NamespacedName)
		}
	})

	t.Run("ignores missing cluster", func(t *testing.T) {
		t.Parallel()

		restore := &openbaov1alpha1.OpenBaoRestore{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "payments",
				Name:      "restore-missing-cluster",
			},
			Spec: openbaov1alpha1.OpenBaoRestoreSpec{
				Cluster: "missing",
				Source: openbaov1alpha1.RestoreSource{
					Key: "snapshots/backup.snap",
					Target: openbaov1alpha1.BackupTarget{
						Endpoint: "https://backup.example.internal",
						Bucket:   "backups",
					},
				},
			},
		}

		requests := reconciler.mapRestoreToClaim(ctx, restore)
		if len(requests) != 0 {
			t.Fatalf("request count = %d, want 0", len(requests))
		}
	})
}
