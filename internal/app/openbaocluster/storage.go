package openbaocluster

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
)

const (
	defaultReasonStorageInvalidSize             = "StorageInvalidSize"
	defaultReasonStorageShrinkNotSupported      = "StorageShrinkNotSupported"
	defaultReasonStorageResizeNotSupported      = "StorageResizeNotSupported"
	defaultReasonStorageClassChangeNotSupported = "StorageClassChangeNotSupported"
	defaultReasonStorageRestartRequired         = "StorageRestartRequired"
	storageVolumeDataPrefix                     = "data-"
	storageRequeueShort                         = 5 * time.Second
)

// StorageReasonPolicy configures storage-related error reason values.
type StorageReasonPolicy struct {
	InvalidSize             string
	ShrinkNotSupported      string
	ResizeNotSupported      string
	StorageClassChangeError string
	RestartRequired         string
}

func (p StorageReasonPolicy) invalidSizeReason() string {
	if strings.TrimSpace(p.InvalidSize) != "" {
		return p.InvalidSize
	}
	return defaultReasonStorageInvalidSize
}

func (p StorageReasonPolicy) shrinkNotSupportedReason() string {
	if strings.TrimSpace(p.ShrinkNotSupported) != "" {
		return p.ShrinkNotSupported
	}
	return defaultReasonStorageShrinkNotSupported
}

func (p StorageReasonPolicy) resizeNotSupportedReason() string {
	if strings.TrimSpace(p.ResizeNotSupported) != "" {
		return p.ResizeNotSupported
	}
	return defaultReasonStorageResizeNotSupported
}

func (p StorageReasonPolicy) storageClassChangeReason() string {
	if strings.TrimSpace(p.StorageClassChangeError) != "" {
		return p.StorageClassChangeError
	}
	return defaultReasonStorageClassChangeNotSupported
}

func (p StorageReasonPolicy) restartRequiredReason() string {
	if strings.TrimSpace(p.RestartRequired) != "" {
		return p.RestartRequired
	}
	return defaultReasonStorageRestartRequired
}

// StorageDependencies provides external dependencies for storage reconciliation.
type StorageDependencies struct {
	Client   client.Client
	Recorder events.EventRecorder
}

type storageReconciler struct {
	deps    StorageDependencies
	reasons StorageReasonPolicy
}

// NewStorageReconciler creates a SubReconciler that handles PVC storage expansion workflows.
func NewStorageReconciler(deps StorageDependencies, reasons StorageReasonPolicy) SubReconciler {
	return &storageReconciler{
		deps:    deps,
		reasons: reasons,
	}
}

func (r *storageReconciler) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	return ReconcileStorage(ctx, logger, cluster, r.deps, r.reasons)
}

// ReconcileStorage validates and applies supported PVC storage expansion changes.
func ReconcileStorage(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	deps StorageDependencies,
	reasons StorageReasonPolicy,
) (recon.Result, error) {
	if cluster == nil {
		return recon.Result{}, nil
	}
	if deps.Client == nil {
		return recon.Result{}, fmt.Errorf("storage client is required")
	}

	desiredQty, desiredStorageClassName, err := desiredStorageSpec(cluster, reasons)
	if err != nil {
		return recon.Result{}, err
	}

	pvcs, err := listClusterPVCs(ctx, deps.Client, cluster)
	if err != nil {
		return recon.Result{}, err
	}
	if len(pvcs) == 0 {
		return recon.Result{}, nil
	}

	if err := validateStorageChangeAllowed(desiredQty, desiredStorageClassName, pvcs, reasons); err != nil {
		return recon.Result{}, err
	}

	patched, err := expandPVCs(ctx, deps.Client, deps.Recorder, cluster, logger, desiredQty, pvcs, reasons)
	if err != nil {
		return recon.Result{}, err
	}
	if patched > 0 {
		logger.Info("Requested PVC storage expansion", "count", patched, "desired", desiredQty.String())
	}

	return recon.Result{}, nil
}

func desiredStorageSpec(cluster *openbaov1alpha1.OpenBaoCluster, reasons StorageReasonPolicy) (resource.Quantity, string, error) {
	desiredQty, err := resource.ParseQuantity(cluster.Spec.Storage.Size)
	if err != nil {
		return resource.Quantity{}, "", operatorerrors.WithReason(
			reasons.invalidSizeReason(),
			operatorerrors.WrapPermanentConfig(fmt.Errorf("invalid spec.storage.size %q: %w", cluster.Spec.Storage.Size, err)),
		)
	}

	var desiredStorageClassName string
	if cluster.Spec.Storage.StorageClassName != nil && *cluster.Spec.Storage.StorageClassName != "" {
		desiredStorageClassName = *cluster.Spec.Storage.StorageClassName
	}

	return desiredQty, desiredStorageClassName, nil
}

func listClusterPVCs(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) ([]corev1.PersistentVolumeClaim, error) {
	var pvcList corev1.PersistentVolumeClaimList
	if err := c.List(ctx, &pvcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{labelOpenBaoCluster: cluster.Name}),
	); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return nil, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to list PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err))
		}
		return nil, fmt.Errorf("failed to list PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	return pvcList.Items, nil
}

func validateStorageChangeAllowed(desiredQty resource.Quantity, desiredStorageClassName string, pvcs []corev1.PersistentVolumeClaim, reasons StorageReasonPolicy) error {
	for i := range pvcs {
		pvc := &pvcs[i]

		if desiredStorageClassName != "" && pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != desiredStorageClassName {
			return operatorerrors.WithReason(
				reasons.storageClassChangeReason(),
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
				reasons.shrinkNotSupportedReason(),
				operatorerrors.WrapPermanentConfig(fmt.Errorf(
					"spec.storage.size cannot be decreased (requested %s but PVC %s already requests %s); revert the change",
					desiredQty.String(), pvc.Name, curr.String(),
				)),
			)
		}
	}

	return nil
}

func expandPVCs(
	ctx context.Context,
	c client.Client,
	recorder events.EventRecorder,
	cluster *openbaov1alpha1.OpenBaoCluster,
	logger logr.Logger,
	desiredQty resource.Quantity,
	pvcs []corev1.PersistentVolumeClaim,
	reasons StorageReasonPolicy,
) (int, error) {
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

		if err := c.Patch(ctx, pvc, client.MergeFrom(orig)); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
				return patched, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to patch PVC %s/%s for resize: %w", pvc.Namespace, pvc.Name, err))
			}
			if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) {
				return patched, operatorerrors.WithReason(
					reasons.resizeNotSupportedReason(),
					operatorerrors.WrapPermanentConfig(fmt.Errorf("PVC %s cannot be expanded to %s: %w", pvc.Name, desiredQty.String(), err)),
				)
			}
			return patched, fmt.Errorf("failed to patch PVC %s/%s for resize: %w", pvc.Namespace, pvc.Name, err)
		}

		patched++
		if recorder != nil {
			recorder.Eventf(cluster, nil, corev1.EventTypeNormal, "PVCResize", "PVCResize", "Resizing PVC %s from %s to %s", pvc.Name, currentQty.String(), desiredQty.String())
		}
	}

	return patched, nil
}

// StoragePodClient exposes pod-targeted OpenBao API actions needed for storage restarts.
type StoragePodClient interface {
	IsLeader(ctx context.Context) (bool, error)
	StepDownLeader(ctx context.Context) error
}

// StoragePodClientFactory constructs pod-targeted OpenBao API clients.
type StoragePodClientFactory func(cluster *openbaov1alpha1.OpenBaoCluster, podName string) (StoragePodClient, error)

// StorageResizeRestartDependencies provides dependencies for filesystem resize restarts.
type StorageResizeRestartDependencies struct {
	Client            client.Client
	APIReader         client.Reader
	Recorder          events.EventRecorder
	SmartClientConfig portopenbao.ClientConfig
	ClientForPodFunc  StoragePodClientFactory
}

type storageResizeRestartReconciler struct {
	deps    StorageResizeRestartDependencies
	reasons StorageReasonPolicy
}

// NewStorageResizeRestartReconciler creates a SubReconciler that handles filesystem resize restarts.
func NewStorageResizeRestartReconciler(deps StorageResizeRestartDependencies, reasons StorageReasonPolicy) SubReconciler {
	return &storageResizeRestartReconciler{
		deps:    deps,
		reasons: reasons,
	}
}

func (r *storageResizeRestartReconciler) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	return ReconcileStorageResizeRestart(ctx, logger, cluster, r.deps, r.reasons)
}

// ReconcileStorageResizeRestart performs controlled pod restarts when PVC filesystem expansion is pending.
func ReconcileStorageResizeRestart(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	deps StorageResizeRestartDependencies,
	reasons StorageReasonPolicy,
) (recon.Result, error) {
	if cluster == nil || !cluster.Status.Initialized {
		return recon.Result{}, nil
	}
	if deps.Client == nil {
		return recon.Result{}, fmt.Errorf("storage restart client is required")
	}

	apiReader := deps.APIReader
	if apiReader == nil {
		apiReader = deps.Client
	}

	var pvcList corev1.PersistentVolumeClaimList
	if err := apiReader.List(ctx, &pvcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{labelOpenBaoCluster: cluster.Name}),
	); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return recon.Result{}, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to list PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err))
		}
		return recon.Result{}, fmt.Errorf("failed to list PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if cluster.Spec.Maintenance == nil || !cluster.Spec.Maintenance.Enabled {
		if anyPVCFileSystemResizePending(pvcList.Items) {
			return recon.Result{}, operatorerrors.WithReason(
				reasons.restartRequiredReason(),
				operatorerrors.WrapPermanentPrerequisitesMissing(fmt.Errorf(
					"PVC filesystem resize is pending and requires a pod restart; enable spec.maintenance.enabled=true to allow the operator to perform controlled restarts, or restart the pods manually",
				)),
			)
		}
		return recon.Result{}, nil
	}

	if cluster.Status.Phase == openbaov1alpha1.ClusterPhaseUpgrading ||
		(cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.Phase != "" && cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle) {
		return recon.Result{RequeueAfter: storageRequeueShort}, nil
	}

	targetPod, err := nextPodNeedingFSResizeRestart(ctx, deps.Client, cluster, pvcList.Items)
	if err != nil {
		return recon.Result{}, err
	}
	if targetPod == nil {
		return recon.Result{}, nil
	}

	if !isPodReady(targetPod) {
		return recon.Result{RequeueAfter: storageRequeueShort}, nil
	}

	actions, err := clientForPod(cluster, targetPod.Name, deps.SmartClientConfig, deps.ClientForPodFunc)
	if err != nil {
		return recon.Result{}, operatorerrors.WrapTransientConnection(fmt.Errorf("failed to create OpenBao client for pod %s: %w", targetPod.Name, err))
	}

	isLeader, err := actions.IsLeader(ctx)
	if err != nil {
		return recon.Result{}, operatorerrors.WrapTransientConnection(fmt.Errorf("failed to check leadership for pod %s: %w", targetPod.Name, err))
	}

	if isLeader {
		if cluster.Spec.Replicas > 1 {
			logger.Info("Pod requires filesystem resize restart but is leader; stepping down first", "pod", targetPod.Name)
			if err := actions.StepDownLeader(ctx); err != nil {
				return recon.Result{}, operatorerrors.WrapTransientConnection(fmt.Errorf("failed to step down leader %s before restart: %w", targetPod.Name, err))
			}
			if deps.Recorder != nil {
				deps.Recorder.Eventf(cluster, nil, corev1.EventTypeNormal, "PVCResizeLeaderStepDown", "PVCResizeLeaderStepDown", "Leader %s stepped down to complete filesystem resize", targetPod.Name)
			}
			return recon.Result{RequeueAfter: storageRequeueShort}, nil
		}
		logger.Info("Pod requires filesystem resize restart and is leader in a single-replica cluster; restarting without step-down", "pod", targetPod.Name)
	}

	logger.Info("Restarting pod to complete filesystem resize", "pod", targetPod.Name)
	if err := deps.Client.Delete(ctx, targetPod); err != nil && !apierrors.IsNotFound(err) {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return recon.Result{}, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to delete pod %s/%s for filesystem resize restart: %w", targetPod.Namespace, targetPod.Name, err))
		}
		return recon.Result{}, fmt.Errorf("failed to delete pod %s/%s for filesystem resize restart: %w", targetPod.Namespace, targetPod.Name, err)
	}

	if deps.Recorder != nil {
		deps.Recorder.Eventf(cluster, nil, corev1.EventTypeNormal, "PVCResizePodRestart", "PVCResizePodRestart", "Restarted pod %s to complete filesystem resize", targetPod.Name)
	}

	return recon.Result{RequeueAfter: storageRequeueShort}, nil
}

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
		wantRev = inframanager.BlueGreenActiveRevision(cluster)
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
	cluster *openbaov1alpha1.OpenBaoCluster,
	podName string,
	smartClientConfig portopenbao.ClientConfig,
	factory StoragePodClientFactory,
) (StoragePodClient, error) {
	if factory != nil {
		return factory(cluster, podName)
	}

	headlessServiceName := cluster.Name
	podDNS := fmt.Sprintf("%s.%s.%s.svc:8200", podName, headlessServiceName, cluster.Namespace)
	cfg := smartClientConfig
	cfg.BaseURL = "https://" + podDNS

	return portopenbao.NewClient(cfg)
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
