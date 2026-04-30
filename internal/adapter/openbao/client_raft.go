package openbao

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// JoinRaftClusterRequest represents the payload sent to PUT /v1/sys/storage/raft/join.
type JoinRaftClusterRequest struct {
	LeaderAPIAddr string `json:"leader_api_addr"`
	Retry         bool   `json:"retry,omitempty"`
	NonVoter      bool   `json:"non_voter,omitempty"`
}

// JoinRaftClusterResponse represents the response from PUT /v1/sys/storage/raft/join.
type JoinRaftClusterResponse struct {
	Joined bool `json:"joined"`
}

type raftPeerActionRequest struct {
	ServerID string `json:"server_id"`
}

// UpdateRaftConfigurationRequest represents the payload sent to PUT /v1/sys/storage/raft/configuration.
type UpdateRaftConfigurationRequest struct {
	Servers []portopenbao.RaftServer `json:"servers"`
}

// JoinRaftCluster joins a node to the Raft cluster.
func (c *Client) JoinRaftCluster(ctx context.Context, leaderAPIAddr string, retry bool, nonVoter bool) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for raft join operation")
	}
	if leaderAPIAddr == "" {
		return fmt.Errorf("leaderAPIAddr is required")
	}

	reqBody := JoinRaftClusterRequest{
		LeaderAPIAddr: leaderAPIAddr,
		Retry:         retry,
		NonVoter:      nonVoter,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal raft join request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, http.MethodPut, apiPathRaftJoin, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create raft join request: %w", err)
	}
	httpReq.Header.Set("X-Vault-Token", c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, body, err := c.doAndReadAll(httpReq, nil, "failed to execute raft join request")
	if err != nil {
		if translatedErr := translateRaftAPIErrorFromChain(err, portopenbao.ErrAlreadyJoined, "already joined"); translatedErr != nil {
			return translatedErr
		}
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		apiErr := portopenbao.NewAPIError("raft join request failed", resp.StatusCode, body)
		if translatedErr := translateRaftAPIErrorFromChain(apiErr, portopenbao.ErrAlreadyJoined, "already joined"); translatedErr != nil {
			return translatedErr
		}
		return apiErr
	}

	var joinResp JoinRaftClusterResponse
	if err := json.Unmarshal(body, &joinResp); err != nil {
		return nil
	}
	if !joinResp.Joined {
		return fmt.Errorf("node was not joined to cluster (already initialized as standalone)")
	}

	return nil
}

// ReadRaftConfiguration reads the current Raft cluster configuration.
func (c *Client) ReadRaftConfiguration(ctx context.Context) (*portopenbao.RaftConfigurationResponse, error) {
	if c.token == "" {
		return nil, fmt.Errorf("authentication token required for raft configuration read")
	}

	req, err := c.newRequest(ctx, http.MethodGet, apiPathRaftConfiguration, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft configuration request: %w", err)
	}
	req.Header.Set("X-Vault-Token", c.token)

	resp, body, err := c.doAndReadAll(req, nil, "failed to execute raft configuration request")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, portopenbao.NewAPIError("raft configuration request failed", resp.StatusCode, body)
	}

	type raftConfigEnvelope struct {
		Data *portopenbao.RaftConfigurationResponse `json:"data,omitempty"`
		portopenbao.RaftConfigurationResponse
	}

	var envelope raftConfigEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse raft configuration response: %w", err)
	}
	if envelope.Data != nil {
		return envelope.Data, nil
	}
	return &envelope.RaftConfigurationResponse, nil
}

// ReadRaftAutopilotState reads the Raft Autopilot cluster state.
func (c *Client) ReadRaftAutopilotState(ctx context.Context) (*portopenbao.RaftAutopilotStateResponse, error) {
	if c.token == "" {
		return nil, fmt.Errorf("authentication token required for raft autopilot state read")
	}

	req, err := c.newRequest(ctx, http.MethodGet, apiPathRaftAutopilotState, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft autopilot state request: %w", err)
	}
	req.Header.Set("X-Vault-Token", c.token)

	resp, body, err := c.doAndReadAll(req, nil, "failed to execute raft autopilot state request")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %w", ErrAutopilotNotAvailable, portopenbao.NewAPIError("raft autopilot state request failed", resp.StatusCode, body))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, portopenbao.NewAPIError("raft autopilot state request failed", resp.StatusCode, body)
	}

	type raftAutopilotEnvelope struct {
		Data *portopenbao.RaftAutopilotStateResponse `json:"data,omitempty"`
		portopenbao.RaftAutopilotStateResponse
	}

	var envelope raftAutopilotEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse raft autopilot state response: %w", err)
	}
	if envelope.Data != nil {
		return envelope.Data, nil
	}
	return &envelope.RaftAutopilotStateResponse, nil
}

// ConfigureRaftAutopilot sets the Raft Autopilot configuration.
func (c *Client) ConfigureRaftAutopilot(ctx context.Context, config portopenbao.AutopilotConfig) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for raft autopilot configuration")
	}

	bodyBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal autopilot config: %w", err)
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, apiPathRaftAutopilotConfig, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create autopilot config request: %w", err)
	}
	httpReq.Header.Set("X-Vault-Token", c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, body, err := c.doAndReadAll(httpReq, nil, "failed to execute autopilot config request")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return portopenbao.NewAPIError("autopilot config request failed", resp.StatusCode, body)
	}

	return nil
}

func (c *Client) executeRaftPeerAction(ctx context.Context, serverID string, path, action string) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for raft %s operation", action)
	}
	if serverID == "" {
		return fmt.Errorf("serverID is required")
	}

	bodyBytes, err := json.Marshal(raftPeerActionRequest{ServerID: serverID})
	if err != nil {
		return fmt.Errorf("failed to marshal raft %s request: %w", action, err)
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, path, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create raft %s request: %w", action, err)
	}
	httpReq.Header.Set("X-Vault-Token", c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, body, err := c.doAndReadAll(httpReq, nil, fmt.Sprintf("failed to execute raft %s request", action))
	if err != nil {
		if translatedErr := translateRaftPeerActionAPIError(err, action); translatedErr != nil {
			return translatedErr
		}
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		apiErr := portopenbao.NewAPIError(fmt.Sprintf("raft %s request failed", action), resp.StatusCode, body)
		if translatedErr := translateRaftPeerActionAPIError(apiErr, action); translatedErr != nil {
			return translatedErr
		}
		return apiErr
	}

	return nil
}

// RemoveRaftPeer removes a peer from the Raft cluster.
func (c *Client) RemoveRaftPeer(ctx context.Context, serverID string) error {
	return c.executeRaftPeerAction(ctx, serverID, apiPathRaftRemovePeer, "remove-peer")
}

// PromoteRaftPeer promotes a non-voter peer to a voter in the Raft cluster.
func (c *Client) PromoteRaftPeer(ctx context.Context, serverID string) error {
	return c.executeRaftPeerAction(ctx, serverID, apiPathRaftPromotePeer, "promote")
}

// DemoteRaftPeer demotes a voter peer to a non-voter in the Raft cluster.
func (c *Client) DemoteRaftPeer(ctx context.Context, serverID string) error {
	return c.executeRaftPeerAction(ctx, serverID, apiPathRaftDemotePeer, "demote")
}

// UpdateRaftConfiguration updates the Raft cluster configuration.
func (c *Client) UpdateRaftConfiguration(ctx context.Context, servers []portopenbao.RaftServer) error {
	if c.token == "" {
		return fmt.Errorf("authentication token required for raft configuration update")
	}
	if len(servers) == 0 {
		return fmt.Errorf("servers list cannot be empty")
	}

	reqBody := UpdateRaftConfigurationRequest{Servers: servers}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal raft configuration update request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, http.MethodPut, apiPathRaftUpdateConfig, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create raft configuration update request: %w", err)
	}
	httpReq.Header.Set("X-Vault-Token", c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, body, err := c.doAndReadAll(httpReq, nil, "failed to execute raft configuration update request")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return portopenbao.NewAPIError("raft configuration update request failed", resp.StatusCode, body)
	}

	return nil
}

func raftResponseContainsAny(responseBody []byte, patterns ...string) bool {
	message := strings.ToLower(string(responseBody))
	for _, pattern := range patterns {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}

func translateRaftAPIErrorFromChain(err error, sentinel error, patterns ...string) error {
	if err == nil || sentinel == nil {
		return nil
	}

	var apiErr *portopenbao.APIError
	if !errors.As(err, &apiErr) || apiErr == nil {
		return nil
	}

	if !raftResponseContainsAny([]byte(apiErr.ResponseBody), patterns...) {
		return nil
	}

	return fmt.Errorf("%w: %w", sentinel, apiErr)
}

func translateRaftPeerActionAPIError(err error, action string) error {
	switch action {
	case "promote":
		return translateRaftAPIErrorFromChain(err, portopenbao.ErrAlreadyVoter,
			"already a voter",
			"already voter",
			"not a non-voter",
			"not non-voter",
		)
	case "demote":
		return translateRaftAPIErrorFromChain(err, portopenbao.ErrAlreadyNonVoter,
			"already a non-voter",
			"already non-voter",
			"already non voter",
		)
	default:
		return nil
	}
}
