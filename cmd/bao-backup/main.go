package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/openbao/operator/internal/backup"
	"github.com/openbao/operator/internal/constants"
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
	defaultTLSCAPath       = constants.PathTLSCACert
	defaultTokenPath       = constants.PathBackupToken
	defaultCredentialsPath = constants.PathBackupCredentials
	defaultJWTTokenPath    = constants.PathBackupJWTToken

	// Default region for S3-compatible storage
	defaultRegion = constants.DefaultS3Region
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
	BackupRegion         string

	// Authentication
	AuthMethod   string
	JWTAuthRole  string
	OpenBaoToken string
	JWTToken     string

	// TLS
	TLSCACert []byte

	// Storage credentials
	StorageCredentials *storage.Credentials

	// S3 upload configuration
	PartSize    int64
	Concurrency int32
}

// loadConfig loads configuration from environment variables and mounted files.
func loadConfig() (*Config, error) {
	cfg := &Config{}

	// Load cluster information
	cfg.ClusterNamespace = strings.TrimSpace(os.Getenv(constants.EnvClusterNamespace))
	if cfg.ClusterNamespace == "" {
		return nil, fmt.Errorf("%s environment variable is required", constants.EnvClusterNamespace)
	}

	cfg.ClusterName = strings.TrimSpace(os.Getenv(constants.EnvClusterName))
	if cfg.ClusterName == "" {
		return nil, fmt.Errorf("%s environment variable is required", constants.EnvClusterName)
	}

	replicasStr := strings.TrimSpace(os.Getenv(constants.EnvClusterReplicas))
	if replicasStr == "" {
		return nil, fmt.Errorf("%s environment variable is required", constants.EnvClusterReplicas)
	}
	replicas, err := strconv.ParseInt(replicasStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid CLUSTER_REPLICAS value %q: %w", replicasStr, err)
	}
	cfg.ClusterReplicas = int32(replicas)

	// Load storage configuration
	cfg.BackupEndpoint = strings.TrimSpace(os.Getenv(constants.EnvBackupEndpoint))
	if cfg.BackupEndpoint == "" {
		return nil, fmt.Errorf("%s environment variable is required", constants.EnvBackupEndpoint)
	}

	cfg.BackupBucket = strings.TrimSpace(os.Getenv(constants.EnvBackupBucket))
	if cfg.BackupBucket == "" {
		return nil, fmt.Errorf("%s environment variable is required", constants.EnvBackupBucket)
	}

	cfg.BackupPathPrefix = strings.TrimSpace(os.Getenv(constants.EnvBackupPathPrefix))
	cfg.BackupFilenamePrefix = strings.TrimSpace(os.Getenv(constants.EnvBackupFilenamePrefix))

	cfg.BackupRegion = strings.TrimSpace(os.Getenv(constants.EnvBackupRegion))
	if cfg.BackupRegion == "" {
		cfg.BackupRegion = defaultRegion
	}

	usePathStyleStr := strings.TrimSpace(os.Getenv(constants.EnvBackupUsePathStyle))
	if usePathStyleStr != "" {
		var err error
		cfg.BackupUsePathStyle, err = strconv.ParseBool(usePathStyleStr)
		if err != nil {
			return nil, fmt.Errorf("invalid BACKUP_USE_PATH_STYLE value %q: %w", usePathStyleStr, err)
		}
	}

	// Load TLS CA certificate
	caCertPath := defaultTLSCAPath
	if envPath := strings.TrimSpace(os.Getenv(constants.EnvTLSCAPath)); envPath != "" {
		caCertPath = envPath
	}
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read TLS CA certificate from %q: %w", caCertPath, err)
	}
	cfg.TLSCACert = caCert

	// Determine authentication method
	cfg.AuthMethod = strings.TrimSpace(os.Getenv(constants.EnvBackupAuthMethod))

	// Try to load JWT token from projected volume
	jwtTokenPath := defaultJWTTokenPath
	if envPath := strings.TrimSpace(os.Getenv(constants.EnvJWTTokenPath)); envPath != "" {
		jwtTokenPath = envPath
	}
	jwtToken, err := os.ReadFile(jwtTokenPath)
	if err == nil && len(jwtToken) > 0 {
		cfg.JWTToken = strings.TrimSpace(string(jwtToken))
		// If auth method not explicitly set, prefer JWT Auth
		if cfg.AuthMethod == "" {
			cfg.AuthMethod = constants.BackupAuthMethodJWT
		}
	}

	// Load JWT Auth role if using JWT Auth
	if cfg.AuthMethod == constants.BackupAuthMethodJWT || (cfg.AuthMethod == "" && cfg.JWTToken != "") {
		cfg.JWTAuthRole = strings.TrimSpace(os.Getenv(constants.EnvBackupJWTAuthRole))
		if cfg.JWTAuthRole == "" {
			return nil, fmt.Errorf("BACKUP_JWT_AUTH_ROLE is required when using JWT authentication")
		}
		if cfg.JWTToken == "" {
			return nil, fmt.Errorf("JWT token not found at %q", jwtTokenPath)
		}
		cfg.AuthMethod = constants.BackupAuthMethodJWT
	} else {
		// Fall back to static token
		tokenPath := defaultTokenPath
		if envPath := strings.TrimSpace(os.Getenv(constants.EnvBackupTokenPath)); envPath != "" {
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
		cfg.AuthMethod = constants.BackupAuthMethodToken
	}

	// Load storage credentials if provided
	credsPath := defaultCredentialsPath
	if envPath := strings.TrimSpace(os.Getenv(constants.EnvBackupCredentialsPath)); envPath != "" {
		credsPath = envPath
	}

	creds, err := loadStorageCredentials(credsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load storage credentials: %w", err)
	}
	cfg.StorageCredentials = creds

	// Load S3 upload configuration (optional, with defaults)
	partSizeStr := strings.TrimSpace(os.Getenv(constants.EnvBackupPartSize))
	if partSizeStr != "" {
		partSize, err := strconv.ParseInt(partSizeStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid BACKUP_PART_SIZE value %q: %w", partSizeStr, err)
		}
		cfg.PartSize = partSize
	}

	concurrencyStr := strings.TrimSpace(os.Getenv(constants.EnvBackupConcurrency))
	if concurrencyStr != "" {
		concurrency, err := strconv.ParseInt(concurrencyStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid BACKUP_CONCURRENCY value %q: %w", concurrencyStr, err)
		}
		cfg.Concurrency = int32(concurrency)
	}

	return cfg, nil
}

// validatePath ensures a file path is within the base directory and doesn't contain path traversal.
func validatePath(baseDir, filePath string) (string, error) {
	// Clean the base directory and file path
	cleanBase := filepath.Clean(baseDir)
	cleanPath := filepath.Clean(filePath)

	// Ensure the path is within the base directory
	relPath, err := filepath.Rel(cleanBase, cleanPath)
	if err != nil {
		return "", fmt.Errorf("invalid path %q relative to base %q: %w", filePath, baseDir, err)
	}

	// Check for path traversal attempts
	if strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("path %q attempts to escape base directory %q", filePath, baseDir)
	}

	return cleanPath, nil
}

// loadStorageCredentials loads S3 credentials from the mounted directory.
func loadStorageCredentials(credsPath string) (*storage.Credentials, error) {
	creds := &storage.Credentials{}

	// Validate and clean the base credentials path
	cleanCredsPath, err := validatePath("/", credsPath)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials path: %w", err)
	}

	// Check if credentials directory exists
	if _, err := os.Stat(cleanCredsPath); os.IsNotExist(err) {
		// No credentials provided - will use default credential chain
		return nil, nil
	}

	// Load access key ID
	accessKeyPath := filepath.Join(cleanCredsPath, "accessKeyId")
	validatedAccessKeyPath, err := validatePath(cleanCredsPath, accessKeyPath)
	if err != nil {
		return nil, fmt.Errorf("invalid access key path: %w", err)
	}
	// #nosec G304 -- Path is validated to be within base directory
	if data, err := os.ReadFile(validatedAccessKeyPath); err == nil {
		creds.AccessKeyID = strings.TrimSpace(string(data))
	}

	// Load secret access key
	secretKeyPath := filepath.Join(cleanCredsPath, "secretAccessKey")
	validatedSecretKeyPath, err := validatePath(cleanCredsPath, secretKeyPath)
	if err != nil {
		return nil, fmt.Errorf("invalid secret key path: %w", err)
	}
	// #nosec G304 -- Path is validated to be within base directory
	if data, err := os.ReadFile(validatedSecretKeyPath); err == nil {
		creds.SecretAccessKey = strings.TrimSpace(string(data))
	}

	// Validate that if one is provided, both must be provided
	if (creds.AccessKeyID != "" && creds.SecretAccessKey == "") ||
		(creds.AccessKeyID == "" && creds.SecretAccessKey != "") {
		return nil, fmt.Errorf("both accessKeyId and secretAccessKey must be provided if using static credentials")
	}

	// Load optional fields
	sessionTokenPath := filepath.Join(cleanCredsPath, "sessionToken")
	validatedSessionTokenPath, err := validatePath(cleanCredsPath, sessionTokenPath)
	if err != nil {
		return nil, fmt.Errorf("invalid session token path: %w", err)
	}
	// #nosec G304 -- Path is validated to be within base directory
	if data, err := os.ReadFile(validatedSessionTokenPath); err == nil {
		creds.SessionToken = strings.TrimSpace(string(data))
	}

	regionPath := filepath.Join(cleanCredsPath, "region")
	validatedRegionPath, err := validatePath(cleanCredsPath, regionPath)
	if err != nil {
		return nil, fmt.Errorf("invalid region path: %w", err)
	}
	// #nosec G304 -- Path is validated to be within base directory
	if data, err := os.ReadFile(validatedRegionPath); err == nil {
		creds.Region = strings.TrimSpace(string(data))
	} else {
		// Use default region if not provided
		creds.Region = defaultRegion
	}

	caCertPath := filepath.Join(cleanCredsPath, "caCert")
	validatedCACertPath, err := validatePath(cleanCredsPath, caCertPath)
	if err != nil {
		return nil, fmt.Errorf("invalid CA cert path: %w", err)
	}
	// #nosec G304 -- Path is validated to be within base directory
	if data, err := os.ReadFile(validatedCACertPath); err == nil {
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

	// Allow the cluster DNS suffix (for example, ".cluster.local") to be configured
	// via environment variable. When empty, we rely on Kubernetes search paths and
	// use the short ".svc" form.
	clusterDomainSuffix := strings.TrimSpace(os.Getenv("CLUSTER_DOMAIN_SUFFIX"))

	for attempt := 0; attempt < maxRetries; attempt++ {
		for i := int32(0); i < cfg.ClusterReplicas; i++ {
			podName := fmt.Sprintf("%s-%d", cfg.ClusterName, i)
			host := fmt.Sprintf("%s.%s.%s.svc", podName, cfg.ClusterName, cfg.ClusterNamespace)
			if clusterDomainSuffix != "" {
				host = host + clusterDomainSuffix
			}
			podURL := fmt.Sprintf("https://%s:%d", host, constants.PortAPI)

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
	if cfg.AuthMethod == constants.BackupAuthMethodJWT {
		// Create a client without token for authentication
		client, err := openbao.NewClient(openbao.ClientConfig{
			BaseURL: leaderURL,
			CACert:  cfg.TLSCACert,
		})
		if err != nil {
			return "", fmt.Errorf("failed to create OpenBao client: %w", err)
		}

		token, err := client.LoginJWT(ctx, cfg.JWTAuthRole, cfg.JWTToken)
		if err != nil {
			return "", fmt.Errorf("failed to authenticate using JWT Auth: %w", err)
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

	// Ensure region is set (required by S3 client)
	if cfg.StorageCredentials == nil {
		cfg.StorageCredentials = &storage.Credentials{
			Region: cfg.BackupRegion,
		}
	} else if cfg.StorageCredentials.Region == "" {
		cfg.StorageCredentials.Region = cfg.BackupRegion
	}

	// Create storage client
	storageClient, err := storage.NewS3ClientFromCredentials(
		ctx,
		cfg.BackupEndpoint,
		cfg.BackupBucket,
		cfg.StorageCredentials,
		cfg.BackupUsePathStyle,
		cfg.PartSize,
		cfg.Concurrency,
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: failed to create storage client: %v\n", err)
		os.Exit(exitStorageError)
	}

	// Stream snapshot directly to storage using a pipe
	// The Snapshot method writes to a writer, and Upload reads from a reader
	pr, pw := io.Pipe()

	// Start snapshot in a goroutine, writing to the pipe writer
	snapshotErrCh := make(chan error, 1)
	go func() {
		defer func() {
			if err := pw.Close(); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "bao-backup warning: failed to close pipe writer: %v\n", err)
			}
		}()
		snapshotErrCh <- baoClient.Snapshot(ctx, pw)
	}()

	// Upload to storage, reading from the pipe reader
	// This will block until the snapshot is complete or an error occurs
	if err := storageClient.Upload(ctx, backupKey, pr, -1); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: failed to upload backup: %v\n", err)
		os.Exit(exitStorageError)
	}

	// Close the reader and check for snapshot errors
	_ = pr.Close()
	if err := <-snapshotErrCh; err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: failed to get snapshot: %v\n", err)
		os.Exit(exitSnapshotError)
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
