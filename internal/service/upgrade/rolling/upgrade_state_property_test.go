package rolling

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/testutil/proptest"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"pgregory.net/rapid"
)

func TestUpgradeDecisionProperties(t *testing.T) {
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

		before := cluster.DeepCopy()
		decision := decideUpgrade(cluster)
		if !reflect.DeepEqual(before, cluster) {
			rt.Fatal("decision changed observed state")
		}
		pending := strings.TrimSpace(retryRequest) != "" && strings.TrimSpace(retryRequest) != strings.TrimSpace(lastHandledRetry)
		failed := upgrade.UpgradeFailed(upgradeProgress)
		matchingTarget := upgradeProgress != nil && upgradeProgress.TargetVersion == specVersion
		valid := false
		switch decision.action {
		case upgradeIdle:
			valid = upgradeProgress == nil && (currentVersion == "" || currentVersion == specVersion)
		case upgradeStart:
			valid = upgradeProgress == nil && currentVersion != "" && currentVersion != specVersion
		case upgradeResume:
			valid = matchingTarget && !failed
		case upgradeRetarget:
			valid = upgradeProgress != nil && !matchingTarget
		case upgradeRetry:
			valid = matchingTarget && failed && pending
		case upgradeWaitForRetry:
			valid = matchingTarget && failed && !pending
		}
		if !valid {
			rt.Fatalf("action %s violates its preconditions: cluster=%+v", decision.action, cluster)
		}
		wantRetry, wantIgnored := "", ""
		if pending {
			if matchingTarget && failed {
				wantRetry = strings.TrimSpace(retryRequest)
			} else {
				wantIgnored = strings.TrimSpace(retryRequest)
			}
		}
		if decision.retryRequest != wantRetry || decision.acknowledgements.Retry != wantIgnored {
			rt.Fatalf("retry=%q ignored=%q, want retry=%q ignored=%q", decision.retryRequest, decision.acknowledgements.Retry, wantRetry, wantIgnored)
		}
	})
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
