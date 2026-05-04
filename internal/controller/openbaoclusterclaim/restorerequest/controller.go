package restorerequest

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	apprestorerequest "github.com/dc-tec/openbao-operator/internal/app/openbaoclusterclaim/restorerequest"
	"github.com/dc-tec/openbao-operator/internal/controller/openbaoclusterclaim/requestwatch"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/observability"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const controllerNameOpenBaoClusterClaimRestoreRequest = "openbaoclusterclaimrestorerequest"

type OpenBaoClusterClaimRestoreRequestReconciler struct {
	client.Client
	Reader              client.Reader
	Scheme              *runtime.Scheme
	EnableServiceClaims bool
	AppReconciler       apprestorerequest.Reconciler
}

func (r *OpenBaoClusterClaimRestoreRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.AppReconciler == nil {
		return ctrl.Result{}, fmt.Errorf("openbaoclusterclaimrestorerequest app reconciler is not configured")
	}
	result, err := r.AppReconciler.Reconcile(ctx, req.NamespacedName, log.FromContext(ctx).WithName(controllerNameOpenBaoClusterClaimRestoreRequest))
	r.syncMetrics(ctx, req.NamespacedName)
	return ctrl.Result{RequeueAfter: result.RequeueAfter}, err
}

func (r *OpenBaoClusterClaimRestoreRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
		r.AppReconciler = apprestorerequest.NewReconciler(apprestorerequest.Runtime{
			Client:              r.Client,
			Reader:              mgr.GetAPIReader(),
			EnableServiceClaims: r.EnableServiceClaims,
		})
	}

	mapper := r.requestMapper()
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{}).
		Named(controllerNameOpenBaoClusterClaimRestoreRequest)
	if r.EnableServiceClaims {
		builder = builder.Watches(
			&openbaov1alpha1.OpenBaoClusterClaim{},
			handler.EnqueueRequestsFromMapFunc(mapper.FromClaim()),
		).Watches(
			&openbaov1alpha1.OpenBaoCluster{},
			handler.EnqueueRequestsFromMapFunc(mapper.FromClaimManagedCluster()),
		).Watches(
			&openbaov1alpha1.OpenBaoRestore{},
			handler.EnqueueRequestsFromMapFunc(r.mapRestoreExecutionToRequest),
		)
	}
	return builder.Complete(r)
}

func (r *OpenBaoClusterClaimRestoreRequestReconciler) syncMetrics(ctx context.Context, key client.ObjectKey) {
	requestwatch.SyncMetrics(
		ctx,
		key,
		r.Reader,
		r.Client,
		func() *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest {
			return &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{}
		},
		observability.SyncClaimRestoreRequest,
		observability.ClearClaimRestoreRequest,
	)
}

func (r *OpenBaoClusterClaimRestoreRequestReconciler) mapRestoreExecutionToRequest(
	_ context.Context,
	obj client.Object,
) []reconcile.Request {
	restore, ok := obj.(*openbaov1alpha1.OpenBaoRestore)
	if !ok || restore == nil {
		return nil
	}
	requestNamespace := restore.Labels[constants.LabelOpenBaoClaimNamespace]
	requestName := restore.Labels[constants.LabelOpenBaoClaimRestoreRequest]
	if requestNamespace == "" || requestName == "" {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{Namespace: requestNamespace, Name: requestName},
	}}
}

func (r *OpenBaoClusterClaimRestoreRequestReconciler) requestMapper() requestwatch.Mapper[
	*openbaov1alpha1.OpenBaoClusterClaimRestoreRequest,
	*openbaov1alpha1.OpenBaoClusterClaimRestoreRequestList,
] {
	return requestwatch.Mapper[
		*openbaov1alpha1.OpenBaoClusterClaimRestoreRequest,
		*openbaov1alpha1.OpenBaoClusterClaimRestoreRequestList,
	]{
		Reader: r.Client,
		NewList: func() *openbaov1alpha1.OpenBaoClusterClaimRestoreRequestList {
			return &openbaov1alpha1.OpenBaoClusterClaimRestoreRequestList{}
		},
		Items: func(list *openbaov1alpha1.OpenBaoClusterClaimRestoreRequestList) []*openbaov1alpha1.OpenBaoClusterClaimRestoreRequest {
			return requestwatch.ObjectPointers(list.Items)
		},
		ClaimName: func(request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest) string {
			return request.Spec.ClaimRef.Name
		},
	}
}
