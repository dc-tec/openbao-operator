package openbao

import "context"

// AutopilotConfig represents the configuration for Raft Autopilot.
type AutopilotConfig struct {
	CleanupDeadServers             bool   `json:"cleanup_dead_servers"`
	DeadServerLastContactThreshold string `json:"dead_server_last_contact_threshold,omitempty"`
	MinQuorum                      int    `json:"min_quorum,omitempty"`
	LastContactThreshold           string `json:"last_contact_threshold,omitempty"`
	MaxTrailingLogs                int    `json:"max_trailing_logs,omitempty"`
	ServerStabilizationTime        string `json:"server_stabilization_time,omitempty"`
}

// AutopilotConfigurer configures Raft Autopilot state on an authenticated OpenBao client.
type AutopilotConfigurer interface {
	ConfigureRaftAutopilot(ctx context.Context, config AutopilotConfig) error
}
