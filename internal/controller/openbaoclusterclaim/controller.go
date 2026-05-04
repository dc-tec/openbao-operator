package openbaoclusterclaim

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaoclusterclaim "github.com/dc-tec/openbao-operator/internal/app/openbaoclusterclaim"
	"github.com/dc-tec/openbao-operator/internal/controller/openbaoclusterclaim/claimwatch"
	"github.com/dc-tec/openbao-operator/internal/controller/openbaoclusterclaim/watchutil"
	"github.com/dc-tec/openbao-operator/internal/platform/observability"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

const controllerNameOpenBaoClusterClaim = "openbaoclusterclaim"

type OpenBaoClusterClaimReconciler struct {
	client.Client
	Reader                   client.Reader
	Scheme                   *runtime.Scheme
	EnableServiceClaims      bool
	SameClusterNetwork       appopenbaoclusterclaim.SameClusterNetworkConfig
	SameClusterTransitUnseal appopenbaoclusterclaim.SameClusterTransitUnsealConfig
	AppReconciler            appopenbaoclusterclaim.Reconciler
}

func (r *OpenBaoClusterClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.AppReconciler == nil {
		return ctrl.Result{}, fmt.Errorf("openbaoclusterclaim app reconciler is not configured")
	}

	result, err := r.AppReconciler.Reconcile(ctx, req.NamespacedName, log.FromContext(ctx).WithName(controllerNameOpenBaoClusterClaim))
	r.syncMetrics(ctx, req.NamespacedName)
	return ctrl.Result{RequeueAfter: result.RequeueAfter}, err
}

func (r *OpenBaoClusterClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}
	if r.Scheme == nil {
		r.Scheme = mgr.GetScheme()
	}
	if r.Reader == nil {
		r.Reader = mgr.GetAPIReader()
	}
	if r.AppReconciler == nil {
		r.AppReconciler = appopenbaoclusterclaim.NewReconciler(appopenbaoclusterclaim.Runtime{
			Client:                   r.Client,
			Reader:                   mgr.GetAPIReader(),
			Scheme:                   r.Scheme,
			EnableServiceClaims:      r.EnableServiceClaims,
			SameClusterNetwork:       r.SameClusterNetwork,
			SameClusterTransitUnseal: r.SameClusterTransitUnseal,
		})
	}

	tenantMapper := claimwatch.TenantMapper{Reader: r.Client}
	restoreMapper := claimwatch.RestoreMapper{Reader: r.Client}
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoClusterClaim{}).
		Watches(
			&openbaov1alpha1.OpenBaoTenant{},
			handler.EnqueueRequestsFromMapFunc(tenantMapper.FromTenant()),
		).
		Named(controllerNameOpenBaoClusterClaim)

	if r.EnableServiceClaims {
		builder = builder.Watches(
			&openbaov1alpha1.OpenBaoCluster{},
			handler.EnqueueRequestsFromMapFunc(claimwatch.FromManagedCluster()),
		).Watches(
			&openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{},
			handler.EnqueueRequestsFromMapFunc(claimwatch.FromUpgradeRequest()),
		).Watches(
			&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{},
			handler.EnqueueRequestsFromMapFunc(claimwatch.FromBackupRequest()),
		).Watches(
			&openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{},
			handler.EnqueueRequestsFromMapFunc(claimwatch.FromRestoreRequest()),
		).Watches(
			&openbaov1alpha1.OpenBaoRestore{},
			handler.EnqueueRequestsFromMapFunc(restoreMapper.FromRestore()),
		)
	}

	return builder.Complete(r)
}

func (r *OpenBaoClusterClaimReconciler) syncMetrics(ctx context.Context, key client.ObjectKey) {
	watchutil.SyncMetrics(
		ctx,
		key,
		r.Reader,
		r.Client,
		func() *openbaov1alpha1.OpenBaoClusterClaim {
			return &openbaov1alpha1.OpenBaoClusterClaim{}
		},
		observability.SyncClaim,
		observability.ClearClaim,
	)
}
