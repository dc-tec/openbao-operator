package openbaocluster

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	openbao "github.com/dc-tec/openbao-operator/internal/openbao"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
	"github.com/dc-tec/openbao-operator/internal/revision"
)

type storageReconciler struct {
	client   client.Client
	recorder events.EventRecorder
}

func (r *storageReconciler) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	if cluster == nil {
		return recon.Result{}, nil
	}

	desiredQty, desiredStorageClassName, err := desiredStorageSpec(cluster)
	if err != nil {
		return recon.Result{}, err
	}

	pvcs, err := r.listClusterPVCs(ctx, cluster)
	if err != nil {
		return recon.Result{}, err
	}
	if len(pvcs) == 0 {
		return recon.Result{}, nil
	}

	if err := validateStorageChangeAllowed(desiredQty, desiredStorageClassName, pvcs); err != nil {
		return recon.Result{}, err
	}

	patched, err := r.expandPVCs(ctx, cluster, logger, desiredQty, pvcs)
	if err != nil {
		return recon.Result{}, err
	}
	if patched > 0 {
		logger.Info("Requested PVC storage expansion", "count", patched, "desired", desiredQty.String())
	}

	return recon.Result{}, nil
}

func desiredStorageSpec(cluster *openbaov1alpha1.OpenBaoCluster) (resource.Quantity, string, error) {
	desiredQty, err := resource.ParseQuantity(cluster.Spec.Storage.Size)
	if err != nil {
		return resource.Quantity{}, "", operatorerrors.WithReason(
			ReasonStorageInvalidSize,
			operatorerrors.WrapPermanentConfig(fmt.Errorf("invalid spec.storage.size %q: %w", cluster.Spec.Storage.Size, err)),
		)
	}

	var desiredStorageClassName string
	if cluster.Spec.Storage.StorageClassName != nil && *cluster.Spec.Storage.StorageClassName != "" {
		desiredStorageClassName = *cluster.Spec.Storage.StorageClassName
	}

	return desiredQty, desiredStorageClassName, nil
}

func (r *storageReconciler) listClusterPVCs(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) ([]corev1.PersistentVolumeClaim, error) {
	var pvcList corev1.PersistentVolumeClaimList
	if err := r.client.List(ctx, &pvcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{constants.LabelOpenBaoCluster: cluster.Name}),
	); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return nil, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to list PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err))
		}
		return nil, fmt.Errorf("failed to list PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	return pvcList.Items, nil
}

func validateStorageChangeAllowed(desiredQty resource.Quantity, desiredStorageClassName string, pvcs []corev1.PersistentVolumeClaim) error {
	for i := range pvcs {
		pvc := &pvcs[i]

		if desiredStorageClassName != "" && pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != desiredStorageClassName {
			return operatorerrors.WithReason(
				ReasonStorageClassChangeNotSupported,
				operatorerrors.WrapPermanentConfig(fmt.Errorf(
					"spec.storage.storageClassName cannot be changed for an existing cluster (PVC %s has %q, desired %q)",
					pvc.Name, *pvc.Spec.StorageClassName, desiredStorageClassName,
				)),
			)
		}

		curr, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if !ok {
			continue
		}
		if desiredQty.Cmp(curr) < 0 {
			return operatorerrors.WithReason(
				ReasonStorageShrinkNotSupported,
				operatorerrors.WrapPermanentConfig(fmt.Errorf(
					"spec.storage.size cannot be decreased (requested %s but PVC %s already requests %s); revert the change",
					desiredQty.String(), pvc.Name, curr.String(),
				)),
			)
		}
	}

	return nil
}

func (r *storageReconciler) expandPVCs(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, logger logr.Logger, desiredQty resource.Quantity, pvcs []corev1.PersistentVolumeClaim) (int, error) {
	patched := 0
	for i := range pvcs {
		pvc := &pvcs[i]

		currentQty, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if !ok {
			logger.V(1).Info("PVC missing storage request; skipping", "pvc", pvc.Name)
			continue
		}
		if desiredQty.Cmp(currentQty) <= 0 {
			continue
		}

		orig := pvc.DeepCopy()
		if pvc.Spec.Resources.Requests == nil {
			pvc.Spec.Resources.Requests = corev1.ResourceList{}
		}
		pvc.Spec.Resources.Requests[corev1.ResourceStorage] = desiredQty

		if err := r.client.Patch(ctx, pvc, client.MergeFrom(orig)); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
				return patched, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to patch PVC %s/%s for resize: %w", pvc.Namespace, pvc.Name, err))
			}
			if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) {
				return patched, operatorerrors.WithReason(
					ReasonStorageResizeNotSupported,
					operatorerrors.WrapPermanentConfig(fmt.Errorf("PVC %s cannot be expanded to %s: %w", pvc.Name, desiredQty.String(), err)),
				)
			}
			return patched, fmt.Errorf("failed to patch PVC %s/%s for resize: %w", pvc.Namespace, pvc.Name, err)
		}

		patched++
		if r.recorder != nil {
			r.recorder.Eventf(cluster, nil, corev1.EventTypeNormal, "PVCResize", "", "Resizing PVC %s from %s to %s", pvc.Name, currentQty.String(), desiredQty.String())
		}
	}

	return patched, nil
}

type podClientFactory func(cluster *openbaov1alpha1.OpenBaoCluster, podName string) (openbao.ClusterActions, error)

type storageResizeRestartReconciler struct {
	client            client.Client
	apiReader         client.Reader
	recorder          events.EventRecorder
	smartClientConfig openbao.ClientConfig
	clientForPodFunc  podClientFactory
}

func (r *storageResizeRestartReconciler) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	if cluster == nil || !cluster.Status.Initialized {
		return recon.Result{}, nil
	}

	// List PVCs once to avoid redundant API calls.
	var pvcList corev1.PersistentVolumeClaimList
	if err := r.apiReader.List(ctx, &pvcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{constants.LabelOpenBaoCluster: cluster.Name}),
	); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return recon.Result{}, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to list PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err))
		}
		return recon.Result{}, fmt.Errorf("failed to list PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if cluster.Spec.Maintenance == nil || !cluster.Spec.Maintenance.Enabled {
		if anyPVCFileSystemResizePending(pvcList.Items) {
			return recon.Result{}, operatorerrors.WithReason(
				ReasonStorageRestartRequired,
				operatorerrors.WrapPermanentPrerequisitesMissing(fmt.Errorf(
					"PVC filesystem resize is pending and requires a pod restart; enable spec.maintenance.enabled=true to allow the operator to perform controlled restarts, or restart the pods manually",
				)),
			)
		}
		return recon.Result{}, nil
	}

	// Avoid disruptive restarts while upgrades are in progress.
	if cluster.Status.Phase == openbaov1alpha1.ClusterPhaseUpgrading ||
		(cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.Phase != "" && cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle) {
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	targetPod, err := r.nextPodNeedingFSResizeRestart(ctx, cluster, pvcList.Items)
	if err != nil {
		return recon.Result{}, err
	}
	if targetPod == nil {
		return recon.Result{}, nil
	}

	if !isPodReady(targetPod) {
		// Don't chain restarts while the cluster is still converging.
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	actions, err := r.clientForPod(cluster, targetPod.Name)
	if err != nil {
		// If we can't talk to the pod, deleting it might be disruptive. Retry instead.
		return recon.Result{}, operatorerrors.WrapTransientConnection(fmt.Errorf("failed to create OpenBao client for pod %s: %w", targetPod.Name, err))
	}

	isLeader, err := actions.IsLeader(ctx)
	if err != nil {
		return recon.Result{}, operatorerrors.WrapTransientConnection(fmt.Errorf("failed to check leadership for pod %s: %w", targetPod.Name, err))
	}

	if isLeader {
		// Single-replica clusters have no peer to transfer leadership to; stepping down will fail.
		// In maintenance mode, accept a brief restart to complete the filesystem resize.
		if cluster.Spec.Replicas > 1 {
			logger.Info("Pod requires filesystem resize restart but is leader; stepping down first", "pod", targetPod.Name)
			if err := actions.StepDownLeader(ctx); err != nil {
				return recon.Result{}, operatorerrors.WrapTransientConnection(fmt.Errorf("failed to step down leader %s before restart: %w", targetPod.Name, err))
			}
			if r.recorder != nil {
				r.recorder.Eventf(cluster, nil, corev1.EventTypeNormal, "PVCResizeLeaderStepDown", "", "Leader %s stepped down to complete filesystem resize", targetPod.Name)
			}
			return recon.Result{RequeueAfter: constants.RequeueShort}, nil
		}
		logger.Info("Pod requires filesystem resize restart and is leader in a single-replica cluster; restarting without step-down", "pod", targetPod.Name)
	}

	logger.Info("Restarting pod to complete filesystem resize", "pod", targetPod.Name)
	if err := r.client.Delete(ctx, targetPod); err != nil && !apierrors.IsNotFound(err) {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return recon.Result{}, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to delete pod %s/%s for filesystem resize restart: %w", targetPod.Namespace, targetPod.Name, err))
		}
		return recon.Result{}, fmt.Errorf("failed to delete pod %s/%s for filesystem resize restart: %w", targetPod.Namespace, targetPod.Name, err)
	}

	if r.recorder != nil {
		r.recorder.Eventf(cluster, nil, corev1.EventTypeNormal, "PVCResizePodRestart", "", "Restarted pod %s to complete filesystem resize", targetPod.Name)
	}

	return recon.Result{RequeueAfter: constants.RequeueShort}, nil
}

func anyPVCFileSystemResizePending(pvcs []corev1.PersistentVolumeClaim) bool {
	for i := range pvcs {
		if pvcHasFileSystemResizePending(&pvcs[i]) {
			return true
		}
	}
	return false
}

func (r *storageResizeRestartReconciler) nextPodNeedingFSResizeRestart(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, pvcs []corev1.PersistentVolumeClaim) (*corev1.Pod, error) {
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

	// Ensure we only restart pods belonging to the active revision when Blue/Green is enabled.
	var wantRev string
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen {
		wantRev = revision.OpenBaoClusterRevision(cluster.Spec.Version, cluster.Spec.Image, cluster.Spec.Replicas)
		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueRevision != "" {
			wantRev = cluster.Status.BlueGreen.BlueRevision
		}
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
		if err := r.client.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: candidatePodName}, pod); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
				return nil, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to get pod %s/%s for filesystem resize restart: %w", cluster.Namespace, candidatePodName, err))
			}
			return nil, fmt.Errorf("failed to get pod %s/%s for filesystem resize restart: %w", cluster.Namespace, candidatePodName, err)
		}

		if wantRev != "" {
			if gotRev := strings.TrimSpace(pod.Labels[constants.LabelOpenBaoRevision]); gotRev != wantRev {
				continue
			}
		}

		active, present, _ := openbao.ParseBoolLabel(pod.Labels, openbao.LabelActive)
		if present && active {
			leaderCandidate = pod
			continue
		}

		return pod, nil
	}

	return leaderCandidate, nil
}

func (r *storageResizeRestartReconciler) clientForPod(cluster *openbaov1alpha1.OpenBaoCluster, podName string) (openbao.ClusterActions, error) {
	if r.clientForPodFunc != nil {
		return r.clientForPodFunc(cluster, podName)
	}

	headlessServiceName := cluster.Name
	podDNS := fmt.Sprintf("%s.%s.%s.svc:8200", podName, headlessServiceName, cluster.Namespace)
	baseURL := "https://" + podDNS

	cfg := r.smartClientConfig
	cfg.BaseURL = baseURL

	return openbao.NewClient(cfg)
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
	prefix := constants.VolumeData + "-"
	if !strings.HasPrefix(pvcName, prefix) {
		return "", false
	}
	return strings.TrimPrefix(pvcName, prefix), true
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
	if !pod.DeletionTimestamp.IsZero() {
		return false
	}
	for i := range pod.Status.Conditions {
		cond := pod.Status.Conditions[i]
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
