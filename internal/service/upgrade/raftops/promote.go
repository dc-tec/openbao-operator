package raftops

import (
	"context"
	"errors"
	"fmt"
	"time"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

const (
	defaultPromoteVerifyMaxAttempts = 60
	defaultPromoteVerifyInterval    = time.Second
)

type raftPeerPromoter interface {
	raftConfigurationReader
	PromoteRaftPeer(context.Context, string) error
}

type raftConfigurationReader interface {
	ReadRaftConfiguration(context.Context) (*portopenbao.RaftConfigurationResponse, error)
}

func promoteRaftPeerAndVerify(ctx context.Context, client raftPeerPromoter, serverID string) (bool, error) {
	return promoteRaftPeerAndVerifyWithPolicy(ctx, client, serverID, RetryPolicy{
		MaxAttempts:     defaultPromoteVerifyMaxAttempts,
		AttemptInterval: defaultPromoteVerifyInterval,
	})
}

func promoteRaftPeerAndVerifyWithPolicy(
	ctx context.Context,
	client raftPeerPromoter,
	serverID string,
	policy RetryPolicy,
) (bool, error) {
	promoteErr := client.PromoteRaftPeer(ctx, serverID)
	alreadyVoterResponse := errors.Is(promoteErr, portopenbao.ErrAlreadyVoter)

	isVoter, verifyErr := waitForRaftServerVoter(ctx, client, serverID, policy)
	if verifyErr == nil && isVoter {
		return alreadyVoterResponse || promoteErr != nil, nil
	}

	if promoteErr != nil {
		if verifyErr != nil {
			return false, fmt.Errorf("%w; failed to verify raft voter state after promote error: %v", promoteErr, verifyErr)
		}

		return false, promoteErr
	}

	if verifyErr != nil {
		return false, fmt.Errorf("failed to verify raft voter state after promote request: %w", verifyErr)
	}

	return false, fmt.Errorf("raft server %q did not become a voter after promote request", serverID)
}

func waitForRaftServerVoter(ctx context.Context, client raftConfigurationReader, serverID string, policy RetryPolicy) (bool, error) {
	policy = NormalizeRetryPolicy(policy)

	var lastErr error
	lastReadFailed := false
	for _, attempt := range AttemptOrdinals(policy.MaxAttempts) {
		isVoter, err := raftServerIsVoter(ctx, client, serverID)
		if err == nil && isVoter {
			return true, nil
		}
		if err != nil {
			lastErr = err
			lastReadFailed = true
		} else {
			lastReadFailed = false
		}

		if attempt == policy.MaxAttempts-1 || policy.AttemptInterval <= 0 {
			continue
		}
		if waitErr := waitForPromoteVerifyInterval(ctx, policy.AttemptInterval); waitErr != nil {
			if lastErr != nil {
				return false, fmt.Errorf("%v; %w", lastErr, waitErr)
			}
			return false, waitErr
		}
	}

	if lastReadFailed && lastErr != nil {
		return false, lastErr
	}
	return false, nil
}

func waitForPromoteVerifyInterval(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return nil
	}

	timer := time.NewTimer(interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func raftServerIsVoter(ctx context.Context, client raftConfigurationReader, serverID string) (bool, error) {
	config, err := client.ReadRaftConfiguration(ctx)
	if err != nil {
		return false, err
	}
	if config == nil {
		return false, nil
	}

	for _, server := range config.Config.Servers {
		if server.NodeID == serverID {
			return server.Voter, nil
		}
	}

	return false, nil
}
