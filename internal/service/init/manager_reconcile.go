package init

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

const (
	selfInitConfigMapUpdateReason = "reconcile completed self-init ConfigMap"
)

// Reconcile checks if the OpenBao cluster is initialized and initializes it if needed.
// During initial cluster creation, this ensures:
// 1. Only 1 pod is running (enforced by infra manager)
// 2. That pod is initialized using bao operator init (unless self-init is enabled)
// 3. After initialization, the cluster status is updated to allow scaling to desired replicas
//
// When self-initialization is enabled (spec.selfInit.enabled = true):
// - The Operator does NOT execute bao operator init
// - The Operator only monitors for initialized=true via bao status
// - No root token Secret is created (OpenBao auto-revokes it during self-init)
// - Status.SelfInitialized is set to true after successful initialization
//
// This should only be called during initial cluster creation. Once initialized, subsequent
// reconciles will skip this step.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	if cluster.Status.Initialized {
		logger.V(1).Info("OpenBao cluster is already initialized; skipping initialization")
		return recon.Result{}, nil
	}

	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	logger.Info("Checking if OpenBao cluster needs initialization",
		"namespace", cluster.Namespace,
		"name", cluster.Name,
		"selfInitEnabled", selfInitEnabled)

	pod, result, err := m.loadInitializationPod(ctx, logger, cluster)
	if err != nil || result != nil {
		if err != nil {
			return recon.Result{}, err
		}
		return *result, nil
	}

	if selfInitEnabled {
		return m.reconcileSelfInit(ctx, logger, cluster, pod)
	}

	return m.reconcileOperatorInit(ctx, logger, cluster, pod, selfInitEnabled)
}

func (m *Manager) loadInitializationPod(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (*corev1.Pod, *recon.Result, error) {
	pod, err := m.findFirstPod(ctx, cluster)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find pod for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if pod == nil {
		logger.Info("No pods found; waiting for pod to be created")
		result := recon.Result{RequeueAfter: constants.RequeueShort}
		return nil, &result, nil
	}

	logger.Info("Found pod for initialization", "pod", pod.Name, "phase", pod.Status.Phase)
	if !isContainerRunning(pod) {
		logContainerNotReady(logger, pod)
		logger.Info("Container not ready for initialization yet; waiting", "pod", pod.Name, "phase", pod.Status.Phase)
		result := recon.Result{RequeueAfter: constants.RequeueShort}
		return nil, &result, nil
	}

	return pod, nil, nil
}

func (m *Manager) reconcileSelfInit(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	pod *corev1.Pod,
) (recon.Result, error) {
	emitNormalEvent(m.recorder, cluster, ReasonInitStarted, "Self-initialization in progress for cluster %s", cluster.Name)

	if handled, result, err := m.reconcileSelfInitFromServiceLabels(ctx, logger, cluster, pod); handled || err != nil {
		return result, err
	}

	if handled, result, err := m.reconcileSelfInitFromPodReadiness(ctx, logger, cluster, pod); handled || err != nil {
		return result, err
	}

	return m.reconcileSelfInitFromHealth(ctx, logger, cluster, pod)
}

func (m *Manager) reconcileSelfInitFromServiceLabels(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	pod *corev1.Pod,
) (bool, recon.Result, error) {
	initializedLabel, hasInitializedLabel, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelInitialized)
	if err != nil {
		logger.V(1).Info("Invalid OpenBao initialized label value", "pod", pod.Name, "error", err)
	}

	sealedLabel, hasSealedLabel, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelSealed)
	if err != nil {
		logger.V(1).Info("Invalid OpenBao sealed label value", "pod", pod.Name, "error", err)
	}

	if !hasInitializedLabel || !hasSealedLabel || !initializedLabel || sealedLabel {
		return false, recon.Result{}, nil
	}

	logger.Info("OpenBao service registration labels indicate initialized and unsealed; marking cluster as initialized", "pod", pod.Name)
	if err := m.markSelfInitComplete(ctx, logger, cluster); err != nil {
		return true, recon.Result{}, err
	}
	return true, recon.Result{RequeueAfter: constants.RequeueShort}, nil
}

func (m *Manager) reconcileSelfInitFromPodReadiness(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	pod *corev1.Pod,
) (bool, recon.Result, error) {
	if isPodReady(pod) {
		logger.Info("OpenBao pod is Ready; marking cluster as initialized", "pod", pod.Name)
		if err := m.markSelfInitComplete(ctx, logger, cluster); err != nil {
			return true, recon.Result{}, err
		}
		return true, recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	return false, recon.Result{}, nil
}

func (m *Manager) reconcileSelfInitFromHealth(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	pod *corev1.Pod,
) (recon.Result, error) {
	client, err := m.newOpenBaoClient(ctx, cluster)
	if err != nil {
		logger.Info("Self-initialization health observation is not ready yet; will retry", "pod", pod.Name, "error", err)
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	healthCtx, cancel := context.WithTimeout(ctx, selfInitHealthTimeout)
	defer cancel()

	health, err := client.Health(healthCtx)
	if err != nil {
		logger.Info("Self-initialization health observation failed; will retry", "pod", pod.Name, "error", err)
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}
	if health != nil && health.Initialized && !health.Sealed {
		logger.Info("OpenBao health endpoint reports initialized and unsealed; marking cluster as initialized", "pod", pod.Name)
		if err := m.markSelfInitComplete(ctx, logger, cluster); err != nil {
			return recon.Result{}, err
		}
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}
	if health != nil {
		logger.Info("Self-initialization is enabled; waiting for OpenBao health to report initialized and unsealed", "pod", pod.Name, "initialized", health.Initialized, "sealed", health.Sealed)
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	logger.Info("Self-initialization is enabled; waiting for OpenBao health observation", "pod", pod.Name)
	return recon.Result{RequeueAfter: constants.RequeueShort}, nil
}

func (m *Manager) markSelfInitComplete(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if err := m.reconcileCompletedSelfInitConfigMap(ctx, logger, cluster); err != nil {
		return err
	}

	cluster.Status.Initialized = true
	cluster.Status.SelfInitialized = true

	if err := m.raftManager.ReconcileAutopilotConfig(ctx, logger, cluster); err != nil {
		if operatorerrors.IsTransient(err) {
			logger.V(1).Info("Raft Autopilot configuration not ready for self-init cluster; will retry via reconciler", "error", err)
		} else {
			logger.Error(err, "Failed to configure Raft Autopilot for self-init cluster; will retry via reconciler")
		}
	}
	emitNormalEvent(m.recorder, cluster, ReasonInitCompleted, "Self-initialization completed for cluster %s", cluster.Name)
	return nil
}

func (m *Manager) reconcileCompletedSelfInitConfigMap(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if m.clientset == nil {
		return fmt.Errorf("kubernetes clientset is required to %s", selfInitConfigMapUpdateReason)
	}

	configMaps := m.clientset.CoreV1().ConfigMaps(cluster.Namespace)
	cmName := resourceidentity.ConfigInitMapName(cluster)
	configMap, err := configMaps.Get(ctx, cmName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Self-init ConfigMap not found during completion; bootstrap reconcile will create inert ConfigMap", "configmap", cmName)
			return nil
		}
		return operatorerrors.WrapTransientKubernetesAPI(
			fmt.Errorf("failed to get self-init ConfigMap %s/%s while completing self-init: %w", cluster.Namespace, cmName, err),
		)
	}

	if configMap.Data != nil && configMap.Data[constants.OpenBaoConfigFileName] == constants.CompletedSelfInitConfig && len(configMap.Data) == 1 {
		return nil
	}

	updated := configMap.DeepCopy()
	updated.Data = map[string]string{
		constants.OpenBaoConfigFileName: constants.CompletedSelfInitConfig,
	}
	if _, err := configMaps.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return operatorerrors.WrapTransientKubernetesAPI(
			fmt.Errorf("failed to update self-init ConfigMap %s/%s while completing self-init: %w", cluster.Namespace, cmName, err),
		)
	}

	logger.Info("Reconciled inert self-init ConfigMap after self-initialization completed", "configmap", cmName)
	return nil
}

func (m *Manager) reconcileOperatorInit(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	pod *corev1.Pod,
	selfInitEnabled bool,
) (recon.Result, error) {
	if result := m.ensureTLSServerSecretReady(ctx, logger, cluster, pod); result != nil {
		return *result, nil
	}

	logger.Info("Attempting to initialize OpenBao cluster using HTTP API")
	emitNormalEvent(m.recorder, cluster, ReasonInitStarted, "Operator initialization started for cluster %s", cluster.Name)

	logging.LogAuditEvent(logger, logging.EventInitStarted, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"self_init_enabled": fmt.Sprintf("%t", selfInitEnabled),
	})

	if err := m.initializeCluster(ctx, logger, cluster); err != nil {
		if operatorerrors.IsTransient(err) {
			logger.Info("Initialization will be retried on next reconcile", "cluster", cluster.Name)
			return recon.Result{RequeueAfter: constants.RequeueShort}, nil
		}

		logger.Error(err, "Failed to initialize OpenBao cluster")
		logging.LogAuditEvent(logger, logging.EventInitFailed, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"error":             err.Error(),
		})
		emitWarningEvent(m.recorder, cluster, ReasonInitFailed, "Initialization failed for cluster %s: %v", cluster.Name, err)
		return recon.Result{}, fmt.Errorf("failed to initialize OpenBao cluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	cluster.Status.Initialized = true
	logger.Info("OpenBao cluster initialized successfully via HTTP API")

	logging.LogAuditEvent(logger, logging.EventInitCompleted, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"self_init_enabled": fmt.Sprintf("%t", selfInitEnabled),
	})
	emitNormalEvent(m.recorder, cluster, ReasonInitCompleted, "Initialization completed for cluster %s", cluster.Name)

	return recon.Result{RequeueAfter: constants.RequeueShort}, nil
}

func (m *Manager) ensureTLSServerSecretReady(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	pod *corev1.Pod,
) *recon.Result {
	tlsServerSecretName := cluster.Name + constants.SuffixTLSServer
	_, err := m.clientset.CoreV1().Secrets(cluster.Namespace).Get(ctx, tlsServerSecretName, metav1.GetOptions{})
	if err == nil {
		return nil
	}

	if apierrors.IsNotFound(err) {
		logger.Info("TLS server Secret not found yet; waiting for TLS reconciliation", "pod", pod.Name, "secret", tlsServerSecretName)
		result := recon.Result{RequeueAfter: constants.RequeueShort}
		return &result
	}

	logger.Info("Failed to check TLS server Secret (will retry)", "pod", pod.Name, "secret", tlsServerSecretName, "error", err)
	result := recon.Result{RequeueAfter: constants.RequeueShort}
	return &result
}
