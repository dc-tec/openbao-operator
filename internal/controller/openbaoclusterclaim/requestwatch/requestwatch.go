package requestwatch

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/controller/openbaoclusterclaim/watchutil"
)

type Mapper[T client.Object, L client.ObjectList] struct {
	Reader    client.Reader
	NewList   func() L
	Items     func(L) []T
	ClaimName func(T) string
}

func (m Mapper[T, L]) FromClaim() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		claim, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaim)
		if !ok || claim == nil {
			return nil
		}
		return m.ForClaim(ctx, claim.Namespace, claim.Name)
	}
}

func (m Mapper[T, L]) FromClaimManagedCluster() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		cluster, ok := obj.(*openbaov1alpha1.OpenBaoCluster)
		if !ok || cluster == nil {
			return nil
		}
		requests := watchutil.RequestForClaimManagedLabels(cluster.Labels)
		if len(requests) != 1 {
			return nil
		}
		key := requests[0].NamespacedName
		return m.ForClaim(ctx, key.Namespace, key.Name)
	}
}

func (m Mapper[T, L]) ForClaim(ctx context.Context, namespace string, claimName string) []reconcile.Request {
	if m.Reader == nil || m.NewList == nil || m.Items == nil || m.ClaimName == nil ||
		namespace == "" || claimName == "" {
		return nil
	}

	list := m.NewList()
	if err := m.Reader.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil
	}

	items := m.Items(list)
	requests := make([]reconcile.Request, 0, len(items))
	for _, item := range items {
		if m.ClaimName(item) != claimName {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(item),
		})
	}
	return requests
}

func ObjectPointers[T any](items []T) []*T {
	pointers := make([]*T, 0, len(items))
	for i := range items {
		pointers = append(pointers, &items[i])
	}
	return pointers
}
