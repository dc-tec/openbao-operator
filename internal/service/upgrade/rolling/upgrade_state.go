package rolling

import (
	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

type upgradeAction string

const (
	upgradeIdle         upgradeAction = "idle"
	upgradeStart        upgradeAction = "start"
	upgradeResume       upgradeAction = "resume"
	upgradeRetry        upgradeAction = "retry"
	upgradeRetarget     upgradeAction = "retarget"
	upgradeWaitForRetry upgradeAction = "wait-for-retry"
)

type upgradeDecision struct {
	action           upgradeAction
	retryRequest     string
	acknowledgements upgrade.RequestAcknowledgements
}

// decideUpgrade selects work from the observed state without changing it.
// A retry is acknowledged by the retry checkpoint; an ignored request is
// acknowledged by the enclosing AdminOps reconciliation.
func decideUpgrade(cluster *openbaov1alpha1.OpenBaoCluster) upgradeDecision {
	progress := cluster.Status.Upgrade
	retryPending := upgrade.RetryRequestPending(cluster)
	decision := upgradeDecision{}
	switch {
	case progress == nil:
		decision.action = upgradeIdle
		if cluster.Status.CurrentVersion != "" && cluster.Spec.Version != cluster.Status.CurrentVersion {
			decision.action = upgradeStart
		}
	case cluster.Spec.Version != progress.TargetVersion:
		decision.action = upgradeRetarget
	case !upgrade.UpgradeFailed(progress):
		decision.action = upgradeResume
	case retryPending:
		decision.action = upgradeRetry
		decision.retryRequest = upgrade.RetryRequestValue(cluster)
	default:
		decision.action = upgradeWaitForRetry
	}
	if retryPending && decision.action != upgradeRetry {
		decision.acknowledgements.Retry = upgrade.RetryRequestValue(cluster)
	}
	return decision
}
