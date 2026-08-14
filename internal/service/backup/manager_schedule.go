package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/hardenedcontract"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

func validateBackupHardenedConfiguration(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Spec.Backup == nil {
		return nil
	}
	if cluster.Spec.Profile != openbaov1alpha1.ProfileHardened {
		return nil
	}
	if cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) == 0 {
		return operatorerrors.WithReason(
			constants.ReasonNetworkEgressRulesRequired,
			operatorerrors.WrapPermanentConfig(fmt.Errorf(
				"hardened profile with backups enabled requires explicit spec.network.egressRules so backup Jobs can reach the object storage endpoint",
			)),
		)
	}
	if !hardenedcontract.EgressRulesExplicit(cluster.Spec.Network.EgressRules) {
		return operatorerrors.WithReason(
			constants.ReasonSecurityViolation,
			operatorerrors.WrapPermanentConfig(fmt.Errorf(
				"hardened profile with backups enabled requires spec.network.egressRules entries to be port-scoped and target explicit non-wildcard peers",
			)),
		)
	}
	if violation := hardenedcontract.EvaluateStorageTarget("Backup", cluster.Spec.Backup.Target); violation != nil {
		return operatorerrors.WithReason(
			violation.Reason,
			operatorerrors.WrapPermanentConfig(fmt.Errorf("%s", violation.Message)),
		)
	}
	return nil
}

// checkBackupDue determines if a backup should be executed now.
// Returns (shouldReturn, result, error) where shouldReturn indicates early return.
func (m *Manager) checkBackupDue(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	now time.Time,
	scheduledTime time.Time,
	manualTrigger bool,
) (bool, recon.Result, error) {
	if manualTrigger || !now.Before(scheduledTime) {
		return false, recon.Result{}, nil
	}

	timeUntilDue := scheduledTime.Sub(now)
	logger.V(1).Info("Backup not due yet", "scheduledTime", scheduledTime, "now", now, "timeUntilDue", timeUntilDue)

	jobResult, err := m.checkForCompletedJobs(ctx, logger, cluster)
	if err != nil {
		return true, recon.Result{}, fmt.Errorf("failed to check for completed backup jobs: %w", err)
	}
	if jobResult.completed && backupOperationLock.IsHeldBy(cluster.Status.OperationLock) {
		if err := m.releaseBackupLock(ctx, logger, cluster, "after completed Job processing"); err != nil {
			return true, recon.Result{RequeueAfter: constants.RequeueShort}, nil
		}
	}
	if jobResult.statusUpdated {
		logger.Info("Found completed backup job, requesting requeue to persist status")
		return true, recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	return true, recon.Result{RequeueAfter: timeUntilDue}, nil
}
