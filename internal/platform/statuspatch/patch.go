package statuspatch

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchMerge applies a status merge patch using the original object as the patch base.
func PatchMerge(ctx context.Context, c client.Client, obj client.Object, original client.Object) error {
	if c == nil {
		return fmt.Errorf("client is required")
	}
	if obj == nil {
		return fmt.Errorf("object is required")
	}
	if original == nil {
		return fmt.Errorf("original object is required")
	}

	return c.Status().Patch(ctx, obj, client.MergeFrom(original))
}
