package openbao

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
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

// Matches reports whether current agrees with the fields sent by the desired
// configuration. Zero integers and empty durations retain their omitempty meaning.
func (desired AutopilotConfig) Matches(current AutopilotConfig) bool {
	return desired.CleanupDeadServers == current.CleanupDeadServers &&
		(desired.MinQuorum == 0 || desired.MinQuorum == current.MinQuorum) &&
		(desired.MaxTrailingLogs == 0 || desired.MaxTrailingLogs == current.MaxTrailingLogs) &&
		managedDurationMatches(desired.DeadServerLastContactThreshold, current.DeadServerLastContactThreshold) &&
		managedDurationMatches(desired.LastContactThreshold, current.LastContactThreshold) &&
		managedDurationMatches(desired.ServerStabilizationTime, current.ServerStabilizationTime)
}

func managedDurationMatches(desired, current string) bool {
	if desired == "" {
		return true
	}
	desiredDuration, valid := parseAutopilotDuration(desired)
	currentDuration, currentErr := time.ParseDuration(current)
	// OpenBao's TypeDurationSecond fields truncate the supplied duration to seconds.
	return valid && currentErr == nil && desiredDuration.Truncate(time.Second) == currentDuration
}

// OpenBao accepts Go durations, integer seconds, and integer days.
func parseAutopilotDuration(value string) (time.Duration, bool) {
	unit := time.Second
	number := value
	if strings.HasSuffix(value, "d") {
		unit = 24 * time.Hour
		number = strings.TrimSuffix(value, "d")
	}
	if n, err := strconv.ParseInt(number, 10, 64); err == nil {
		if n < 0 || n > int64((1<<63-1)/unit) {
			return 0, false
		}
		return time.Duration(n) * unit, true
	}
	duration, err := time.ParseDuration(value)
	return duration, err == nil && duration >= 0
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
