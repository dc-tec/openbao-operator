package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	backupconfig "github.com/dc-tec/openbao-operator/internal/backup"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/storage"
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
)

// findLeader discovers the current Raft leader by querying health endpoints.
// It retries with exponential backoff to handle cases where pods are still starting up
// after scale-up operations.
func findLeader(ctx context.Context, cfg *backupconfig.ExecutorConfig) (string, error) {
	// Retry with exponential backoff: 1s, 2s, 4s, 8s, 16s
	maxRetries := 5
	baseDelay := 1 * time.Second

	// Allow the cluster DNS suffix (for example, ".cluster.local") to be configured
	// via environment variable. When empty, we rely on Kubernetes search paths and
	// use the short ".svc" form.
	clusterDomainSuffix := strings.TrimSpace(os.Getenv("CLUSTER_DOMAIN_SUFFIX"))

	fmt.Printf("findLeader: Starting leader discovery for %d replicas (statefulset=%s)\n",
		cfg.ClusterReplicas, cfg.StatefulSetName)

	for attempt := 0; attempt < maxRetries; attempt++ {
		fmt.Printf("findLeader: Attempt %d/%d\n", attempt+1, maxRetries)
		for i := int32(0); i < cfg.ClusterReplicas; i++ {
			// Use StatefulSetName for pod name (may include revision for Blue/Green)
			// but ClusterName for service name (headless service is always cluster name)
			podName := fmt.Sprintf("%s-%d", cfg.StatefulSetName, i)
			host := fmt.Sprintf("%s.%s.%s.svc", podName, cfg.ClusterName, cfg.ClusterNamespace)
			if clusterDomainSuffix != "" {
				host = host + clusterDomainSuffix
			}
			podURL := fmt.Sprintf("https://%s:%d", host, constants.PortAPI)

			fmt.Printf("findLeader: Checking pod %s at %s\n", podName, podURL)

			// Create a client without token for health checks
			client, err := openbao.NewClient(openbao.ClientConfig{
				BaseURL: podURL,
				CACert:  cfg.TLSCACert,
			})
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

		// If we've exhausted all retries, return error
		if attempt == maxRetries-1 {
			break
		}

		// Wait before retrying with exponential backoff
		delay := baseDelay * time.Duration(1<<uint(attempt))
		fmt.Printf("findLeader: No leader found, waiting %v before retry...\n", delay)
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
func authenticate(ctx context.Context, cfg *backupconfig.ExecutorConfig, leaderURL string) (string, error) {
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

func run(ctx context.Context) error {
	flag.Parse()

	// Load configuration
	cfg, err := backupconfig.LoadExecutorConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Find leader with timeout (allow up to ~30 seconds for retries)
	leaderCtx, leaderCancel := context.WithTimeout(ctx, 30*time.Second)
	defer leaderCancel()

	leaderURL, err := findLeader(leaderCtx, cfg)
	if err != nil {
		return fmt.Errorf("failed to find leader: %w", err)
	}

	// Authenticate
	token, err := authenticate(ctx, cfg, leaderURL)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	// Create OpenBao client for leader
	baoClient, err := openbao.NewClient(openbao.ClientConfig{
		BaseURL: leaderURL,
		Token:   token,
		CACert:  cfg.TLSCACert,
	})
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	// Use explicitly provided backup key if available, otherwise generate one
	backupKey := cfg.BackupKey
	if backupKey == "" {
		// Generate backup key (legacy behavior)
		var err error
		backupKey, err = backupconfig.GenerateBackupKey(
			cfg.BackupPathPrefix,
			cfg.ClusterNamespace,
			cfg.ClusterName,
			cfg.BackupFilenamePrefix,
			time.Now().UTC(),
		)
		if err != nil {
			return fmt.Errorf("failed to generate backup key: %w", err)
		}
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
	storageClient, err := storage.OpenS3Bucket(ctx, storage.S3ClientConfig{
		Endpoint:        cfg.BackupEndpoint,
		Bucket:          cfg.BackupBucket,
		Region:          cfg.StorageCredentials.Region,
		AccessKeyID:     cfg.StorageCredentials.AccessKeyID,
		SecretAccessKey: cfg.StorageCredentials.SecretAccessKey,
		SessionToken:    cfg.StorageCredentials.SessionToken,
		CACert:          cfg.StorageCredentials.CACert,
		UsePathStyle:    cfg.BackupUsePathStyle,
	})
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}
	defer func() {
		_ = storageClient.Close()
	}()

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
	if err := storageClient.Upload(ctx, backupKey, pr); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		return fmt.Errorf("failed to upload backup: %w", err)
	}

	// Close the reader and check for snapshot errors
	_ = pr.Close()
	if err := <-snapshotErrCh; err != nil {
		return fmt.Errorf("failed to get snapshot: %w", err)
	}

	// Verify upload
	objInfo, err := storageClient.Head(ctx, backupKey)
	if err != nil {
		return fmt.Errorf("failed to verify backup upload: %w", err)
	}
	if objInfo == nil {
		return fmt.Errorf("backup verification failed: object not found after upload")
	}
	if objInfo.Size == 0 {
		return fmt.Errorf("backup verification failed: uploaded object has zero size")
	}

	// Success
	_, _ = fmt.Fprintf(os.Stdout, "Backup completed successfully: %s (size: %d bytes)\n", backupKey, objInfo.Size)

	return nil
}

// runRestore executes the restore operation.
func runRestore(ctx context.Context) error {
	flag.Parse()

	fmt.Println("Starting restore operation...")

	// Load configuration (reuse backup config for common settings)
	cfg, err := backupconfig.LoadExecutorConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	fmt.Printf("Configuration loaded - cluster=%s, namespace=%s, replicas=%d\n",
		cfg.ClusterName, cfg.ClusterNamespace, cfg.ClusterReplicas)

	// Get restore-specific configuration from environment
	restoreKey := os.Getenv("RESTORE_KEY")
	if restoreKey == "" {
		return fmt.Errorf("RESTORE_KEY environment variable is required")
	}
	fmt.Printf("Restore key: %s\n", restoreKey)

	restoreBucket := os.Getenv("RESTORE_BUCKET")
	if restoreBucket == "" {
		restoreBucket = cfg.BackupBucket // Fall back to backup bucket if not specified
	}

	restoreEndpoint := os.Getenv("RESTORE_ENDPOINT")
	if restoreEndpoint == "" {
		restoreEndpoint = cfg.BackupEndpoint // Fall back to backup endpoint
	}

	restoreRegion := os.Getenv("RESTORE_REGION")
	if restoreRegion == "" {
		restoreRegion = cfg.BackupRegion
	}

	usePathStyle := os.Getenv("RESTORE_USE_PATH_STYLE") == "true"

	// Find leader with timeout (60s to allow for retries in findLeader)
	fmt.Println("Finding cluster leader...")
	leaderCtx, leaderCancel := context.WithTimeout(ctx, 60*time.Second)
	defer leaderCancel()

	leaderURL, err := findLeader(leaderCtx, cfg)
	if err != nil {
		return fmt.Errorf("failed to find leader: %w", err)
	}
	fmt.Printf("Found leader at: %s\n", leaderURL)

	// Authenticate
	fmt.Printf("Authenticating to leader (method=%s)...\n", cfg.AuthMethod)
	token, err := authenticate(ctx, cfg, leaderURL)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	fmt.Println("Authentication successful")

	// Create OpenBao client for leader
	baoClient, err := openbao.NewClient(openbao.ClientConfig{
		BaseURL: leaderURL,
		Token:   token,
		CACert:  cfg.TLSCACert,
	})
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	// Ensure region is set
	if cfg.StorageCredentials == nil {
		cfg.StorageCredentials = &storage.Credentials{
			Region: restoreRegion,
		}
	} else if cfg.StorageCredentials.Region == "" {
		cfg.StorageCredentials.Region = restoreRegion
	}

	// Create storage client for downloading
	fmt.Println("Creating storage client...")
	storageClient, err := storage.OpenS3Bucket(ctx, storage.S3ClientConfig{
		Endpoint:        restoreEndpoint,
		Bucket:          restoreBucket,
		Region:          cfg.StorageCredentials.Region,
		AccessKeyID:     cfg.StorageCredentials.AccessKeyID,
		SecretAccessKey: cfg.StorageCredentials.SecretAccessKey,
		SessionToken:    cfg.StorageCredentials.SessionToken,
		CACert:          cfg.StorageCredentials.CACert,
		UsePathStyle:    usePathStyle,
	})
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}
	defer func() {
		_ = storageClient.Close()
	}()
	fmt.Println("Storage client created")

	// Verify snapshot exists before downloading
	fmt.Printf("Verifying snapshot exists: %s\n", restoreKey)
	objInfo, err := storageClient.Head(ctx, restoreKey)
	if err != nil {
		return fmt.Errorf("failed to verify snapshot exists: %w", err)
	}
	if objInfo == nil {
		return fmt.Errorf("snapshot not found: %s", restoreKey)
	}

	_, _ = fmt.Fprintf(os.Stdout, "Found snapshot: %s (size: %d bytes)\n", restoreKey, objInfo.Size)

	// Download snapshot from storage
	fmt.Println("Downloading snapshot from storage...")
	reader, err := storageClient.Download(ctx, restoreKey)
	if err != nil {
		return fmt.Errorf("failed to download snapshot: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()
	fmt.Println("Snapshot downloaded successfully")

	// Perform restore
	fmt.Println("Restoring snapshot to cluster...")
	if err := baoClient.Restore(ctx, reader); err != nil {
		return fmt.Errorf("failed to restore snapshot: %w", err)
	}

	// Success
	_, _ = fmt.Fprintf(os.Stdout, "Restore completed successfully from: %s\n", restoreKey)
	return nil
}

func main() {
	ctx := context.Background()

	// Check executor mode
	mode := os.Getenv("EXECUTOR_MODE")
	var err error

	switch mode {
	case "restore":
		err = runRestore(ctx)
	case "backup", "":
		// Default to backup mode for backward compatibility
		err = run(ctx)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "unknown EXECUTOR_MODE: %s (expected 'backup' or 'restore')\n", mode)
		os.Exit(exitConfigError)
	}

	if err != nil {
		prefix := "bao-backup"
		if mode == "restore" {
			prefix = "bao-restore"
		}
		_, _ = fmt.Fprintf(os.Stderr, "%s error: %v\n", prefix, err)
		// Determine exit code based on error type
		errStr := err.Error()
		switch {
		case strings.Contains(errStr, "failed to load configuration"):
			os.Exit(exitConfigError)
		case strings.Contains(errStr, "failed to authenticate"):
			os.Exit(exitAuthError)
		case strings.Contains(errStr, "failed to find leader"):
			os.Exit(exitLeaderDiscovery)
		case strings.Contains(errStr, "failed to get snapshot") ||
			strings.Contains(errStr, "failed to restore snapshot"):
			os.Exit(exitSnapshotError)
		case strings.Contains(errStr, "failed to upload backup") ||
			strings.Contains(errStr, "failed to download snapshot") ||
			strings.Contains(errStr, "failed to create storage client"):
			os.Exit(exitStorageError)
		case strings.Contains(errStr, "failed to verify"):
			os.Exit(exitVerificationError)
		default:
			os.Exit(exitConfigError)
		}
	}
	os.Exit(exitSuccess)
}
