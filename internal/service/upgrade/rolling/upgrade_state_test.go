package rolling

import (
	"testing"

	"github.com/stretchr/testify/require"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func TestDecideUpgrade(t *testing.T) {
	t.Parallel()
	failure := &openbaov1alpha1.ControllerErrorStatus{Reason: upgrade.ReasonUpgradeFailed}
	tests := []struct {
		name             string
		currentVersion   string
		targetVersion    string
		failure          *openbaov1alpha1.ControllerErrorStatus
		retry            string
		handledRetry     string
		wantAction       upgradeAction
		wantRetry        string
		wantIgnoredRetry string
	}{
		{name: "initial version", wantAction: upgradeIdle},
		{name: "matching version", currentVersion: "2.5.0", wantAction: upgradeIdle},
		{name: "new version", currentVersion: "2.4.0", wantAction: upgradeStart},
		{name: "downgrade still needs validation", currentVersion: "2.6.0", wantAction: upgradeStart},
		{name: "retry without observed version", retry: "retry-1", wantAction: upgradeIdle, wantIgnoredRetry: "retry-1"},
		{name: "retry while idle", currentVersion: "2.5.0", retry: "retry-1", wantAction: upgradeIdle, wantIgnoredRetry: "retry-1"},
		{name: "retry before start", currentVersion: "2.4.0", retry: "retry-1", wantAction: upgradeStart, wantIgnoredRetry: "retry-1"},
		{name: "active upgrade", targetVersion: "2.5.0", wantAction: upgradeResume},
		{name: "empty failure is not failed", targetVersion: "2.5.0", failure: &openbaov1alpha1.ControllerErrorStatus{}, wantAction: upgradeResume},
		{name: "retry during healthy upgrade", targetVersion: "2.5.0", retry: "retry-1", wantAction: upgradeResume, wantIgnoredRetry: "retry-1"},
		{name: "failed without retry", targetVersion: "2.5.0", failure: failure, wantAction: upgradeWaitForRetry},
		{name: "failed with empty retry", targetVersion: "2.5.0", failure: failure, retry: " \t", wantAction: upgradeWaitForRetry},
		{name: "failed with handled retry", targetVersion: "2.5.0", failure: failure, retry: " retry-1 ", handledRetry: "retry-1", wantAction: upgradeWaitForRetry},
		{name: "failed with fresh retry", targetVersion: "2.5.0", failure: failure, retry: " retry-2 ", handledRetry: "retry-1", wantAction: upgradeRetry, wantRetry: "retry-2"},
		{name: "retarget healthy upgrade", targetVersion: "2.4.1", wantAction: upgradeRetarget},
		{name: "retarget failed upgrade", targetVersion: "2.4.1", failure: failure, wantAction: upgradeRetarget},
		{name: "retarget takes precedence over retry", targetVersion: "2.4.1", failure: failure, retry: "retry-1", wantAction: upgradeRetarget, wantIgnoredRetry: "retry-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cluster := &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version: "2.5.0",
					Upgrade: &openbaov1alpha1.UpgradeConfig{Requests: &openbaov1alpha1.UpgradeRequestConfig{Retry: tt.retry}},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion:  tt.currentVersion,
					UpgradeRequests: &openbaov1alpha1.UpgradeRequestStatus{LastHandledRetry: tt.handledRetry},
				},
			}
			if tt.targetVersion != "" {
				cluster.Status.Upgrade = &openbaov1alpha1.UpgradeProgress{TargetVersion: tt.targetVersion, Failure: tt.failure}
			}
			before := cluster.DeepCopy()
			decision := decideUpgrade(cluster)
			require.Equal(t, upgradeDecision{
				action: tt.wantAction, retryRequest: tt.wantRetry,
				acknowledgements: upgrade.RequestAcknowledgements{Retry: tt.wantIgnoredRetry},
			}, decision)
			require.Equal(t, before, cluster, "decisions must preserve observed state")
		})
	}
}
