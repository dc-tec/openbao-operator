package openbaocluster

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
)

func anyPVCFileSystemResizePending(pvcs []corev1.PersistentVolumeClaim) bool {
	for i := range pvcs {
		if pvcHasFileSystemResizePending(&pvcs[i]) {
			return true
		}
	}
	return false
}

func nextPodNeedingFSResizeRestart(
	ctx context.Context,
	c client.Client,
	cluster *openbaov1alpha1.OpenBaoCluster,
	pvcs []corev1.PersistentVolumeClaim,
) (*corev1.Pod, error) {
	candidatePodNames := make([]string, 0, 1)
	for i := range pvcs {
		pvc := &pvcs[i]
		if !pvcHasFileSystemResizePending(pvc) {
			continue
		}
		podName, ok := podNameForDataPVC(pvc.Name)
		if !ok {
			continue
		}
		candidatePodNames = append(candidatePodNames, podName)
	}

	if len(candidatePodNames) == 0 {
		return nil, nil
	}

	var wantRev string
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen {
		wantRev = workloadsvc.BlueGreenActiveRevision(cluster)
	}

	unique := make(map[string]struct{}, len(candidatePodNames))
	candidates := make([]string, 0, len(candidatePodNames))
	for _, name := range candidatePodNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := unique[name]; ok {
			continue
		}
		unique[name] = struct{}{}
		candidates = append(candidates, name)
	}

	sort.Slice(candidates, func(i, j int) bool {
		oi, okI := podOrdinal(candidates[i])
		oj, okJ := podOrdinal(candidates[j])
		if okI && okJ {
			return oi < oj
		}
		if okI {
			return true
		}
		if okJ {
			return false
		}
		return candidates[i] < candidates[j]
	})

	var leaderCandidate *corev1.Pod
	for _, candidatePodName := range candidates {
		pod := &corev1.Pod{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: candidatePodName}, pod); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
				return nil, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to get pod %s/%s for filesystem resize restart: %w", cluster.Namespace, candidatePodName, err))
			}
			return nil, fmt.Errorf("failed to get pod %s/%s for filesystem resize restart: %w", cluster.Namespace, candidatePodName, err)
		}

		if wantRev != "" {
			if gotRev := strings.TrimSpace(pod.Labels[labelOpenBaoRevision]); gotRev != wantRev {
				continue
			}
		}

		active, present, _ := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelActive)
		if present && active {
			leaderCandidate = pod
			continue
		}

		return pod, nil
	}

	return leaderCandidate, nil
}

func clientForPod(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
	podName string,
	factory StoragePodClientFactory,
) (StoragePodClient, error) {
	if factory != nil {
		return factory(ctx, cluster, podName)
	}
	return nil, fmt.Errorf("OpenBao pod client factory is not configured")
}

func pvcHasFileSystemResizePending(pvc *corev1.PersistentVolumeClaim) bool {
	if pvc == nil {
		return false
	}
	for i := range pvc.Status.Conditions {
		c := pvc.Status.Conditions[i]
		if c.Type == corev1.PersistentVolumeClaimFileSystemResizePending && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func podNameForDataPVC(pvcName string) (string, bool) {
	if !strings.HasPrefix(pvcName, storageVolumeDataPrefix) {
		return "", false
	}
	return strings.TrimPrefix(pvcName, storageVolumeDataPrefix), true
}

func podOrdinal(podName string) (int, bool) {
	podName = strings.TrimSpace(podName)
	if podName == "" {
		return 0, false
	}
	idx := strings.LastIndex(podName, "-")
	if idx < 0 || idx == len(podName)-1 {
		return 0, false
	}
	raw := podName[idx+1:]
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func isPodReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	for i := range pod.Status.Conditions {
		c := pod.Status.Conditions[i]
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
