package rolling

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

const (
	// headlessServiceSuffix is appended to cluster name for the headless service.
	headlessServiceSuffix = ""
)

var (
	// ErrNoUpgradeToken indicates that no suitable upgrade token is configured.
	ErrNoUpgradeToken = errors.New("no upgrade token configured: either spec.upgrade.jwtAuthRole or spec.upgrade.tokenSecretRef must be set")
)

// Manager reconciles version and Raft-aware upgrade behavior for an OpenBaoCluster.
type Manager struct {
	client        client.Client
	scheme        *runtime.Scheme
	clientFactory upgrade.OpenBaoClientFactory
	clientConfig  openbaoapi.ClientConfig
}

// NewManager constructs a Manager that uses the provided Kubernetes client and scheme.
func NewManager(c client.Client, scheme *runtime.Scheme, clientConfig openbaoapi.ClientConfig) *Manager {
	return &Manager{
		client:        c,
		scheme:        scheme,
		clientFactory: upgrade.DefaultOpenBaoClientFactory,
		clientConfig:  clientConfig,
	}
}

// NewManagerWithClientFactory constructs a Manager with a custom OpenBao client factory.
// This is primarily used for testing.
func NewManagerWithClientFactory(c client.Client, scheme *runtime.Scheme, factory upgrade.OpenBaoClientFactory, clientConfig openbaoapi.ClientConfig) *Manager {
	if factory == nil {
		factory = upgrade.DefaultOpenBaoClientFactory
	}
	return &Manager{
		client:        c,
		scheme:        scheme,
		clientFactory: factory,
		clientConfig:  clientConfig,
	}
}

// Reconcile ensures upgrades progress safely for the given OpenBaoCluster.
//
// The upgrade state machine follows these phases:
//  1. Detection: Check if upgrade is needed or if we're resuming an existing one
//  2. Pre-upgrade Validation: Validate version, check cluster health
//  3. Initialize Upgrade: Set up upgrade state, lock StatefulSet partition
//  4. Pod-by-Pod Update: Step down leader if needed, update each pod in reverse ordinal order
//  5. Finalization: Clear upgrade state, update current version
//
// Returns (shouldRequeue, error) where shouldRequeue indicates if reconciliation should be requeued immediately.
//
//nolint:gocyclo // Upgrade reconciliation is an orchestrator with guard clauses and phase transitions.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {

	logger = logger.WithValues(
		"specVersion", cluster.Spec.Version,
		"statusVersion", cluster.Status.CurrentVersion,
	)

	metrics := upgrade.NewMetrics(cluster.Namespace, cluster.Name)

	// Check if cluster is initialized; upgrades can only proceed after initialization
	if !cluster.Status.Initialized {
		logger.V(1).Info("Cluster not initialized; skipping upgrade reconciliation")
		return recon.Result{RequeueAfter: constants.RequeueStandard}, nil
	}

	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active && cluster.Spec.BreakGlassAck != cluster.Status.BreakGlass.Nonce {
		logger.Info("Cluster is in break glass mode; halting upgrade reconciliation",
			"breakGlassReason", cluster.Status.BreakGlass.Reason,
			"breakGlassNonce", cluster.Status.BreakGlass.Nonce)
		return recon.Result{RequeueAfter: constants.RequeueStandard}, nil
	}

	// Phase 1: Detection - determine if upgrade is needed
	upgradeNeeded, resumeUpgrade := m.detectUpgradeState(logger, cluster)

	if !upgradeNeeded && !resumeUpgrade {
		if upgrade.IsUpgradeOperationLockHeldByUs(cluster.Status.OperationLock) {
			if err := upgrade.ReleaseUpgradeOperationLock(ctx, m.client, cluster); err != nil && !upgrade.IsOperationLockHeld(err) {
				logger.Error(err, "Failed to release stale upgrade operation lock")
			}
		}

		// No upgrade needed, ensure metrics reflect idle state
		metrics.SetInProgress(false)
		metrics.SetStatus(upgrade.UpgradeStatusNone)
		return recon.Result{}, nil
	}

	lockMessage := fmt.Sprintf("upgrade to %s", cluster.Spec.Version)
	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.TargetVersion != "" {
		lockMessage = fmt.Sprintf("upgrade to %s (in progress)", cluster.Status.Upgrade.TargetVersion)
	}
	if err := upgrade.AcquireUpgradeOperationLock(ctx, m.client, cluster, lockMessage); err != nil {
		if upgrade.IsOperationLockHeld(err) {
			if cluster.Status.Upgrade != nil {
				upgrade.SetUpgradeFailed(&cluster.Status, upgrade.ReasonUpgradeFailed, "upgrade halted due to concurrent operation lock", cluster.Generation)
				metrics.SetStatus(upgrade.UpgradeStatusFailed)
				return recon.Result{}, fmt.Errorf("upgrade in progress but operation lock is held by another operation: %w", err)
			}
			logger.Info("Upgrade blocked by operation lock", "error", err.Error())
			return recon.Result{RequeueAfter: constants.RequeueStandard}, nil
		}
		return recon.Result{}, fmt.Errorf("failed to acquire upgrade operation lock: %w", err)
	}

	// Handle resume scenario where spec.version changed mid-upgrade
	if resumeUpgrade && cluster.Status.Upgrade != nil {
		if cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion {
			logger.Info("Spec.Version changed during upgrade; clearing upgrade state and starting fresh",
				"previousTarget", cluster.Status.Upgrade.TargetVersion,
				"newTarget", cluster.Spec.Version)
			upgrade.ClearUpgrade(&cluster.Status, upgrade.ReasonVersionMismatch,
				fmt.Sprintf("Target version changed from %s to %s during upgrade",
					cluster.Status.Upgrade.TargetVersion, cluster.Spec.Version),
				cluster.Generation)
			// Continuing will re-evaluate and start fresh
		}
	}

	// Ensure upgrade ServiceAccount exists (for JWT Auth)
	if err := upgrade.EnsureUpgradeServiceAccount(ctx, m.client, cluster, "openbao-operator"); err != nil {
		return recon.Result{}, fmt.Errorf("failed to ensure upgrade ServiceAccount: %w", err)
	}

	// Phase 2: Pre-upgrade Validation
	if err := m.validateUpgrade(ctx, logger, cluster); err != nil {
		return recon.Result{}, err
	}

	// Phase 3: Pre-upgrade Snapshot (if enabled)
	// Note: This happens after validation to ensure cluster is healthy before snapshot
	// If backup is in progress (snapshotComplete=false), this will return nil to requeue, preventing upgrade initialization
	// We check this even if Status.Upgrade is set to handle edge cases where upgrade was initialized
	// but snapshot is still running (shouldn't happen, but we check defensively)
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.PreUpgradeSnapshot {
		// Check if pre-upgrade snapshot is required and complete
		snapshotComplete, err := m.handlePreUpgradeSnapshot(ctx, logger, cluster)
		if err != nil {
			return recon.Result{}, err
		}
		if !snapshotComplete {
			logger.Info("Pre-upgrade snapshot in progress, waiting...")
			// If upgrade was already initialized, we should not proceed with pod updates
			// Return early to wait for snapshot completion
			return recon.Result{RequeueAfter: constants.RequeueShort}, nil
		}
	}

	// Phase 4: Initialize Upgrade (if not resuming)
	// Only reached if pre-upgrade snapshot is complete or not enabled
	if cluster.Status.Upgrade == nil {
		if err := m.initializeUpgrade(ctx, logger, cluster); err != nil {
			return recon.Result{}, err
		}
	}

	// Update metrics for in-progress upgrade
	metrics.SetInProgress(true)
	metrics.SetStatus(upgrade.UpgradeStatusRunning)
	if cluster.Status.Upgrade != nil {
		metrics.SetPodsCompleted(len(cluster.Status.Upgrade.CompletedPods))
		metrics.SetTotalPods(int(cluster.Spec.Replicas))
		metrics.SetPartition(cluster.Status.Upgrade.CurrentPartition)
	}

	// Phase 5: Pod-by-Pod Update
	completed, err := m.performPodByPodUpgrade(ctx, logger, cluster, metrics)
	if err != nil {
		upgrade.SetUpgradeFailed(&cluster.Status, upgrade.ReasonUpgradeFailed, err.Error(), cluster.Generation)
		metrics.SetStatus(upgrade.UpgradeStatusFailed)

		// Update status using SSA (eliminates race conditions)
		if statusErr := m.patchStatusSSA(ctx, cluster); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after upgrade failure")
		}
		return recon.Result{}, err
	}

	if !completed {
		// Upgrade is still in progress; save state and requeue
		// Update status using SSA (eliminates race conditions)
		if err := m.patchStatusSSA(ctx, cluster); err != nil {
			return recon.Result{}, fmt.Errorf("failed to update upgrade progress: %w", err)
		}
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	// Phase 6: Finalization
	if err := m.finalizeUpgrade(ctx, logger, cluster, metrics); err != nil {
		return recon.Result{}, err
	}

	return recon.Result{}, nil
}
