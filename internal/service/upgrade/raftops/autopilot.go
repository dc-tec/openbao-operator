package raftops

import (
	"fmt"
	"sort"
	"strings"

	openbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// GreenAutopilotServerObservation captures a Green server seen during sync evaluation.
type GreenAutopilotServerObservation struct {
	PodName string
	Server  openbao.RaftAutopilotServerState
}

// GreenSyncEvaluation describes how far Green servers have progressed toward
// the required sync target.
type GreenSyncEvaluation struct {
	AllSynced        bool
	MaxDelta         uint64
	MissingGreen     int
	UnhealthyGreen   int
	MissingPods      []string
	UnhealthyServers []GreenAutopilotServerObservation
}

// EvaluateGreenSyncFromAutopilot computes Green sync progress from autopilot state.
func EvaluateGreenSyncFromAutopilot(cfg *ExecutorConfig, state *openbao.RaftAutopilotStateResponse, targetIndex uint64) GreenSyncEvaluation {
	evaluation := GreenSyncEvaluation{
		AllSynced: true,
	}

	if cfg == nil || state == nil {
		evaluation.AllSynced = false
		return evaluation
	}

	for _, i := range ReplicaOrdinals(cfg.ClusterReplicas) {
		greenPodName := RevisionPodName(cfg.ClusterName, cfg.GreenRevision, i)
		server, found := FindAutopilotServerForPod(state, greenPodName)
		if !found {
			evaluation.AllSynced = false
			evaluation.MissingGreen++
			evaluation.MissingPods = append(evaluation.MissingPods, greenPodName)
			continue
		}

		if !server.Healthy {
			evaluation.UnhealthyGreen++
			evaluation.UnhealthyServers = append(evaluation.UnhealthyServers, GreenAutopilotServerObservation{
				PodName: greenPodName,
				Server:  server,
			})
		}

		var delta uint64
		if targetIndex > server.LastIndex {
			delta = targetIndex - server.LastIndex
		}
		if delta > evaluation.MaxDelta {
			evaluation.MaxDelta = delta
		}
		if delta > cfg.SyncThreshold {
			evaluation.AllSynced = false
		}
	}

	return evaluation
}

// FindAutopilotServerForPod finds the autopilot server state for the pod.
func FindAutopilotServerForPod(state *openbao.RaftAutopilotStateResponse, podName string) (openbao.RaftAutopilotServerState, bool) {
	if state == nil {
		return openbao.RaftAutopilotServerState{}, false
	}

	for _, server := range state.Servers {
		if RaftAutopilotServerMatchesPod(server, podName) {
			return server, true
		}
	}

	return openbao.RaftAutopilotServerState{}, false
}

// AutopilotServerDebugNames returns stable debug names for logging.
func AutopilotServerDebugNames(state *openbao.RaftAutopilotStateResponse) []string {
	if state == nil {
		return nil
	}

	serverNames := make([]string, 0, len(state.Servers))
	for key, server := range state.Servers {
		serverNames = append(serverNames, fmt.Sprintf("%s(id=%s,name=%s,addr=%s)", key, server.ID, server.Name, server.Address))
	}
	sort.Strings(serverNames)
	return serverNames
}

// RaftAutopilotLeaderLastIndex returns the leader last index when identifiable.
func RaftAutopilotLeaderLastIndex(state *openbao.RaftAutopilotStateResponse) (uint64, bool) {
	if state == nil {
		return 0, false
	}

	if state.Leader != "" {
		if server, ok := state.Servers[state.Leader]; ok {
			return server.LastIndex, true
		}

		for _, server := range state.Servers {
			if server.ID == state.Leader || server.Name == state.Leader || server.Status == "leader" {
				return server.LastIndex, true
			}
		}
	}

	for _, server := range state.Servers {
		if server.Status == "leader" {
			return server.LastIndex, true
		}
	}

	return 0, false
}

// RaftAutopilotMaxLastIndex returns the maximum last index across all servers.
func RaftAutopilotMaxLastIndex(state *openbao.RaftAutopilotStateResponse) uint64 {
	if state == nil {
		return 0
	}

	var max uint64
	for _, server := range state.Servers {
		if server.LastIndex > max {
			max = server.LastIndex
		}
	}

	return max
}

// RaftAutopilotServerMatchesPod reports whether the server maps to the pod.
func RaftAutopilotServerMatchesPod(server openbao.RaftAutopilotServerState, podName string) bool {
	if podName == "" {
		return false
	}

	if server.ID == podName || server.Name == podName {
		return true
	}

	return strings.Contains(server.Address, podName)
}

// CountMissingGreenServers counts how many Green servers are absent from raft config.
func CountMissingGreenServers(cfg *ExecutorConfig, config *portopenbao.RaftConfigurationResponse) int {
	if cfg == nil || config == nil {
		return 0
	}

	missing := 0
	for _, i := range ReplicaOrdinals(cfg.ClusterReplicas) {
		greenPodName := RevisionPodName(cfg.ClusterName, cfg.GreenRevision, i)
		found := false
		for _, server := range config.Config.Servers {
			if server.NodeID == greenPodName || strings.Contains(server.Address, greenPodName) {
				found = true
				break
			}
		}
		if !found {
			missing++
		}
	}

	return missing
}
