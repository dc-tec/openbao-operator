package snapshot

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	servicebackup "github.com/dc-tec/openbao-operator/internal/service/backup"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

// ValidationOptions controls shared pre-upgrade snapshot prerequisite checks.
type ValidationOptions struct {
	MissingBackupMessage  string
	RequireEndpoint       bool
	RequireTokenSecret    bool
	NetworkErrorMessage   string
	AuthenticationMessage string
}

// ValidatePreUpgradeSnapshotPrerequisites applies the common backup config,
// network, auth, and optional token-secret checks used by upgrade strategies.
func ValidatePreUpgradeSnapshotPrerequisites(
	ctx context.Context,
	reader client.Reader,
	cluster *openbaov1alpha1.OpenBaoCluster,
	opts ValidationOptions,
) error {
	if err := RequireBackupConfig(cluster, opts.RequireEndpoint, opts.MissingBackupMessage); err != nil {
		return err
	}
	if err := ValidateHardenedNetwork(cluster, opts.NetworkErrorMessage); err != nil {
		return err
	}
	if err := ValidateBackupAuth(cluster, opts.AuthenticationMessage); err != nil {
		return err
	}
	if !opts.RequireTokenSecret {
		return nil
	}

	secretName, ok := BackupTokenSecretName(cluster)
	if !ok {
		return nil
	}
	return EnsureBackupTokenSecretExists(ctx, reader, cluster.Namespace, secretName)
}

// ResolvePreUpgradeSnapshotExecutorDigest resolves and verifies the helper
// image digest used for pre-upgrade snapshot Jobs.
func ResolvePreUpgradeSnapshotExecutorDigest(
	ctx context.Context,
	logger logr.Logger,
	verifier imageverify.Verifier,
	cluster *openbaov1alpha1.OpenBaoCluster,
	failureReason string,
	failureMessage string,
) (string, error) {
	executorImage, err := servicebackup.GetBackupExecutorImage(cluster)
	if err != nil {
		return "", fmt.Errorf("failed to determine pre-upgrade snapshot executor image: %w", err)
	}

	return upgrade.VerifyOperatorImageDigest(
		ctx,
		logger,
		verifier,
		cluster,
		executorImage,
		failureReason,
		failureMessage,
	)
}
