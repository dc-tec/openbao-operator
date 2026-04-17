package infra

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
	"github.com/dc-tec/openbao-operator/internal/adapter/kube"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
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
	acmeCacheVolumeName      = "acme-cache"
	kubeAPIAccessVolumeName  = "kube-api-access"
	configFileName           = "config.hcl"
	configTemplatePath       = "/etc/bao/config/config.hcl"
	configInitTemplatePath   = "/etc/bao/config-init/config.hcl"
	publicServiceSuffix      = "-public"
	acmeServiceSuffix        = "-acme"
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
	openBaoBinaryName        = constants.BinaryBao
	configHashAnnotation     = "openbao.org/config-hash"
	unsealTypeTransit        = "transit"
)

// Manager reconciles infrastructure resources such as ConfigMaps, StatefulSets, and Services for an OpenBaoCluster.
type Manager struct {
	client             client.Client
	reader             client.Reader
	scheme             *runtime.Scheme
	workload           *workloadsvc.Manager
	operatorNamespace  string
	oidcIssuer         string
	oidcDiscoveryURL   string
	oidcDiscoveryCAPEM string
	oidcJWKSURL        string
	oidcJWKSCAPEM      string
	oidcJWTKeys        []string
	Platform           string
}

// NewManager constructs a Manager that uses the provided Kubernetes client.
// The scheme is used to set OwnerReferences on created resources for garbage collection.
// operatorNamespace is the namespace where the operator is deployed, used for NetworkPolicy rules.
// oidcIssuer and oidcJWTKeys are the OIDC configuration discovered at operator startup.
func NewManager(c client.Client, scheme *runtime.Scheme, operatorNamespace string, oidcIssuer string, oidcJWTKeys []string, platform string) *Manager {
	return &Manager{
		client:            c,
		reader:            c,
		scheme:            scheme,
		workload:          workloadsvc.NewManager(c, scheme, platform),
		operatorNamespace: operatorNamespace,
		oidcIssuer:        oidcIssuer,
		oidcJWTKeys:       oidcJWTKeys,
		Platform:          platform,
	}
}

// NewManagerWithReader constructs a Manager with a dedicated reader.
// Use this when the controller-runtime client is backed by a namespace-scoped cache
// (e.g. single-tenant mode) but the operator must still read cluster/system resources
// outside the watched namespace (e.g. default/kubernetes Service).
func NewManagerWithReader(c client.Client, r client.Reader, scheme *runtime.Scheme, operatorNamespace string, oidcIssuer string, oidcJWTKeys []string, platform string) *Manager {
	m := NewManager(c, scheme, operatorNamespace, oidcIssuer, oidcJWTKeys, platform)
	if r != nil {
		m.reader = r
		m.workload.WithReader(r)
	}
	return m
}

// NewManagerWithReaderAndOIDCConfig constructs a Manager with a dedicated reader
// and overlays the runtime OIDC configuration when one is available.
func NewManagerWithReaderAndOIDCConfig(
	c client.Client,
	r client.Reader,
	scheme *runtime.Scheme,
	operatorNamespace string,
	oidcConfig *portauth.OIDCConfig,
	platform string,
) *Manager {
	issuer := ""
	var jwtKeys []string
	if oidcConfig != nil {
		issuer = oidcConfig.IssuerURL
		jwtKeys = oidcConfig.JWKSKeys
	}

	m := NewManagerWithReader(c, r, scheme, operatorNamespace, issuer, jwtKeys, platform)
	m.SetOIDCConfig(oidcConfig)
	return m
}

// SetOIDCConfig overlays dynamic JWT validation settings discovered at runtime.
// This preserves compatibility with older tests and call sites that still pass
// static JWT keys through the constructor while letting production code prefer
// dynamic jwks_url configuration when available.
func (m *Manager) SetOIDCConfig(config *portauth.OIDCConfig) {
	if m == nil || config == nil {
		return
	}
	m.oidcIssuer = config.IssuerURL
	m.oidcDiscoveryURL = config.OIDCDiscoveryURL
	m.oidcDiscoveryCAPEM = config.OIDCDiscoveryCAPEM
	m.oidcJWKSURL = config.JWKSURL
	m.oidcJWKSCAPEM = config.JWKSCAPEM
	m.oidcJWTKeys = append([]string(nil), config.JWKSKeys...)
}

// Reconcile ensures infrastructure resources are aligned with the desired state for the given OpenBaoCluster.
//
// The current implementation focuses on:
//   - Managing a per-cluster static auto-unseal Secret (only when using static seal).
//   - Rendering a config.hcl ConfigMap that injects TLS paths, storage configuration, retry_join, and seal configuration.
//   - Reconciling a headless StatefulSet-backed Service, an optional external Service/Ingress, and the StatefulSet itself.
//
// spec contains all parameters needed for StatefulSet reconciliation, including revision, images, and skip logic.
// This decouples the infrastructure layer from upgrade strategy knowledge.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, spec workloadsvc.StatefulSetSpec) error {
	// Only create unseal secret if using static seal (default or explicit)
	if usesStaticSeal(cluster) {
		if err := m.ensureUnsealSecret(ctx, logger, cluster); err != nil {
			return err
		}
	}

	if err := m.validateUnsealPrerequisites(ctx, cluster); err != nil {
		return err
	}

	if err := m.runACMEPreflight(ctx, logger, cluster); err != nil {
		return err
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

	if err := m.reconcilePreStatefulSet(ctx, logger, cluster, configContent); err != nil {
		return err
	}

	return m.workload.Reconcile(ctx, logger, cluster, configContent, spec)
}

func (m *Manager) reconcilePreStatefulSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, configContent string) error {
	if err := m.ensureConfigMap(ctx, logger, cluster, configContent); err != nil {
		return err
	}

	// Create a separate ConfigMap for self-init blocks (only mounted for pod-0).
	if err := m.ensureSelfInitConfigMap(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureHeadlessService(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureExternalService(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureACMEChallengeService(ctx, logger, cluster); err != nil {
		return err
	}

	if err := m.ensureACMESharedCachePVC(ctx, logger, cluster); err != nil {
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

	// Backup/restore/upgrade-snapshot Jobs are excluded from the primary pod
	// NetworkPolicy (they often need different egress). Ensure they still run
	// under an explicit policy.
	if err := m.ensureJobNetworkPolicy(ctx, logger, cluster); err != nil {
		return err
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
	applyConfig, err := kube.ToApplyConfiguration(obj, m.client)
	if err != nil {
		return fmt.Errorf("failed to convert object to ApplyConfiguration: %w", err)
	}

	// ROLLING UPGRADE SAFETY: For non-BlueGreen clusters, do not apply the StatefulSet
	// updateStrategy via SSA. The rolling upgrade manager owns RollingUpdate.Partition and
	// patches it via a strategic merge patch to orchestrate upgrades. Applying updateStrategy
	// here would risk clearing or overriding the partition and causing uncontrolled rollouts.
	if sts, ok := obj.(*appsv1.StatefulSet); ok &&
		(cluster.Spec.Upgrade == nil || cluster.Spec.Upgrade.Strategy != openbaov1alpha1.UpdateStrategyBlueGreen) {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
		if err != nil {
			return fmt.Errorf("failed to convert StatefulSet to unstructured: %w", err)
		}

		if spec, ok := u["spec"].(map[string]any); ok {
			delete(spec, "updateStrategy")
		}

		unstructuredObj := &unstructured.Unstructured{Object: u}
		gvk := sts.GetObjectKind().GroupVersionKind()
		if gvk.Empty() {
			gvk, err = m.client.GroupVersionKindFor(sts)
			if err != nil {
				return fmt.Errorf("failed to resolve GVK for StatefulSet: %w", err)
			}
		}
		unstructuredObj.SetGroupVersionKind(gvk)
		applyConfig = client.ApplyConfigurationFromUnstructured(unstructuredObj)
	}

	applyOpts := []client.ApplyOption{
		client.ForceOwnership,
		client.FieldOwner("openbao-operator"),
	}

	if err := m.client.Apply(ctx, applyConfig, applyOpts...); err != nil {
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

// Cleanup handles resources that require special deletion logic beyond Kubernetes Garbage Collection.
//
// Most infrastructure resources (StatefulSet, Services, ConfigMaps, RBAC, etc.) have OwnerReferences
// set to the OpenBaoCluster and are automatically deleted by Kubernetes GC when the cluster is deleted.
// This method only handles resources that need explicit policy-based handling:
//   - PVCs: Only deleted when DeletionPolicy is DeletePVCs or DeleteAll
//
// Note: Secret preservation for DeletionPolicy=Retain is handled by the deletion controller
// (deletion.go orphanSecretsForRetention) which removes OwnerReferences before finalization.
//
// It is safe to call Cleanup multiple times; missing resources are treated as successfully deleted.
func (m *Manager) Cleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, policy openbaov1alpha1.DeletionPolicy) error {
	if policy == "" {
		policy = openbaov1alpha1.DeletionPolicyRetain
	}

	logger = logger.WithValues("deletionPolicy", string(policy))
	logger.Info("Processing cleanup for deleted OpenBaoCluster",
		"note", "Most resources are deleted by Kubernetes GC via OwnerReferences")

	// PVCs require explicit deletion based on policy because they are not owned by the
	// StatefulSet (they use volumeClaimTemplates which creates independent PVCs).
	// Kubernetes GC does not automatically delete these when the OpenBaoCluster is deleted.
	if policy == openbaov1alpha1.DeletionPolicyDeletePVCs || policy == openbaov1alpha1.DeletionPolicyDeleteAll {
		if err := m.deletePVCs(ctx, cluster); err != nil {
			return fmt.Errorf("failed to delete PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}
		logger.Info("PVCs deleted per deletion policy")
	} else {
		logger.Info("Preserving PVCs per Retain policy")
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
func configInitMapName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + configInitMapSuffix
}

func tlsServerSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixTLSServer
}

func headlessServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name
}

func externalServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + publicServiceSuffix
}

func acmeServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + acmeServiceSuffix
}

func externalServiceNameBlue(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return externalServiceName(cluster) + "-blue"
}

func externalServiceNameGreen(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return externalServiceName(cluster) + "-green"
}

// statefulSetNameWithRevision returns the StatefulSet name for a given revision.
// If rev is empty, returns the cluster name (for backward compatibility).
// Otherwise, returns "<cluster-name>-<revision>".
