package bluegreen

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	portbackup "github.com/dc-tec/openbao-operator/internal/port/backup"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portinfra "github.com/dc-tec/openbao-operator/internal/port/infra"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
)

var (
	// ErrBlueGreenNotConfigured indicates that blue/green upgrade strategy is not configured.
	ErrBlueGreenNotConfigured = errors.New("blue/green upgrade strategy not configured")
	// ErrRevisionCalculationFailed indicates that revision calculation failed.
	ErrRevisionCalculationFailed = errors.New("revision calculation failed")
)

// Manager manages blue/green upgrade operations for OpenBaoCluster.
type Manager struct {
	client                client.Client
	reader                client.Reader
	scheme                *runtime.Scheme
	infraRuntime          portinfra.BlueGreenRuntime
	backupRuntime         portbackup.PreUpgradeSnapshotRuntime
	recorder              events.EventRecorder
	clientFactory         raftops.OpenBaoClientFactory
	clusterOps            ClusterOps
	clientConfig          portopenbao.ClientConfig
	imageVerifier         imageverify.Verifier
	operatorImageVerifier imageverify.Verifier
	Platform              string
}

// NewManager constructs a Manager.
func NewManager(
	c client.Client,
	scheme *runtime.Scheme,
	infraRuntime portinfra.BlueGreenRuntime,
	backupRuntime portbackup.PreUpgradeSnapshotRuntime,
	clientConfig portopenbao.ClientConfig,
	imageVerifier imageverify.Verifier,
	operatorImageVerifier imageverify.Verifier,
	platform string,
	recorder ...events.EventRecorder,
) *Manager {
	var eventRecorder events.EventRecorder
	if len(recorder) > 0 {
		eventRecorder = recorder[0]
	}
	mgr := &Manager{
		client:                c,
		reader:                c,
		scheme:                scheme,
		infraRuntime:          infraRuntime,
		backupRuntime:         backupRuntime,
		recorder:              eventRecorder,
		clientFactory:         raftops.DefaultOpenBaoClientFactory,
		clientConfig:          clientConfig,
		imageVerifier:         imageVerifier,
		operatorImageVerifier: operatorImageVerifier,
		Platform:              platform,
	}
	mgr.clusterOps = newOpenBaoClusterOps(c, mgr.clientFactory)
	return mgr
}

// WithReader configures a live reader for lock/status read-before-write flows.
func (m *Manager) WithReader(reader client.Reader) *Manager {
	if reader != nil {
		m.reader = reader
	}
	return m
}

func NewManagerWithClientFactory(
	c client.Client,
	scheme *runtime.Scheme,
	infraRuntime portinfra.BlueGreenRuntime,
	backupRuntime portbackup.PreUpgradeSnapshotRuntime,
	clientFactory raftops.OpenBaoClientFactory,
	clientConfig portopenbao.ClientConfig,
	imageVerifier imageverify.Verifier,
	operatorImageVerifier imageverify.Verifier,
	platform string,
	recorder ...events.EventRecorder,
) *Manager {
	mgr := NewManager(c, scheme, infraRuntime, backupRuntime, clientConfig, imageVerifier, operatorImageVerifier, platform, recorder...)
	if clientFactory != nil {
		mgr.clientFactory = clientFactory
	}
	mgr.clusterOps = newOpenBaoClusterOps(c, mgr.clientFactory)
	return mgr
}

func requeueShort() recon.Result {
	return recon.Result{RequeueAfter: constants.RequeueShort}
}

func requeueStandard() recon.Result {
	return recon.Result{RequeueAfter: constants.RequeueStandard}
}

func requeueAfter(duration time.Duration) recon.Result {
	if duration <= 0 {
		return recon.Result{}
	}
	return recon.Result{RequeueAfter: duration}
}

// Reconcile manages the blue/green upgrade state machine.
// This implements the controller workload sub-reconciler contract.
// Returns (result, error) where result indicates whether (and when) reconciliation should be requeued.
//
// Note: Image verification for the Blue StatefulSet is handled by the infra reconciler which runs before this.
// For Green resources (StatefulSet and snapshot Jobs), we verify and pin digests here to ensure the
// target images are validated when ImageVerification is enabled.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	updateStrategy := "RollingUpdate"
	if cluster.Spec.Upgrade != nil {
		updateStrategy = string(cluster.Spec.Upgrade.Strategy)
	}
	logger.Info("Manager reconciling",
		"updateStrategy", updateStrategy,
		"currentVersion", cluster.Status.CurrentVersion,
		"specVersion", cluster.Spec.Version,
		"initialized", cluster.Status.Initialized,
		"blueGreenPhase", blueGreenPhaseString(cluster))

	// Use spec image (infra reconciler handles verification)
	verifiedImageDigest := cluster.Spec.Image

	handled, result := m.handleBreakGlassAck(logger, cluster)
	if handled {
		return result, nil
	}

	if handled, result, err := m.handleManualRollbackRequest(ctx, logger, cluster); handled || err != nil {
		return result, err
	}

	return m.reconcileBlueGreen(ctx, logger, cluster, verifiedImageDigest)
}
