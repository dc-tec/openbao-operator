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
)

func ApplyOwned(ctx context.Context, c client.Client, scheme *runtime.Scheme, owner client.Object, obj client.Object) error {
	if err := PrepareOwned(obj, owner, scheme); err != nil {
		return err
	}
	applyConfig, err := kube.ToApplyConfiguration(obj, c)
	if err != nil {
		return fmt.Errorf("failed to convert object to ApplyConfiguration: %w", err)
	}
	return ApplyConfiguration(ctx, c, obj, applyConfig)
}

func ApplyUnowned(ctx context.Context, c client.Client, obj client.Object) error {
	applyConfig, err := kube.ToApplyConfiguration(obj, c)
	if err != nil {
		return fmt.Errorf("failed to convert object to ApplyConfiguration: %w", err)
	}
	return ApplyConfiguration(ctx, c, obj, applyConfig)
}

func PrepareOwned(obj client.Object, owner client.Object, scheme *runtime.Scheme) error {
	if scheme == nil {
		return fmt.Errorf("scheme is required")
	}
	if err := controllerutil.SetControllerReference(owner, obj, scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}
	return nil
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
