package init

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/logging"
	"github.com/openbao/operator/internal/openbao"
)

const (
	// openBaoInitTimeout is the timeout for initialization operations
	openBaoInitTimeout = 30 * time.Second
	// openBaoAPIPort is the OpenBao HTTPS API port.
	openBaoAPIPort = 8200
	// tlsCASecretSuffix matches the CA Secret name suffix used by the TLS manager.
	tlsCASecretSuffix = "-tls-ca"
	// rootTokenSecretSuffix is appended to the cluster name to form the root token Secret name.
	rootTokenSecretSuffix = "-root-token"
	// rootTokenSecretKey is the key used to store the root token in the Secret data.
	rootTokenSecretKey = "token"
)

// Manager handles OpenBao cluster initialization.
type Manager struct {
	config    *rest.Config
	clientset kubernetes.Interface
}

// NewManager creates a new initialization Manager.
func NewManager(config *rest.Config, clientset kubernetes.Interface) *Manager {
	return &Manager{
		config:    config,
		clientset: clientset,
	}
}

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
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// If already initialized, nothing to do
	if cluster.Status.Initialized {
		logger.V(1).Info("OpenBao cluster is already initialized; skipping initialization")
		return nil
	}

	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	logger.Info("Checking if OpenBao cluster needs initialization",
		"namespace", cluster.Namespace,
		"name", cluster.Name,
		"selfInitEnabled", selfInitEnabled)

	// Find the first pod (should be pod-0 during initial creation)
	pod, err := m.findFirstPod(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to find pod for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if pod == nil {
		logger.Info("No pods found; waiting for pod to be created")
		return nil
	}

	logger.Info("Found pod for initialization", "pod", pod.Name, "phase", pod.Status.Phase)

	// Wait for container to be running (not necessarily ready, since readiness probe
	// may fail until OpenBao is initialized)
	if !isContainerRunning(pod) {
		logger.Info("Container not running yet; waiting", "pod", pod.Name, "phase", pod.Status.Phase)
		return nil
	}

	initializedLabel, hasInitializedLabel, err := openbao.ParseBoolLabel(pod.Labels, openbao.LabelInitialized)
	if err != nil {
		logger.V(1).Info("Invalid OpenBao initialized label value", "pod", pod.Name, "error", err)
	}

	sealedLabel, hasSealedLabel, err := openbao.ParseBoolLabel(pod.Labels, openbao.LabelSealed)
	if err != nil {
		logger.V(1).Info("Invalid OpenBao sealed label value", "pod", pod.Name, "error", err)
	}

	// Prefer Kubernetes service registration labels when available. This avoids
	// direct OpenBao API calls from the operator and matches the cluster's
	// observed state from OpenBao itself.
	if hasInitializedLabel && hasSealedLabel {
		if initializedLabel && !sealedLabel {
			logger.Info("OpenBao service registration labels indicate initialized and unsealed; marking cluster as initialized", "pod", pod.Name)
			cluster.Status.Initialized = true
			if selfInitEnabled {
				cluster.Status.SelfInitialized = true
			}
			return nil
		}
	}

	// When self-initialization is enabled, the operator does not (and in hardened
	// deployments often cannot) call the OpenBao API directly. Instead, we treat
	// pod readiness as the signal that OpenBao has finished self-initializing and
	// is unsealed. The infra manager's readiness probe is configured to only pass
	// once OpenBao is initialized and unsealed.
	if selfInitEnabled {
		if isPodReady(pod) {
			logger.Info("OpenBao pod is Ready; marking cluster as initialized", "pod", pod.Name)
			cluster.Status.Initialized = true
			cluster.Status.SelfInitialized = true
			return nil
		}

		logger.Info("Self-initialization is enabled; waiting for pod to become Ready", "pod", pod.Name)
		return nil
	}

	// Check if cluster is already initialized using the HTTP health endpoint.
	initialized, err := m.checkInitialized(ctx, logger, cluster)
	if err != nil {
		logger.V(1).Info("Failed to check initialization status (OpenBao may still be starting)", "pod", pod.Name, "error", err)
		return nil // Don't fail, will retry on next reconcile
	}

	if initialized {
		logger.Info("OpenBao cluster is already initialized", "pod", pod.Name, "selfInitEnabled", selfInitEnabled)
		// Update status to mark as initialized (this will trigger scaling in infra manager)
		cluster.Status.Initialized = true
		if selfInitEnabled {
			cluster.Status.SelfInitialized = true
		}
		return nil
	}

	// Cluster is not initialized - initialize it using the OpenBao HTTP API.
	logger.Info("Initializing OpenBao cluster using HTTP API")

	// Audit log: Cluster initialization operation
	logging.LogAuditEvent(logger, "Init", map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"self_init_enabled": fmt.Sprintf("%t", selfInitEnabled),
	})

	if err := m.initializeCluster(ctx, logger, cluster); err != nil {
		logger.Error(err, "Failed to initialize OpenBao cluster")
		logging.LogAuditEvent(logger, "InitFailed", map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"error":             err.Error(),
		})
		return fmt.Errorf("failed to initialize OpenBao cluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	// Mark cluster as initialized
	// Note: This modifies the cluster object in memory, and the controller will update the status
	cluster.Status.Initialized = true
	logger.Info("OpenBao cluster initialized successfully via HTTP API")

	// Audit log: Cluster initialization completed
	logging.LogAuditEvent(logger, "InitCompleted", map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"self_init_enabled": fmt.Sprintf("%t", selfInitEnabled),
	})

	return nil
}

func isPodReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

// findFirstPod finds the first pod (pod-0) for the given cluster.
// During initial cluster creation, this should be the only pod.
func (m *Manager) findFirstPod(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (*corev1.Pod, error) {
	podList, err := m.clientset.CoreV1().Pods(cluster.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set(map[string]string{
			"app.kubernetes.io/instance":   cluster.Name,
			"app.kubernetes.io/name":       "openbao",
			"app.kubernetes.io/managed-by": "openbao-operator",
		}).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Find pod-0 (the first pod, which should be the only one during initialization)
	for i := range podList.Items {
		pod := &podList.Items[i]
		// pod-0 is the first pod in a StatefulSet
		if strings.HasSuffix(pod.Name, "-0") {
			return pod, nil
		}
	}

	// If no pod-0 is found yet, return nil so that initialization can be retried
	// on the next reconcile once the StatefulSet controller creates the first pod.
	return nil, nil
}

// checkInitialized checks if OpenBao is initialized by querying the HTTP health endpoint.
// It connects to the pod-0 DNS name using the per-cluster TLS CA and inspects the
// initialized flag from /v1/sys/health.
func (m *Manager) checkInitialized(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	client, err := m.newOpenBaoClient(ctx, cluster)
	if err != nil {
		logger.V(1).Info("Failed to create OpenBao client for initialization check", "error", err)
		return false, nil
	}

	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	health, err := client.Health(healthCtx)
	if err != nil {
		// OpenBao may still be starting; treat this as "not initialized" and retry on the next reconcile.
		logger.V(1).Info("Failed to query OpenBao health during initialization check", "error", err)
		return false, nil
	}

	return health.Initialized, nil
}

// initializeCluster explicitly initializes OpenBao using the HTTP API (PUT /v1/sys/init).
// With static auto-unseal, this should rarely be needed, but we provide it as a fallback.
func (m *Manager) initializeCluster(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	client, err := m.newOpenBaoClient(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client for initialization: %w", err)
	}

	initCtx, cancel := context.WithTimeout(ctx, openBaoInitTimeout)
	defer cancel()

	// The operator always uses static auto-unseal, so we must not include
	// secret_shares and secret_threshold in the init request.
	// OpenBao will initialize using the static seal key configured in config.hcl.
	initResp, err := client.Init(initCtx, openbao.InitRequest{
		SecretShares:    nil,
		SecretThreshold: nil,
	})
	if err != nil {
		// If the cluster is already initialized, the API returns an error. Detect this
		// case via the error message and treat it as a no-op.
		if contains(err.Error(), "already initialized") {
			logger.Info("OpenBao cluster is already initialized (detected during HTTP init attempt)")
			return nil
		}

		return fmt.Errorf("failed to initialize OpenBao via HTTP API: %w", err)
	}

	if err := m.storeRootToken(ctx, logger, cluster, initResp.RootToken); err != nil {
		return err
	}

	logger.Info("OpenBao cluster initialized successfully via HTTP API")
	return nil
}

// newOpenBaoClient constructs a minimal OpenBao client for talking to the pod-0 instance
// of the StatefulSet using the per-cluster TLS CA bundle.
func (m *Manager) newOpenBaoClient(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (*openbao.Client, error) {
	if strings.TrimSpace(cluster.Name) == "" || strings.TrimSpace(cluster.Namespace) == "" {
		return nil, fmt.Errorf("cluster name and namespace are required to build OpenBao client")
	}

	// The server certificate SANs include pod DNS names and the headless Service name.
	// We target pod-0 directly for initialization.
	baseURL := fmt.Sprintf("https://%s-0.%s.%s.svc:%d", cluster.Name, cluster.Name, cluster.Namespace, openBaoAPIPort)

	// Load the per-cluster CA certificate generated by the TLS manager.
	caSecretName := cluster.Name + tlsCASecretSuffix
	secret, err := m.clientset.CoreV1().Secrets(cluster.Namespace).Get(ctx, caSecretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("TLS CA Secret %s/%s not found", cluster.Namespace, caSecretName)
		}
		return nil, fmt.Errorf("failed to get TLS CA Secret %s/%s: %w", cluster.Namespace, caSecretName, err)
	}

	caCert, ok := secret.Data["ca.crt"]
	if !ok || len(caCert) == 0 {
		return nil, fmt.Errorf("TLS CA Secret %s/%s missing 'ca.crt' key", cluster.Namespace, caSecretName)
	}

	client, err := openbao.NewClient(openbao.ClientConfig{
		BaseURL: baseURL,
		CACert:  caCert,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenBao client for %s: %w", baseURL, err)
	}

	return client, nil
}

func (m *Manager) storeRootToken(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, rootToken string) error {
	if strings.TrimSpace(rootToken) == "" {
		return nil
	}

	secretName := cluster.Name + rootTokenSecretSuffix

	secretsClient := m.clientset.CoreV1().Secrets(cluster.Namespace)

	existing, err := secretsClient.Get(ctx, secretName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get root token Secret %s/%s: %w", cluster.Namespace, secretName, err)
	}

	// Build labels for the secret
	secretLabels := map[string]string{
		"app.kubernetes.io/name":       "openbao",
		"app.kubernetes.io/instance":   cluster.Name,
		"app.kubernetes.io/managed-by": "openbao-operator",
		"openbao.org/cluster":          cluster.Name,
	}

	// Build OwnerReference for garbage collection when the OpenBaoCluster is deleted.
	// Note: We use controller=true to mark this as the controlling owner.
	blockOwnerDeletion := true
	controller := true
	ownerRef := metav1.OwnerReference{
		APIVersion:         cluster.APIVersion,
		Kind:               cluster.Kind,
		Name:               cluster.Name,
		UID:                cluster.UID,
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &controller,
	}

	if apierrors.IsNotFound(err) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            secretName,
				Namespace:       cluster.Namespace,
				Labels:          secretLabels,
				OwnerReferences: []metav1.OwnerReference{ownerRef},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				rootTokenSecretKey: []byte(rootToken),
			},
		}

		if _, createErr := secretsClient.Create(ctx, secret, metav1.CreateOptions{}); createErr != nil {
			return fmt.Errorf("failed to create root token Secret %s/%s: %w", cluster.Namespace, secretName, createErr)
		}

		return nil
	}

	if existing.Data == nil {
		existing.Data = make(map[string][]byte)
	}

	existing.Data[rootTokenSecretKey] = []byte(rootToken)

	// Ensure labels and owner reference are set on existing secret
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	for k, v := range secretLabels {
		existing.Labels[k] = v
	}

	// Add owner reference if not already present
	hasOwnerRef := false
	for _, ref := range existing.OwnerReferences {
		if ref.UID == cluster.UID {
			hasOwnerRef = true
			break
		}
	}
	if !hasOwnerRef {
		existing.OwnerReferences = append(existing.OwnerReferences, ownerRef)
	}

	if _, updateErr := secretsClient.Update(ctx, existing, metav1.UpdateOptions{}); updateErr != nil {
		return fmt.Errorf("failed to update root token Secret %s/%s: %w", cluster.Namespace, secretName, updateErr)
	}

	return nil
}

// isContainerRunning checks if the OpenBao container is running.
// This is used instead of isPodReady because the readiness probe may fail
// until OpenBao is initialized, creating a chicken-and-egg problem.
func isContainerRunning(pod *corev1.Pod) bool {
	// Check if pod is in Running phase
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	// Check if the openbao container is running
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == "openbao" {
			return status.State.Running != nil
		}
	}

	return false
}

// contains checks if a string contains a substring (case-insensitive).
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
