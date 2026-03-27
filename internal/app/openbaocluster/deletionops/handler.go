package deletionops

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/observability"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
)

// Dependencies contains external collaborators required for deletion orchestration.
type Dependencies struct {
	Client             client.Client
	APIReader          client.Reader
	Scheme             *runtime.Scheme
	OperatorNamespace  string
	OIDCIssuer         string
	OIDCDiscoveryURL   string
	OIDCDiscoveryCAPEM string
	OIDCJWKSURL        string
	OIDCJWKSCAPEM      string
	OIDCJWTKeys        []string
	Platform           string
	RetentionSecrets   []string
}

// Handle applies the deletion policy for an OpenBaoCluster.
func Handle(ctx context.Context, logger logr.Logger, deps Dependencies, cluster *openbaov1alpha1.OpenBaoCluster) error {
	policy := cluster.Spec.DeletionPolicy
	if policy == "" {
		policy = openbaov1alpha1.DeletionPolicyRetain
	}

	logger.Info("Applying DeletionPolicy for OpenBaoCluster", "deletionPolicy", string(policy))

	// CRITICAL: When DeletionPolicy is Retain, orphan required secrets before finalizer removal.
	if policy == openbaov1alpha1.DeletionPolicyRetain {
		if err := OrphanRetentionSecrets(ctx, logger, deps.Client, cluster, deps.RetentionSecrets); err != nil {
			return fmt.Errorf("failed to orphan retention secrets: %w", err)
		}
	}

	// Clear per-cluster metrics to avoid stale series after deletion.
	clusterMetrics := observability.NewClusterMetrics(cluster.Namespace, cluster.Name)
	clusterMetrics.Clear()

	infraMgr := inframanager.NewManagerWithReaderAndOIDCConfig(
		deps.Client,
		deps.APIReader,
		deps.Scheme,
		deps.OperatorNamespace,
		&portauth.OIDCConfig{
			IssuerURL:          deps.OIDCIssuer,
			OIDCDiscoveryURL:   deps.OIDCDiscoveryURL,
			OIDCDiscoveryCAPEM: deps.OIDCDiscoveryCAPEM,
			JWKSURL:            deps.OIDCJWKSURL,
			JWKSCAPEM:          deps.OIDCJWKSCAPEM,
			JWKSKeys:           deps.OIDCJWTKeys,
		},
		deps.Platform,
	)
	if err := infraMgr.Cleanup(ctx, logger, cluster, policy); err != nil {
		return err
	}

	// Backup deletion for DeletionPolicyDeleteAll will be implemented alongside BackupManager data path.
	return nil
}
