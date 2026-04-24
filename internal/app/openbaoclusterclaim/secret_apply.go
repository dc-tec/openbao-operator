package openbaoclusterclaim

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/dc-tec/openbao-operator/internal/platform/resourceapply"
)

func applySecretWithFallback(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	secret *corev1.Secret,
) error {
	if secret == nil {
		return fmt.Errorf("secret is required")
	}

	desired := secret.DeepCopy()
	if owner != nil {
		if scheme == nil {
			return fmt.Errorf("scheme is required for owned Secret apply")
		}
		if err := controllerutil.SetControllerReference(owner, desired, scheme); err != nil {
			return fmt.Errorf("set controller reference on desired Secret %s/%s: %w", desired.Namespace, desired.Name, err)
		}
	}

	if err := applyDesiredSecret(ctx, c, scheme, owner, desired); err != nil {
		return err
	}
	return nil
}

func applyDesiredSecret(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	desired *corev1.Secret,
) error {
	var err error
	if owner != nil {
		if scheme == nil {
			return fmt.Errorf("scheme is required for owned Secret apply")
		}
		err = resourceapply.ApplyOwned(ctx, c, scheme, owner, desired)
	} else {
		err = resourceapply.ApplyUnowned(ctx, c, desired)
	}
	if err == nil {
		return nil
	}
	if !errors.Is(err, resourceapply.ErrApplySchemaMismatch) {
		return err
	}

	return upsertSecretWithoutSSA(ctx, c, desired)
}

func upsertSecretWithoutSSA(
	ctx context.Context,
	c client.Client,
	desired *corev1.Secret,
) error {
	current := &corev1.Secret{}
	key := client.ObjectKeyFromObject(desired)
	if err := c.Get(ctx, key, current); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return c.Create(ctx, desired.DeepCopy())
	}

	original := current.DeepCopy()
	current.Type = desired.Type
	current.Labels = copyStringMap(desired.Labels)
	current.Annotations = copyStringMap(desired.Annotations)
	current.Data = copySecretData(desired.Data)
	current.OwnerReferences = append([]metav1.OwnerReference(nil), desired.OwnerReferences...)
	if reflect.DeepEqual(original, current) {
		return nil
	}
	return c.Patch(ctx, current, client.MergeFrom(original))
}
