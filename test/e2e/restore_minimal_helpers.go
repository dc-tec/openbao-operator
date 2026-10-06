//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
)

// Keep both namespaces enrolled until requests and clusters finish deletion.
// Never acknowledge an uncertain restore or remove a finalizer during teardown.
func cleanupMinimalFixtures(ctx context.Context, c client.Client, fixtures ...*framework.Framework) error {
	for _, f := range fixtures {
		if f == nil {
			continue
		}
		requests := &api.OpenBaoRestoreList{}
		if err := c.List(ctx, requests, client.InNamespace(f.Namespace)); err != nil {
			return err
		}
		for i := range requests.Items {
			if err := deleteMinimalFixtureObject(ctx, c, &requests.Items[i]); err != nil {
				return err
			}
		}
	}

	for _, f := range fixtures {
		if f == nil {
			continue
		}
		clusters := &api.OpenBaoClusterList{}
		if err := c.List(ctx, clusters, client.InNamespace(f.Namespace)); err != nil {
			return err
		}
		for i := range clusters.Items {
			if err := deleteMinimalFixtureObject(ctx, c, &clusters.Items[i]); err != nil {
				return err
			}
		}
	}

	for _, f := range fixtures {
		if f == nil {
			continue
		}
		tenant := &api.OpenBaoTenant{ObjectMeta: metav1.ObjectMeta{Name: f.TenantName, Namespace: f.OperatorNamespace}}
		if err := deleteMinimalFixtureObject(ctx, c, tenant); err != nil {
			return err
		}
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: f.Namespace}}
		if err := deleteMinimalFixtureObject(ctx, c, namespace); err != nil {
			return err
		}
	}

	return nil
}

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
