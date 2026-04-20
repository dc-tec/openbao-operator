package openbao

import (
	"context"
	"encoding/json"
)

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

// RaftAutopilotServerState represents one server observed in the Autopilot state.
type RaftAutopilotServerState struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Address     string          `json:"address"`
	NodeStatus  string          `json:"node_status"`
	LastContact string          `json:"last_contact"`
	LastTerm    uint64          `json:"last_term"`
	LastIndex   uint64          `json:"last_index"`
	Healthy     bool            `json:"healthy"`
	StableSince string          `json:"stable_since"`
	Status      string          `json:"status"`
	Meta        json.RawMessage `json:"meta,omitempty"`
}

// RaftAutopilotStateResponse represents the response from the raft Autopilot state API.
type RaftAutopilotStateResponse struct {
	Healthy          bool                                `json:"healthy"`
	FailureTolerance int                                 `json:"failure_tolerance"`
	Servers          map[string]RaftAutopilotServerState `json:"servers"`
	Leader           string                              `json:"leader"`
	Voters           []string                            `json:"voters"`
	NonVoters        []string                            `json:"non_voters"`
}
