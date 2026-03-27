package upgrade

import (
	"fmt"
	"sort"
	"strings"

	openbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

type greenAutopilotServerObservation struct {
	PodName string
	Server  openbao.RaftAutopilotServerState
}

type greenSyncEvaluation struct {
	AllSynced        bool
	MaxDelta         uint64
	MissingGreen     int
	UnhealthyGreen   int
	MissingPods      []string
	UnhealthyServers []greenAutopilotServerObservation
}

func evaluateGreenSyncFromAutopilot(cfg *ExecutorConfig, state *openbao.RaftAutopilotStateResponse, targetIndex uint64) greenSyncEvaluation {
	evaluation := greenSyncEvaluation{
		AllSynced: true,
	}

	if cfg == nil || state == nil {
		evaluation.AllSynced = false
		return evaluation
	}

	for _, i := range replicaOrdinals(cfg.ClusterReplicas) {
		greenPodName := revisionPodName(cfg.ClusterName, cfg.GreenRevision, i)
		server, found := findAutopilotServerForPod(state, greenPodName)
		if !found {
			evaluation.AllSynced = false
			evaluation.MissingGreen++
			evaluation.MissingPods = append(evaluation.MissingPods, greenPodName)
			continue
		}

		if !server.Healthy {
			evaluation.UnhealthyGreen++
			evaluation.UnhealthyServers = append(evaluation.UnhealthyServers, greenAutopilotServerObservation{
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

func findAutopilotServerForPod(state *openbao.RaftAutopilotStateResponse, podName string) (openbao.RaftAutopilotServerState, bool) {
	if state == nil {
		return openbao.RaftAutopilotServerState{}, false
	}

	for _, server := range state.Servers {
		if raftAutopilotServerMatchesPod(server, podName) {
			return server, true
		}
	}

	return openbao.RaftAutopilotServerState{}, false
}

func autopilotServerDebugNames(state *openbao.RaftAutopilotStateResponse) []string {
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

func raftAutopilotLeaderLastIndex(state *openbao.RaftAutopilotStateResponse) (uint64, bool) {
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

func raftAutopilotMaxLastIndex(state *openbao.RaftAutopilotStateResponse) uint64 {
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

func raftAutopilotServerMatchesPod(server openbao.RaftAutopilotServerState, podName string) bool {
	if podName == "" {
		return false
	}

	if server.ID == podName || server.Name == podName {
		return true
	}

	return strings.Contains(server.Address, podName)
}

func countMissingGreenServers(cfg *ExecutorConfig, config *portopenbao.RaftConfigurationResponse) int {
	if cfg == nil || config == nil {
		return 0
	}

	missing := 0
	for _, i := range replicaOrdinals(cfg.ClusterReplicas) {
		greenPodName := revisionPodName(cfg.ClusterName, cfg.GreenRevision, i)
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
