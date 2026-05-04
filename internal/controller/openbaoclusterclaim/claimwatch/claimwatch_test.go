package claimwatch

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlreconcile "sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestTenantMapperFromTenant(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newTestScheme(t)
	mapper := TenantMapper{
		Reader: fake.NewClientBuilder().
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
	}

	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      "payments",
		},
	}

	requests := mapper.FromTenant()(ctx, tenant)
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].NamespacedName != (client.ObjectKey{Namespace: "payments", Name: "payments-bao"}) {
		t.Fatalf("request key = %#v, want payments/payments-bao", requests[0].NamespacedName)
	}
}

func TestFromManagedCluster(t *testing.T) {
	t.Parallel()

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

		requests := FromManagedCluster()(ctx, cluster)
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

		requests := FromManagedCluster()(ctx, cluster)
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

		requests := FromManagedCluster()(ctx, cluster)
		if len(requests) != 0 {
			t.Fatalf("request count = %d, want 0", len(requests))
		}
	})
}

func TestWorkflowRequestMappers(t *testing.T) {
	t.Parallel()

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
			mapFn: FromUpgradeRequest(),
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
			mapFn: FromBackupRequest(),
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
			mapFn: FromRestoreRequest(),
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

func TestRestoreMapperFromRestore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newTestScheme(t)
	mapper := RestoreMapper{
		Reader: fake.NewClientBuilder().
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

		requests := mapper.FromRestore()(ctx, restore)
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

		requests := mapper.FromRestore()(ctx, restore)
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
