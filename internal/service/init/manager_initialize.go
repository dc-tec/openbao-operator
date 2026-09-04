package init

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// initializeCluster explicitly initializes OpenBao using the HTTP API (PUT /v1/sys/init).
// With static auto-unseal, this should rarely be needed, but we provide it as a fallback.
func (m *Manager) initializeCluster(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if err := m.preflightRootTokenStorage(ctx, cluster); err != nil {
		return err
	}

	client, err := m.newOpenBaoClient(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client for initialization: %w", err)
	}

	healthCheckTimeout := 10 * time.Second
	healthCtx, healthCancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer healthCancel()

	healthResp, healthErr := client.Health(healthCtx)
	if healthErr != nil {
		if operatorerrors.IsTransientConnection(healthErr) {
			logger.Info("OpenBao HTTPS endpoint not ready yet; will retry on next reconcile", "cluster", cluster.Name, "error", healthErr)
			return operatorerrors.WrapTransientConnection(healthErr)
		}
		logger.Info("Health check failed; will retry on next reconcile", "cluster", cluster.Name, "error", healthErr)
		return operatorerrors.WrapTransientConnection(healthErr)
	}

	if healthResp != nil {
		logger.V(1).Info("Health check succeeded", "cluster", cluster.Name, "initialized", healthResp.Initialized, "sealed", healthResp.Sealed)
		if healthResp.Initialized {
			logger.Info("OpenBao cluster is already initialized (detected via health check)", "cluster", cluster.Name)
			if err := m.ensureRootTokenSecretPresent(ctx, cluster); err != nil {
				logger.Info("OpenBao is initialized but root token Secret is not available yet; will retry", "cluster", cluster.Name, "error", err)
				return err
			}
			return nil
		}
	}

	initCtx, cancel := context.WithTimeout(ctx, openBaoInitTimeout)
	defer cancel()

	logger.Info("Calling OpenBao Init API", "cluster", cluster.Name, "timeout", openBaoInitTimeout)
	initResp, err := client.Init(initCtx, openbao.InitRequest{
		SecretShares:    nil,
		SecretThreshold: nil,
	})
	if err != nil {
		logger.Info("Init API call returned error", "cluster", cluster.Name, "error", err)
		if errors.Is(err, portopenbao.ErrAlreadyInitialized) {
			logger.Info("OpenBao cluster is already initialized (detected during HTTP init attempt)", "cluster", cluster.Name)
			if secretErr := m.ensureRootTokenSecretPresent(ctx, cluster); secretErr != nil {
				logger.Info("OpenBao is initialized but root token Secret is not available yet; will retry", "cluster", cluster.Name, "error", secretErr)
				return secretErr
			}
			return nil
		}

		if operatorerrors.IsTransientConnection(err) {
			logger.Info("OpenBao pod not ready to accept connections yet; will retry on next reconcile", "cluster", cluster.Name, "error", err)
			return operatorerrors.WrapTransientConnection(err)
		}

		return fmt.Errorf("failed to initialize OpenBao via HTTP API: %w", err)
	}

	if err := m.storeRootToken(ctx, logger, cluster, initResp.RootToken); err != nil {
		return err
	}

	if err := m.raftRuntime.ConfigureAutopilot(ctx, logger, cluster, initResp.RootToken); err != nil {
		logger.Error(err, "Failed to configure Raft Autopilot; dead server cleanup may not work automatically")
	}

	logger.Info("OpenBao cluster initialized successfully via HTTP API")
	return nil
}
