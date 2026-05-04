package claimwatch

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/controller/openbaoclusterclaim/watchutil"
)

type TenantMapper struct {
	Reader client.Reader
}

func (m TenantMapper) FromTenant() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		tenant, ok := obj.(*openbaov1alpha1.OpenBaoTenant)
		if !ok || tenant == nil || m.Reader == nil {
			return nil
		}

		var claimList openbaov1alpha1.OpenBaoClusterClaimList
		if err := m.Reader.List(ctx, &claimList, client.InNamespace(tenant.Namespace)); err != nil {
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
}

type RestoreMapper struct {
	Reader client.Reader
}

func (m RestoreMapper) FromRestore() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		restore, ok := obj.(*openbaov1alpha1.OpenBaoRestore)
		if !ok || restore == nil || restore.Namespace == "" || restore.Spec.Cluster == "" || m.Reader == nil {
			return nil
		}

		cluster := &openbaov1alpha1.OpenBaoCluster{}
		key := client.ObjectKey{Namespace: restore.Namespace, Name: restore.Spec.Cluster}
		if err := m.Reader.Get(ctx, key, cluster); err != nil {
			return nil
		}
		return watchutil.RequestForClaimManagedLabels(cluster.Labels)
	}
}

func FromManagedCluster() handler.MapFunc {
	return func(_ context.Context, obj client.Object) []reconcile.Request {
		cluster, ok := obj.(*openbaov1alpha1.OpenBaoCluster)
		if !ok || cluster == nil {
			return nil
		}
		return watchutil.RequestForClaimManagedLabels(cluster.Labels)
	}
}

func FromUpgradeRequest() handler.MapFunc {
	return func(_ context.Context, obj client.Object) []reconcile.Request {
		request, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest)
		if !ok || request == nil {
			return nil
		}
		return watchutil.RequestFor(request.Namespace, request.Spec.ClaimRef.Name)
	}
}

func FromBackupRequest() handler.MapFunc {
	return func(_ context.Context, obj client.Object) []reconcile.Request {
		request, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaimBackupRequest)
		if !ok || request == nil {
			return nil
		}
		return watchutil.RequestFor(request.Namespace, request.Spec.ClaimRef.Name)
	}
}

func FromRestoreRequest() handler.MapFunc {
	return func(_ context.Context, obj client.Object) []reconcile.Request {
		request, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaimRestoreRequest)
		if !ok || request == nil {
			return nil
		}
		return watchutil.RequestFor(request.Namespace, request.Spec.ClaimRef.Name)
	}
}
