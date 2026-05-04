package watchutil

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func RequestFor(namespace string, name string) []reconcile.Request {
	if namespace == "" || name == "" {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{Namespace: namespace, Name: name},
	}}
}

func RequestForClaimManagedLabels(labels map[string]string) []reconcile.Request {
	if labels[constants.LabelOpenBaoOwnershipMode] != constants.LabelValueOpenBaoOwnershipClaimManaged {
		return nil
	}
	return RequestFor(labels[constants.LabelOpenBaoClaimNamespace], labels[constants.LabelOpenBaoClaimName])
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
