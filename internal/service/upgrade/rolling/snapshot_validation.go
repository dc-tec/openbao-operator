package rolling

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func (m *Manager) validatePreUpgradeSnapshotPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if err := validatePreUpgradeSnapshotNetwork(cluster); err != nil {
		return err
	}
	if err := m.validateBackupConfig(ctx, cluster); err != nil {
		return operatorerrors.WithReason(upgrade.ReasonPreUpgradeBackupFailed, fmt.Errorf("pre-upgrade backup configuration invalid: %w", err))
	}
	return nil
}

func validatePreUpgradeSnapshotNetwork(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Spec.Profile != openbaov1alpha1.ProfileHardened {
		return nil
	}
	if cluster.Spec.Network != nil && len(cluster.Spec.Network.EgressRules) > 0 {
		return nil
	}

	return operatorerrors.WithReason(
		constants.ReasonNetworkEgressRulesRequired,
		operatorerrors.WrapPermanentConfig(fmt.Errorf(
			"hardened profile with pre-upgrade snapshots enabled requires explicit spec.network.egressRules so backup Jobs can reach the object storage endpoint",
		)),
	)
}

// validateBackupConfig validates that backup configuration is present and valid.
func (m *Manager) validateBackupConfig(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	backupCfg := cluster.Spec.Backup
	if backupCfg == nil {
		return fmt.Errorf("backup configuration is required when preUpgradeSnapshot is enabled")
	}

	hasJWTAuth := strings.TrimSpace(backupCfg.JWTAuthRole) != ""
	if !hasJWTAuth &&
		cluster.Spec.SelfInit != nil &&
		cluster.Spec.SelfInit.OIDC != nil &&
		cluster.Spec.SelfInit.OIDC.Enabled {
		hasJWTAuth = true
	}

	hasTokenSecret := backupCfg.TokenSecretRef != nil && strings.TrimSpace(backupCfg.TokenSecretRef.Name) != ""
	if !hasJWTAuth && !hasTokenSecret {
		return fmt.Errorf("backup authentication is required: either jwtAuthRole or tokenSecretRef must be set")
	}

	if !hasTokenSecret {
		return nil
	}

	return m.ensureBackupTokenSecretExists(ctx, cluster, backupCfg.TokenSecretRef.Name)
}

func (m *Manager) ensureBackupTokenSecretExists(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, secretName string) error {
	secretKey := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      secretName,
	}

	secret := &corev1.Secret{}
	if err := m.client.Get(ctx, secretKey, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("backup token Secret %s/%s not found", cluster.Namespace, secretName)
		}
		return fmt.Errorf("failed to get backup token Secret %s/%s: %w", cluster.Namespace, secretName, err)
	}

	return nil
}

func operatorImageVerificationFailurePolicy(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Spec.OperatorImageVerification == nil {
		return constants.ImageVerificationFailurePolicyBlock
	}
	if strings.TrimSpace(cluster.Spec.OperatorImageVerification.FailurePolicy) == "" {
		return constants.ImageVerificationFailurePolicyBlock
	}
	return cluster.Spec.OperatorImageVerification.FailurePolicy
}
