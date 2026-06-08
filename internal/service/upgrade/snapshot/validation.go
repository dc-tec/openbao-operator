package snapshot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/hardenedcontract"
)

// ValidateHardenedNetwork ensures hardened clusters explicitly allow snapshot
// job egress.
func ValidateHardenedNetwork(cluster *openbaov1alpha1.OpenBaoCluster, message string) error {
	if cluster == nil || cluster.Spec.Profile != openbaov1alpha1.ProfileHardened {
		return nil
	}
	if cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) == 0 {
		return operatorerrors.WithReason(
			constants.ReasonNetworkEgressRulesRequired,
			operatorerrors.WrapPermanentConfig(errors.New(message)),
		)
	}
	if !hardenedcontract.EgressRulesExplicit(cluster.Spec.Network.EgressRules) {
		return operatorerrors.WithReason(
			constants.ReasonSecurityViolation,
			operatorerrors.WrapPermanentConfig(errors.New("hardened profile requires spec.network.egressRules entries to be port-scoped and target explicit non-wildcard peers")),
		)
	}
	return nil
}

// RequireBackupConfig ensures backup config exists and, when required, that the
// target endpoint is configured.
func RequireBackupConfig(cluster *openbaov1alpha1.OpenBaoCluster, requireEndpoint bool, message string) error {
	if cluster == nil || cluster.Spec.Backup == nil {
		return errors.New(message)
	}
	if requireEndpoint && strings.TrimSpace(cluster.Spec.Backup.Target.Endpoint) == "" {
		return errors.New(message)
	}
	return nil
}

// ValidateBackupAuth ensures backup auth is configured via JWT role, token
// secret, or self-init OIDC.
func ValidateBackupAuth(cluster *openbaov1alpha1.OpenBaoCluster, message string) error {
	if cluster == nil || cluster.Spec.Backup == nil {
		return errors.New(message)
	}

	hasJWTAuth := strings.TrimSpace(cluster.Spec.Backup.JWTAuthRole) != ""
	if !hasJWTAuth &&
		cluster.Spec.SelfInit != nil &&
		cluster.Spec.SelfInit.OIDC != nil &&
		cluster.Spec.SelfInit.OIDC.Enabled {
		hasJWTAuth = true
	}

	hasTokenSecret := cluster.Spec.Backup.TokenSecretRef != nil &&
		strings.TrimSpace(cluster.Spec.Backup.TokenSecretRef.Name) != ""

	if hasJWTAuth || hasTokenSecret {
		return nil
	}

	return errors.New(message)
}

// ValidateHardenedBackupTarget ensures pre-upgrade snapshot Jobs do not use
// backup target escape hatches on existing Hardened specs.
func ValidateHardenedBackupTarget(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Spec.Profile != openbaov1alpha1.ProfileHardened || cluster.Spec.Backup == nil {
		return nil
	}
	if violation := hardenedcontract.EvaluateStorageTarget("Backup", cluster.Spec.Backup.Target); violation != nil {
		return operatorerrors.WithReason(
			violation.Reason,
			operatorerrors.WrapPermanentConfig(errors.New(violation.Message)),
		)
	}
	return nil
}

// BackupTokenSecretName returns the configured backup token secret name when present.
func BackupTokenSecretName(cluster *openbaov1alpha1.OpenBaoCluster) (string, bool) {
	if cluster == nil || cluster.Spec.Backup == nil || cluster.Spec.Backup.TokenSecretRef == nil {
		return "", false
	}
	name := strings.TrimSpace(cluster.Spec.Backup.TokenSecretRef.Name)
	if name == "" {
		return "", false
	}
	return name, true
}

// EnsureBackupTokenSecretExists checks that the backup token secret exists.
func EnsureBackupTokenSecretExists(ctx context.Context, c client.Reader, namespace, secretName string) error {
	if c == nil {
		return fmt.Errorf("client is required to validate backup token Secret %s/%s", namespace, secretName)
	}

	secretKey := types.NamespacedName{
		Namespace: namespace,
		Name:      secretName,
	}

	secret := &corev1.Secret{}
	if err := c.Get(ctx, secretKey, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("backup token Secret %s/%s not found", namespace, secretName)
		}
		return fmt.Errorf("failed to get backup token Secret %s/%s: %w", namespace, secretName, err)
	}

	return nil
}
