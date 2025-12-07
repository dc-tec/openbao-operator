package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/openbao/operator/internal/backup"
	"github.com/openbao/operator/internal/openbao"
	"github.com/openbao/operator/internal/storage"
)

const (
	// Exit codes
	exitSuccess           = 0
	exitConfigError       = 1
	exitAuthError         = 2
	exitLeaderDiscovery   = 3
	exitSnapshotError     = 4
	exitStorageError      = 5
	exitVerificationError = 6

	// Default paths
	defaultTLSCAPath       = "/etc/bao/tls/ca.crt"
	defaultTokenPath       = "/etc/bao/backup/token/token"
	defaultCredentialsPath = "/etc/bao/backup/credentials"
	defaultK8sTokenPath    = "/var/run/secrets/kubernetes.io/serviceaccount/token"

	// Default region for S3-compatible storage
	defaultRegion = "us-east-1"

	// Auth methods
	authMethodKubernetes = "kubernetes"
)

// Config holds the backup executor configuration.
type Config struct {
	// Cluster information
	ClusterNamespace string
	ClusterName      string
	ClusterReplicas  int32

	// Storage configuration
	BackupEndpoint       string
	BackupBucket         string
	BackupPathPrefix     string
	BackupFilenamePrefix string // Added to support pre-upgrade prefixes
	BackupUsePathStyle   bool

	// Authentication
	AuthMethod                    string
	KubernetesAuthRole            string
	OpenBaoToken                  string
	KubernetesServiceAccountToken string

	// TLS
	TLSCACert []byte

	// Storage credentials
	StorageCredentials *storage.Credentials
}

// loadConfig loads configuration from environment variables and mounted files.
func loadConfig() (*Config, error) {
	cfg := &Config{}

	// Load cluster information
	cfg.ClusterNamespace = strings.TrimSpace(os.Getenv("CLUSTER_NAMESPACE"))
	if cfg.ClusterNamespace == "" {
		return nil, fmt.Errorf("CLUSTER_NAMESPACE environment variable is required")
	}

	cfg.ClusterName = strings.TrimSpace(os.Getenv("CLUSTER_NAME"))
	if cfg.ClusterName == "" {
		return nil, fmt.Errorf("CLUSTER_NAME environment variable is required")
	}

	replicasStr := strings.TrimSpace(os.Getenv("CLUSTER_REPLICAS"))
	if replicasStr == "" {
		return nil, fmt.Errorf("CLUSTER_REPLICAS environment variable is required")
	}
	replicas, err := strconv.ParseInt(replicasStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid CLUSTER_REPLICAS value %q: %w", replicasStr, err)
	}
	cfg.ClusterReplicas = int32(replicas)

	// Load storage configuration
	cfg.BackupEndpoint = strings.TrimSpace(os.Getenv("BACKUP_ENDPOINT"))
	if cfg.BackupEndpoint == "" {
		return nil, fmt.Errorf("BACKUP_ENDPOINT environment variable is required")
	}

	cfg.BackupBucket = strings.TrimSpace(os.Getenv("BACKUP_BUCKET"))
	if cfg.BackupBucket == "" {
		return nil, fmt.Errorf("BACKUP_BUCKET environment variable is required")
	}

	cfg.BackupPathPrefix = strings.TrimSpace(os.Getenv("BACKUP_PATH_PREFIX"))
	cfg.BackupFilenamePrefix = strings.TrimSpace(os.Getenv("BACKUP_FILENAME_PREFIX"))

	usePathStyleStr := strings.TrimSpace(os.Getenv("BACKUP_USE_PATH_STYLE"))
	if usePathStyleStr != "" {
		var err error
		cfg.BackupUsePathStyle, err = strconv.ParseBool(usePathStyleStr)
		if err != nil {
			return nil, fmt.Errorf("invalid BACKUP_USE_PATH_STYLE value %q: %w", usePathStyleStr, err)
		}
	}

	// Load TLS CA certificate
	caCertPath := defaultTLSCAPath
	if envPath := strings.TrimSpace(os.Getenv("TLS_CA_PATH")); envPath != "" {
		caCertPath = envPath
	}
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read TLS CA certificate from %q: %w", caCertPath, err)
	}
	cfg.TLSCACert = caCert

	// Determine authentication method
	cfg.AuthMethod = strings.TrimSpace(os.Getenv("BACKUP_AUTH_METHOD"))

	// Try to load Kubernetes ServiceAccount token
	k8sTokenPath := defaultK8sTokenPath
	if envPath := strings.TrimSpace(os.Getenv("K8S_TOKEN_PATH")); envPath != "" {
		k8sTokenPath = envPath
	}
	k8sToken, err := os.ReadFile(k8sTokenPath)
	if err == nil && len(k8sToken) > 0 {
		cfg.KubernetesServiceAccountToken = strings.TrimSpace(string(k8sToken))
		// If auth method not explicitly set, prefer Kubernetes Auth
		if cfg.AuthMethod == "" {
			cfg.AuthMethod = authMethodKubernetes
		}
	}

	// Load Kubernetes Auth role if using Kubernetes Auth
	if cfg.AuthMethod == authMethodKubernetes || (cfg.AuthMethod == "" && cfg.KubernetesServiceAccountToken != "") {
		cfg.KubernetesAuthRole = strings.TrimSpace(os.Getenv("BACKUP_KUBERNETES_AUTH_ROLE"))
		if cfg.KubernetesAuthRole == "" {
			return nil, fmt.Errorf("BACKUP_KUBERNETES_AUTH_ROLE is required when using Kubernetes authentication")
		}
		if cfg.KubernetesServiceAccountToken == "" {
			return nil, fmt.Errorf("kubernetes ServiceAccount token not found at %q", k8sTokenPath)
		}
		cfg.AuthMethod = authMethodKubernetes
	} else {
		// Fall back to static token
		tokenPath := defaultTokenPath
		if envPath := strings.TrimSpace(os.Getenv("BACKUP_TOKEN_PATH")); envPath != "" {
			tokenPath = envPath
		}
		token, err := os.ReadFile(tokenPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read OpenBao token from %q: %w", tokenPath, err)
		}
		cfg.OpenBaoToken = strings.TrimSpace(string(token))
		if cfg.OpenBaoToken == "" {
			return nil, fmt.Errorf("OpenBao token is empty")
		}
		cfg.AuthMethod = "token"
	}

	// Load storage credentials if provided
	credsPath := defaultCredentialsPath
	if envPath := strings.TrimSpace(os.Getenv("BACKUP_CREDENTIALS_PATH")); envPath != "" {
		credsPath = envPath
	}

	creds, err := loadStorageCredentials(credsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load storage credentials: %w", err)
	}
	cfg.StorageCredentials = creds

	return cfg, nil
}

// loadStorageCredentials loads S3 credentials from the mounted directory.
func loadStorageCredentials(credsPath string) (*storage.Credentials, error) {
	creds := &storage.Credentials{}

	// Check if credentials directory exists
	if _, err := os.Stat(credsPath); os.IsNotExist(err) {
		// No credentials provided - will use default credential chain
		return nil, nil
	}

	// Load access key ID
	accessKeyPath := filepath.Join(credsPath, "accessKeyId")
	if data, err := os.ReadFile(accessKeyPath); err == nil {
		creds.AccessKeyID = strings.TrimSpace(string(data))
	}

	// Load secret access key
	secretKeyPath := filepath.Join(credsPath, "secretAccessKey")
	if data, err := os.ReadFile(secretKeyPath); err == nil {
		creds.SecretAccessKey = strings.TrimSpace(string(data))
	}

	// Validate that if one is provided, both must be provided
	if (creds.AccessKeyID != "" && creds.SecretAccessKey == "") ||
		(creds.AccessKeyID == "" && creds.SecretAccessKey != "") {
		return nil, fmt.Errorf("both accessKeyId and secretAccessKey must be provided if using static credentials")
	}

	// Load optional fields
	sessionTokenPath := filepath.Join(credsPath, "sessionToken")
	if data, err := os.ReadFile(sessionTokenPath); err == nil {
		creds.SessionToken = strings.TrimSpace(string(data))
	}

	regionPath := filepath.Join(credsPath, "region")
	if data, err := os.ReadFile(regionPath); err == nil {
		creds.Region = strings.TrimSpace(string(data))
	} else {
		// Use default region if not provided
		creds.Region = defaultRegion
	}

	caCertPath := filepath.Join(credsPath, "caCert")
	if data, err := os.ReadFile(caCertPath); err == nil {
		creds.CACert = data
	}

	// If no credentials were loaded, return nil to use default credential chain
	if creds.AccessKeyID == "" && creds.SecretAccessKey == "" {
		return nil, nil
	}

	return creds, nil
}

// findLeader discovers the current Raft leader by querying health endpoints.
// It retries with exponential backoff to handle cases where pods are still starting up
// after scale-up operations.
func findLeader(ctx context.Context, cfg *Config) (string, error) {
	// Retry with exponential backoff: 1s, 2s, 4s, 8s, 16s
	maxRetries := 5
	baseDelay := 1 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		for i := int32(0); i < cfg.ClusterReplicas; i++ {
			podName := fmt.Sprintf("%s-%d", cfg.ClusterName, i)
			podURL := fmt.Sprintf("https://%s.%s.%s.svc:8200", podName, cfg.ClusterName, cfg.ClusterNamespace)

			// Create a client without token for health checks
			client, err := openbao.NewClient(openbao.ClientConfig{
				BaseURL: podURL,
				CACert:  cfg.TLSCACert,
			})
			if err != nil {
				// Log but continue to next pod
				continue
			}

			isLeader, err := client.IsLeader(ctx)
			if err != nil {
				// Log but continue to next pod
				continue
			}

			if isLeader {
				return podURL, nil
			}
		}

		// If we've exhausted all retries, return error
		if attempt == maxRetries-1 {
			break
		}

		// Wait before retrying with exponential backoff
		delay := baseDelay * time.Duration(1<<uint(attempt))
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("context cancelled while finding leader: %w", ctx.Err())
		case <-time.After(delay):
			// Continue to next retry
		}
	}

	return "", fmt.Errorf("no leader found among %d pods after %d attempts", cfg.ClusterReplicas, maxRetries)
}

// authenticate authenticates to OpenBao and returns a token.
func authenticate(ctx context.Context, cfg *Config, leaderURL string) (string, error) {
	if cfg.AuthMethod == authMethodKubernetes {
		// Create a client without token for authentication
		client, err := openbao.NewClient(openbao.ClientConfig{
			BaseURL: leaderURL,
			CACert:  cfg.TLSCACert,
		})
		if err != nil {
			return "", fmt.Errorf("failed to create OpenBao client: %w", err)
		}

		token, err := client.KubernetesAuthLogin(ctx, cfg.KubernetesAuthRole, cfg.KubernetesServiceAccountToken)
		if err != nil {
			return "", fmt.Errorf("failed to authenticate using Kubernetes Auth: %w", err)
		}

		return token, nil
	}

	// Use static token
	return cfg.OpenBaoToken, nil
}

func main() {
	flag.Parse()

	ctx := context.Background()

	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: failed to load configuration: %v\n", err)
		os.Exit(exitConfigError)
	}

	// Find leader with timeout (allow up to ~30 seconds for retries)
	leaderCtx, leaderCancel := context.WithTimeout(ctx, 30*time.Second)
	defer leaderCancel()

	leaderURL, err := findLeader(leaderCtx, cfg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: failed to find leader: %v\n", err)
		os.Exit(exitLeaderDiscovery)
	}

	// Authenticate
	token, err := authenticate(ctx, cfg, leaderURL)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: failed to authenticate: %v\n", err)
		os.Exit(exitAuthError)
	}

	// Create OpenBao client for leader
	baoClient, err := openbao.NewClient(openbao.ClientConfig{
		BaseURL: leaderURL,
		Token:   token,
		CACert:  cfg.TLSCACert,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: failed to create OpenBao client: %v\n", err)
		os.Exit(exitConfigError)
	}

	// Generate backup key
	backupKey, err := backup.GenerateBackupKey(
		cfg.BackupPathPrefix,
		cfg.ClusterNamespace,
		cfg.ClusterName,
		cfg.BackupFilenamePrefix,
		time.Now().UTC(),
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: failed to generate backup key: %v\n", err)
		os.Exit(exitConfigError)
	}

	// Stream snapshot
	snapshotResp, err := baoClient.Snapshot(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: failed to get snapshot: %v\n", err)
		os.Exit(exitSnapshotError)
	}
	defer func() {
		_ = snapshotResp.Body.Close()
	}()

	// Ensure region is set (required by S3 client)
	if cfg.StorageCredentials == nil {
		cfg.StorageCredentials = &storage.Credentials{
			Region: defaultRegion,
		}
	} else if cfg.StorageCredentials.Region == "" {
		cfg.StorageCredentials.Region = defaultRegion
	}

	// Create storage client
	storageClient, err := storage.NewS3ClientFromCredentials(
		ctx,
		cfg.BackupEndpoint,
		cfg.BackupBucket,
		cfg.StorageCredentials,
		cfg.BackupUsePathStyle,
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: failed to create storage client: %v\n", err)
		os.Exit(exitStorageError)
	}

	// Upload to storage
	if err := storageClient.Upload(ctx, backupKey, snapshotResp.Body, snapshotResp.ContentLength); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: failed to upload backup: %v\n", err)
		os.Exit(exitStorageError)
	}

	// Verify upload
	objInfo, err := storageClient.Head(ctx, backupKey)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: failed to verify backup upload: %v\n", err)
		os.Exit(exitVerificationError)
	}
	if objInfo == nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: backup verification failed: object not found after upload\n")
		os.Exit(exitVerificationError)
	}
	if objInfo.Size == 0 {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: backup verification failed: uploaded object has zero size\n")
		os.Exit(exitVerificationError)
	}

	// Success
	_, _ = fmt.Fprintf(os.Stdout, "Backup completed successfully: %s (size: %d bytes)\n", backupKey, objInfo.Size)
	os.Exit(exitSuccess)
}
