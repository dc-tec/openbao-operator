package infra

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

// InferActiveRevisionFromPods attempts to infer the currently running revision for a cluster
// by inspecting pod labels. This is primarily used to recover from restarts where status may
// not yet contain (or may contain an incorrect) BlueRevision.
func InferActiveRevisionFromPods(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if c == nil {
		return "", fmt.Errorf("client is required")
	}
	if cluster == nil {
		return "", fmt.Errorf("cluster is required")
	}

	pods := &corev1.PodList{}
	if err := c.List(ctx, pods,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{
			constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
			constants.LabelAppInstance:    cluster.Name,
			constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
			constants.LabelOpenBaoCluster: cluster.Name,
		}),
	); err != nil {
		return "", fmt.Errorf("failed to list pods for revision inference: %w", err)
	}

	type counts struct {
		ready int
		total int
	}
	byRevision := map[string]counts{}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		rev := ""
		if pod.Labels != nil {
			rev = pod.Labels[constants.LabelOpenBaoRevision]
		}
		if rev == "" {
			continue
		}
		c := byRevision[rev]
		c.total++
		if pod.Status.Phase == corev1.PodRunning && isPodReady(pod) {
			c.ready++
		}
		byRevision[rev] = c
	}

	if len(byRevision) == 0 {
		return "", nil
	}

	type candidate struct {
		rev   string
		ready int
		total int
	}
	candidates := make([]candidate, 0, len(byRevision))
	for rev, c := range byRevision {
		candidates = append(candidates, candidate{rev: rev, ready: c.ready, total: c.total})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].ready != candidates[j].ready {
			return candidates[i].ready > candidates[j].ready
		}
		if candidates[i].total != candidates[j].total {
			return candidates[i].total > candidates[j].total
		}
		return candidates[i].rev < candidates[j].rev
	})

	return candidates[0].rev, nil
}

func isPodReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
