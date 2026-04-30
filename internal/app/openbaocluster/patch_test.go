package openbaocluster

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestPatchStatusOwnedFields_PreservesPointerClearsInApplyPayload(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "status-owned",
			Namespace: "default",
		},
	}

	scheme := newPatchTestScheme(t)
	var capturedOptions client.SubResourceApplyOptions
	var subResourceName string
	var payload []byte

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceApply: func(_ context.Context, _ client.Client, subResource string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
				var err error
				payload, err = json.Marshal(obj)
				if err != nil {
					return err
				}
				subResourceName = subResource
				capturedOptions = *(&client.SubResourceApplyOptions{}).ApplyOpts(opts)
				return nil
			},
		}).
		Build()

	if err := PatchStatusOwnedFields(context.Background(), k8sClient, cluster); err != nil {
		t.Fatalf("PatchStatusOwnedFields() error = %v", err)
	}

	if subResourceName != "status" {
		t.Fatalf("subResourceName = %q, want status", subResourceName)
	}
	if capturedOptions.FieldManager != constants.FieldOwnerStatus {
		t.Fatalf("FieldManager = %q, want %q", capturedOptions.FieldManager, constants.FieldOwnerStatus)
	}
	gotPayload := string(payload)
	for _, want := range []string{`"readReplicas":null`, `"lastBackupTime":null`} {
		if !strings.Contains(gotPayload, want) {
			t.Fatalf("apply payload missing %s: %s", want, gotPayload)
		}
	}
}

func TestPatchAdminOpsOwnedFields_PatchesAdminOpsFieldsWithoutBackup(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "adminops-status",
			Namespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Backup: &openbaov1alpha1.BackupStatus{
				LastFailureReason: "existing-backup-state",
			},
		},
	}
	original := cluster.DeepCopy()
	desired := cluster.DeepCopy()
	desired.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{Phase: openbaov1alpha1.PhasePromoting}
	desired.Status.UpgradeRequests = &openbaov1alpha1.UpgradeRequestStatus{LastHandledPromote: "req-1"}
	desired.Status.Backup = &openbaov1alpha1.BackupStatus{LastFailureReason: "should-not-be-patched"}
	desired.Status.BreakGlass = &openbaov1alpha1.BreakGlassStatus{
		Active:  true,
		Reason:  openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed,
		Message: "manual recovery required",
	}
	desired.Status.AdminOps = &openbaov1alpha1.AdminOpsControllerStatus{
		LastError: &openbaov1alpha1.ControllerErrorStatus{Reason: "Test", Message: "boom"},
	}

	scheme := newPatchTestScheme(t)
	var capturedOptions client.SubResourceApplyOptions
	var applyCalls int
	var subResourceName string

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(cluster).
		WithObjects(cluster.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceApply: func(ctx context.Context, c client.Client, subResource string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
				applyCalls++
				subResourceName = subResource
				capturedOptions = *(&client.SubResourceApplyOptions{}).ApplyOpts(opts)
				return c.Status().Apply(ctx, obj, opts...)
			},
		}).
		Build()

	if err := PatchAdminOpsOwnedFields(context.Background(), k8sClient, logr.Discard(), original, desired, "test"); err != nil {
		t.Fatalf("PatchAdminOpsOwnedFields() error = %v", err)
	}

	if applyCalls < 1 {
		t.Fatalf("status apply calls = %d, want >=1", applyCalls)
	}
	if subResourceName != "status" {
		t.Fatalf("subResourceName = %q, want status", subResourceName)
	}
	if capturedOptions.FieldManager != constants.FieldOwnerAdminOpsStatus {
		t.Fatalf("FieldManager = %q, want %q", capturedOptions.FieldManager, constants.FieldOwnerAdminOpsStatus)
	}
	if applyCalls > 1 {
		if capturedOptions.Force == nil || !*capturedOptions.Force {
			t.Fatalf("Force = %v, want true on conflict-retry apply", capturedOptions.Force)
		}
	} else if capturedOptions.Force != nil && *capturedOptions.Force {
		t.Fatalf("Force = %v, want unset/false when no retry is needed", capturedOptions.Force)
	}

	stored := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), stored); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if !reflect.DeepEqual(stored.Status.BlueGreen, desired.Status.BlueGreen) {
		t.Fatalf("stored blueGreen = %#v, want %#v", stored.Status.BlueGreen, desired.Status.BlueGreen)
	}
	if !reflect.DeepEqual(stored.Status.UpgradeRequests, desired.Status.UpgradeRequests) {
		t.Fatalf("stored upgradeRequests = %#v, want %#v", stored.Status.UpgradeRequests, desired.Status.UpgradeRequests)
	}
	if !reflect.DeepEqual(stored.Status.BreakGlass, desired.Status.BreakGlass) {
		t.Fatalf("stored breakGlass = %#v, want %#v", stored.Status.BreakGlass, desired.Status.BreakGlass)
	}
	if !reflect.DeepEqual(stored.Status.AdminOps, desired.Status.AdminOps) {
		t.Fatalf("stored adminOps = %#v, want %#v", stored.Status.AdminOps, desired.Status.AdminOps)
	}
	if !reflect.DeepEqual(stored.Status.Backup, original.Status.Backup) {
		t.Fatalf("stored backup = %#v, want preserved original %#v", stored.Status.Backup, original.Status.Backup)
	}
}

func TestPatchAdminOpsOwnedFields_IgnoresBackupOnlyChanges(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "adminops-backup-only",
			Namespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Backup: &openbaov1alpha1.BackupStatus{
				LastFailureReason: "existing-backup-state",
			},
		},
	}
	original := cluster.DeepCopy()
	desired := cluster.DeepCopy()
	desired.Status.Backup = &openbaov1alpha1.BackupStatus{LastFailureReason: "new-backup-state"}

	scheme := newPatchTestScheme(t)
	var applyCalls int

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(cluster).
		WithObjects(cluster.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceApply: func(ctx context.Context, c client.Client, subResource string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
				applyCalls++
				return c.Status().Apply(ctx, obj, opts...)
			},
		}).
		Build()

	if err := PatchAdminOpsOwnedFields(context.Background(), k8sClient, logr.Discard(), original, desired, "backup-only"); err != nil {
		t.Fatalf("PatchAdminOpsOwnedFields() error = %v", err)
	}

	if applyCalls != 0 {
		t.Fatalf("status apply calls = %d, want 0 for backup-only change", applyCalls)
	}

	stored := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), stored); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !reflect.DeepEqual(stored.Status.Backup, original.Status.Backup) {
		t.Fatalf("stored backup = %#v, want original %#v", stored.Status.Backup, original.Status.Backup)
	}
}

func TestPatchWorkloadOwnedFields_IgnoresDeletedCluster(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workload-deleted",
			Namespace: "default",
		},
	}
	original := cluster.DeepCopy()
	desired := cluster.DeepCopy()
	desired.Status.Workload = &openbaov1alpha1.WorkloadControllerStatus{
		LastError: &openbaov1alpha1.ControllerErrorStatus{
			Reason:  "Test",
			Message: "status update raced with deletion",
		},
	}

	scheme := newPatchTestScheme(t)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(cluster).
		WithObjects(cluster.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceApply: func(context.Context, client.Client, string, runtime.ApplyConfiguration, ...client.SubResourceApplyOption) error {
				return apierrors.NewNotFound(schema.GroupResource{Group: openbaov1alpha1.GroupVersion.Group, Resource: "openbaoclusters"}, cluster.Name)
			},
		}).
		Build()

	if err := PatchWorkloadOwnedFields(context.Background(), k8sClient, logr.Discard(), original, desired, "workload-deleted"); err != nil {
		t.Fatalf("PatchWorkloadOwnedFields() error = %v", err)
	}
}

func newPatchTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	return scheme
}
