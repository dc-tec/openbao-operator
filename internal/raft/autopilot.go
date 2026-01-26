package raft

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"github.com/dc-tec/openbao-operator/internal/openbao"
)

const (
	// rootTokenSecretKey is the key used to store the root token in the Secret data.
	rootTokenSecretKey = "token"
)

// Manager handles Raft Autopilot configuration for OpenBao clusters.
type Manager struct {
	clientset kubernetes.Interface
	clientMgr *openbao.ClientManager
}

// NewManager creates a new Raft Autopilot Manager.
func NewManager(clientset kubernetes.Interface, clientMgr *openbao.ClientManager) *Manager {
	return &Manager{
		clientset: clientset,
		clientMgr: clientMgr,
	}
}

// BuildAutopilotConfig constructs the Autopilot configuration from CRD settings or defaults.
// It uses profile-aware logic to calculate safe defaults for min_quorum:
// - Hardened: Never drop below 3, or use replicas if replicas > 3
// - Development: Use replicas (minimum 1) to allow single-node clusters
func BuildAutopilotConfig(cluster *openbaov1alpha1.OpenBaoCluster) openbao.AutopilotConfig {
	// Initialize with defaults
	config := openbao.AutopilotConfig{
		CleanupDeadServers:             true,
		DeadServerLastContactThreshold: "5m",
		LastContactThreshold:           "10s",
		MaxTrailingLogs:                1000,
		ServerStabilizationTime:        "10s",
	}

	// Track if user explicitly set CleanupDeadServers
	cleanupDeadServersOverridden := false

	// Apply user overrides
	if cluster.Spec.Configuration != nil &&
		cluster.Spec.Configuration.Raft != nil &&
		cluster.Spec.Configuration.Raft.Autopilot != nil {
		userConfig := cluster.Spec.Configuration.Raft.Autopilot
		if userConfig.CleanupDeadServers != nil {
			config.CleanupDeadServers = *userConfig.CleanupDeadServers
			cleanupDeadServersOverridden = true
		}
		if userConfig.DeadServerLastContactThreshold != "" {
			config.DeadServerLastContactThreshold = userConfig.DeadServerLastContactThreshold
		}
		if userConfig.ServerStabilizationTime != "" {
			config.ServerStabilizationTime = userConfig.ServerStabilizationTime
		}
		if userConfig.LastContactThreshold != "" {
			config.LastContactThreshold = userConfig.LastContactThreshold
		}
		if userConfig.MaxTrailingLogs != nil {
			config.MaxTrailingLogs = int(*userConfig.MaxTrailingLogs)
		}
		if userConfig.MinQuorum != nil {
			config.MinQuorum = int(*userConfig.MinQuorum)
		}
	}

	// Calculate MinQuorum if not set by user
	if config.MinQuorum == 0 {
		if cluster.Spec.Profile == openbaov1alpha1.ProfileHardened {
			config.MinQuorum = 3
			if cluster.Spec.Replicas > 3 {
				config.MinQuorum = int(cluster.Spec.Replicas)
			}
		} else {
			config.MinQuorum = int(cluster.Spec.Replicas)
			if config.MinQuorum < 1 {
				config.MinQuorum = 1
			}
		}
	}

	// OpenBao requires MinQuorum >= 3 for CleanupDeadServers to be enabled.
	// If the user didn't explicitly request it, force it to false for small clusters to ensure reconcile succeeds.
	if config.MinQuorum < 3 && !cleanupDeadServersOverridden {
		config.CleanupDeadServers = false
	}

	return config
}

// ReconcileAutopilotConfig reconciles Raft Autopilot configuration for an initialized cluster.
// This is called during Day 2 operations (e.g., when replicas or autopilot config changes).
// It handles authentication via root token (non-SelfInit) or JWT (SelfInit).
// For JWT authentication, it uses a hybrid approach:
// 1. Try reading from projected volume (if operator pod has it mounted)
// 2. Fallback to TokenRequest API if projected volume is not available
func (m *Manager) ReconcileAutopilotConfig(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Only reconcile if cluster is initialized
	if !cluster.Status.Initialized {
		return nil
	}

	// Log the Profile and replicas being used for debugging
	logger.Info("Reconciling autopilot config",
		"profile", cluster.Spec.Profile,
		"replicas", cluster.Spec.Replicas,
		"cluster", cluster.Name,
		"namespace", cluster.Namespace,
	)

	// Build desired Autopilot configuration
	desiredConfig := BuildAutopilotConfig(cluster)

	// Log the calculated min_quorum for debugging
	logger.Info("Calculated autopilot config",
		"min_quorum", desiredConfig.MinQuorum,
		"cleanup_dead_servers", desiredConfig.CleanupDeadServers,
		"profile", cluster.Spec.Profile,
		"replicas", cluster.Spec.Replicas,
	)

	// Get authenticated client
	// For SelfInit clusters, use ClientManager which handles JWT authentication
	// For non-SelfInit clusters, use root token from Secret
	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	var client *openbao.Client
	var err error

	if selfInitEnabled {
		// Use ClientManager for JWT authentication (SelfInit)
		client, err = m.newOpenBaoClient(ctx, logger, cluster)
		if err != nil {
			return fmt.Errorf("failed to create OpenBao client for autopilot config: %w", err)
		}
	} else {
		// Use root token from Secret (non-SelfInit)
		secretName := cluster.Name + constants.SuffixRootToken
		secret, err := m.clientset.CoreV1().Secrets(cluster.Namespace).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				logger.V(1).Info("Root token Secret not found; skipping autopilot config reconciliation", "secret", secretName)
				return nil
			}
			return fmt.Errorf("failed to get root token Secret %s/%s: %w", cluster.Namespace, secretName, err)
		}

		rootToken, ok := secret.Data[rootTokenSecretKey]
		if !ok || len(rootToken) == 0 {
			logger.V(1).Info("Root token Secret missing token data; skipping autopilot config reconciliation", "secret", secretName)
			return nil
		}

		client, err = m.newOpenBaoClientWithToken(ctx, cluster, string(rootToken))
		if err != nil {
			return fmt.Errorf("failed to create authenticated OpenBao client: %w", err)
		}
	}

	logger.Info("Reconciling Raft Autopilot configuration",
		"cleanup_dead_servers", desiredConfig.CleanupDeadServers,
		"dead_server_last_contact_threshold", desiredConfig.DeadServerLastContactThreshold,
		"min_quorum", desiredConfig.MinQuorum,
		"server_stabilization_time", desiredConfig.ServerStabilizationTime,
	)

	autopilotCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.ConfigureRaftAutopilot(autopilotCtx, desiredConfig); err != nil {
		// Don't fail reconciliation on autopilot config errors - log and continue
		logger.Error(err, "Failed to update Raft Autopilot configuration; will retry on next reconcile")
		return operatorerrors.WrapTransientConnection(
			fmt.Errorf("failed to configure Raft Autopilot: %w", err),
		)
	}

	logger.V(1).Info("Raft Autopilot configuration reconciled successfully")
	return nil
}

// ConfigureAutopilot configures Raft Autopilot for automatic dead server cleanup.
// This is called after cluster initialization with the root token.
func (m *Manager) ConfigureAutopilot(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, rootToken string) error {
	// Create authenticated client with root token
	client, err := m.newOpenBaoClientWithToken(ctx, cluster, rootToken)
	if err != nil {
		return fmt.Errorf("failed to create authenticated OpenBao client: %w", err)
	}

	// Build Autopilot configuration from CRD or use defaults
	config := BuildAutopilotConfig(cluster)

	logger.Info("Configuring Raft Autopilot",
		"cleanup_dead_servers", config.CleanupDeadServers,
		"dead_server_last_contact_threshold", config.DeadServerLastContactThreshold,
		"min_quorum", config.MinQuorum,
	)

	autopilotCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.ConfigureRaftAutopilot(autopilotCtx, config); err != nil {
		return fmt.Errorf("failed to configure Raft Autopilot: %w", err)
	}

	logger.Info("Raft Autopilot configured successfully")
	return nil
}

// newOpenBaoClient constructs an authenticated OpenBao client for talking to the pod-0 instance
// using JWT authentication via ClientManager.
func (m *Manager) newOpenBaoClient(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (*openbao.Client, error) {
	if strings.TrimSpace(cluster.Name) == "" || strings.TrimSpace(cluster.Namespace) == "" {
		return nil, fmt.Errorf("cluster name and namespace are required")
	}

	baseURL := fmt.Sprintf("https://%s-0.%s.%s.svc:%d", cluster.Name, cluster.Name, cluster.Namespace, constants.PortAPI)

	// Get TLS CA for validation
	caCert, err := m.getTLSCACert(ctx, cluster)
	if err != nil {
		return nil, err
	}

	// Get client factory
	clusterKey := fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name)
	factory := m.clientMgr.FactoryFor(clusterKey, caCert)
	if factory == nil {
		return nil, fmt.Errorf("client manager returned nil factory for cluster %s", clusterKey)
	}

	// Get JWT Token (Hybrid: Projected Volume -> TokenRequest)
	jwtToken, err := m.getJWTToken(logger)
	if err != nil {
		return nil, err
	}

	// Create authenticated client
	client, err := factory.NewWithJWT(ctx, baseURL, constants.RoleNameOperator, jwtToken)
	if err != nil {
		return nil, m.handleJWTAuthError(cluster, err)
	}

	return client, nil
}

// getTLSCACert retrieves the CA certificate from the cluster's TLS CA secret.
func (m *Manager) getTLSCACert(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) ([]byte, error) {
	caSecretName := cluster.Name + constants.SuffixTLSCA
	secret, err := m.clientset.CoreV1().Secrets(cluster.Namespace).Get(ctx, caSecretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsForbidden(err) {
			return nil, operatorerrors.WrapTransientKubernetesAPI(
				fmt.Errorf("failed to get TLS CA Secret %s/%s: %w", cluster.Namespace, caSecretName, err),
			)
		}
		return nil, fmt.Errorf("failed to get TLS CA Secret %s/%s: %w", cluster.Namespace, caSecretName, err)
	}

	caCert, ok := secret.Data["ca.crt"]
	if !ok || len(caCert) == 0 {
		return nil, fmt.Errorf("TLS CA Secret %s/%s missing 'ca.crt' key", cluster.Namespace, caSecretName)
	}
	return caCert, nil
}

// getJWTToken retrieves a JWT token for the operator from the projected volume.
func (m *Manager) getJWTToken(logger logr.Logger) (string, error) {
	projectedTokenPath := "/var/run/secrets/tokens/openbao-token"
	tokenBytes, err := os.ReadFile(projectedTokenPath)
	if err != nil {
		return "", fmt.Errorf("failed to read JWT token from projected volume at %s: %w (ensure operator deployment has projected volume mounted)", projectedTokenPath, err)
	}

	token := strings.TrimSpace(string(tokenBytes))
	if len(token) == 0 {
		return "", fmt.Errorf("JWT token file at %s is empty", projectedTokenPath)
	}

	logger.V(1).Info("Successfully read JWT token from projected volume", "path", projectedTokenPath)
	return token, nil
}

// handleJWTAuthError provides helpful error messages for common JWT auth failures.
func (m *Manager) handleJWTAuthError(cluster *openbaov1alpha1.OpenBaoCluster, err error) error {
	if strings.Contains(err.Error(), "status 404") {
		guidance := "Enable JWT auth via spec.selfInit.oidc.enabled: true or configure JWT via self-init requests"
		if cluster.Status.Initialized {
			guidance = "Manually configure JWT authentication via OpenBao API/CLI"
		}
		return operatorerrors.WrapPermanentPrerequisitesMissing(
			fmt.Errorf("JWT authentication method not enabled in OpenBao cluster %s/%s. %s",
				cluster.Namespace, cluster.Name, guidance),
		)
	}
	if strings.Contains(err.Error(), "status 400") {
		guidance := fmt.Sprintf("Ensure JWT role '%s' is configured", constants.RoleNameOperator)
		if cluster.Status.Initialized {
			guidance = "Manually configure JWT role via OpenBao API/CLI"
		}
		return operatorerrors.WrapPermanentPrerequisitesMissing(
			fmt.Errorf("JWT authentication failed for cluster %s/%s. %s. Original error: %w",
				cluster.Namespace, cluster.Name, guidance, err),
		)
	}
	return fmt.Errorf("failed to create authenticated OpenBao client: %w", err)
}

// newOpenBaoClientWithToken creates an authenticated OpenBao client with the given token.
func (m *Manager) newOpenBaoClientWithToken(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, token string) (*openbao.Client, error) {
	if strings.TrimSpace(cluster.Name) == "" || strings.TrimSpace(cluster.Namespace) == "" {
		return nil, fmt.Errorf("cluster name and namespace are required to build OpenBao client")
	}

	baseURL := fmt.Sprintf("https://%s-0.%s.%s.svc:%d", cluster.Name, cluster.Name, cluster.Namespace, constants.PortAPI)

	caSecretName := cluster.Name + constants.SuffixTLSCA
	secret, err := m.clientset.CoreV1().Secrets(cluster.Namespace).Get(ctx, caSecretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsForbidden(err) {
			return nil, operatorerrors.WrapTransientKubernetesAPI(
				fmt.Errorf("failed to get TLS CA Secret %s/%s: %w", cluster.Namespace, caSecretName, err),
			)
		}
		return nil, fmt.Errorf("failed to get TLS CA Secret %s/%s: %w", cluster.Namespace, caSecretName, err)
	}

	caCert, ok := secret.Data["ca.crt"]
	if !ok || len(caCert) == 0 {
		return nil, fmt.Errorf("TLS CA Secret %s/%s missing 'ca.crt' key", cluster.Namespace, caSecretName)
	}

	// Create OpenBao client using the ClientManager for proper state isolation.
	clusterKey := fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name)
	factory := m.clientMgr.FactoryFor(clusterKey, caCert)
	if factory == nil {
		return nil, fmt.Errorf("client manager returned nil factory for cluster %s", clusterKey)
	}

	client, err := factory.NewWithToken(baseURL, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenBao client for %s: %w", baseURL, err)
	}

	return client, nil
}
