package rolling

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/testutil/proptest"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"pgregory.net/rapid"
)

func TestDetectUpgradeStateRetryProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		specVersion := versionGenerator().Draw(rt, "spec_version")
		currentVersion := versionGenerator().Draw(rt, "current_version")
		retryRequest := rollingRequestValueGenerator().Draw(rt, "retry_request")
		lastHandledRetry := rollingRequestValueGenerator().Draw(rt, "last_handled_retry")
		upgradeProgress := upgradeProgressGenerator(specVersion).Draw(rt, "upgrade_progress")

		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "property-upgrade",
				Namespace: "default",
			},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Version: specVersion,
				Upgrade: &openbaov1alpha1.UpgradeConfig{
					Requests: &openbaov1alpha1.UpgradeRequestConfig{
						Retry: retryRequest,
					},
				},
			},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				CurrentVersion: currentVersion,
				Upgrade:        upgradeProgress,
				UpgradeRequests: &openbaov1alpha1.UpgradeRequestStatus{
					LastHandledRetry: lastHandledRetry,
				},
			},
		}

		wantUpgradeNeeded, wantResumeUpgrade, wantHandledRetry := expectedDetectUpgradeState(cluster)

		gotUpgradeNeeded, gotResumeUpgrade := (&Manager{}).detectUpgradeState(logr.Discard(), cluster)

		if gotUpgradeNeeded != wantUpgradeNeeded {
			t.Fatalf("upgradeNeeded = %t, want %t for cluster=%+v", gotUpgradeNeeded, wantUpgradeNeeded, cluster)
		}
		if gotResumeUpgrade != wantResumeUpgrade {
			t.Fatalf("resumeUpgrade = %t, want %t for cluster=%+v", gotResumeUpgrade, wantResumeUpgrade, cluster)
		}
		gotHandledRetry := ""
		if cluster.Status.UpgradeRequests != nil {
			gotHandledRetry = cluster.Status.UpgradeRequests.LastHandledRetry
		}
		if gotHandledRetry != wantHandledRetry {
			t.Fatalf("LastHandledRetry = %q, want %q for retryRequest=%q upgrade=%+v",
				gotHandledRetry, wantHandledRetry, retryRequest, upgradeProgress)
		}
	})
}

func expectedDetectUpgradeState(cluster *openbaov1alpha1.OpenBaoCluster) (bool, bool, string) {
	storedHandledRetry := ""
	handledRetry := ""
	if cluster.Status.UpgradeRequests != nil {
		storedHandledRetry = cluster.Status.UpgradeRequests.LastHandledRetry
		handledRetry = strings.TrimSpace(storedHandledRetry)
	}
	retryRequest := upgrade.RetryRequestValue(cluster)
	retryPending := retryRequest != "" && retryRequest != handledRetry

	if retryPending &&
		(cluster.Status.Upgrade == nil ||
			!upgrade.UpgradeFailed(cluster.Status.Upgrade) ||
			cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion) {
		storedHandledRetry = retryRequest
		retryPending = false
	}

	if cluster.Status.Upgrade != nil {
		if upgrade.UpgradeFailed(cluster.Status.Upgrade) {
			if cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion {
				return false, true, storedHandledRetry
			}
			if !retryPending {
				return false, false, storedHandledRetry
			}
			return false, true, storedHandledRetry
		}
		return false, true, storedHandledRetry
	}

	if cluster.Status.CurrentVersion == "" {
		return false, false, storedHandledRetry
	}
	if cluster.Spec.Version == cluster.Status.CurrentVersion {
		return false, false, storedHandledRetry
	}
	return true, false, storedHandledRetry
}

func upgradeProgressGenerator(specVersion string) *rapid.Generator[*openbaov1alpha1.UpgradeProgress] {
	return rapid.Custom(func(t *rapid.T) *openbaov1alpha1.UpgradeProgress {
		if !rapid.Bool().Draw(t, "has_upgrade_progress") {
			return nil
		}
		targetVersion := versionGenerator().Draw(t, "target_version")
		if rapid.Bool().Draw(t, "target_matches_spec") {
			targetVersion = specVersion
		}
		progress := &openbaov1alpha1.UpgradeProgress{
			FromVersion:      versionGenerator().Draw(t, "from_version"),
			TargetVersion:    targetVersion,
			CurrentPartition: int32(rapid.IntRange(0, 5).Draw(t, "current_partition")),
		}
		switch rapid.IntRange(0, 2).Draw(t, "failure_mode") {
		case 1:
			progress.Failure = &openbaov1alpha1.ControllerErrorStatus{
				Reason:  rollingReasonGenerator().Draw(t, "structured_reason"),
				Message: "structured failure",
			}
		case 2:
			progress.Failure = &openbaov1alpha1.ControllerErrorStatus{}
		}
		return progress
	})
}

func versionGenerator() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.Just(""),
		rapid.Map(rapid.IntRange(1, 4), func(minor int) string {
			return fmt.Sprintf("2.%d.0", minor)
		}),
	)
}

func rollingReasonGenerator() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{
		upgrade.ReasonUpgradeFailed,
		upgrade.ReasonPodNotReady,
		upgrade.ReasonStepDownTimeout,
	})
}

func rollingRequestValueGenerator() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		prefix := rapid.SampledFrom([]string{"", " ", "\t"}).Draw(t, "prefix")
		value := proptest.OptionalIdentifier().Draw(t, "value")
		suffix := rapid.SampledFrom([]string{"", " ", "\n"}).Draw(t, "suffix")
		return prefix + value + suffix
	})
}
