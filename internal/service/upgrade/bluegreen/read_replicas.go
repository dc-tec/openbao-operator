package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

func (m *Manager) ensureSteadyReadReplicasScaledDown(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, bool, error) {
	if cluster == nil || cluster.Spec.ReadReplicas == nil || cluster.Spec.ReadReplicas.Replicas == 0 {
		return recon.Result{}, false, nil
	}

	reader := m.reader
	if reader == nil {
		reader = m.client
	}

	readStatefulSet := &appsv1.StatefulSet{}
	key := client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      resourceidentity.ReadReplicaStatefulSetName(cluster),
	}
	if err := reader.Get(ctx, key, readStatefulSet); err != nil {
		if apierrors.IsNotFound(err) {
			return recon.Result{}, false, nil
		}
		return recon.Result{}, true, fmt.Errorf("failed to get steady read-replica StatefulSet %s/%s: %w", key.Namespace, key.Name, err)
	}

	if readReplicaStatefulSetScaledDown(readStatefulSet) {
		return recon.Result{}, false, nil
	}

	logger.Info(
		"Waiting for steady read replicas to scale down before blue/green continues",
		"statefulSet", readStatefulSet.Name,
		"specReplicas", derefReplicas(readStatefulSet.Spec.Replicas),
		"statusReplicas", readStatefulSet.Status.Replicas,
		"readyReplicas", readStatefulSet.Status.ReadyReplicas,
		"currentReplicas", readStatefulSet.Status.CurrentReplicas,
	)
	return recon.Result{RequeueAfter: constants.RequeueShort}, true, nil
}

func readReplicaStatefulSetScaledDown(sts *appsv1.StatefulSet) bool {
	if sts == nil {
		return true
	}
	if sts.Status.ObservedGeneration < sts.Generation {
		return false
	}
	if derefReplicas(sts.Spec.Replicas) != 0 {
		return false
	}
	return sts.Status.Replicas == 0 && sts.Status.ReadyReplicas == 0 && sts.Status.CurrentReplicas == 0
}

func derefReplicas(replicas *int32) int32 {
	if replicas == nil {
		return 0
	}
	return *replicas
}
