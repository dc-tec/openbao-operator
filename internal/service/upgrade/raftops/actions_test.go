package raftops

import (
	"context"
	"net/http"
	"testing"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/go-logr/logr"
)

const testRollingLeaderURL0 = "https://vault-0"

type rollingStepDownStubClient struct {
	stepDownCalls int
	stepDownErr   error
}

func (c *rollingStepDownStubClient) ReadRaftConfiguration(context.Context) (*portopenbao.RaftConfigurationResponse, error) {
	return nil, nil
}

func (c *rollingStepDownStubClient) DemoteRaftPeer(context.Context, string) error {
	return nil
}

func (c *rollingStepDownStubClient) StepDown(ctx context.Context) error {
	c.stepDownCalls++
	return c.stepDownErr
}

func TestRunRollingStepDownLeaderWithFuncs_RetriesUntilLeaderChanges(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		ClusterName:     "vault",
		ClusterReplicas: 3,
		BlueRevision:    "active-revision",
	}
	client := &rollingStepDownStubClient{}
	var searchedRevisions []string

	err := runRollingStepDownLeaderWithFuncs(
		context.Background(),
		logr.Discard(),
		cfg,
		RetryPolicy{MaxAttempts: 3},
		func(_ context.Context, _ *ExecutorConfig, revision string) (string, error) {
			searchedRevisions = append(searchedRevisions, revision)
			return testRollingLeaderURL0, nil
		},
		func(context.Context, string) (LeaderTransferClient, error) {
			return client, nil
		},
		func(context.Context, logr.Logger, *ExecutorConfig, string) (string, error) {
			if client.stepDownCalls == 1 {
				return testRollingLeaderURL0, nil
			}
			return "https://vault-1", nil
		},
	)
	if err != nil {
		t.Fatalf("runRollingStepDownLeaderWithFuncs() error = %v, want nil", err)
	}
	if client.stepDownCalls != 2 {
		t.Fatalf("stepDownCalls = %d, want 2", client.stepDownCalls)
	}
	if len(searchedRevisions) != 2 {
		t.Fatalf("searched revisions = %v, want two searches", searchedRevisions)
	}
	for _, revision := range searchedRevisions {
		if revision != cfg.BlueRevision {
			t.Fatalf("searched revision = %q, want active revision %q", revision, cfg.BlueRevision)
		}
	}
}

func TestRunRollingStepDownLeaderWithFuncs_FailsAfterBoundedSameLeaderRetries(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		ClusterName:     "vault",
		ClusterReplicas: 3,
	}
	client := &rollingStepDownStubClient{}

	err := runRollingStepDownLeaderWithFuncs(
		context.Background(),
		logr.Discard(),
		cfg,
		RetryPolicy{MaxAttempts: 3},
		func(context.Context, *ExecutorConfig, string) (string, error) {
			return testRollingLeaderURL0, nil
		},
		func(context.Context, string) (LeaderTransferClient, error) {
			return client, nil
		},
		func(context.Context, logr.Logger, *ExecutorConfig, string) (string, error) {
			return testRollingLeaderURL0, nil
		},
	)
	if err == nil {
		t.Fatal("runRollingStepDownLeaderWithFuncs() error = nil, want retry exhaustion")
	}
	if client.stepDownCalls != 3 {
		t.Fatalf("stepDownCalls = %d, want 3", client.stepDownCalls)
	}
	if got, want := err.Error(), "leader step-down did not transfer leadership after 3 attempts"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestRunRollingStepDownLeaderWithFuncs_StopsOnFatalStepDownError(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		ClusterName:     "vault",
		ClusterReplicas: 3,
	}
	client := &rollingStepDownStubClient{
		stepDownErr: portopenbao.NewAPIError("step-down forbidden", http.StatusForbidden, nil),
	}

	err := runRollingStepDownLeaderWithFuncs(
		context.Background(),
		logr.Discard(),
		cfg,
		RetryPolicy{MaxAttempts: 3},
		func(context.Context, *ExecutorConfig, string) (string, error) {
			return testRollingLeaderURL0, nil
		},
		func(context.Context, string) (LeaderTransferClient, error) {
			return client, nil
		},
		func(context.Context, logr.Logger, *ExecutorConfig, string) (string, error) {
			return "https://vault-1", nil
		},
	)
	if err == nil {
		t.Fatal("runRollingStepDownLeaderWithFuncs() error = nil, want fatal step-down error")
	}
	if client.stepDownCalls != 1 {
		t.Fatalf("stepDownCalls = %d, want 1", client.stepDownCalls)
	}
}
