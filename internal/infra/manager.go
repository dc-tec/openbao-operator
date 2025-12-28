package infra

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	configbuilder "github.com/openbao/operator/internal/config"
	"github.com/openbao/operator/internal/constants"
	operatorerrors "github.com/openbao/operator/internal/errors"
	"github.com/openbao/operator/internal/revision"
)

const (
	configInitMapSuffix      = "-config-init"
	unsealSecretKey          = "key"
	unsealKeyBytes           = 32
	dataVolumeName           = constants.VolumeData
	tlsVolumeName            = constants.VolumeTLS
	configVolumeName         = constants.VolumeConfig
	configInitVolumeName     = "config-init"
	configRenderedVolumeName = "config-rendered"
	unsealVolumeName         = "unseal"
	tmpVolumeName            = "tmp"
	utilsVolumeName          = "utils"
	kubeAPIAccessVolumeName  = "kube-api-access"
	configFileName           = "config.hcl"
	configTemplatePath       = "/etc/bao/config/config.hcl"
	configInitTemplatePath   = "/etc/bao/config-init/config.hcl"
	publicServiceSuffix      = "-public"
	httpRouteSuffix          = "-httproute"
	tlsRouteSuffix           = "-tlsroute"
	backendTLSPolicySuffix   = "-backend-tls-policy"
	openBaoConfigMountPath   = constants.PathConfig
	openBaoRenderedConfig    = "/etc/bao/rendered-config/config.hcl"
	openBaoTLSMountPath      = constants.PathTLS
	openBaoUnsealMountPath   = "/etc/bao/unseal"
	openBaoDataPath          = constants.PathData
	serviceAccountMountPath  = "/var/run/secrets/kubernetes.io/serviceaccount"
	kubeRootCAConfigMapName  = "kube-root-ca.crt"
	openBaoBinaryName        = constants.BinaryNameOpenBao
	configHashAnnotation     = "openbao.org/config-hash"

	// OpenBao images are built to run as a non-root user with stable UID/GID.
	// The StatefulSet security context pins these IDs so that both the main
	// OpenBao container and the config-init container run as non-root even if
	// the image metadata defaults to root.
	openBaoUserID  = constants.UserOpenBao
	openBaoGroupID = constants.GroupOpenBao

	// File permissions: 0440 for secrets (owner/group read-only), 0644 for configs
	secretFileMode = int32(0440)
	// serviceAccountFileMode matches the secret file mode for projected tokens/CA.
	serviceAccountFileMode = int32(0440)
	// serviceAccountTokenExpirationSeconds is the projected token TTL.
	serviceAccountTokenExpirationSeconds = int64(3600)
)

// Manager reconciles infrastructure resources such as ConfigMaps, StatefulSets, and Services for an OpenBaoCluster.
type Manager struct {
	client            client.Client
	scheme            *runtime.Scheme
	operatorNamespace string
	oidcIssuer        string
	oidcJWTKeys       []string
}

// NewManager constructs a Manager that uses the provided Kubernetes client.
// The scheme is used to set OwnerReferences on created resources for garbage collection.
// operatorNamespace is the namespace where the operator is deployed, used for NetworkPolicy rules.
// oidcIssuer and oidcJWTKeys are the OIDC configuration discovered at operator startup.
func NewManager(c client.Client, scheme *runtime.Scheme, operatorNamespace string, oidcIssuer string, oidcJWTKeys []string) *Manager {
	return &Manager{
		client:            c,
		scheme:            scheme,
		operatorNamespace: operatorNamespace,
		oidcIssuer:        oidcIssuer,
		oidcJWTKeys:       oidcJWTKeys,
	}
}

// Reconcile ensures infrastructure resources are aligned with the desired state for the given OpenBaoCluster.
//
// The current implementation focuses on:
//   - Managing a per-cluster static auto-unseal Secret (only when using static seal).
//   - Rendering a config.hcl ConfigMap that injects TLS paths, storage configuration, retry_join, and seal configuration.
//   - Reconciling a headless StatefulSet-backed Service, an optional external Service/Ingress, and the StatefulSet itself.
//   - Managing the Sentinel sidecar controller for drift detection (if enabled).
//
// verifiedImageDigest is the verified image digest (e.g., "openbao/openbao@sha256:abc...") from image verification.
// If provided, it will be used instead of cluster.Spec.Image to prevent TOCTOU attacks.
// If empty, cluster.Spec.Image will be used.
// verifiedSentinelDigest is the verified Sentinel image digest (if provided).
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string, verifiedSentinelDigest string) error {
	// Only create unseal secret if using static seal (default or explicit)
	if usesStaticSeal(cluster) {
		if err := m.ensureUnsealSecret(ctx, logger, cluster); err != nil {
			return err
		}
	}

	infraDetails := configbuilder.InfrastructureDetails{
		HeadlessServiceName: headlessServiceName(cluster),
		Namespace:           cluster.Namespace,
		APIPort:             constants.PortAPI,
		ClusterPort:         constants.PortCluster,
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

	if err := m.ensureTLSRoute(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureGatewayCAConfigMap(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureBackendTLSPolicy(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureServiceAccount(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureRBAC(ctx, logger, cluster); err != nil {
		return err
	}

	// CRITICAL: Create NetworkPolicy BEFORE StatefulSet to ensure pods boot up
	// in a protected state. This prevents a race condition where pods could
	// be running without network restrictions.
	if err := m.ensureNetworkPolicy(ctx, logger, cluster); err != nil {
		return err
	}

	// Determine if we're in blue/green mode and need revision-based naming
	revisionName := ""
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen {
		// During blue/green deployments, the active ("Blue") StatefulSet is revisioned.
		// Use the stored BlueRevision when available so infra continues to reconcile Blue
		// after the user updates spec.version/spec.image to start an upgrade.
		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueRevision != "" {
			// Skip StatefulSet creation during DemotingBlue/Cleanup phases.
			// The BlueGreenManager handles StatefulSet lifecycle during these phases.
			// If we don't skip, we'll keep recreating the old Blue StatefulSet that's being deleted.
			if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseDemotingBlue ||
				cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseCleanup {
				logger.Info("Skipping Blue StatefulSet reconciliation during cleanup phase",
					"phase", cluster.Status.BlueGreen.Phase,
					"blueRevision", cluster.Status.BlueGreen.BlueRevision)
				return nil
			}
			revisionName = cluster.Status.BlueGreen.BlueRevision
		} else {
			// Bootstrap revision for initial reconciliation before BlueGreen status is initialized.
			revisionName = revision.OpenBaoClusterRevision(cluster.Spec.Version, cluster.Spec.Image, cluster.Spec.Replicas)
		}
	}

	if err := m.EnsureStatefulSetWithRevision(ctx, logger, cluster, configContent, verifiedImageDigest, revisionName, false); err != nil {
		return err
	}

	// Manage Sentinel deployment
	if cluster.Spec.Sentinel != nil && cluster.Spec.Sentinel.Enabled {
		if err := m.ensureSentinelServiceAccount(ctx, logger, cluster); err != nil {
			return err
		}
		if err := m.ensureSentinelRBAC(ctx, logger, cluster); err != nil {
			return err
		}
		if err := m.ensureSentinelDeployment(ctx, logger, cluster, verifiedSentinelDigest); err != nil {
			return err
		}
	} else {
		// Sentinel is disabled, clean up resources
		if err := m.ensureSentinelCleanup(ctx, logger, cluster); err != nil {
			return err
		}
	}

	return nil
}

// applyResource uses Server-Side Apply to create or update a Kubernetes resource.
// This eliminates the need for Get-then-Create-or-Update logic and manual diffing.
//
// The resource must have TypeMeta, ObjectMeta (with Name and Namespace), and the desired Spec set.
// Owner references are set automatically if the resource supports them.
func (m *Manager) applyResource(ctx context.Context, obj client.Object, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Set owner reference for garbage collection
	if err := controllerutil.SetControllerReference(cluster, obj, m.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Use Server-Side Apply with ForceOwnership to ensure the operator manages this resource
	patchOpts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner("openbao-operator"),
	}

	if err := m.client.Patch(ctx, obj, client.Apply, patchOpts...); err != nil {
		// Wrap transient Kubernetes API errors (rate limiting, temporary failures)
		if operatorerrors.IsTransientKubernetesAPI(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		// Check for conflict errors which are typically transient
		if apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
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

	// Delete pods first to ensure they're cleaned up even if StatefulSet deletion
	// is blocked or pods become orphaned. The operator can delete pods it manages.
	if err := m.deletePods(ctx, logger, cluster); err != nil {
		return fmt.Errorf("failed to delete pods for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

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
		constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:    cluster.Name,
		constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster: cluster.Name,
	}
}

func podSelectorLabels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	return podSelectorLabelsWithRevision(cluster, "")
}

// podSelectorLabelsWithRevision returns pod selector labels including the revision label.
// If rev is empty, returns base labels (for backward compatibility).
// Otherwise, includes the revision label for blue/green deployments.
func podSelectorLabelsWithRevision(cluster *openbaov1alpha1.OpenBaoCluster, rev string) map[string]string {
	labels := infraLabels(cluster)
	if rev != "" {
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[constants.LabelOpenBaoRevision] = rev
	}
	return labels
}

func unsealSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixUnsealKey
}

func configMapName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixConfigMap
}

// configMapNameWithRevision returns the ConfigMap name for a given revision.
// If rev is empty, returns the cluster's base ConfigMap name.
// Otherwise, returns "<cluster-name>-config-<revision>".
func configMapNameWithRevision(cluster *openbaov1alpha1.OpenBaoCluster, rev string) string {
	if rev == "" {
		return configMapName(cluster)
	}
	return fmt.Sprintf("%s%s-%s", cluster.Name, constants.SuffixConfigMap, rev)
}

func configInitMapName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + configInitMapSuffix
}

func tlsServerSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixTLSServer
}

func tlsCASecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixTLSCA
}

func headlessServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name
}

func externalServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + publicServiceSuffix
}

func statefulSetName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return statefulSetNameWithRevision(cluster, "")
}

// statefulSetNameWithRevision returns the StatefulSet name for a given revision.
// If rev is empty, returns the cluster name (for backward compatibility).
// Otherwise, returns "<cluster-name>-<revision>".
func statefulSetNameWithRevision(cluster *openbaov1alpha1.OpenBaoCluster, rev string) string {
	if rev == "" {
		return cluster.Name
	}
	return fmt.Sprintf("%s-%s", cluster.Name, rev)
}

func int32Ptr(v int32) *int32 {
	return &v
}
