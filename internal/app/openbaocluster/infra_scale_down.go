package openbaocluster

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func (r *infraReconciler) handleScaleDownSafety(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, desiredReplicas int32, currentSTS *appsv1.StatefulSet) error {
	if currentSTS.Spec.Replicas == nil {
		return nil
	}
	currentReplicas := *currentSTS.Spec.Replicas
	if desiredReplicas >= currentReplicas {
		return nil
	}

	victimOrdinal := currentReplicas - 1
	victimPodName := fmt.Sprintf("%s-%d", currentSTS.Name, victimOrdinal)

	logger := log.FromContext(ctx).WithValues("victim", victimPodName, "currentReplicas", currentReplicas, "desiredReplicas", desiredReplicas)
	logger.Info("Detected scale down operation; checking victim leadership")

	victimClient, err := r.clientForPod(ctx, cluster, victimPodName)
	if err != nil {
		logger.Error(err, "Failed to create client for victim pod; assuming safe to remove")
		return nil
	}

	isLeader, err := victimClient.IsLeader(ctx)
	if err != nil {
		logger.Error(err, "Failed to check leadership of victim pod; assuming safe to remove (pod might be down)")
		return nil
	}

	if isLeader {
		logger.Info("Victim pod is the Active Leader. Attempting graceful step-down.")
		if err := victimClient.StepDownLeader(ctx); err != nil {
			return fmt.Errorf("failed to step down leader %s: %w", victimPodName, err)
		}
		return fmt.Errorf("waiting for leader step-down on %s to complete", victimPodName)
	}

	logger.Info("Victim pod is a follower. Safe to scale down.")
	return nil
}

func (r *infraReconciler) clientForPod(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (ScaleDownPodClient, error) {
	if r.deps.Pods.ClientForPodFunc != nil {
		return r.deps.Pods.ClientForPodFunc(ctx, cluster, podName)
	}
	return nil, fmt.Errorf("OpenBao pod client factory is not configured")
}
