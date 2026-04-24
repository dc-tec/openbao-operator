//go:build e2e
// +build e2e

/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func cleanupOpenBaoCustomResources(ctx context.Context) error {
	cfg, scheme, err := buildSuiteClientConfig()
	if err != nil {
		return err
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create cleanup client: %w", err)
	}

	if err := deleteAllOpenBaoCustomResources(ctx, c); err != nil {
		return err
	}

	if err := waitForOpenBaoCustomResourcesDeleted(ctx, c, 10*time.Second, 1*time.Second); err == nil {
		return nil
	}

	if err := removeFinalizersFromOpenBaoCustomResources(ctx, c); err != nil {
		return err
	}
	if err := deleteAllOpenBaoCustomResources(ctx, c); err != nil {
		return err
	}

	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		remainingTimeout := time.Until(deadline)
		if remainingTimeout > 10*time.Second {
			remainingTimeout = 10 * time.Second
		}
		if remainingTimeout > 0 {
			if err := waitForOpenBaoCustomResourcesDeleted(ctx, c, remainingTimeout, 1*time.Second); err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Some resources may still exist after cleanup: %v\n", err)
			}
		}
	}

	return nil
}

func buildSuiteClientConfig() (*rest.Config, *runtime.Scheme, error) {
	cfg, err := ctrlconfig.GetConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get kube config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, nil, fmt.Errorf("failed to add client-go scheme: %w", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		return nil, nil, fmt.Errorf("failed to add openbao scheme: %w", err)
	}

	return cfg, scheme, nil
}

func deleteAllOpenBaoCustomResources(ctx context.Context, c client.Client) error {
	var claims openbaov1alpha1.OpenBaoClusterClaimList
	if err := c.List(ctx, &claims); err != nil {
		return fmt.Errorf("failed to list OpenBaoClusterClaims: %w", err)
	}
	for i := range claims.Items {
		claim := claims.Items[i]
		if err := c.Delete(ctx, &claim); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete OpenBaoClusterClaim %s/%s: %w", claim.Namespace, claim.Name, err)
		}
	}

	var clusters openbaov1alpha1.OpenBaoClusterList
	if err := c.List(ctx, &clusters); err != nil {
		return fmt.Errorf("failed to list OpenBaoClusters: %w", err)
	}
	for i := range clusters.Items {
		cluster := clusters.Items[i]
		if isClaimManagedCluster(&cluster) {
			continue
		}
		if err := c.Delete(ctx, &cluster); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}
	}

	var tenants openbaov1alpha1.OpenBaoTenantList
	if err := c.List(ctx, &tenants); err != nil {
		return fmt.Errorf("failed to list OpenBaoTenants: %w", err)
	}
	for i := range tenants.Items {
		tenant := tenants.Items[i]
		if err := c.Delete(ctx, &tenant); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete OpenBaoTenant %s/%s: %w", tenant.Namespace, tenant.Name, err)
		}
	}

	var namespaces corev1.NamespaceList
	if err := c.List(ctx, &namespaces); err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}
	for i := range namespaces.Items {
		ns := namespaces.Items[i]
		if !strings.HasPrefix(ns.Name, "e2e-") {
			continue
		}
		if err := c.Delete(ctx, &ns); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete namespace %q: %w", ns.Name, err)
		}
	}

	return nil
}

func waitForOpenBaoCustomResourcesDeleted(ctx context.Context, c client.Client, timeout time.Duration, pollInterval time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if pollInterval <= 0 {
		return fmt.Errorf("poll interval must be positive")
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		var claims openbaov1alpha1.OpenBaoClusterClaimList
		if err := c.List(ctx, &claims); err != nil {
			return fmt.Errorf("failed to list OpenBaoClusterClaims: %w", err)
		}
		var clusters openbaov1alpha1.OpenBaoClusterList
		if err := c.List(ctx, &clusters); err != nil {
			return fmt.Errorf("failed to list OpenBaoClusters: %w", err)
		}
		var tenants openbaov1alpha1.OpenBaoTenantList
		if err := c.List(ctx, &tenants); err != nil {
			return fmt.Errorf("failed to list OpenBaoTenants: %w", err)
		}

		if len(claims.Items) == 0 && len(clusters.Items) == 0 && len(tenants.Items) == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled while waiting for OpenBao custom resources to be deleted: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf(
				"timed out waiting for OpenBao custom resources to be deleted (claims=%d clusters=%d tenants=%d)",
				len(claims.Items),
				len(clusters.Items),
				len(tenants.Items),
			)
		case <-ticker.C:
		}
	}
}

func removeFinalizersFromOpenBaoCustomResources(ctx context.Context, c client.Client) error {
	var claims openbaov1alpha1.OpenBaoClusterClaimList
	if err := c.List(ctx, &claims); err != nil {
		return fmt.Errorf("failed to list OpenBaoClusterClaims for finalizer removal: %w", err)
	}
	for i := range claims.Items {
		claim := claims.Items[i]
		if len(claim.Finalizers) == 0 {
			continue
		}
		original := claim.DeepCopy()
		claim.Finalizers = nil
		if err := c.Patch(ctx, &claim, client.MergeFrom(original)); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to remove finalizers from OpenBaoClusterClaim %s/%s: %w", claim.Namespace, claim.Name, err)
		}
	}

	var clusters openbaov1alpha1.OpenBaoClusterList
	if err := c.List(ctx, &clusters); err != nil {
		return fmt.Errorf("failed to list OpenBaoClusters for finalizer removal: %w", err)
	}
	for i := range clusters.Items {
		cluster := clusters.Items[i]
		if len(cluster.Finalizers) == 0 {
			continue
		}
		original := cluster.DeepCopy()
		cluster.Finalizers = nil
		if err := c.Patch(ctx, &cluster, client.MergeFrom(original)); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to remove finalizers from OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}
	}

	var tenants openbaov1alpha1.OpenBaoTenantList
	if err := c.List(ctx, &tenants); err != nil {
		return fmt.Errorf("failed to list OpenBaoTenants for finalizer removal: %w", err)
	}
	for i := range tenants.Items {
		tenant := tenants.Items[i]
		if len(tenant.Finalizers) == 0 {
			continue
		}
		original := tenant.DeepCopy()
		tenant.Finalizers = nil
		if err := c.Patch(ctx, &tenant, client.MergeFrom(original)); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to remove finalizers from OpenBaoTenant %s/%s: %w", tenant.Namespace, tenant.Name, err)
		}
	}

	return nil
}

func isClaimManagedCluster(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil {
		return false
	}
	return cluster.Labels[constants.LabelOpenBaoOwnershipMode] == constants.LabelValueOpenBaoOwnershipClaimManaged
}
