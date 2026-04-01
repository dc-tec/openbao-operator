package upgrade

import "testing"

func TestProgressMetricHelpers_HandleNil(t *testing.T) {
	t.Parallel()

	SetRunningProgressMetrics(nil, 3, 1, 2)
	SetInactiveProgressMetrics(nil)
	SetTerminalProgressMetrics(nil, UpgradeStatusSuccess)
	ClearProgressMetrics(nil)
}
