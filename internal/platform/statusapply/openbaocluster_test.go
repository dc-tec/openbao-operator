package statusapply

import (
	"context"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const statusSubresourceName = "status"

func TestApplyOpenBaoClusterAdminOpsStatus_PersistsFullAdminOpsPlane(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "adminops-plane",
			Namespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			UpgradeRequests: &openbaov1alpha1.UpgradeRequestStatus{
				LastHandledRetry: "retry-1",
			},
			Backup: &openbaov1alpha1.BackupStatus{
				LastFailureReason: "backup-failed",
			},
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase: openbaov1alpha1.PhaseSyncing,
			},
			BreakGlass: &openbaov1alpha1.BreakGlassStatus{
				Active: true,
				Nonce:  "nonce-1",
			},
			AdminOps: &openbaov1alpha1.AdminOpsControllerStatus{
				LastError: &openbaov1alpha1.ControllerErrorStatus{Reason: "Existing"},
			},
		},
	}

	scheme := newOpenBaoClusterStatusTestScheme(t)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(cluster).
		WithObjects(cluster.DeepCopy()).
		Build()

	cluster.Status.Upgrade = &openbaov1alpha1.UpgradeProgress{
		TargetVersion: "2.5.0",
		FromVersion:   "2.4.4",
	}

	if err := ApplyOpenBaoClusterAdminOpsStatus(context.Background(), k8sClient, cluster, OpenBaoClusterAdminOpsStatusApplyOptions{}); err != nil {
		t.Fatalf("ApplyOpenBaoClusterAdminOpsStatus() error = %v", err)
	}

	stored := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), stored); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if !reflect.DeepEqual(stored.Status.Upgrade, cluster.Status.Upgrade) {
		t.Fatalf("stored upgrade = %#v, want %#v", stored.Status.Upgrade, cluster.Status.Upgrade)
	}
	if !reflect.DeepEqual(stored.Status.UpgradeRequests, cluster.Status.UpgradeRequests) {
		t.Fatalf("stored upgradeRequests = %#v, want %#v", stored.Status.UpgradeRequests, cluster.Status.UpgradeRequests)
	}
	if !reflect.DeepEqual(stored.Status.Backup, cluster.Status.Backup) {
		t.Fatalf("stored backup = %#v, want %#v", stored.Status.Backup, cluster.Status.Backup)
	}
	if !reflect.DeepEqual(stored.Status.BlueGreen, cluster.Status.BlueGreen) {
		t.Fatalf("stored blueGreen = %#v, want %#v", stored.Status.BlueGreen, cluster.Status.BlueGreen)
	}
	if !reflect.DeepEqual(stored.Status.BreakGlass, cluster.Status.BreakGlass) {
		t.Fatalf("stored breakGlass = %#v, want %#v", stored.Status.BreakGlass, cluster.Status.BreakGlass)
	}
	if !reflect.DeepEqual(stored.Status.AdminOps, cluster.Status.AdminOps) {
		t.Fatalf("stored adminOps = %#v, want %#v", stored.Status.AdminOps, cluster.Status.AdminOps)
	}
}

func TestApplyOpenBaoClusterAdminOpsStatus_ApplyOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		force     bool
		wantForce bool
	}{
		{
			name:      "without force ownership",
			force:     false,
			wantForce: false,
		},
		{
			name:      "with force ownership",
			force:     true,
			wantForce: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "adminops-options",
					Namespace: "default",
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					AdminOps: &openbaov1alpha1.AdminOpsControllerStatus{},
				},
			}

			var capturedOptions client.SubResourceApplyOptions
			var subResourceName string

			k8sClient := fake.NewClientBuilder().
				WithScheme(newOpenBaoClusterStatusTestScheme(t)).
				WithStatusSubresource(cluster).
				WithObjects(cluster.DeepCopy()).
				WithInterceptorFuncs(interceptor.Funcs{
					SubResourceApply: func(ctx context.Context, c client.Client, subResource string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
						subResourceName = subResource
						capturedOptions = *(&client.SubResourceApplyOptions{}).ApplyOpts(opts)
						return c.Status().Apply(ctx, obj, opts...)
					},
				}).
				Build()

			err := ApplyOpenBaoClusterAdminOpsStatus(context.Background(), k8sClient, cluster, OpenBaoClusterAdminOpsStatusApplyOptions{
				ForceOwnership: tt.force,
			})
			if err != nil {
				t.Fatalf("ApplyOpenBaoClusterAdminOpsStatus() error = %v", err)
			}

			if subResourceName != statusSubresourceName {
				t.Fatalf("subResourceName = %q, want %s", subResourceName, statusSubresourceName)
			}
			if capturedOptions.FieldManager != constants.FieldOwnerAdminOpsStatus {
				t.Fatalf("FieldManager = %q, want %q", capturedOptions.FieldManager, constants.FieldOwnerAdminOpsStatus)
			}

			force := capturedOptions.Force != nil && *capturedOptions.Force
			if force != tt.wantForce {
				t.Fatalf("Force = %v, want %v", force, tt.wantForce)
			}
		})
	}
}

func newOpenBaoClusterStatusTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(client-go) error = %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(openbaov1alpha1) error = %v", err)
	}

	return scheme
}
