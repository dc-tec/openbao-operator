package bluegreen

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

// ensureExecutionState recovers inputs for operations started by older operators.
// A changed spec is never used to reconstruct an existing workload's population or image.
func (m *Manager) ensureExecutionState(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	status := cluster.Status.BlueGreen
	if status == nil || status.Phase == openbaov1alpha1.PhaseIdle {
		return nil
	}
	if status.GreenReplicas > 0 && status.GreenImage != "" && status.BlueReplicas > 0 {
		return nil
	}
	if status.GreenRevision == "" && status.Phase != openbaov1alpha1.PhaseRestoringReadReplicas {
		return nil
	}

	if err := m.recoverGreenTarget(ctx, cluster); err != nil {
		return err
	}
	if status.Phase == openbaov1alpha1.PhaseRestoringReadReplicas {
		core.PromoteBlueGreenTarget(cluster)
	}
	if status.BlueReplicas > 0 {
		return nil
	}
	sts, err := m.readExecutionStatefulSet(ctx, cluster, upgrade.StableVoterStatefulSetName(cluster))
	if err == nil {
		status.BlueReplicas = *sts.Spec.Replicas
		return nil
	}
	if apierrors.IsNotFound(err) && isPastPointOfNoReturn(status.Phase) {
		// Blue may already be deleted. These phases cannot invoke consensus repair.
		return nil
	}
	return fmt.Errorf("recover Blue replica count: %w", err)
}

func (m *Manager) recoverGreenTarget(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	status := cluster.Status.BlueGreen
	if status.GreenReplicas > 0 && status.GreenImage != "" {
		return nil
	}
	revision := status.GreenRevision
	if status.Phase == openbaov1alpha1.PhaseRestoringReadReplicas {
		revision = status.BlueRevision
	}
	sts, err := m.readExecutionStatefulSet(ctx, cluster, cluster.Name+"-"+revision)
	if err != nil {
		if apierrors.IsNotFound(err) && status.Phase == openbaov1alpha1.PhaseDeployingGreen && revision == m.calculateRevision(cluster) {
			status.GreenImage = cluster.Spec.Image
			status.GreenVersion = cluster.Spec.Version
			status.GreenReplicas = cluster.Spec.Replicas
			return nil
		}
		return fmt.Errorf("recover Green target: %w", err)
	}
	status.GreenReplicas = *sts.Spec.Replicas
	for _, container := range sts.Spec.Template.Spec.Containers {
		if container.Name == constants.ContainerBao {
			status.GreenImage = container.Image
			break
		}
	}
	if status.GreenImage == "" {
		return fmt.Errorf("recover Green target: StatefulSet %s has no OpenBao image", sts.Name)
	}
	// Probe labels describe the running version even when the image uses a digest.
	pods, err := m.getPodsByRevision(ctx, cluster, revision)
	if err != nil {
		return err
	}
	for _, pod := range pods {
		if version := pod.Labels[portopenbao.LabelVersion]; version != "" {
			status.GreenVersion = version
			break
		}
	}
	return nil
}

func (m *Manager) readExecutionStatefulSet(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, name string) (*appsv1.StatefulSet, error) {
	reader := m.reader
	if reader == nil {
		reader = m.client
	}
	if reader == nil {
		return nil, fmt.Errorf("StatefulSet reader is not configured")
	}
	sts := &appsv1.StatefulSet{}
	if err := reader.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: name}, sts); err != nil {
		return nil, err
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas < 1 {
		return nil, fmt.Errorf("StatefulSet %s has no positive replica count", name)
	}
	return sts, nil
}
