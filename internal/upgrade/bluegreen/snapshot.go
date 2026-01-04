package bluegreen

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
)

func podSnapshotsFromPods(pods []corev1.Pod) ([]podSnapshot, error) {
	snapshots := make([]podSnapshot, 0, len(pods))
	for i := range pods {
		pod := &pods[i]

		sealed, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelSealed)
		if err != nil {
			return nil, fmt.Errorf("failed to parse sealed label on pod %s: %w", pod.Name, err)
		}

		active := false
		isActive, isActivePresent, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
		if err == nil && isActivePresent && isActive {
			active = true
		}

		snapshots = append(snapshots, podSnapshot{
			Ready:    isPodReady(pod),
			Unsealed: present && !sealed,
			Active:   active,
			Deleting: pod.DeletionTimestamp != nil,
		})
	}
	return snapshots, nil
}
