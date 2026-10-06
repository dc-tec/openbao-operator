//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func deleteMinimalFixtureObject(ctx context.Context, c client.Client, obj client.Object) error {
	key := client.ObjectKeyFromObject(obj)
	expectedUID := obj.GetUID()
	if err := c.Get(ctx, key, obj); err != nil {
		return client.IgnoreNotFound(err)
	}

	uid := obj.GetUID()
	if expectedUID != "" && expectedUID != uid {
		return fmt.Errorf("fixture %T %s was replaced", obj, key)
	}

	if err := c.Delete(ctx, obj, client.Preconditions{UID: &uid}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if err := wait.PollUntilContextTimeout(ctx, time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		err := c.Get(ctx, key, obj)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if obj.GetUID() != uid {
			return false, fmt.Errorf("fixture %T %s was replaced during deletion", obj, key)
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("wait for fixture %T %s deletion: %w", obj, key, err)
	}

	return nil
}
