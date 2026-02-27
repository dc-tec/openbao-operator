package openbaocluster

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/auth"
	"github.com/dc-tec/openbao-operator/internal/logging"
	"github.com/dc-tec/openbao-operator/internal/observability"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	operatorpredicates "github.com/dc-tec/openbao-operator/internal/predicates"
	"github.com/dc-tec/openbao-operator/internal/revision"
	"github.com/dc-tec/openbao-operator/internal/security"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

// ReconcileMetrics records reconcile duration and categorized errors.
type ReconcileMetrics interface {
	ObserveDuration(durationSeconds float64)
	IncrementError(reason string)
}

// ClusterMetrics records per-cluster readiness and phase gauges.
type ClusterMetrics interface {
	SetReadyReplicas(readyReplicas int32)
	SetPhase(phase openbaov1alpha1.ClusterPhase)
	Clear()
}

// NewReconcileMetrics returns a controller reconcile metrics recorder.
func NewReconcileMetrics(namespace, name, controller string) ReconcileMetrics {
	return observability.NewReconcileMetrics(namespace, name, controller)
}

// NewClusterMetrics returns per-cluster metrics helpers.
func NewClusterMetrics(namespace, name string) ClusterMetrics {
	return observability.NewClusterMetrics(namespace, name)
}

// LogRetentionSecretOrphaned emits a structured audit event for orphaned retention secrets.
func LogRetentionSecretOrphaned(logger logr.Logger, clusterNamespace, clusterName, secretName, deletionPolicy string) {
	logging.LogAuditEvent(logger, logging.EventRetentionSecretOrphaned, map[string]string{
		"cluster_namespace": clusterNamespace,
		"cluster_name":      clusterName,
		"secret_name":       secretName,
		"deletion_policy":   deletionPolicy,
	})
}

// PredicateOptions configures OpenBaoCluster reconcile triggers.
type PredicateOptions struct {
	ReconcileOnUpgradeStatus   bool
	ReconcileOnBackupStatus    bool
	ReconcileOnBlueGreenStatus bool
	ReconcileOnBreakGlass      bool
	ReconcileOnWorkloadError   bool
	ReconcileOnAdminOpsError   bool
}

// OpenBaoClusterPredicateWithOptions builds the canonical OpenBaoCluster update predicate.
func OpenBaoClusterPredicateWithOptions(opts PredicateOptions) predicate.Predicate {
	return operatorpredicates.OpenBaoClusterPredicateWithOptions(operatorpredicates.OpenBaoClusterPredicateOptions{
		ReconcileOnUpgradeStatus:   opts.ReconcileOnUpgradeStatus,
		ReconcileOnBackupStatus:    opts.ReconcileOnBackupStatus,
		ReconcileOnBlueGreenStatus: opts.ReconcileOnBlueGreenStatus,
		ReconcileOnBreakGlass:      opts.ReconcileOnBreakGlass,
		ReconcileOnWorkloadError:   opts.ReconcileOnWorkloadError,
		ReconcileOnAdminOpsError:   opts.ReconcileOnAdminOpsError,
	})
}

// NewImageVerifier constructs the shared cluster image verifier.
func NewImageVerifier(logger logr.Logger, k8sClient client.Client) imageverify.Verifier {
	return security.NewImageVerifier(logger, k8sClient, nil)
}

// IsMainImageVerificationEnabled reports whether main workload image verification is enabled.
func IsMainImageVerificationEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return security.IsMainImageVerificationEnabled(cluster)
}

// IsOperatorImageVerificationEnabled reports whether operator helper image verification is enabled.
func IsOperatorImageVerificationEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return security.IsOperatorImageVerificationEnabled(cluster)
}

// VerifyImageForCluster verifies a primary OpenBao image according to cluster policy.
func VerifyImageForCluster(
	ctx context.Context,
	logger logr.Logger,
	verifier imageverify.Verifier,
	cluster *openbaov1alpha1.OpenBaoCluster,
	imageRef string,
) (string, error) {
	return security.VerifyImageForCluster(ctx, logger, verifier, cluster, imageRef)
}

// VerifyOperatorImageForCluster verifies an operator-managed helper image according to cluster policy.
func VerifyOperatorImageForCluster(
	ctx context.Context,
	logger logr.Logger,
	verifier imageverify.Verifier,
	cluster *openbaov1alpha1.OpenBaoCluster,
	imageRef string,
) (string, error) {
	return security.VerifyOperatorImageForCluster(ctx, logger, verifier, cluster, imageRef)
}

// OIDCConfig contains discovered issuer and key material for JWT bootstrap.
type OIDCConfig struct {
	IssuerURL string
	JWKSKeys  []string
}

// DiscoverOIDCConfig resolves OIDC issuer metadata from the Kubernetes API.
func DiscoverOIDCConfig(ctx context.Context, cfg *rest.Config) (*OIDCConfig, error) {
	discovered, err := auth.DiscoverConfig(ctx, cfg, "")
	if err != nil {
		return nil, err
	}
	if discovered == nil {
		return nil, nil
	}
	return &OIDCConfig{
		IssuerURL: discovered.IssuerURL,
		JWKSKeys:  discovered.JWKSKeys,
	}, nil
}

// OIDCDiscoveryStatusCode extracts an HTTP status code from OIDC discovery errors.
func OIDCDiscoveryStatusCode(err error) (int, bool) {
	var statusErr *auth.HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode, true
	}
	return 0, false
}

// OpenBaoClusterRevision computes the deterministic revision used by blue/green status logic.
func OpenBaoClusterRevision(version, image string, replicas int32) string {
	return revision.OpenBaoClusterRevision(version, image, replicas)
}

// IsVersionDowngrade reports whether moving from one version to another is a downgrade.
func IsVersionDowngrade(from, to string) (bool, error) {
	change, err := upgrade.CompareVersions(from, to)
	if err != nil {
		return false, err
	}
	return change == upgrade.VersionChangeDowngrade, nil
}
