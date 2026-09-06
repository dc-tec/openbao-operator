package provisioner

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	controllermetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appprovisioner "github.com/dc-tec/openbao-operator/internal/app/provisioner"
	"github.com/dc-tec/openbao-operator/internal/platform/observability"
)

func TestProvisionerReconcileMetrics_Lifecycle(t *testing.T) {
	for _, tc := range []struct {
		name                                                                                string
		absent, deleting, alreadyFinalized, clusterRemaining, failPatch, childAbsent, clear bool
	}{
		{name: "owner absent", absent: true, clear: true},
		{name: "target namespace absent"},
		{name: "finalization completed", deleting: true, clear: true},
		{name: "already finalized", deleting: true, alreadyFinalized: true, clear: true},
		{name: "waiting for cluster deletion", deleting: true, clusterRemaining: true},
		{name: "finalizer patch failed", deleting: true, failPatch: true},
		{name: "child cleanup not found", deleting: true, childAbsent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := client.ObjectKey{Namespace: "metrics", Name: t.Name()}
			const targetNamespace = "target"
			ownerReads := 0
			builder := fake.NewClientBuilder().WithScheme(testScheme).
				WithStatusSubresource(&openbaov1alpha1.OpenBaoTenant{}).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*openbaov1alpha1.OpenBaoTenant); ok {
							ownerReads++
						}
						return c.Get(ctx, key, obj, opts...)
					},
					Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
						if tc.failPatch {
							return fmt.Errorf("finalizer patch failed")
						}
						return c.Patch(ctx, obj, patch, opts...)
					},
				})
			if !tc.absent {
				tenant := &openbaov1alpha1.OpenBaoTenant{ObjectMeta: metav1.ObjectMeta{
					Namespace: key.Namespace, Name: key.Name, Finalizers: []string{"test.example/retained"},
				}, Spec: openbaov1alpha1.OpenBaoTenantSpec{TargetNamespace: targetNamespace}}
				if !tc.alreadyFinalized {
					tenant.Finalizers = append(tenant.Finalizers, openbaov1alpha1.OpenBaoTenantFinalizer)
				}
				if tc.deleting {
					now := metav1.Now()
					tenant.DeletionTimestamp = &now
				}
				builder.WithObjects(tenant)
			}
			if tc.clusterRemaining {
				builder.WithObjects(&openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{Namespace: targetNamespace, Name: "cluster"}})
			}
			c := builder.Build()
			service := metricProvisioner{Provisioner: newProvisionerManager(t, c)}
			if tc.childAbsent {
				service.cleanupError = apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, "child")
			}
			r := &NamespaceProvisionerReconciler{Client: c, APIReader: c, Provisioner: service, OperatorNamespace: key.Namespace}
			seedReconcileMetrics(t, key, controllerNameNamespaceProvisioner)
			result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: key})
			if tc.failPatch || tc.childAbsent {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, ownerReads, "metric cleanup must reuse the existing tenant read")
			counts := reconcileMetricCounts(t, key, controllerNameNamespaceProvisioner)
			if tc.clear {
				require.Empty(t, counts)
			} else {
				require.Equal(t, 2.0, counts["duration"])
				require.Equal(t, 1.0, counts["FirstReason"])
				require.Equal(t, 1.0, counts["SecondReason"])
			}
			if tc.clusterRemaining || !tc.absent && !tc.deleting {
				require.Positive(t, result.RequeueAfter)
			}
		})
	}
}

type metricProvisioner struct {
	appprovisioner.Provisioner
	cleanupError error
}

func (p metricProvisioner) CleanupTenantResources(ctx context.Context, namespace string) error {
	if p.cleanupError != nil {
		return p.cleanupError
	}
	return p.Provisioner.CleanupTenantResources(ctx, namespace)
}

func reconcileMetricCounts(t *testing.T, key client.ObjectKey, controller string) map[string]float64 {
	t.Helper()
	families, err := controllermetrics.Registry.Gather()
	require.NoError(t, err)
	counts := make(map[string]float64)
	for _, family := range families {
		if family.GetName() != "openbao_reconcile_duration_seconds" && family.GetName() != "openbao_reconcile_errors_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string)
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["namespace"] != key.Namespace || labels["name"] != key.Name || labels["controller"] != controller {
				continue
			}
			if family.GetName() == "openbao_reconcile_duration_seconds" {
				counts["duration"] = float64(metric.GetHistogram().GetSampleCount())
			} else {
				counts[labels["reason"]] = metric.GetCounter().GetValue()
			}
		}
	}
	return counts
}

func seedReconcileMetrics(t *testing.T, key client.ObjectKey, controller string) {
	t.Helper()
	m := observability.NewReconcileMetrics(key.Namespace, key.Name, controller)
	t.Cleanup(m.Clear)
	m.ObserveDuration(1)
	m.IncrementError("FirstReason")
	m.IncrementError("SecondReason")
}
