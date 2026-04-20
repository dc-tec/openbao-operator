package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
)

func (r *infraReconciler) reconcileDisabledReadReplicas(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, readSpec workloadsvc.StatefulSetSpec, readCurrentSTS *appsv1.StatefulSet, readCurrentSTSFound bool) (bool, error) {
	workloadManager := r.newWorkloadManager()

	if readCurrentSTSFound {
		if readSpec.Replicas != 0 || !statefulSetSettledAtReplicas(readCurrentSTS, 0) {
			if err := workloadManager.ScaleStatefulSetIfExists(ctx, logger, cluster, readSpec, readSpec.Replicas); err != nil {
				return false, err
			}

			logger.Info(
				"Read replicas are disabled; waiting for read StatefulSet to drain before cleanup",
				"statefulset", readSpec.Name,
				"appliedReplicas", readSpec.Replicas,
			)
			return true, nil
		}

		if err := workloadManager.DeleteStatefulSetIfExists(ctx, logger, cluster, readSpec); err != nil {
			return false, err
		}
	}

	if err := workloadManager.DeleteConfigMapIfExists(ctx, logger, cluster, readSpec); err != nil {
		return false, err
	}

	return false, nil
}
