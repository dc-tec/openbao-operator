package upgrade

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// UpgradeFailureReason returns the normalized failure reason for a rolling upgrade.
func UpgradeFailureReason(progress *openbaov1alpha1.UpgradeProgress) string {
	if progress == nil || progress.Failure == nil {
		return ""
	}
	return strings.TrimSpace(progress.Failure.Reason)
}

// UpgradeFailureMessage returns the normalized failure message for a rolling upgrade.
func UpgradeFailureMessage(progress *openbaov1alpha1.UpgradeProgress) string {
	if progress == nil || progress.Failure == nil {
		return ""
	}
	return strings.TrimSpace(progress.Failure.Message)
}

// UpgradeFailureAt returns the timestamp for the current rolling upgrade failure.
func UpgradeFailureAt(progress *openbaov1alpha1.UpgradeProgress) *metav1.Time {
	if progress == nil || progress.Failure == nil {
		return nil
	}
	return progress.Failure.At
}

// UpgradeFailed reports whether the rolling upgrade is currently in failed
// state, based on the normalized failure reason.
func UpgradeFailed(progress *openbaov1alpha1.UpgradeProgress) bool {
	return UpgradeFailureReason(progress) != ""
}
