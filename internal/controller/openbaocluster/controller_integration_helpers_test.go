//go:build integration
// +build integration

package openbaocluster

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type testCompositeReconciler struct {
	parent *OpenBaoClusterReconciler
}

func (r *testCompositeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	statusReconciler := &openBaoClusterStatusReconciler{parent: r.parent}
	workloadReconciler := &openBaoClusterWorkloadReconciler{parent: r.parent}
	adminOpsReconciler := &openBaoClusterAdminOpsReconciler{parent: r.parent}

	if result, err := statusReconciler.Reconcile(ctx, req); err != nil {
		return result, err
	}
	if result, err := workloadReconciler.Reconcile(ctx, req); err != nil {
		return result, err
	}
	if result, err := adminOpsReconciler.Reconcile(ctx, req); err != nil {
		return result, err
	}
	return statusReconciler.Reconcile(ctx, req)
}
