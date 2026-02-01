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
// It returns the revision and the container image associated with it.
func InferActiveRevisionFromPods(ctx context.Context, c client.Reader, cluster *openbaov1alpha1.OpenBaoCluster) (string, string, error) {
	if c == nil {
		return "", "", fmt.Errorf("client is required")
	}
	if cluster == nil {
		return "", "", fmt.Errorf("cluster is required")
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
		return "", "", fmt.Errorf("failed to list pods for revision inference: %w", err)
	}

	type counts struct {
		ready int
		total int
		image string
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
		if c.image == "" {
			for _, container := range pod.Spec.Containers {
				if container.Name == "openbao" {
					c.image = container.Image
					break
				}
			}
		}
		byRevision[rev] = c
	}

	if len(byRevision) == 0 {
		return "", "", nil
	}

	type candidate struct {
		rev   string
		ready int
		total int
		image string
	}
	candidates := make([]candidate, 0, len(byRevision))
	for rev, c := range byRevision {
		candidates = append(candidates, candidate{rev: rev, ready: c.ready, total: c.total, image: c.image})
	}

	// When Blue/Green status has BlueImage set, restrict to revisions matching that image
	// so we never infer Green (target image) as Blue during an upgrade.
	//
	// NOTE: Do not hard-filter candidates by BlueImage. This helper is used precisely when
	// status may be missing/stale/incorrect after restarts, and BlueImage can be wrong
	// (e.g., backfilled from spec.image during an in-progress upgrade). Instead, treat
	// currentVersion/blueImage as weak hints during tie-breaking.
	expectedFromCurrentVersion := ""
	if cluster.Status.CurrentVersion != "" &&
		cluster.Spec.Version != "" &&
		cluster.Status.CurrentVersion != cluster.Spec.Version {
		// During an upgrade, CurrentVersion reflects the active ("Blue") cluster.
		expectedFromCurrentVersion = constants.GetOpenBaoImage(cluster.Status.CurrentVersion)
	}
	blueHint := ""
	if cluster.Status.BlueGreen != nil {
		blueHint = cluster.Status.BlueGreen.BlueImage
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].ready != candidates[j].ready {
			return candidates[i].ready > candidates[j].ready
		}
		if candidates[i].total != candidates[j].total {
			return candidates[i].total > candidates[j].total
		}
		score := func(c candidate) int {
			if expectedFromCurrentVersion != "" && c.image == expectedFromCurrentVersion {
				return 2
			}
			if blueHint != "" && c.image == blueHint {
				return 1
			}
			return 0
		}
		if score(candidates[i]) != score(candidates[j]) {
			return score(candidates[i]) > score(candidates[j])
		}
		return candidates[i].rev < candidates[j].rev
	})

	return candidates[0].rev, candidates[0].image, nil
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
