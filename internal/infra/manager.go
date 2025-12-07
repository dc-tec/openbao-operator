package infra

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	configbuilder "github.com/openbao/operator/internal/config"
)

const (
	configMapSuffix          = "-config"
	configInitMapSuffix      = "-config-init"
	unsealSecretSuffix       = "-unseal-key"
	unsealSecretKey          = "key"
	unsealKeyBytes           = 32
	dataVolumeName           = "data"
	tlsVolumeName            = "tls"
	configVolumeName         = "config"
	configInitVolumeName     = "config-init"
	configRenderedVolumeName = "config-rendered"
	unsealVolumeName         = "unseal"
	tmpVolumeName            = "tmp"
	configFileName           = "config.hcl"
	configTemplatePath       = "/etc/bao/config/config.hcl"
	configInitTemplatePath   = "/etc/bao/config-init/config.hcl"
	publicServiceSuffix      = "-public"
	httpRouteSuffix          = "-httproute"
	tlsServerSecretSuffix    = "-tls-server"
	tlsCASecretSuffix        = "-tls-ca"
	openBaoContainerName     = "openbao"
	openBaoContainerPort     = 8200
	openBaoClusterPort       = 8201
	openBaoConfigMountPath   = "/etc/bao/config"
	openBaoRenderedConfig    = "/etc/bao/rendered-config/config.hcl"
	openBaoTLSMountPath      = "/etc/bao/tls"
	openBaoUnsealMountPath   = "/etc/bao/unseal"
	openBaoDataPath          = "/bao/data"
	openBaoBinaryName        = "bao"
	openBaoHealthPath        = "/v1/sys/health?standbyok=true&sealedcode=204&uninitcode=204"
	serviceAccountSuffix     = "-serviceaccount"
	configHashAnnotation     = "openbao.org/config-hash"

	labelAppName        = "app.kubernetes.io/name"
	labelAppInstance    = "app.kubernetes.io/instance"
	labelAppManagedBy   = "app.kubernetes.io/managed-by"
	labelOpenBaoCluster = "openbao.org/cluster"

	// OpenBao images are built to run as a non-root user with stable UID/GID.
	// The StatefulSet security context pins these IDs so that both the main
	// OpenBao container and the config-init container run as non-root even if
	// the image metadata defaults to root.
	openBaoUserID  = int64(100)
	openBaoGroupID = int64(1000)

	// File permissions: 0440 for secrets (owner/group read-only), 0644 for configs
	secretFileMode = int32(0440)
)

// Manager reconciles infrastructure resources such as ConfigMaps, StatefulSets, and Services for an OpenBaoCluster.
type Manager struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewManager constructs a Manager that uses the provided Kubernetes client.
// The scheme is used to set OwnerReferences on created resources for garbage collection.
func NewManager(c client.Client, scheme *runtime.Scheme) *Manager {
	return &Manager{
		client: c,
		scheme: scheme,
	}
}

// Reconcile ensures infrastructure resources are aligned with the desired state for the given OpenBaoCluster.
//
// The current implementation focuses on:
//   - Managing a per-cluster static auto-unseal Secret (only when using static seal).
//   - Rendering a config.hcl ConfigMap that injects TLS paths, storage configuration, retry_join, and seal configuration.
//   - Reconciling a headless StatefulSet-backed Service, an optional external Service/Ingress, and the StatefulSet itself.
//
// verifiedImageDigest is the verified image digest (e.g., "openbao/openbao@sha256:abc...") from image verification.
// If provided, it will be used instead of cluster.Spec.Image to prevent TOCTOU attacks.
// If empty, cluster.Spec.Image will be used.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string) error {
	// Only create unseal secret if using static seal (default or explicit)
	if usesStaticSeal(cluster) {
		if err := m.ensureUnsealSecret(ctx, logger, cluster); err != nil {
			return err
		}
	}

	infraDetails := configbuilder.InfrastructureDetails{
		HeadlessServiceName: headlessServiceName(cluster),
		Namespace:           cluster.Namespace,
		APIPort:             openBaoContainerPort,
		ClusterPort:         openBaoClusterPort,
	}

	renderedConfig, err := configbuilder.RenderHCL(cluster, infraDetails)
	if err != nil {
		return fmt.Errorf("failed to render config.hcl for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	configContent := string(renderedConfig)

	if err := m.ensureConfigMap(ctx, logger, cluster, configContent); err != nil {
		return err
	}

	// Create a separate ConfigMap for self-init blocks (only mounted for pod-0)
	if err := m.ensureSelfInitConfigMap(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureHeadlessService(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureExternalService(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureIngress(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureHTTPRoute(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureGatewayCAConfigMap(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureServiceAccount(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureRBAC(ctx, logger, cluster); err != nil {
		return err
	}

	// Check if Kubernetes Auth RBAC is configured (operator does not create it automatically
	// due to security/compliance considerations - users must create it manually)
	if err := m.checkKubernetesAuthRBAC(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureNetworkPolicy(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureStatefulSet(ctx, logger, cluster, configContent, verifiedImageDigest); err != nil {
		return err
	}

	return nil
}

// Cleanup removes Kubernetes infrastructure resources associated with the given OpenBaoCluster according to the provided DeletionPolicy.
//
// It is safe to call Cleanup multiple times; missing resources are treated as successfully deleted.
func (m *Manager) Cleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, policy openbaov1alpha1.DeletionPolicy) error {
	if policy == "" {
		policy = openbaov1alpha1.DeletionPolicyRetain
	}

	logger = logger.WithValues("deletionPolicy", string(policy))
	logger.Info("Cleaning up infrastructure for deleted OpenBaoCluster")

	if err := m.deleteStatefulSet(ctx, cluster); err != nil {
		return fmt.Errorf("failed to delete StatefulSet for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if err := m.deleteConfigMap(ctx, cluster); err != nil {
		return fmt.Errorf("failed to delete config ConfigMap for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if err := m.deleteServices(ctx, cluster); err != nil {
		return fmt.Errorf("failed to delete Services for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if err := m.deleteIngress(ctx, cluster); err != nil {
		return fmt.Errorf("failed to delete Ingress for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if err := m.deleteHTTPRoute(ctx, cluster); err != nil {
		return fmt.Errorf("failed to delete HTTPRoute for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if err := m.deleteNetworkPolicy(ctx, cluster); err != nil {
		return fmt.Errorf("failed to delete NetworkPolicy for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	// CRITICAL: Only delete secrets if we are NOT retaining data.
	// If DeletionPolicy is Retain, we must preserve the unseal key along with the PVCs,
	// otherwise the encrypted data becomes unrecoverable.
	if policy != openbaov1alpha1.DeletionPolicyRetain {
		if err := m.deleteSecrets(ctx, cluster); err != nil {
			return fmt.Errorf("failed to delete Secrets for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}
	} else {
		logger.Info("Skipping Secret deletion due to Retain policy; unseal keys must be preserved with data")
	}

	// Note: We don't delete the ClusterRoleBinding automatically since we don't create it.
	// Users are responsible for cleaning up cluster-scoped RBAC resources.

	if err := m.deleteRBAC(ctx, cluster); err != nil {
		return fmt.Errorf("failed to delete RBAC for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if err := m.deleteServiceAccount(ctx, cluster); err != nil {
		return fmt.Errorf("failed to delete ServiceAccount for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if policy == openbaov1alpha1.DeletionPolicyDeletePVCs || policy == openbaov1alpha1.DeletionPolicyDeleteAll {
		if err := m.deletePVCs(ctx, cluster); err != nil {
			return fmt.Errorf("failed to delete PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}
	}

	return nil
}

// Helper functions used across multiple files

func infraLabels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	return map[string]string{
		labelAppName:        "openbao",
		labelAppInstance:    cluster.Name,
		labelAppManagedBy:   "openbao-operator",
		labelOpenBaoCluster: cluster.Name,
	}
}

func podSelectorLabels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	return infraLabels(cluster)
}

func unsealSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + unsealSecretSuffix
}

func configMapName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + configMapSuffix
}

func configInitMapName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + configInitMapSuffix
}

func tlsServerSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + tlsServerSecretSuffix
}

func tlsCASecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + tlsCASecretSuffix
}

func headlessServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name
}

func externalServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + publicServiceSuffix
}

func statefulSetName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name
}

func int32Ptr(v int32) *int32 {
	return &v
}
