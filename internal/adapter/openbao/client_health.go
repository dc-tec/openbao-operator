package openbao

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// LeaderStatusResponse represents the response from GET /v1/sys/leader.
type LeaderStatusResponse struct {
	HAEnabled            bool   `json:"ha_enabled"`
	IsSelf               bool   `json:"is_self"`
	LeaderAddress        string `json:"leader_address"`
	LeaderClusterAddress string `json:"leader_cluster_address"`
}

// Health queries the OpenBao health endpoint and returns the current node state.
// This endpoint does not require authentication by default.
func (c *Client) Health(ctx context.Context) (*portopenbao.HealthStatus, error) {
	req, err := c.newRequest(ctx, http.MethodGet, apiPathSysHealth, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create health request: %w", err)
	}
	if c.hasAuth() {
		if err := c.authorize(req); err != nil {
			return nil, fmt.Errorf("failed to authorize health request: %w", err)
		}
	}

	statusCode, body, err := c.doAndReadAll(req, nil, "failed to query health endpoint")
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusForbidden {
		return nil, fmt.Errorf("health check forbidden: check operator permissions: %w",
			portopenbao.NewAPIError("health check request failed", statusCode, body),
		)
	}

	var health portopenbao.HealthStatus
	if err := json.Unmarshal(body, &health); err != nil {
		return nil, fmt.Errorf("failed to parse health response: %w", err)
	}

	return &health, nil
}

// IsLeader determines if the connected node is the Raft leader.
func (c *Client) IsLeader(ctx context.Context) (bool, error) {
	health, err := c.Health(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check leader status: %w", err)
	}
	return health.Initialized && !health.Sealed && !health.Standby && !health.PerformanceStandby, nil
}

// StepDown requests the leader to step down and trigger a new election.
func (c *Client) StepDown(ctx context.Context) error {
	if err := c.requireAuth("step-down operation"); err != nil {
		return err
	}

	req, err := c.newRequest(ctx, http.MethodPut, apiPathSysStepDown, nil)
	if err != nil {
		return fmt.Errorf("failed to create step-down request: %w", err)
	}
	if err := c.authorizeStepDown(req); err != nil {
		return fmt.Errorf("failed to authorize step-down request: %w", err)
	}

	statusCode, body, err := c.doAndReadAll(req, nil, "failed to execute step-down request")
	if err != nil {
		return err
	}
	if statusCode != http.StatusNoContent && statusCode != http.StatusOK {
		return portopenbao.NewAPIError("step-down request failed", statusCode, body)
	}

	return nil
}

func (c *Client) authorizeStepDown(req *http.Request) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	inlineAuth, ok := c.auth.(inlineJWTAuthorizer)
	if !ok {
		return c.authorize(req)
	}

	// OpenBao checks sys/step-down permissions against a persisted token entry,
	// so inline auth cannot satisfy this endpoint's sudo/root policy check.
	token, _, err := c.LoginJWT(req.Context(), inlineAuth.role, inlineAuth.jwt)
	if err != nil {
		return fmt.Errorf("failed to authenticate using standard JWT for step-down request: %w", err)
	}
	req.Header.Set(headerVaultToken, token)
	req.Header.Del(headerInlineAuthPath)
	req.Header.Del(headerInlineAuthOperation)
	req.Header.Del(headerInlineAuthParameterRole)
	req.Header.Del(headerInlineAuthParameterJWT)
	return nil
}

// LeaderStatus queries the OpenBao leader endpoint and returns the leader status.
func (c *Client) LeaderStatus(ctx context.Context) (*LeaderStatusResponse, error) {
	req, err := c.newRequest(ctx, http.MethodGet, apiPathSysLeader, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create leader status request: %w", err)
	}

	_, body, err := c.doAndReadAll(req, nil, "failed to query leader status endpoint")
	if err != nil {
		return nil, err
	}

	var leaderStatus LeaderStatusResponse
	if err := json.Unmarshal(body, &leaderStatus); err != nil {
		return nil, fmt.Errorf("failed to parse leader status response: %w", err)
	}

	return &leaderStatus, nil
}

// IsHealthy returns true if the node is initialized, unsealed, and reachable.
func (c *Client) IsHealthy(ctx context.Context) (bool, error) {
	health, err := c.Health(ctx)
	if err != nil {
		return false, err
	}
	if !health.Initialized || health.Sealed {
		return false, nil
	}
	if health.Standby {
		leaderStatus, err := c.LeaderStatus(ctx)
		if err != nil {
			return false, nil
		}
		if leaderStatus.LeaderAddress == "" {
			return false, nil
		}
	}

	return true, nil
}

// IsSealed checks if the OpenBao cluster is sealed.
func (c *Client) IsSealed(ctx context.Context) (bool, error) {
	health, err := c.Health(ctx)
	if err != nil {
		return false, err
	}
	return health.Sealed, nil
}

// StepDownLeader requests the leader to step down and trigger a new election.
func (c *Client) StepDownLeader(ctx context.Context) error {
	return c.StepDown(ctx)
}
