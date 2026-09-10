package openbao

import "testing"

func TestAutopilotConfigMatches(t *testing.T) {
	const invalidDuration = "bad"
	base := AutopilotConfig{
		CleanupDeadServers: true, MinQuorum: 3, MaxTrailingLogs: 1000,
		DeadServerLastContactThreshold: "5m", LastContactThreshold: "10s", ServerStabilizationTime: "10s",
	}
	for _, tt := range []struct {
		name   string
		change func(*AutopilotConfig, *AutopilotConfig)
		want   bool
	}{
		{"unchanged", func(_, _ *AutopilotConfig) {}, true},
		{"equivalent durations", func(_, c *AutopilotConfig) {
			c.DeadServerLastContactThreshold = "300s"
			c.LastContactThreshold = "10000ms"
			c.ServerStabilizationTime = "0m10s"
		}, true},
		{"integer seconds", func(d, _ *AutopilotConfig) { d.LastContactThreshold = "10" }, true},
		{"integer days", func(d, c *AutopilotConfig) {
			d.DeadServerLastContactThreshold = "1d"
			c.DeadServerLastContactThreshold = "24h0m0s"
		}, true},
		{"fractional seconds truncate", func(d, _ *AutopilotConfig) { d.LastContactThreshold = "10999ms" }, true},
		{"observed fractional seconds are drift", func(_, c *AutopilotConfig) { c.LastContactThreshold = "10.5s" }, false},
		{"negative duration", func(d, c *AutopilotConfig) { d.LastContactThreshold = "-10s"; c.LastContactThreshold = "-10s" }, false},
		{"overflow seconds", func(d, _ *AutopilotConfig) { d.LastContactThreshold = "9223372036854775807" }, false},
		{"overflow days", func(d, _ *AutopilotConfig) { d.LastContactThreshold = "106752d" }, false},
		{"fractional days unsupported", func(d, _ *AutopilotConfig) { d.LastContactThreshold = "0.5d" }, false},
		{"cleanup false is managed", func(d, _ *AutopilotConfig) { d.CleanupDeadServers = false }, false},
		{"minimum quorum drift", func(_, c *AutopilotConfig) { c.MinQuorum = 5 }, false},
		{"trailing logs drift", func(_, c *AutopilotConfig) { c.MaxTrailingLogs++ }, false},
		{"dead server threshold drift", func(_, c *AutopilotConfig) { c.DeadServerLastContactThreshold = "6m" }, false},
		{"last contact drift", func(_, c *AutopilotConfig) { c.LastContactThreshold = "11s" }, false},
		{"stabilization drift", func(_, c *AutopilotConfig) { c.ServerStabilizationTime = "11s" }, false},
		{"omitted fields", func(d, _ *AutopilotConfig) { *d = AutopilotConfig{CleanupDeadServers: true} }, true},
		{"zero duration is managed", func(d, _ *AutopilotConfig) { d.LastContactThreshold = "0s" }, false},
		{"equivalent zero durations", func(d, c *AutopilotConfig) { d.LastContactThreshold = "0s"; c.LastContactThreshold = "0" }, true},
		{"malformed desired", func(d, c *AutopilotConfig) {
			d.LastContactThreshold = invalidDuration
			c.LastContactThreshold = invalidDuration
		}, false},
		{"malformed observed", func(_, c *AutopilotConfig) { c.LastContactThreshold = invalidDuration }, false},
		{"missing observed duration", func(_, c *AutopilotConfig) { c.LastContactThreshold = "" }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			desired, current := base, base
			tt.change(&desired, &current)
			if got := desired.Matches(current); got != tt.want {
				t.Errorf("Matches() = %t, want %t", got, tt.want)
			}
		})
	}
}
