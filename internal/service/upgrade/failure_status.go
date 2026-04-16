package upgrade

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// UpgradeFailureReason returns the normalized failure reason for a rolling
// upgrade. Structured Failure is preferred; legacy fields are fallback.
func UpgradeFailureReason(progress *openbaov1alpha1.UpgradeProgress) string {
	if progress == nil {
		return ""
	}
	if progress.Failure != nil {
		return strings.TrimSpace(progress.Failure.Reason)
	}
	return strings.TrimSpace(progress.LastErrorReason)
}

// UpgradeFailureMessage returns the normalized failure message for a rolling
// upgrade. Structured Failure is preferred; legacy fields are fallback.
func UpgradeFailureMessage(progress *openbaov1alpha1.UpgradeProgress) string {
	if progress == nil {
		return ""
	}
	if progress.Failure != nil {
		return strings.TrimSpace(progress.Failure.Message)
	}
	return strings.TrimSpace(progress.LastErrorMessage)
}

// UpgradeFailureAt returns the timestamp for the current rolling upgrade
// failure, preferring structured Failure.
func UpgradeFailureAt(progress *openbaov1alpha1.UpgradeProgress) *metav1.Time {
	if progress == nil {
		return nil
	}
	if progress.Failure != nil {
		return progress.Failure.At
	}
	return progress.LastErrorAt
}

// UpgradeFailed reports whether the rolling upgrade is currently in failed
// state, based on the normalized failure reason.
func UpgradeFailed(progress *openbaov1alpha1.UpgradeProgress) bool {
	return UpgradeFailureReason(progress) != ""
}
