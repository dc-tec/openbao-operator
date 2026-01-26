package backup

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/kube"
)

func applyResource(ctx context.Context, c client.Client, scheme *runtime.Scheme, obj client.Object, cluster *openbaov1alpha1.OpenBaoCluster, fieldOwner string) error {
	if scheme == nil {
		return fmt.Errorf("scheme is required")
	}

	if err := controllerutil.SetControllerReference(cluster, obj, scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	applyConfig, err := kube.ToApplyConfiguration(obj, c)
	if err != nil {
		return fmt.Errorf("failed to convert object to ApplyConfiguration: %w", err)
	}

	applyOpts := []client.ApplyOption{
		client.ForceOwnership,
		client.FieldOwner(fieldOwner),
	}

	if err := c.Apply(ctx, applyConfig, applyOpts...); err != nil {
		return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}
