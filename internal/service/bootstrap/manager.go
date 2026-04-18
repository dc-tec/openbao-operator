package bootstrap

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceapply"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	configurationservice "github.com/dc-tec/openbao-operator/internal/service/configuration"
)

const (
	unsealSecretKey   = "key"
	unsealKeyBytes    = 32
	configFileName    = "config.hcl"
	unsealTypeTransit = "transit"
)

// Manager reconciles bootstrap/configuration resources for an OpenBaoCluster.
type Manager struct {
	client             client.Client
	reader             client.Reader
	scheme             *runtime.Scheme
	operatorNamespace  string
	oidcIssuer         string
	oidcDiscoveryURL   string
	oidcDiscoveryCAPEM string
	oidcJWKSURL        string
	oidcJWKSCAPEM      string
	oidcJWTKeys        []string
}

func NewManager(c client.Client, scheme *runtime.Scheme, operatorNamespace string) *Manager {
	return &Manager{
		client:            c,
		reader:            c,
		scheme:            scheme,
		operatorNamespace: operatorNamespace,
	}
}

func NewManagerWithReader(c client.Client, r client.Reader, scheme *runtime.Scheme, operatorNamespace string) *Manager {
	m := NewManager(c, scheme, operatorNamespace)
	if r != nil {
		m.reader = r
	}
	return m
}

func NewManagerWithReaderAndOIDCConfig(
	c client.Client,
	r client.Reader,
	scheme *runtime.Scheme,
	operatorNamespace string,
	oidcConfig *portauth.OIDCConfig,
) *Manager {
	m := NewManagerWithReader(c, r, scheme, operatorNamespace)
	m.SetOIDCConfig(oidcConfig)
	return m
}

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

// PrepareWorkload ensures bootstrap-owned resources are aligned and returns rendered config.hcl content.
func (m *Manager) PrepareWorkload(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if usesStaticSeal(cluster) {
		if err := m.ensureUnsealSecret(ctx, logger, cluster); err != nil {
			return "", err
		}
	}

	if err := m.validateUnsealPrerequisites(ctx, cluster); err != nil {
		return "", err
	}

	configContent, err := configurationservice.Render(cluster, configurationservice.RenderOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to render config.hcl for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if err := m.reconcilePreStatefulSet(ctx, logger, cluster, configContent); err != nil {
		return "", err
	}

	return configContent, nil
}

func (m *Manager) reconcilePreStatefulSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, configContent string) error {
	if err := m.ensureConfigMap(ctx, logger, cluster, configContent); err != nil {
		return err
	}
	if err := m.ensureSelfInitConfigMap(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureACMESharedCachePVC(ctx, logger, cluster); err != nil {
		return err
	}
	return nil
}

func (m *Manager) applyResource(ctx context.Context, obj client.Object, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return resourceapply.ApplyOwned(ctx, m.client, m.scheme, cluster, obj)
}
