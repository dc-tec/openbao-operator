package backuprequest

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appbackuprequest "github.com/dc-tec/openbao-operator/internal/app/openbaoclusterclaim/backuprequest"
	"github.com/dc-tec/openbao-operator/internal/controller/openbaoclusterclaim/requestwatch"
	"github.com/dc-tec/openbao-operator/internal/platform/observability"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

const controllerNameOpenBaoClusterClaimBackupRequest = "openbaoclusterclaimbackuprequest"

type OpenBaoClusterClaimBackupRequestReconciler struct {
	client.Client
	Reader              client.Reader
	Scheme              *runtime.Scheme
	EnableServiceClaims bool
	AppReconciler       appbackuprequest.Reconciler
}

func (r *OpenBaoClusterClaimBackupRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.AppReconciler == nil {
		return ctrl.Result{}, fmt.Errorf("openbaoclusterclaimbackuprequest app reconciler is not configured")
	}
	result, err := r.AppReconciler.Reconcile(ctx, req.NamespacedName, log.FromContext(ctx).WithName(controllerNameOpenBaoClusterClaimBackupRequest))
	r.syncMetrics(ctx, req.NamespacedName)
	return ctrl.Result{RequeueAfter: result.RequeueAfter}, err
}

func (r *OpenBaoClusterClaimBackupRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
		r.AppReconciler = appbackuprequest.NewReconciler(appbackuprequest.Runtime{
			Client:              r.Client,
			Reader:              mgr.GetAPIReader(),
			EnableServiceClaims: r.EnableServiceClaims,
		})
	}

	mapper := r.requestMapper()
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{}).
		Named(controllerNameOpenBaoClusterClaimBackupRequest)
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

func (r *OpenBaoClusterClaimBackupRequestReconciler) syncMetrics(ctx context.Context, key client.ObjectKey) {
	requestwatch.SyncMetrics(
		ctx,
		key,
		r.Reader,
		r.Client,
		func() *openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
			return &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{}
		},
		observability.SyncClaimBackupRequest,
		observability.ClearClaimBackupRequest,
	)
}

func (r *OpenBaoClusterClaimBackupRequestReconciler) requestMapper() requestwatch.Mapper[
	*openbaov1alpha1.OpenBaoClusterClaimBackupRequest,
	*openbaov1alpha1.OpenBaoClusterClaimBackupRequestList,
] {
	return requestwatch.Mapper[
		*openbaov1alpha1.OpenBaoClusterClaimBackupRequest,
		*openbaov1alpha1.OpenBaoClusterClaimBackupRequestList,
	]{
		Reader: r.Client,
		NewList: func() *openbaov1alpha1.OpenBaoClusterClaimBackupRequestList {
			return &openbaov1alpha1.OpenBaoClusterClaimBackupRequestList{}
		},
		Items: func(list *openbaov1alpha1.OpenBaoClusterClaimBackupRequestList) []*openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
			return requestwatch.ObjectPointers(list.Items)
		},
		ClaimName: func(request *openbaov1alpha1.OpenBaoClusterClaimBackupRequest) string {
			return request.Spec.ClaimRef.Name
		},
	}
}
