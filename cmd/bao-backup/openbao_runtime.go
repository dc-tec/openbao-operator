package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	backupconfig "github.com/dc-tec/openbao-operator/internal/service/backup"
)

func newOpenBaoClientConfig(cfg *backupconfig.ExecutorConfig) portopenbao.ClientConfig {
	return portopenbao.ClientConfig{
		CACert:                         cfg.TLSCACert,
		TLSServerName:                  cfg.TLSServerName,
		RateLimitQPS:                   cfg.RateLimitQPS,
		RateLimitBurst:                 cfg.RateLimitBurst,
		CircuitBreakerFailureThreshold: cfg.CircuitBreakerFailureThreshold,
		CircuitBreakerOpenDuration:     parseDuration(cfg.CircuitBreakerOpenDuration),
		JWTAuthStrategy:                cfg.JWTAuthStrategy,
	}
}

// findLeader discovers the current Raft leader by querying health endpoints.
// It retries with exponential backoff to handle cases where pods are still starting up
// after scale-up operations.
func findLeader(ctx context.Context, cfg *backupconfig.ExecutorConfig) (string, error) {
	mgr := openbao.NewClientManager(newOpenBaoClientConfig(cfg))
	defer mgr.Close()

	factory := mgr.FactoryFor("leader-discovery", cfg.TLSCACert)

	const maxRetries = 5
	baseDelay := 1 * time.Second

	clusterDomainSuffix := strings.TrimSpace(os.Getenv("CLUSTER_DOMAIN_SUFFIX"))

	fmt.Printf("findLeader: Starting leader discovery for %d replicas (statefulset=%s)\n",
		cfg.ClusterReplicas, cfg.StatefulSetName)

	for attempt := 0; attempt < maxRetries; attempt++ {
		fmt.Printf("findLeader: Attempt %d/%d\n", attempt+1, maxRetries)
		for i := int32(0); i < cfg.ClusterReplicas; i++ {
			podName := fmt.Sprintf("%s-%d", cfg.StatefulSetName, i)
			host := fmt.Sprintf("%s.%s.%s.svc", podName, cfg.ClusterName, cfg.ClusterNamespace)
			if clusterDomainSuffix != "" {
				host += clusterDomainSuffix
			}
			podURL := fmt.Sprintf("https://%s:%d", host, constants.PortAPI)

			fmt.Printf("findLeader: Checking pod %s at %s\n", podName, podURL)

			client, err := factory.New(podURL)
			if err != nil {
				fmt.Printf("findLeader: Failed to create client for %s: %v\n", podName, err)
				continue
			}

			isLeader, err := client.IsLeader(ctx)
			if err != nil {
				fmt.Printf("findLeader: IsLeader check failed for %s: %v\n", podName, err)
				continue
			}

			fmt.Printf("findLeader: Pod %s isLeader=%t\n", podName, isLeader)
			if isLeader {
				return podURL, nil
			}
		}

		if attempt == maxRetries-1 {
			break
		}

		delay := baseDelay * time.Duration(1<<uint(attempt))
		fmt.Printf("findLeader: No leader found, waiting %v before retry...\n", delay)
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("context cancelled while finding leader: %w", ctx.Err())
		case <-time.After(delay):
		}
	}

	return "", fmt.Errorf("no leader found among %d pods after %d attempts", cfg.ClusterReplicas, maxRetries)
}

// authenticate prepares OpenBao authentication and returns a reusable token
// only for static token or standard JWT auth. Inline JWT auth is per request.
func authenticate(ctx context.Context, cfg *backupconfig.ExecutorConfig, leaderURL string) (string, error) {
	if cfg.AuthMethod == constants.BackupAuthMethodJWT {
		if portopenbao.NormalizeJWTAuthStrategyOrDefault(cfg.JWTAuthStrategy) == portopenbao.JWTAuthStrategyInline {
			return "", nil
		}
		mgr := openbao.NewClientManager(newOpenBaoClientConfig(cfg))
		defer mgr.Close()
		factory := mgr.FactoryFor("auth", cfg.TLSCACert)
		return factory.LoginJWT(ctx, leaderURL, cfg.JWTAuthRole, cfg.JWTToken)
	}

	return cfg.OpenBaoToken, nil
}

func openClusterClient(
	cfg *backupconfig.ExecutorConfig,
	purpose, leaderURL, token string,
) (portopenbao.ClusterActions, func(), error) {
	clientMgr := openbao.NewClientManager(newOpenBaoClientConfig(cfg))
	factory := clientMgr.FactoryFor(purpose, cfg.TLSCACert)
	var baoClient *openbao.Client
	var err error
	if cfg.AuthMethod == constants.BackupAuthMethodJWT &&
		portopenbao.NormalizeJWTAuthStrategyOrDefault(cfg.JWTAuthStrategy) == portopenbao.JWTAuthStrategyInline {
		baoClient, err = factory.NewWithInlineJWT(leaderURL, cfg.JWTAuthRole, cfg.JWTToken)
	} else {
		baoClient, err = factory.NewWithToken(leaderURL, token)
	}
	if err != nil {
		clientMgr.Close()
		return nil, nil, fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	return baoClient, clientMgr.Close, nil
}

// parseDuration parses a duration string, returning 0 if empty or invalid.
func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: invalid duration %q: %v\n", s, err)
		return 0
	}
	return d
}
