//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	e2ehelpers "github.com/dc-tec/openbao-operator/test/e2e/helpers"
)

const (
	claimE2EEnableEnv                     = "E2E_ENABLE_SERVICE_CLAIMS"
	claimE2EAPIServerEndpointIPsEnv       = "E2E_SERVICE_CLAIMS_API_SERVER_ENDPOINT_IPS"
	claimE2EDNSEndpointIPsEnv             = "E2E_SERVICE_CLAIMS_DNS_ENDPOINT_IPS"
	claimBootstrapAuthSecretKeyHost       = "kubernetes_host"
	claimBootstrapAuthSecretKeyIssuer     = "issuer"
	claimBootstrapAuthSecretDefaultMount  = "kubernetes"
	claimBootstrapSecretEngineDefaultType = "kv"
	claimBootstrapSecretEngineDefaultPath = "secret"
)

func serviceClaimsE2EEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(claimE2EEnableEnv)), "true")
}

func claimTrustedIngressNetworkProfile(name string) *openbaov1alpha1.OpenBaoNetworkProfile {
	return &openbaov1alpha1.OpenBaoNetworkProfile{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openbaov1alpha1.OpenBaoNetworkProfileSpec{
			TrustedIngressPeers: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": "default",
					},
				},
			}},
		},
	}
}

func attachClaimNetworkProfile(profile *openbaov1alpha1.OpenBaoServiceProfile, networkProfileName string) {
	profile.Spec.Network = &openbaov1alpha1.OpenBaoServiceProfileNetworkSpec{
		ProfileRef: &openbaov1alpha1.LocalReference{Name: networkProfileName},
	}
}

func ensureClaimRustFS(ctx context.Context, c client.Client, restCfg *rest.Config, namespace string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	if err := c.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create claim RustFS namespace: %w", err)
	}

	cfg := e2ehelpers.DefaultRustFSConfig()
	cfg.Namespace = namespace
	cfg.Name = rustfsName
	cfg.AccessKey = rustfsAccessKey
	cfg.SecretKey = rustfsSecretKey
	cfg.Buckets = []string{rustfsBucket}

	if err := e2ehelpers.EnsureRustFS(ctx, c, restCfg, cfg); err != nil {
		return fmt.Errorf("failed to deploy claim RustFS: %w", err)
	}
	return nil
}

func operatorJWTAudienceForClaimsE2E() string {
	audience := strings.TrimSpace(os.Getenv("OPENBAO_JWT_AUDIENCE"))
	if audience == "" {
		return "openbao-internal"
	}
	return audience
}

func claimScopedName(prefix, suffix string) string {
	name := strings.ToLower(strings.TrimSpace(prefix + "-" + suffix))
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, ".", "-")
	if len(name) > 63 {
		name = name[:63]
	}
	return strings.Trim(name, "-")
}

type sameClusterClaimCatalog struct {
	OfferingName       string
	ServiceProfileName string
	BootstrapName      string
	ExposureName       string
	BackupProfileName  string
	BackupTargetName   string
	BackupBackendName  string
	AuthSecretName     string
}

func newSameClusterClaimCatalog(scope string) sameClusterClaimCatalog {
	return sameClusterClaimCatalog{
		OfferingName:       claimScopedName("offering", scope),
		ServiceProfileName: claimScopedName("service", scope),
		BootstrapName:      claimScopedName("bootstrap", scope),
		ExposureName:       claimScopedName("exposure", scope),
		BackupProfileName:  claimScopedName("backup", scope),
		BackupTargetName:   claimScopedName("backup-target", scope),
		BackupBackendName:  claimScopedName("backup-backend", scope),
		AuthSecretName:     claimScopedName("authcfg", scope),
	}
}

func (c sameClusterClaimCatalog) bootstrapAuthSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.AuthSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			claimBootstrapAuthSecretKeyHost:   []byte("https://kubernetes.default.svc"),
			claimBootstrapAuthSecretKeyIssuer: []byte("https://kubernetes.default.svc.cluster.local"),
		},
	}
}

func (c sameClusterClaimCatalog) backupProfile() *openbaov1alpha1.OpenBaoBackupProfile {
	return &openbaov1alpha1.OpenBaoBackupProfile{
		ObjectMeta: metav1.ObjectMeta{Name: c.BackupProfileName},
	}
}

func (c sameClusterClaimCatalog) backupTarget() *openbaov1alpha1.OpenBaoBackupTarget {
	return &openbaov1alpha1.OpenBaoBackupTarget{
		ObjectMeta: metav1.ObjectMeta{Name: c.BackupTargetName},
		Spec: openbaov1alpha1.OpenBaoBackupTargetSpec{
			BackendRef: openbaov1alpha1.LocalReference{Name: c.BackupBackendName},
			LocationPolicy: openbaov1alpha1.OpenBaoBackupLocationPolicySpec{
				Location: openbaov1alpha1.OpenBaoBackupLocationSelectionSpec{
					Mode:  openbaov1alpha1.OpenBaoBackupLocationModeFixed,
					Value: "claims-e2e",
				},
				KeyPrefix: openbaov1alpha1.OpenBaoBackupKeyPrefixPolicySpec{
					Template: "claims/{{ claim.namespace }}/{{ claim.name }}",
				},
			},
		},
	}
}

func (c sameClusterClaimCatalog) backupBackend() *openbaov1alpha1.OpenBaoBackupBackend {
	return &openbaov1alpha1.OpenBaoBackupBackend{
		ObjectMeta: metav1.ObjectMeta{Name: c.BackupBackendName},
		Spec: openbaov1alpha1.OpenBaoBackupBackendSpec{
			Driver: openbaov1alpha1.OpenBaoBackupBackendDriverObjectStorage,
			ObjectStorage: &openbaov1alpha1.OpenBaoBackupBackendObjectStorageSpec{
				Provider:     openbaov1alpha1.OpenBaoObjectStorageProviderS3,
				Endpoint:     "https://s3.example.internal",
				Region:       "eu-west-1",
				UsePathStyle: true,
			},
		},
	}
}

func (c sameClusterClaimCatalog) internalExposureClass() *openbaov1alpha1.OpenBaoExposureClass {
	return &openbaov1alpha1.OpenBaoExposureClass{
		ObjectMeta: metav1.ObjectMeta{Name: c.ExposureName},
		Spec: openbaov1alpha1.OpenBaoExposureClassSpec{
			PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeClusterInternal,
			HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
				Mode: openbaov1alpha1.OpenBaoExposureHostnamePolicyModeGenerated,
			},
			ServicePolicy: &openbaov1alpha1.OpenBaoExposureServicePolicySpec{
				Type:           openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP,
				BackendTLSMode: openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired,
			},
		},
	}
}

func (c sameClusterClaimCatalog) secretBootstrapProfile() *openbaov1alpha1.OpenBaoBootstrapProfile {
	return &openbaov1alpha1.OpenBaoBootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: c.BootstrapName},
		Spec: openbaov1alpha1.OpenBaoBootstrapProfileSpec{
			OperatorLifecycleAuth: openbaov1alpha1.OpenBaoBootstrapLifecycleAuthSpec{
				Mode: openbaov1alpha1.OpenBaoBootstrapLifecycleAuthModeJWT,
				JWT:  &openbaov1alpha1.OpenBaoBootstrapLifecycleJWTSpec{Audience: operatorJWTAudienceForClaimsE2E()},
			},
			Auth: &openbaov1alpha1.OpenBaoBootstrapAuthSpec{
				Methods: []openbaov1alpha1.OpenBaoBootstrapAuthMethodSpec{{
					Type: "kubernetes",
					Path: claimBootstrapAuthSecretDefaultMount,
					ConfigRef: &openbaov1alpha1.TypedObjectReference{
						Kind: "Secret",
						Name: c.AuthSecretName,
					},
				}},
			},
			SecretEngines: &openbaov1alpha1.OpenBaoBootstrapSecretEnginesSpec{
				Mounts: []openbaov1alpha1.OpenBaoBootstrapSecretEngineMountSpec{{
					Type: claimBootstrapSecretEngineDefaultType,
					Path: claimBootstrapSecretEngineDefaultPath,
				}},
			},
		},
	}
}

func (c sameClusterClaimCatalog) serviceProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	preUpgradeSnapshot := false

	return &openbaov1alpha1.OpenBaoServiceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: c.ServiceProfileName},
		Spec: openbaov1alpha1.OpenBaoServiceProfileSpec{
			Cluster: openbaov1alpha1.OpenBaoServiceProfileClusterSpec{
				Version:         openBaoVersion,
				Voters:          1,
				SecurityProfile: openbaov1alpha1.ProfileDevelopment,
			},
			Storage: openbaov1alpha1.OpenBaoServiceProfileStorageSpec{
				PrimarySize: "5Gi",
			},
			Bootstrap: openbaov1alpha1.OpenBaoServiceProfileBootstrapSpec{
				Mode:       openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
				ProfileRef: &openbaov1alpha1.LocalReference{Name: c.BootstrapName},
			},
			Exposure: openbaov1alpha1.OpenBaoServiceProfileExposureSpec{
				ClassRef: openbaov1alpha1.LocalReference{Name: c.ExposureName},
			},
			Backup: openbaov1alpha1.OpenBaoServiceProfileBackupSpec{
				ProfileRef: openbaov1alpha1.LocalReference{Name: c.BackupProfileName},
			},
			Lifecycle: openbaov1alpha1.OpenBaoServiceProfileLifecycleSpec{
				UpgradeStrategy:    openbaov1alpha1.UpdateStrategyRollingUpdate,
				PreUpgradeSnapshot: &preUpgradeSnapshot,
			},
		},
	}
}

func (c sameClusterClaimCatalog) serviceOffering() *openbaov1alpha1.OpenBaoServiceOffering {
	return &openbaov1alpha1.OpenBaoServiceOffering{
		ObjectMeta: metav1.ObjectMeta{Name: c.OfferingName},
		Spec: openbaov1alpha1.OpenBaoServiceOfferingSpec{
			CurrentRevisionRef: openbaov1alpha1.LocalReference{Name: c.ServiceProfileName},
		},
	}
}

func (c sameClusterClaimCatalog) sameClusterClaim(namespace, name, tenantName string) *openbaov1alpha1.OpenBaoClusterClaim {
	return &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:          openbaov1alpha1.LocalReference{Name: tenantName},
			ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: c.OfferingName},
		},
	}
}

func createObjects(ctx context.Context, c client.Client, objects ...client.Object) error {
	for _, obj := range objects {
		if err := c.Create(ctx, obj); err != nil {
			return fmt.Errorf("create %T %s: %w", obj, client.ObjectKeyFromObject(obj), err)
		}
	}
	return nil
}

func deleteObjects(ctx context.Context, c client.Client, objects ...client.Object) error {
	for _, obj := range objects {
		if err := c.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete %T %s: %w", obj, client.ObjectKeyFromObject(obj), err)
		}
	}
	return nil
}

//nolint:unparam // Kept configurable so call sites can state their E2E wait contract.
func waitForClaimPhase(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	expected openbaov1alpha1.OpenBaoClusterClaimPhase,
	timeout time.Duration,
	pollInterval time.Duration,
) (*openbaov1alpha1.OpenBaoClusterClaim, error) {
	return waitForClaim(ctx, c, namespace, name, timeout, pollInterval, func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
		return claim.Status.Phase == expected, nil
	})
}

//nolint:unparam // Kept configurable so call sites can state their E2E wait contract.
func waitForClaimPinnedBinding(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	expectedOffering string,
	expectedProfile string,
	timeout time.Duration,
	pollInterval time.Duration,
) (*openbaov1alpha1.OpenBaoClusterClaim, error) {
	return waitForClaim(ctx, c, namespace, name, timeout, pollInterval, func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
		if claim.Status.Applied.ServiceOfferingRef == nil || claim.Status.Applied.ServiceProfileRef == nil {
			return false, nil
		}
		return claim.Status.Applied.ServiceOfferingRef.Name == expectedOffering &&
			claim.Status.Applied.ServiceProfileRef.Name == expectedProfile, nil
	})
}

func waitForClaimEndpoint(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	expectedEndpoint string,
	timeout time.Duration,
	pollInterval time.Duration,
) (*openbaov1alpha1.OpenBaoClusterClaim, error) {
	return waitForClaim(ctx, c, namespace, name, timeout, pollInterval, func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
		return claim.Status.Phase == openbaov1alpha1.OpenBaoClusterClaimPhaseReady &&
			claim.Status.Connection.Endpoint == expectedEndpoint, nil
	})
}

func waitForClaimConnectionSecret(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	timeout time.Duration,
	pollInterval time.Duration,
) (*corev1.Secret, error) {
	var secretRef *openbaov1alpha1.LocalReference
	_, err := waitForClaim(ctx, c, namespace, name, timeout, pollInterval, func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
		if claim.Status.Connection.SecretRef == nil || claim.Status.Connection.Endpoint == "" {
			return false, nil
		}
		secretCopy := *claim.Status.Connection.SecretRef
		secretRef = &secretCopy
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{}
	if err := waitForObjectDeletionOrPresence(ctx, timeout, pollInterval, func() error {
		return c.Get(ctx, types.NamespacedName{Name: secretRef.Name, Namespace: namespace}, secret)
	}, false); err != nil {
		return nil, err
	}

	return secret, nil
}

//nolint:unparam // Kept configurable so call sites can state their E2E wait contract.
func waitForClaimLocalClusterRef(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	timeout time.Duration,
	pollInterval time.Duration,
) (*openbaov1alpha1.NamespacedReference, error) {
	claim, err := waitForClaim(ctx, c, namespace, name, timeout, pollInterval, func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
		return claim.Status.Materialization.LocalRef != nil, nil
	})
	if err != nil {
		return nil, err
	}
	localCopy := *claim.Status.Materialization.LocalRef
	return &localCopy, nil
}

func waitForClusterDeleted(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	timeout time.Duration,
	pollInterval time.Duration,
) error {
	return waitForObjectDeletionOrPresence(ctx, timeout, pollInterval, func() error {
		return c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &openbaov1alpha1.OpenBaoCluster{})
	}, true)
}

//nolint:unparam // Kept configurable so call sites can state their E2E wait contract.
func waitForClaimDeleted(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	timeout time.Duration,
	pollInterval time.Duration,
) error {
	return waitForObjectDeletionOrPresence(ctx, timeout, pollInterval, func() error {
		return c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &openbaov1alpha1.OpenBaoClusterClaim{})
	}, true)
}

func waitForClaimUpgradeRequestState(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	expected openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState,
	timeout time.Duration,
	pollInterval time.Duration,
) (*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest, error) {
	return waitForClaimUpgradeRequest(ctx, c, namespace, name, timeout, pollInterval, func(request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) (bool, error) {
		return request.Status.State == expected, nil
	})
}

func waitForClaimUpgradeCleared(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	timeout time.Duration,
	pollInterval time.Duration,
) (*openbaov1alpha1.OpenBaoClusterClaim, error) {
	return waitForClaim(ctx, c, namespace, name, timeout, pollInterval, func(claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
		return claim.Status.Phase == openbaov1alpha1.OpenBaoClusterClaimPhaseReady &&
			claim.Status.Summary == nil &&
			claim.Status.Upgrade == nil, nil
	})
}

func waitForSecretDeleted(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	timeout time.Duration,
	pollInterval time.Duration,
) error {
	return waitForObjectDeletionOrPresence(ctx, timeout, pollInterval, func() error {
		return c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &corev1.Secret{})
	}, true)
}

func waitForClaim(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	timeout time.Duration,
	pollInterval time.Duration,
	predicate func(*openbaov1alpha1.OpenBaoClusterClaim) (bool, error),
) (*openbaov1alpha1.OpenBaoClusterClaim, error) {
	return waitForClaimObject(ctx, c, namespace, name, "OpenBaoClusterClaim", timeout, pollInterval, func() *openbaov1alpha1.OpenBaoClusterClaim {
		return &openbaov1alpha1.OpenBaoClusterClaim{}
	}, predicate)
}

func waitForClaimUpgradeRequest(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	timeout time.Duration,
	pollInterval time.Duration,
	predicate func(*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) (bool, error),
) (*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest, error) {
	return waitForClaimObject(ctx, c, namespace, name, "OpenBaoClusterClaimUpgradeRequest", timeout, pollInterval, func() *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest {
		return &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{}
	}, predicate)
}

func waitForClaimBackupRequestState(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	expected openbaov1alpha1.OpenBaoClusterClaimBackupRequestState,
	timeout time.Duration,
	pollInterval time.Duration,
) (*openbaov1alpha1.OpenBaoClusterClaimBackupRequest, error) {
	return waitForClaimBackupRequest(ctx, c, namespace, name, timeout, pollInterval, func(request *openbaov1alpha1.OpenBaoClusterClaimBackupRequest) (bool, error) {
		return request.Status.State == expected, nil
	})
}

func waitForClaimBackupRequest(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	timeout time.Duration,
	pollInterval time.Duration,
	predicate func(*openbaov1alpha1.OpenBaoClusterClaimBackupRequest) (bool, error),
) (*openbaov1alpha1.OpenBaoClusterClaimBackupRequest, error) {
	return waitForClaimObject(ctx, c, namespace, name, "OpenBaoClusterClaimBackupRequest", timeout, pollInterval, func() *openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
		return &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{}
	}, predicate)
}

func waitForClaimRestoreRequestState(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	expected openbaov1alpha1.OpenBaoClusterClaimRestoreRequestState,
	timeout time.Duration,
	pollInterval time.Duration,
) (*openbaov1alpha1.OpenBaoClusterClaimRestoreRequest, error) {
	return waitForClaimRestoreRequest(ctx, c, namespace, name, timeout, pollInterval, func(request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest) (bool, error) {
		return request.Status.State == expected, nil
	})
}

func waitForClaimRestoreRequest(
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	timeout time.Duration,
	pollInterval time.Duration,
	predicate func(*openbaov1alpha1.OpenBaoClusterClaimRestoreRequest) (bool, error),
) (*openbaov1alpha1.OpenBaoClusterClaimRestoreRequest, error) {
	return waitForClaimObject(ctx, c, namespace, name, "OpenBaoClusterClaimRestoreRequest", timeout, pollInterval, func() *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest {
		return &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{}
	}, predicate)
}

func waitForClaimObject[T client.Object](
	ctx context.Context,
	c client.Client,
	namespace string,
	name string,
	kind string,
	timeout time.Duration,
	pollInterval time.Duration,
	newObject func() T,
	predicate func(T) (bool, error),
) (T, error) {
	var zero T
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		object := newObject()
		if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, object); err != nil {
			if !apierrors.IsNotFound(err) {
				return zero, fmt.Errorf("get %s %s/%s: %w", kind, namespace, name, err)
			}
		} else {
			ok, err := predicate(object)
			if err != nil {
				return zero, err
			}
			if ok {
				return object, nil
			}
		}

		select {
		case <-ctx.Done():
			return zero, fmt.Errorf("context canceled while waiting for %s %s/%s: %w", kind, namespace, name, ctx.Err())
		case <-deadline.C:
			return zero, fmt.Errorf("timed out waiting for %s %s/%s", kind, namespace, name)
		case <-ticker.C:
		}
	}
}

func waitForObjectDeletionOrPresence(
	ctx context.Context,
	timeout time.Duration,
	pollInterval time.Duration,
	get func() error,
	expectDeleted bool,
) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		err := get()
		if expectDeleted {
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
		} else if err == nil {
			return nil
		} else if !apierrors.IsNotFound(err) {
			return err
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled while waiting for object state: %w", ctx.Err())
		case <-deadline.C:
			if expectDeleted {
				return fmt.Errorf("timed out waiting for object deletion")
			}
			return fmt.Errorf("timed out waiting for object to appear")
		case <-ticker.C:
		}
	}
}
