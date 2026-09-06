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
	"github.com/dc-tec/openbao-operator/internal/port/adminops"
	portbackup "github.com/dc-tec/openbao-operator/internal/port/backup"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	portworkload "github.com/dc-tec/openbao-operator/internal/port/workload"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
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
	workloadRuntime       portworkload.BlueGreenRuntime
	backupRuntime         portbackup.PreUpgradeSnapshotRuntime
	recorder              events.EventRecorder
	clientFactory         raftops.OpenBaoClientFactory
	clusterOps            ClusterOps
	clientConfig          portopenbao.ClientConfig
	imageVerifier         imageverify.Verifier
	operatorImageVerifier imageverify.Verifier
	adminOpsMutator       adminops.StatusMutator
	Platform              string
}

// NewManager constructs a Manager.
func NewManager(
	c client.Client,
	scheme *runtime.Scheme,
	workloadRuntime portworkload.BlueGreenRuntime,
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
		workloadRuntime:       workloadRuntime,
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

// WithAdminOpsStatusMutator configures the adminops-plane status persistence hook.
func (m *Manager) WithAdminOpsStatusMutator(mutator adminops.StatusMutator) *Manager {
	if mutator != nil {
		m.adminOpsMutator = mutator
	}
	return m
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
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (outcome upgrade.ReconcileResult, err error) {
	outcome.Result, err = m.reconcile(ctx, logger, cluster, &outcome.Acknowledgements)
	return outcome, err
}

func (m *Manager) reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, acknowledgements *upgrade.RequestAcknowledgements) (recon.Result, error) {
	updateStrategy := string(upgrade.EffectiveStrategy(cluster))
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

	if handled, result, err := m.handleManualRollbackRequest(ctx, logger, cluster, acknowledgements); handled || err != nil {
		return result, err
	}

	return m.reconcileBlueGreen(ctx, logger, cluster, verifiedImageDigest, acknowledgements)
}
