package requestwatch

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
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
		if cluster.Labels[constants.LabelOpenBaoOwnershipMode] != constants.LabelValueOpenBaoOwnershipClaimManaged {
			return nil
		}
		claimNamespace := cluster.Labels[constants.LabelOpenBaoClaimNamespace]
		claimName := cluster.Labels[constants.LabelOpenBaoClaimName]
		return m.ForClaim(ctx, claimNamespace, claimName)
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

func SyncMetrics[T client.Object](
	ctx context.Context,
	key client.ObjectKey,
	reader client.Reader,
	fallback client.Reader,
	newObject func() T,
	sync func(T),
	clear func(namespace, name string),
) {
	if reader == nil {
		reader = fallback
	}
	if reader == nil || newObject == nil || sync == nil || clear == nil {
		return
	}

	obj := newObject()
	if err := reader.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			clear(key.Namespace, key.Name)
		}
		return
	}

	sync(obj)
}
