package openbaocluster

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	upgradeRequestRetryFieldPath   = "spec.upgrade.requests.retry"
	upgradeRequestPromoteFieldPath = "spec.upgrade.requests.promote"
)

// buildUpgradingCondition builds the Upgrading condition based on upgrade state.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildUpgradingCondition(cluster *openbaov1alpha1.OpenBaoCluster) metav1.Condition {
	rollingUpgradeInProgress := cluster.Status.Upgrade != nil
	upgradeFailed := rollingUpgradeInProgress && rollingUpgradeFailed(cluster.Status.Upgrade)

	blueGreenInProgress := cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.Phase != "" &&
		cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle

	if upgradeFailed && cluster.Status.Upgrade != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionUpgrading),
			Status:  metav1.ConditionFalse,
			Reason:  rollingUpgradeFailureReason(cluster.Status.Upgrade),
			Message: buildRollingUpgradeFailedMessage(cluster),
		}
	}

	if rollingUpgradeInProgress && !upgradeFailed {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionUpgrading),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonInProgress,
			Message: buildRollingUpgradeInProgressMessage(cluster),
		}
	}

	if blueGreenInProgress && cluster.Status.BlueGreen != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionUpgrading),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonInProgress,
			Message: buildBlueGreenUpgradeMessage(cluster),
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionUpgrading),
		Status:  metav1.ConditionFalse,
		Reason:  reasonIdle,
		Message: "No upgrade is currently in progress",
	}
}

func buildBreakGlassConditionMessage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	message := "Break glass mode is active."
	if cluster != nil && cluster.Status.BreakGlass != nil {
		if detail := strings.TrimSpace(cluster.Status.BreakGlass.Message); detail != "" {
			message = ensureSentence(detail)
		}
	}

	return message + " Next step: follow status.breakGlass.steps and set spec.breakGlassAck to status.breakGlass.nonce when it is safe to resume automation."
}

func buildRollingUpgradeInProgressMessage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	from, to := rollingVersionRange(cluster)

	if cluster == nil || cluster.Status.Upgrade == nil {
		return fmt.Sprintf("Rolling upgrade from %s to %s is in progress.", from, to)
	}

	return fmt.Sprintf(
		"Rolling upgrade from %s to %s is in progress (partition=%d).",
		from,
		to,
		cluster.Status.Upgrade.CurrentPartition,
	)
}

func buildRollingUpgradeFailedMessage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	from, to := rollingVersionRange(cluster)
	detail := "The operator recorded a failure."
	if cluster != nil && cluster.Status.Upgrade != nil {
		if message := rollingUpgradeFailureMessage(cluster.Status.Upgrade); message != "" {
			detail = ensureSentence(message)
		}
	}

	return fmt.Sprintf(
		"Rolling upgrade from %s to %s is paused. %s Next step: set %s to a new non-empty value on this OpenBaoCluster to retry.",
		from,
		to,
		detail,
		upgradeRequestRetryFieldPath,
	)
}

func buildBlueGreenUpgradeMessage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	from, to := blueGreenVersionRange(cluster)
	if cluster == nil || cluster.Status.BlueGreen == nil {
		return fmt.Sprintf("Blue/green upgrade from %s to %s is in progress.", from, to)
	}

	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active {
		return fmt.Sprintf(
			"Blue/green upgrade from %s to %s is paused in break glass mode. %s",
			from,
			to,
			buildBreakGlassConditionMessage(cluster),
		)
	}

	status := cluster.Status.BlueGreen
	greenRevision := fallbackLabel(status.GreenRevision, "pending")
	blueRevision := fallbackLabel(status.BlueRevision, "current")

	switch status.Phase {
	case openbaov1alpha1.PhaseDeployingGreen:
		return fmt.Sprintf("Blue/green upgrade from %s to %s is deploying Green revision %s.", from, to, greenRevision)
	case openbaov1alpha1.PhaseJoiningMesh:
		return fmt.Sprintf("Blue/green upgrade from %s to %s is joining Green revision %s to the Raft mesh.", from, to, greenRevision)
	case openbaov1alpha1.PhaseSyncing:
		if manualApprovalRequired(cluster) {
			return fmt.Sprintf(
				"Blue/green upgrade from %s to %s is syncing Green revision %s. Manual promotion is required for this upgrade. Next step: set %s to a new non-empty value when you want the operator to promote Green.",
				from,
				to,
				greenRevision,
				upgradeRequestPromoteFieldPath,
			)
		}
		return fmt.Sprintf("Blue/green upgrade from %s to %s is verifying Green revision %s before promotion.", from, to, greenRevision)
	case openbaov1alpha1.PhasePromoting:
		return fmt.Sprintf("Blue/green upgrade from %s to %s is promoting Green revision %s.", from, to, greenRevision)
	case openbaov1alpha1.PhaseDemotingBlue:
		return fmt.Sprintf("Blue/green upgrade from %s to %s is demoting Blue revision %s after promoting Green revision %s.", from, to, blueRevision, greenRevision)
	case openbaov1alpha1.PhaseCleanup:
		return fmt.Sprintf("Blue/green upgrade from %s to %s is cleaning up Blue revision %s after promoting Green revision %s.", from, to, blueRevision, greenRevision)
	case openbaov1alpha1.PhaseRollingBack:
		return fmt.Sprintf(
			"Blue/green upgrade from %s to %s is rolling back to Blue revision %s. %s",
			from,
			to,
			blueRevision,
			rollbackReasonSentence(status.RollbackReason),
		)
	case openbaov1alpha1.PhaseRollbackCleanup:
		return fmt.Sprintf(
			"Blue/green upgrade from %s to %s is finalizing rollback to Blue revision %s. %s",
			from,
			to,
			blueRevision,
			rollbackReasonSentence(status.RollbackReason),
		)
	default:
		return fmt.Sprintf("Blue/green upgrade from %s to %s is in phase %s.", from, to, status.Phase)
	}
}

func rollingVersionRange(cluster *openbaov1alpha1.OpenBaoCluster) (string, string) {
	if cluster == nil || cluster.Status.Upgrade == nil {
		return fallbackLabel("", "unknown"), fallbackLabel("", "unknown")
	}

	return fallbackLabel(cluster.Status.Upgrade.FromVersion, "unknown"), fallbackLabel(cluster.Status.Upgrade.TargetVersion, "unknown")
}

func blueGreenVersionRange(cluster *openbaov1alpha1.OpenBaoCluster) (string, string) {
	if cluster == nil {
		return "unknown", "unknown"
	}

	return fallbackLabel(cluster.Status.CurrentVersion, "unknown"), fallbackLabel(cluster.Spec.Version, "unknown")
}

func manualApprovalRequired(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil &&
		cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.ManualPromotionRequired
}

func rollbackReasonSentence(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "Rollback is active."
	}
	return ensureSentence("Rollback reason: " + reason)
}

func fallbackLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func ensureSentence(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	switch {
	case strings.HasSuffix(message, "."),
		strings.HasSuffix(message, "!"),
		strings.HasSuffix(message, "?"):
		return message
	default:
		return message + "."
	}
}

func rollingUpgradeFailureReason(progress *openbaov1alpha1.UpgradeProgress) string {
	if progress == nil {
		return ""
	}
	if progress.Failure != nil {
		return strings.TrimSpace(progress.Failure.Reason)
	}
	return strings.TrimSpace(progress.LastErrorReason)
}

func rollingUpgradeFailureMessage(progress *openbaov1alpha1.UpgradeProgress) string {
	if progress == nil {
		return ""
	}
	if progress.Failure != nil {
		return strings.TrimSpace(progress.Failure.Message)
	}
	return strings.TrimSpace(progress.LastErrorMessage)
}

func rollingUpgradeFailed(progress *openbaov1alpha1.UpgradeProgress) bool {
	return rollingUpgradeFailureReason(progress) != ""
}
