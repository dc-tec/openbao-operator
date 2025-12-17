package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	backupconfig "github.com/openbao/operator/internal/backup"
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

	// Generate backup key
	backupKey, err := backupconfig.GenerateBackupKey(
		cfg.BackupPathPrefix,
		cfg.ClusterNamespace,
		cfg.ClusterName,
		cfg.BackupFilenamePrefix,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("failed to generate backup key: %w", err)
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
		return fmt.Errorf("failed to create storage client: %w", err)
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

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-backup error: %v\n", err)
		// Determine exit code based on error type
		errStr := err.Error()
		switch {
		case strings.Contains(errStr, "failed to load configuration"):
			os.Exit(exitConfigError)
		case strings.Contains(errStr, "failed to authenticate"):
			os.Exit(exitAuthError)
		case strings.Contains(errStr, "failed to find leader"):
			os.Exit(exitLeaderDiscovery)
		case strings.Contains(errStr, "failed to get snapshot"):
			os.Exit(exitSnapshotError)
		case strings.Contains(errStr, "failed to upload backup") || strings.Contains(errStr, "failed to create storage client"):
			os.Exit(exitStorageError)
		case strings.Contains(errStr, "failed to verify backup"):
			os.Exit(exitVerificationError)
		default:
			os.Exit(exitConfigError)
		}
	}
	os.Exit(exitSuccess)
}
