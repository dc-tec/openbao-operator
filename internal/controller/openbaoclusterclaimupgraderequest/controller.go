package openbaoclusterclaimupgraderequest

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaoclusterclaimupgraderequest "github.com/dc-tec/openbao-operator/internal/app/openbaoclusterclaimupgraderequest"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
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
	AppReconciler       appopenbaoclusterclaimupgraderequest.Reconciler
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
		r.AppReconciler = appopenbaoclusterclaimupgraderequest.NewReconciler(appopenbaoclusterclaimupgraderequest.Runtime{
			Client:              r.Client,
			Reader:              mgr.GetAPIReader(),
			EnableServiceClaims: r.EnableServiceClaims,
		})
	}
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{}).
		Named(controllerNameOpenBaoClusterClaimUpgradeRequest)
	if r.EnableServiceClaims {
		builder = builder.Watches(
			&openbaov1alpha1.OpenBaoClusterClaim{},
			handler.EnqueueRequestsFromMapFunc(r.mapClaimToUpgradeRequests),
		).Watches(
			&openbaov1alpha1.OpenBaoCluster{},
			handler.EnqueueRequestsFromMapFunc(r.mapClaimManagedClusterToUpgradeRequests),
		)
	}
	return builder.Complete(r)
}

func (r *OpenBaoClusterClaimUpgradeRequestReconciler) syncMetrics(ctx context.Context, key client.ObjectKey) {
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	if reader == nil {
		return
	}

	request := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{}
	if err := reader.Get(ctx, key, request); err != nil {
		if apierrors.IsNotFound(err) {
			observability.ClearClaimUpgradeRequest(key.Namespace, key.Name)
		}
		return
	}

	observability.SyncClaimUpgradeRequest(request)
}

func (r *OpenBaoClusterClaimUpgradeRequestReconciler) mapClaimToUpgradeRequests(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	claim, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaim)
	if !ok || claim == nil {
		return nil
	}
	return r.listUpgradeRequestsForClaim(ctx, claim.Namespace, claim.Name)
}

func (r *OpenBaoClusterClaimUpgradeRequestReconciler) mapClaimManagedClusterToUpgradeRequests(
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
	return r.listUpgradeRequestsForClaim(ctx, claimNamespace, claimName)
}

func (r *OpenBaoClusterClaimUpgradeRequestReconciler) listUpgradeRequestsForClaim(
	ctx context.Context,
	namespace string,
	claimName string,
) []reconcile.Request {
	if r.Client == nil || namespace == "" || claimName == "" {
		return nil
	}

	var requestList openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestList
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
