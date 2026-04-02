package rolling

import (
	"context"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// getClusterPods returns all pods belonging to the cluster.
// It filters out backup job pods and other non-StatefulSet pods.
func (m *Manager) getClusterPods(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		constants.LabelAppInstance:  cluster.Name,
		constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
		constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
	})

	if err := m.client.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return nil, err
	}

	// Filter out backup job pods and other non-StatefulSet pods
	// StatefulSet pods have a name pattern: <cluster-name>-<ordinal>
	// Backup job pods have labels like "openbao.org/component": backup
	filteredPods := make([]corev1.Pod, 0, len(podList.Items))
	statefulSetPrefix := cluster.Name + "-"
	for _, pod := range podList.Items {
		// Skip backup job pods (they have the backup component label)
		if pod.Labels[constants.LabelOpenBaoComponent] == constants.ComponentBackup {
			continue
		}
		// Only include pods that match the StatefulSet naming pattern
		// StatefulSet pods are named like: <cluster-name>-<ordinal>
		if !strings.HasPrefix(pod.Name, statefulSetPrefix) {
			continue
		}
		// Extract the suffix after the cluster name
		suffix := pod.Name[len(statefulSetPrefix):]
		// Verify the suffix is a valid ordinal (numeric)
		if ordinal, err := strconv.Atoi(suffix); err == nil && ordinal >= 0 {
			filteredPods = append(filteredPods, pod)
		}
	}

	// Sort by ordinal (descending) for consistent processing order
	sort.Slice(filteredPods, func(i, j int) bool {
		ordinalI := extractOrdinal(filteredPods[i].Name)
		ordinalJ := extractOrdinal(filteredPods[j].Name)
		return ordinalI > ordinalJ
	})

	return filteredPods, nil
}

// isPodReady checks if a pod has the Ready condition set to True.
func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// extractOrdinal extracts the ordinal number from a StatefulSet pod name.
// For example, "cluster-2" returns 2.
func extractOrdinal(podName string) int {
	parts := strings.Split(podName, "-")
	if len(parts) < 2 {
		return 0
	}
	ordinal, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0
	}
	return ordinal
}
