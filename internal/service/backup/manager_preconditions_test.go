package backup

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/testutil/robustness"
)

func basePreconditionsCluster() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-a",
			Namespace: "tenant-a",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.4.4",
			Backup: &openbaov1alpha1.BackupSchedule{
				JWTAuthRole: testBackupJWTAuthRole,
				Target: openbaov1alpha1.BackupTarget{
					Provider: "s3",
					Bucket:   "bucket-a",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			Phase:          openbaov1alpha1.ClusterPhaseRunning,
			CurrentVersion: "2.4.4",
		},
	}
}

func backupJobForCluster(cluster *openbaov1alpha1.OpenBaoCluster, name string, preUpgrade bool) *batchv1.Job {
	labels := map[string]string{
		constants.LabelAppInstance:      cluster.Name,
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:   cluster.Name,
		constants.LabelOpenBaoComponent: ComponentBackup,
	}
	if preUpgrade {
		labels[constants.LabelOpenBaoBackupType] = "pre-upgrade"
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         cluster.Namespace,
			Labels:            labels,
			CreationTimestamp: metav1.NewTime(time.Now().UTC()),
		},
	}
}

func newManagerWithClient(c client.Client) *Manager {
	return &Manager{
		client: c,
		scheme: testScheme,
	}
}

func TestCheckPreconditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object
		wantErr   string
		exactErr  bool
		expectNil bool
	}{
		{
			name: "fails when cluster is not initialized",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Status.Initialized = false
				return nil
			},
			wantErr: "cluster is not initialized",
		},
		{
			name: "fails when cluster is initializing",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
				return nil
			},
			wantErr: "cluster is initializing",
		},
		{
			name: "fails when upgrade is pending with pre-upgrade snapshot",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Spec.Version = testPendingUpgradeVersion
				cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{PreUpgradeSnapshot: true}
				return nil
			},
			wantErr: "upgrade pending with pre-upgrade snapshot enabled",
		},
		{
			name: "fails when upgrade is pending",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Spec.Version = testPendingUpgradeVersion
				cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{PreUpgradeSnapshot: false}
				return nil
			},
			wantErr:  "upgrade pending",
			exactErr: true,
		},
		{
			name: "fails when upgrade is pending and upgrade config is nil",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Spec.Version = testPendingUpgradeVersion
				cluster.Spec.Upgrade = nil
				return nil
			},
			wantErr:  "upgrade pending",
			exactErr: true,
		},
		{
			name: "fails when downgrade is pending",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Spec.Version = "2.4.3"
				cluster.Spec.Upgrade = nil
				return nil
			},
			wantErr:  "upgrade pending",
			exactErr: true,
		},
		{
			name: "fails when upgrade already in progress",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Status.Upgrade = &openbaov1alpha1.UpgradeProgress{}
				return nil
			},
			wantErr: "upgrade in progress",
		},
		{
			name: "fails when pre-upgrade backup job is active",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				return []client.Object{backupJobForCluster(cluster, "pre-upgrade-job", true)}
			},
			wantErr: "pre-upgrade backup in progress",
		},
		{
			name: "passes when regular backup job is active",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				return []client.Object{backupJobForCluster(cluster, "backup-job", false)}
			},
			expectNil: true,
		},
		{
			name: "fails when backup auth is not configured",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Spec.Backup.JWTAuthRole = ""
				cluster.Spec.Backup.TokenSecretRef = nil
				cluster.Spec.SelfInit = nil
				return nil
			},
			wantErr: "configure jwtAuthRole or tokenSecretRef",
		},
		{
			name: "fails when token secret name is empty and jwt auth is not configured",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Spec.Backup.JWTAuthRole = ""
				cluster.Spec.Backup.TokenSecretRef = &corev1.LocalObjectReference{Name: ""}
				cluster.Spec.SelfInit = nil
				return nil
			},
			wantErr: "configure jwtAuthRole or tokenSecretRef",
		},
		{
			name: "fails when token secret is missing",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Spec.Backup.JWTAuthRole = ""
				cluster.Spec.Backup.TokenSecretRef = &corev1.LocalObjectReference{Name: "missing-token"}
				return nil
			},
			wantErr: "not found",
		},
		{
			name: "passes when token secret exists",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Spec.Backup.JWTAuthRole = ""
				cluster.Spec.Backup.TokenSecretRef = &corev1.LocalObjectReference{Name: "backup-token"}
				return []client.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "backup-token",
							Namespace: cluster.Namespace,
						},
					},
				}
			},
			expectNil: true,
		},
		{
			name: "passes when jwt auth role is configured",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Spec.Backup.JWTAuthRole = testBackupJWTAuthRole
				cluster.Spec.Backup.TokenSecretRef = nil
				return nil
			},
			expectNil: true,
		},
		{
			name: "passes when oidc self-init is enabled without explicit jwt role",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Spec.Backup.JWTAuthRole = ""
				cluster.Spec.Backup.TokenSecretRef = nil
				cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
					Enabled: true,
					OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
						Enabled: true,
					},
				}
				return nil
			},
			expectNil: true,
		},
		{
			name: "passes when current version is empty and desired version differs",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Status.CurrentVersion = ""
				cluster.Spec.Version = testPendingUpgradeVersion
				return nil
			},
			expectNil: true,
		},
		{
			name: "passes when cluster phase is failed",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Status.Phase = openbaov1alpha1.ClusterPhaseFailed
				return nil
			},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cluster := basePreconditionsCluster()
			objs := tt.mutate(cluster)

			fakeClient := fake.NewClientBuilder().
				WithScheme(testScheme).
				WithObjects(objs...).
				Build()

			mgr := newManagerWithClient(fakeClient)
			err := mgr.checkPreconditions(context.Background(), logr.Discard(), cluster)

			if tt.expectNil {
				if err != nil {
					t.Fatalf("checkPreconditions() unexpected error: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("checkPreconditions() error = nil, want substring %q", tt.wantErr)
			}
			if tt.exactErr {
				if err.Error() != tt.wantErr {
					t.Fatalf("checkPreconditions() error = %q, want exact %q", err.Error(), tt.wantErr)
				}
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("checkPreconditions() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestHasInProgressRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		buildClient  func(cluster *openbaov1alpha1.OpenBaoCluster) client.Client
		wantInFlight bool
		wantErr      string
	}{
		{
			name: "returns false when no restores exist",
			buildClient: func(_ *openbaov1alpha1.OpenBaoCluster) client.Client {
				return fake.NewClientBuilder().WithScheme(testScheme).Build()
			},
			wantInFlight: false,
		},
		{
			name: "returns true when restore is running for this cluster",
			buildClient: func(cluster *openbaov1alpha1.OpenBaoCluster) client.Client {
				restore := &openbaov1alpha1.OpenBaoRestore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "restore-a",
						Namespace: cluster.Namespace,
					},
					Spec: openbaov1alpha1.OpenBaoRestoreSpec{
						Cluster: cluster.Name,
						Source: openbaov1alpha1.RestoreSource{
							Target: openbaov1alpha1.BackupTarget{Bucket: "bucket-a"},
							Key:    "snap.snap",
						},
					},
					Status: openbaov1alpha1.OpenBaoRestoreStatus{Phase: openbaov1alpha1.RestorePhaseRunning},
				}
				return fake.NewClientBuilder().WithScheme(testScheme).WithObjects(restore).Build()
			},
			wantInFlight: true,
		},
		{
			name: "ignores running restore for different cluster",
			buildClient: func(cluster *openbaov1alpha1.OpenBaoCluster) client.Client {
				restore := &openbaov1alpha1.OpenBaoRestore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "restore-other-cluster",
						Namespace: cluster.Namespace,
					},
					Spec: openbaov1alpha1.OpenBaoRestoreSpec{
						Cluster: "other-cluster",
						Source: openbaov1alpha1.RestoreSource{
							Target: openbaov1alpha1.BackupTarget{Bucket: "bucket-a"},
							Key:    "snap.snap",
						},
					},
					Status: openbaov1alpha1.OpenBaoRestoreStatus{Phase: openbaov1alpha1.RestorePhaseRunning},
				}
				return fake.NewClientBuilder().WithScheme(testScheme).WithObjects(restore).Build()
			},
			wantInFlight: false,
		},
		{
			name: "ignores completed restore for this cluster",
			buildClient: func(cluster *openbaov1alpha1.OpenBaoCluster) client.Client {
				restore := &openbaov1alpha1.OpenBaoRestore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "restore-completed",
						Namespace: cluster.Namespace,
					},
					Spec: openbaov1alpha1.OpenBaoRestoreSpec{
						Cluster: cluster.Name,
						Source: openbaov1alpha1.RestoreSource{
							Target: openbaov1alpha1.BackupTarget{Bucket: "bucket-a"},
							Key:    "snap.snap",
						},
					},
					Status: openbaov1alpha1.OpenBaoRestoreStatus{Phase: openbaov1alpha1.RestorePhaseCompleted},
				}
				return fake.NewClientBuilder().WithScheme(testScheme).WithObjects(restore).Build()
			},
			wantInFlight: false,
		},
		{
			name: "ignores failed restore for this cluster",
			buildClient: func(cluster *openbaov1alpha1.OpenBaoCluster) client.Client {
				restore := &openbaov1alpha1.OpenBaoRestore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "restore-failed",
						Namespace: cluster.Namespace,
					},
					Spec: openbaov1alpha1.OpenBaoRestoreSpec{
						Cluster: cluster.Name,
						Source: openbaov1alpha1.RestoreSource{
							Target: openbaov1alpha1.BackupTarget{Bucket: "bucket-a"},
							Key:    "snap.snap",
						},
					},
					Status: openbaov1alpha1.OpenBaoRestoreStatus{Phase: openbaov1alpha1.RestorePhaseFailed},
				}
				return fake.NewClientBuilder().WithScheme(testScheme).WithObjects(restore).Build()
			},
			wantInFlight: false,
		},
		{
			name: "treats forbidden list as no restore in progress",
			buildClient: func(_ *openbaov1alpha1.OpenBaoCluster) client.Client {
				injector := robustness.NewInjector(map[robustness.Operation]robustness.Rule{
					robustness.OpList: robustness.Always(
						apierrors.NewForbidden(
							schema.GroupResource{Group: openbaov1alpha1.GroupVersion.Group, Resource: "openbaorestores"},
							"",
							fmt.Errorf("forbidden"),
						),
					),
				})
				return fake.NewClientBuilder().
					WithScheme(testScheme).
					WithInterceptorFuncs(injector.InterceptorFuncs()).
					Build()
			},
			wantInFlight: false,
		},
		{
			name: "returns error when restore list fails unexpectedly",
			buildClient: func(_ *openbaov1alpha1.OpenBaoCluster) client.Client {
				injector := robustness.NewInjector(map[robustness.Operation]robustness.Rule{
					robustness.OpList: robustness.Always(fmt.Errorf("boom")),
				})
				return fake.NewClientBuilder().
					WithScheme(testScheme).
					WithInterceptorFuncs(injector.InterceptorFuncs()).
					Build()
			},
			wantErr: "failed to list OpenBaoRestore resources",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cluster := basePreconditionsCluster()
			mgr := newManagerWithClient(tt.buildClient(cluster))

			got, err := mgr.hasInProgressRestore(context.Background(), logr.Discard(), cluster)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("hasInProgressRestore() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("hasInProgressRestore() error = nil, want substring %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("hasInProgressRestore() error = %q, want substring %q", err.Error(), tt.wantErr)
				}
			}

			if got != tt.wantInFlight {
				t.Fatalf("hasInProgressRestore() = %v, want %v", got, tt.wantInFlight)
			}
		})
	}
}

func TestCheckPreconditions_Idempotent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object
		wantErr string
	}{
		{
			name: "pending upgrade stays stable across retries",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Spec.Version = testPendingUpgradeVersion
				cluster.Spec.Upgrade = nil
				return nil
			},
			wantErr: "upgrade pending",
		},
		{
			name: "ready cluster stays successful across retries",
			mutate: func(cluster *openbaov1alpha1.OpenBaoCluster) []client.Object {
				cluster.Spec.Backup.JWTAuthRole = ""
				cluster.Spec.Backup.TokenSecretRef = &corev1.LocalObjectReference{Name: "backup-token"}
				return []client.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "backup-token",
							Namespace: cluster.Namespace,
						},
					},
				}
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cluster := basePreconditionsCluster()
			objs := tt.mutate(cluster)
			mgr := newManagerWithClient(fake.NewClientBuilder().WithScheme(testScheme).WithObjects(objs...).Build())

			firstErr := mgr.checkPreconditions(context.Background(), logr.Discard(), cluster)
			secondErr := mgr.checkPreconditions(context.Background(), logr.Discard(), cluster)

			if tt.wantErr == "" {
				if firstErr != nil || secondErr != nil {
					t.Fatalf("expected both calls to succeed, got first=%v second=%v", firstErr, secondErr)
				}
				return
			}

			if firstErr == nil || secondErr == nil {
				t.Fatalf("expected both calls to fail, got first=%v second=%v", firstErr, secondErr)
			}
			if firstErr.Error() != secondErr.Error() {
				t.Fatalf("expected stable error across retries, got first=%q second=%q", firstErr.Error(), secondErr.Error())
			}
			if !strings.Contains(firstErr.Error(), tt.wantErr) {
				t.Fatalf("error=%q, want contains %q", firstErr.Error(), tt.wantErr)
			}
		})
	}
}
