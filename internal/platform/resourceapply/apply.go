package resourceapply

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/dc-tec/openbao-operator/internal/adapter/kube"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceownership"
)

func ApplyOwned(ctx context.Context, c client.Client, scheme *runtime.Scheme, owner client.Object, obj client.Object) error {
	resolvedOwner, err := ResolveOwnerIdentity(ctx, c, owner)
	if err != nil {
		return err
	}
	if err := resourceownership.RequireOwnerUID(resolvedOwner); err != nil {
		return err
	}
	if err := EnsureOwnedResourceManageable(ctx, c, resolvedOwner, obj); err != nil {
		return err
	}
	if err := PrepareOwned(obj, resolvedOwner, scheme); err != nil {
		return err
	}
	applyConfig, err := kube.ToApplyConfiguration(obj, c)
	if err != nil {
		return fmt.Errorf("failed to convert object to ApplyConfiguration: %w", err)
	}
	if err := ApplyConfiguration(ctx, c, obj, applyConfig); err != nil {
		return err
	}
	return EnsureOwnedResourceProofStamped(ctx, c, scheme, resolvedOwner, obj)
}

func ResolveOwnerIdentity(ctx context.Context, c client.Client, owner client.Object) (client.Object, error) {
	if owner == nil || owner.GetUID() != "" || c == nil {
		return owner, nil
	}
	liveOwner, ok := owner.DeepCopyObject().(client.Object)
	if !ok {
		return owner, nil
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(owner), liveOwner); err != nil {
		if apierrors.IsNotFound(err) {
			return owner, nil
		}
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return nil, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to resolve owner identity %s/%s: %w", owner.GetNamespace(), owner.GetName(), err))
		}
		return nil, fmt.Errorf("failed to resolve owner identity %s/%s: %w", owner.GetNamespace(), owner.GetName(), err)
	}
	if liveOwner.GetUID() == "" {
		return owner, nil
	}
	return liveOwner, nil
}

func EnsureOwnedResourceManageable(ctx context.Context, c client.Client, owner client.Object, obj client.Object) error {
	if c == nil {
		return fmt.Errorf("client is required")
	}
	if owner == nil {
		return fmt.Errorf("owner is required")
	}
	if obj == nil {
		return fmt.Errorf("object is required")
	}
	existing, ok := obj.DeepCopyObject().(client.Object)
	if !ok {
		return fmt.Errorf("object %T does not implement client.Object", obj)
	}
	prepareObjectForGet(existing, obj)
	if err := c.Get(ctx, client.ObjectKeyFromObject(obj), existing); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to check existing resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		return fmt.Errorf("failed to check existing resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	if err := resourceownership.RequireOwnerProof("manage", existing, owner); err != nil {
		return err
	}
	return nil
}

func ApplyUnowned(ctx context.Context, c client.Client, obj client.Object) error {
	applyConfig, err := kube.ToApplyConfiguration(obj, c)
	if err != nil {
		return fmt.Errorf("failed to convert object to ApplyConfiguration: %w", err)
	}
	return ApplyConfiguration(ctx, c, obj, applyConfig)
}

func ApplyRetained(ctx context.Context, c client.Client, owner client.Object, obj client.Object) error {
	resolvedOwner, err := ResolveOwnerIdentity(ctx, c, owner)
	if err != nil {
		return err
	}
	if err := resourceownership.RequireOwnerUID(resolvedOwner); err != nil {
		return err
	}
	if err := resourceownership.SetOwnerUIDAnnotation(obj, resolvedOwner); err != nil {
		return err
	}
	if err := EnsureOwnedResourceManageable(ctx, c, resolvedOwner, obj); err != nil {
		return err
	}
	if err := ApplyUnowned(ctx, c, obj); err != nil {
		return err
	}
	return EnsureRetainedResourceProofStamped(ctx, c, resolvedOwner, obj)
}

func PrepareOwned(obj client.Object, owner client.Object, scheme *runtime.Scheme) error {
	if scheme == nil {
		return fmt.Errorf("scheme is required")
	}
	if err := resourceownership.RequireOwnerUID(owner); err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(owner, obj, scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}
	if err := resourceownership.SetOwnerUIDAnnotation(obj, owner); err != nil {
		return err
	}
	return nil
}

func EnsureOwnedResourceProofStamped(ctx context.Context, c client.Client, scheme *runtime.Scheme, owner client.Object, obj client.Object) error {
	existing, err := getExistingObject(ctx, c, obj)
	if err != nil {
		return err
	}
	if resourceownership.HasOwnerProof(existing, owner) {
		return nil
	}

	before, ok := existing.DeepCopyObject().(client.Object)
	if !ok {
		return fmt.Errorf("object %T does not implement client.Object", existing)
	}
	if err := PrepareOwned(existing, owner, scheme); err != nil {
		return err
	}
	if err := c.Patch(ctx, existing, client.MergeFrom(before)); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to stamp owner proof on resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		return fmt.Errorf("failed to stamp owner proof on resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}

func EnsureRetainedResourceProofStamped(ctx context.Context, c client.Client, owner client.Object, obj client.Object) error {
	existing, err := getExistingObject(ctx, c, obj)
	if err != nil {
		return err
	}
	if resourceownership.HasOwnerProof(existing, owner) {
		return nil
	}

	before, ok := existing.DeepCopyObject().(client.Object)
	if !ok {
		return fmt.Errorf("object %T does not implement client.Object", existing)
	}
	if err := resourceownership.SetOwnerUIDAnnotation(existing, owner); err != nil {
		return err
	}
	if err := c.Patch(ctx, existing, client.MergeFrom(before)); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to stamp retained owner proof on resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		return fmt.Errorf("failed to stamp retained owner proof on resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}

func getExistingObject(ctx context.Context, c client.Client, obj client.Object) (client.Object, error) {
	if c == nil {
		return nil, fmt.Errorf("client is required")
	}
	if obj == nil {
		return nil, fmt.Errorf("object is required")
	}
	existing, ok := obj.DeepCopyObject().(client.Object)
	if !ok {
		return nil, fmt.Errorf("object %T does not implement client.Object", obj)
	}
	prepareObjectForGet(existing, obj)
	if err := c.Get(ctx, client.ObjectKeyFromObject(obj), existing); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return nil, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to get resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		return nil, fmt.Errorf("failed to get resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return existing, nil
}

func prepareObjectForGet(existing client.Object, obj client.Object) {
	gvk := existing.GetObjectKind().GroupVersionKind()
	existing.SetName(obj.GetName())
	existing.SetNamespace(obj.GetNamespace())
	existing.SetLabels(nil)
	existing.SetAnnotations(nil)
	existing.SetOwnerReferences(nil)
	existing.SetManagedFields(nil)
	existing.SetUID("")
	existing.SetResourceVersion("")
	existing.GetObjectKind().SetGroupVersionKind(gvk)
}

func ApplyConfiguration(ctx context.Context, c client.Client, obj client.Object, applyConfig runtime.ApplyConfiguration) error {
	applyOpts := []client.ApplyOption{client.ForceOwnership, client.FieldOwner(constants.FieldOwnerOpenBaoOperator)}
	if err := c.Apply(ctx, applyConfig, applyOpts...); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}
