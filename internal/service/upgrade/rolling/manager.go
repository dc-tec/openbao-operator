package rolling

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	portbackup "github.com/dc-tec/openbao-operator/internal/port/backup"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
)

const (
	// headlessServiceSuffix is appended to cluster name for the headless service.
	headlessServiceSuffix = ""
)

var (
	// ErrNoUpgradeToken indicates that no suitable upgrade JWT role is configured.
	ErrNoUpgradeToken = errors.New("no upgrade JWT role configured: set spec.upgrade.jwtAuthRole or enable spec.selfInit.oidc.enabled")
)

// Manager reconciles version and Raft-aware upgrade behavior for an OpenBaoCluster.
type Manager struct {
	client                client.Client
	scheme                *runtime.Scheme
	backupRuntime         portbackup.PreUpgradeSnapshotRuntime
	recorder              events.EventRecorder
	clientFactory         raftops.OpenBaoClientFactory
	clientConfig          portopenbao.ClientConfig
	operatorImageVerifier imageverify.Verifier
	Platform              string
}

// NewManager constructs a Manager that uses the provided Kubernetes client and scheme.
func NewManager(
	c client.Client,
	scheme *runtime.Scheme,
	backupRuntime portbackup.PreUpgradeSnapshotRuntime,
	clientConfig portopenbao.ClientConfig,
	operatorImageVerifier imageverify.Verifier,
	platform string,
	recorder ...events.EventRecorder,
) *Manager {
	var eventRecorder events.EventRecorder
	if len(recorder) > 0 {
		eventRecorder = recorder[0]
	}
	return &Manager{
		client:                c,
		scheme:                scheme,
		backupRuntime:         backupRuntime,
		recorder:              eventRecorder,
		clientFactory:         raftops.DefaultOpenBaoClientFactory,
		clientConfig:          clientConfig,
		operatorImageVerifier: operatorImageVerifier,
		Platform:              platform,
	}
}

// NewManagerWithClientFactory constructs a Manager with a custom OpenBao client factory.
// This is primarily used for testing.
func NewManagerWithClientFactory(
	c client.Client,
	scheme *runtime.Scheme,
	backupRuntime portbackup.PreUpgradeSnapshotRuntime,
	factory raftops.OpenBaoClientFactory,
	clientConfig portopenbao.ClientConfig,
	operatorImageVerifier imageverify.Verifier,
	platform string,
	recorder ...events.EventRecorder,
) *Manager {
	if factory == nil {
		factory = raftops.DefaultOpenBaoClientFactory
	}
	var eventRecorder events.EventRecorder
	if len(recorder) > 0 {
		eventRecorder = recorder[0]
	}
	return &Manager{
		client:                c,
		scheme:                scheme,
		backupRuntime:         backupRuntime,
		recorder:              eventRecorder,
		clientFactory:         factory,
		clientConfig:          clientConfig,
		operatorImageVerifier: operatorImageVerifier,
		Platform:              platform,
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
// Returns the reconcile result for follow-up scheduling along with any error.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	logger = logger.WithValues(
		"specVersion", cluster.Spec.Version,
		"statusVersion", cluster.Status.CurrentVersion,
	)

	metrics := upgrade.NewMetrics(cluster.Namespace, cluster.Name)
	strategy := string(openbaov1alpha1.UpdateStrategyRollingUpdate)
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy != "" {
		strategy = string(cluster.Spec.Upgrade.Strategy)
	}

	if result, done := m.shouldSkipUpgradeReconcile(logger, cluster); done {
		return result, nil
	}

	upgradeNeeded, resumeUpgrade := m.detectUpgradeState(logger, cluster)
	if result, done, err := m.ensureUpgradeLock(ctx, logger, cluster, metrics, strategy, upgradeNeeded, resumeUpgrade); done || err != nil {
		return result, err
	}

	if result, done, err := m.prepareUpgradeExecution(ctx, logger, cluster, metrics, strategy, resumeUpgrade); done || err != nil {
		return result, err
	}

	return m.reconcileUpgradeExecution(ctx, logger, cluster, metrics, strategy)
}
