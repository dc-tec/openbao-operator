package restorerequest

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	apprestorerequest "github.com/dc-tec/openbao-operator/internal/app/openbaoclusterclaim/restorerequest"
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

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{}).
		Named(controllerNameOpenBaoClusterClaimRestoreRequest)
	if r.EnableServiceClaims {
		builder = builder.Watches(
			&openbaov1alpha1.OpenBaoClusterClaim{},
			handler.EnqueueRequestsFromMapFunc(r.mapClaimToRestoreRequests),
		).Watches(
			&openbaov1alpha1.OpenBaoCluster{},
			handler.EnqueueRequestsFromMapFunc(r.mapClaimManagedClusterToRestoreRequests),
		).Watches(
			&openbaov1alpha1.OpenBaoRestore{},
			handler.EnqueueRequestsFromMapFunc(r.mapRestoreExecutionToRequest),
		)
	}
	return builder.Complete(r)
}

func (r *OpenBaoClusterClaimRestoreRequestReconciler) syncMetrics(ctx context.Context, key client.ObjectKey) {
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	if reader == nil {
		return
	}

	request := &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{}
	if err := reader.Get(ctx, key, request); err != nil {
		if apierrors.IsNotFound(err) {
			observability.ClearClaimRestoreRequest(key.Namespace, key.Name)
		}
		return
	}

	observability.SyncClaimRestoreRequest(request)
}

func (r *OpenBaoClusterClaimRestoreRequestReconciler) mapClaimToRestoreRequests(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	claim, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaim)
	if !ok || claim == nil {
		return nil
	}
	return r.listRestoreRequestsForClaim(ctx, claim.Namespace, claim.Name)
}

func (r *OpenBaoClusterClaimRestoreRequestReconciler) mapClaimManagedClusterToRestoreRequests(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	cluster, ok := obj.(*openbaov1alpha1.OpenBaoCluster)
	if !ok || cluster == nil {
		return nil
	}
	if cluster.Labels[constants.LabelOpenBaoOwnershipMode] != constants.LabelValueOpenBaoOwnershipClaimManaged {
		return nil
	}
	claimNamespace := cluster.Labels[constants.LabelOpenBaoClaimNamespace]
	claimName := cluster.Labels[constants.LabelOpenBaoClaimName]
	if claimNamespace == "" || claimName == "" {
		return nil
	}
	return r.listRestoreRequestsForClaim(ctx, claimNamespace, claimName)
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

func (r *OpenBaoClusterClaimRestoreRequestReconciler) listRestoreRequestsForClaim(
	ctx context.Context,
	namespace string,
	claimName string,
) []reconcile.Request {
	if r.Client == nil || namespace == "" || claimName == "" {
		return nil
	}

	var requestList openbaov1alpha1.OpenBaoClusterClaimRestoreRequestList
	if err := r.List(ctx, &requestList, client.InNamespace(namespace)); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(requestList.Items))
	for i := range requestList.Items {
		request := &requestList.Items[i]
		if request.Spec.ClaimRef.Name != claimName {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(request),
		})
	}
	return requests
}
