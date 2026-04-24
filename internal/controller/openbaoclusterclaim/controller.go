package openbaoclusterclaim

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaoclusterclaim "github.com/dc-tec/openbao-operator/internal/app/openbaoclusterclaim"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
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

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoClusterClaim{}).
		Watches(
			&openbaov1alpha1.OpenBaoTenant{},
			handler.EnqueueRequestsFromMapFunc(r.mapTenantToClaims),
		).
		Named(controllerNameOpenBaoClusterClaim)

	if r.EnableServiceClaims {
		builder = builder.Watches(
			&openbaov1alpha1.OpenBaoCluster{},
			handler.EnqueueRequestsFromMapFunc(r.mapLocalClusterToClaim),
		).Watches(
			&openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{},
			handler.EnqueueRequestsFromMapFunc(r.mapUpgradeRequestToClaim),
		).Watches(
			&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{},
			handler.EnqueueRequestsFromMapFunc(r.mapBackupRequestToClaim),
		).Watches(
			&openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{},
			handler.EnqueueRequestsFromMapFunc(r.mapRestoreRequestToClaim),
		).Watches(
			&openbaov1alpha1.OpenBaoRestore{},
			handler.EnqueueRequestsFromMapFunc(r.mapRestoreToClaim),
		)
	}

	return builder.Complete(r)
}

func (r *OpenBaoClusterClaimReconciler) syncMetrics(ctx context.Context, key client.ObjectKey) {
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	if reader == nil {
		return
	}

	claim := &openbaov1alpha1.OpenBaoClusterClaim{}
	if err := reader.Get(ctx, key, claim); err != nil {
		if apierrors.IsNotFound(err) {
			observability.ClearClaim(key.Namespace, key.Name)
		}
		return
	}

	observability.SyncClaim(claim)
}

func (r *OpenBaoClusterClaimReconciler) mapTenantToClaims(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	tenant, ok := obj.(*openbaov1alpha1.OpenBaoTenant)
	if !ok || tenant == nil {
		return nil
	}

	var claimList openbaov1alpha1.OpenBaoClusterClaimList
	if err := r.List(ctx, &claimList, client.InNamespace(tenant.Namespace)); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(claimList.Items))
	for i := range claimList.Items {
		claim := &claimList.Items[i]
		if claim.Spec.TenantRef.Name != tenant.Name {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(claim),
		})
	}

	return requests
}

func (r *OpenBaoClusterClaimReconciler) mapLocalClusterToClaim(
	_ context.Context,
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

	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: claimNamespace,
			Name:      claimName,
		},
	}}
}

func (r *OpenBaoClusterClaimReconciler) mapUpgradeRequestToClaim(
	_ context.Context,
	obj client.Object,
) []reconcile.Request {
	request, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest)
	if !ok || request == nil || request.Namespace == "" || request.Spec.ClaimRef.Name == "" {
		return nil
	}

	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: request.Namespace,
			Name:      request.Spec.ClaimRef.Name,
		},
	}}
}

func (r *OpenBaoClusterClaimReconciler) mapBackupRequestToClaim(
	_ context.Context,
	obj client.Object,
) []reconcile.Request {
	request, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaimBackupRequest)
	if !ok || request == nil || request.Namespace == "" || request.Spec.ClaimRef.Name == "" {
		return nil
	}

	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: request.Namespace,
			Name:      request.Spec.ClaimRef.Name,
		},
	}}
}

func (r *OpenBaoClusterClaimReconciler) mapRestoreRequestToClaim(
	_ context.Context,
	obj client.Object,
) []reconcile.Request {
	request, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaimRestoreRequest)
	if !ok || request == nil || request.Namespace == "" || request.Spec.ClaimRef.Name == "" {
		return nil
	}

	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: request.Namespace,
			Name:      request.Spec.ClaimRef.Name,
		},
	}}
}

func (r *OpenBaoClusterClaimReconciler) mapRestoreToClaim(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	restore, ok := obj.(*openbaov1alpha1.OpenBaoRestore)
	if !ok || restore == nil || restore.Namespace == "" || restore.Spec.Cluster == "" {
		return nil
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	key := client.ObjectKey{Namespace: restore.Namespace, Name: restore.Spec.Cluster}
	if err := r.Get(ctx, key, cluster); err != nil {
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

	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: claimNamespace,
			Name:      claimName,
		},
	}}
}
