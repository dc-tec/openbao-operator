package raftops

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// PodLeaderProbe checks whether a specific pod currently serves as the leader.
type PodLeaderProbe func(context.Context, *corev1.Pod) (bool, error)

// FindLeaderPodByLabel returns the single pod marked as leader via labels.
// Deleting pods are ignored. Invalid label values are skipped. If multiple
// leader labels are observed, an error is returned so callers can avoid acting
// on ambiguous state.
func FindLeaderPodByLabel(pods []corev1.Pod) (string, bool, error) {
	leaders := make([]string, 0, 1)
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil {
			continue
		}

		leader, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelActive)
		if err != nil || !present {
			continue
		}
		if leader {
			leaders = append(leaders, pod.Name)
		}
	}

	switch len(leaders) {
	case 0:
		return "", false, nil
	case 1:
		return leaders[0], true, nil
	default:
		return "", false, fmt.Errorf("multiple leaders detected via pod labels (%d)", len(leaders))
	}
}

// ProbeLeaderPod checks eligible pods using the supplied probe and returns the
// first pod confirmed as leader. Pods that are deleting, not running, not
// ready, or explicitly sealed are skipped.
func ProbeLeaderPod(ctx context.Context, logger logr.Logger, pods []corev1.Pod, probe PodLeaderProbe) (string, bool) {
	if probe == nil {
		return "", false
	}

	for i := range pods {
		pod := &pods[i]
		if !eligibleLeaderProbePod(pod) {
			continue
		}

		isLeader, err := probe(ctx, pod)
		if err != nil {
			logger.V(1).Info("Leader check failed for pod", "pod", pod.Name, "error", err)
			continue
		}
		if isLeader {
			return pod.Name, true
		}
	}

	return "", false
}

func eligibleLeaderProbePod(pod *corev1.Pod) bool {
	if pod == nil || pod.DeletionTimestamp != nil {
		return false
	}
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	if !isPodReadyConditionTrue(pod) {
		return false
	}

	sealed, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelSealed)
	return err != nil || !present || !sealed
}

func isPodReadyConditionTrue(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
