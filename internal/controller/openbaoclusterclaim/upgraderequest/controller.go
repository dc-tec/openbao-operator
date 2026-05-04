package upgraderequest

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appupgraderequest "github.com/dc-tec/openbao-operator/internal/app/openbaoclusterclaim/upgraderequest"
	"github.com/dc-tec/openbao-operator/internal/controller/openbaoclusterclaim/requestwatch"
	"github.com/dc-tec/openbao-operator/internal/controller/openbaoclusterclaim/watchutil"
	"github.com/dc-tec/openbao-operator/internal/platform/observability"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

const controllerNameOpenBaoClusterClaimUpgradeRequest = "openbaoclusterclaimupgraderequest"

type OpenBaoClusterClaimUpgradeRequestReconciler struct {
	client.Client
	Reader              client.Reader
	Scheme              *runtime.Scheme
	EnableServiceClaims bool
	AppReconciler       appupgraderequest.Reconciler
}

func (r *OpenBaoClusterClaimUpgradeRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.AppReconciler == nil {
		return ctrl.Result{}, fmt.Errorf("openbaoclusterclaimupgraderequest app reconciler is not configured")
	}
	result, err := r.AppReconciler.Reconcile(ctx, req.NamespacedName, log.FromContext(ctx).WithName(controllerNameOpenBaoClusterClaimUpgradeRequest))
	r.syncMetrics(ctx, req.NamespacedName)
	return ctrl.Result{RequeueAfter: result.RequeueAfter}, err
}

func (r *OpenBaoClusterClaimUpgradeRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
		r.AppReconciler = appupgraderequest.NewReconciler(appupgraderequest.Runtime{
			Client:              r.Client,
			Reader:              mgr.GetAPIReader(),
			EnableServiceClaims: r.EnableServiceClaims,
		})
	}
	mapper := r.requestMapper()
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{}).
		Named(controllerNameOpenBaoClusterClaimUpgradeRequest)
	if r.EnableServiceClaims {
		builder = builder.Watches(
			&openbaov1alpha1.OpenBaoClusterClaim{},
			handler.EnqueueRequestsFromMapFunc(mapper.FromClaim()),
		).Watches(
			&openbaov1alpha1.OpenBaoCluster{},
			handler.EnqueueRequestsFromMapFunc(mapper.FromClaimManagedCluster()),
		)
	}
	return builder.Complete(r)
}

func (r *OpenBaoClusterClaimUpgradeRequestReconciler) syncMetrics(ctx context.Context, key client.ObjectKey) {
	watchutil.SyncMetrics(
		ctx,
		key,
		r.Reader,
		r.Client,
		func() *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest {
			return &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{}
		},
		observability.SyncClaimUpgradeRequest,
		observability.ClearClaimUpgradeRequest,
	)
}

func (r *OpenBaoClusterClaimUpgradeRequestReconciler) requestMapper() requestwatch.Mapper[
	*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest,
	*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestList,
] {
	return requestwatch.Mapper[
		*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest,
		*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestList,
	]{
		Reader: r.Client,
		NewList: func() *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestList {
			return &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestList{}
		},
		Items: func(list *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestList) []*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest {
			return requestwatch.ObjectPointers(list.Items)
		},
		ClaimName: func(request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) string {
			return request.Spec.ClaimRef.Name
		},
	}
}
