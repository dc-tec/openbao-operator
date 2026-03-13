package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/adapter/storage"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	backupconfig "github.com/dc-tec/openbao-operator/internal/service/backup"
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

type restoreSettings struct {
	key          string
	bucket       string
	endpoint     string
	region       string
	usePathStyle bool
}

// findLeader discovers the current Raft leader by querying health endpoints.
// It retries with exponential backoff to handle cases where pods are still starting up
// after scale-up operations.
func findLeader(ctx context.Context, cfg *backupconfig.ExecutorConfig) (string, error) {
	// Create a ClientManager for this operation
	mgr := openbao.NewClientManager(portopenbao.ClientConfig{
		CACert:                         cfg.TLSCACert,
		TLSServerName:                  cfg.TLSServerName,
		RateLimitQPS:                   cfg.RateLimitQPS,
		RateLimitBurst:                 cfg.RateLimitBurst,
		CircuitBreakerFailureThreshold: cfg.CircuitBreakerFailureThreshold,
		CircuitBreakerOpenDuration:     parseDuration(cfg.CircuitBreakerOpenDuration),
	})
	defer mgr.Close()

	// Use a generic cluster key since we are scanning pods in the local cluster
	factory := mgr.FactoryFor("leader-discovery", cfg.TLSCACert)

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
		mgr := openbao.NewClientManager(portopenbao.ClientConfig{
			CACert:                         cfg.TLSCACert,
			TLSServerName:                  cfg.TLSServerName,
			RateLimitQPS:                   cfg.RateLimitQPS,
			RateLimitBurst:                 cfg.RateLimitBurst,
			CircuitBreakerFailureThreshold: cfg.CircuitBreakerFailureThreshold,
			CircuitBreakerOpenDuration:     parseDuration(cfg.CircuitBreakerOpenDuration),
		})
		defer mgr.Close()
		factory := mgr.FactoryFor("auth", cfg.TLSCACert)
		return factory.LoginJWT(ctx, leaderURL, cfg.JWTAuthRole, cfg.JWTToken)
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
	// Create OpenBao client for leader
	clientMgr := openbao.NewClientManager(portopenbao.ClientConfig{
		CACert:                         cfg.TLSCACert,
		TLSServerName:                  cfg.TLSServerName,
		RateLimitQPS:                   cfg.RateLimitQPS,
		RateLimitBurst:                 cfg.RateLimitBurst,
		CircuitBreakerFailureThreshold: cfg.CircuitBreakerFailureThreshold,
		CircuitBreakerOpenDuration:     parseDuration(cfg.CircuitBreakerOpenDuration),
	})
	defer clientMgr.Close()
	factory := clientMgr.FactoryFor("backup", cfg.TLSCACert)
	baoClient, err := factory.NewWithToken(leaderURL, token)
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

	// Create storage client based on provider
	storageClient, err := openStorageClient(ctx, cfg)
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

	settings, err := resolveRestoreSettings(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("Restore key: %s\n", settings.key)

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
	// Create OpenBao client for leader
	clientMgr := openbao.NewClientManager(portopenbao.ClientConfig{
		CACert:                         cfg.TLSCACert,
		TLSServerName:                  cfg.TLSServerName,
		RateLimitQPS:                   cfg.RateLimitQPS,
		RateLimitBurst:                 cfg.RateLimitBurst,
		CircuitBreakerFailureThreshold: cfg.CircuitBreakerFailureThreshold,
		CircuitBreakerOpenDuration:     parseDuration(cfg.CircuitBreakerOpenDuration),
	})
	defer clientMgr.Close()
	factory := clientMgr.FactoryFor("restore", cfg.TLSCACert)
	baoClient, err := factory.NewWithToken(leaderURL, token)
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	// Create a modified config for restore with restore-specific values
	restoreCfg := *cfg
	restoreCfg.BackupBucket = settings.bucket
	restoreCfg.BackupEndpoint = settings.endpoint
	restoreCfg.BackupRegion = settings.region
	restoreCfg.BackupUsePathStyle = settings.usePathStyle

	// Re-check emulator mode for GCS after setting restore endpoint
	// (emulator detection in LoadExecutorConfig only checks BACKUP_ENDPOINT)
	if restoreCfg.BackupProvider == constants.StorageProviderGCS {
		endpointLower := strings.ToLower(settings.endpoint)
		if strings.Contains(endpointLower, "fake-gcs-server") || strings.HasPrefix(endpointLower, "http://") {
			restoreCfg.GCSUseEmulator = true
		}
	}

	// Ensure region is set in credentials for S3
	if restoreCfg.StorageCredentials == nil {
		restoreCfg.StorageCredentials = &blobstore.Credentials{
			Region: settings.region,
		}
	} else if restoreCfg.StorageCredentials.Region == "" {
		restoreCfg.StorageCredentials.Region = settings.region
	}

	// Create storage client for downloading using cloud-agnostic API
	fmt.Println("Creating storage client...")
	storageClient, err := openStorageClient(ctx, &restoreCfg)
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}
	defer func() {
		_ = storageClient.Close()
	}()
	fmt.Println("Storage client created")

	// Verify snapshot exists before downloading
	fmt.Printf("Verifying snapshot exists: %s\n", settings.key)
	objInfo, err := storageClient.Head(ctx, settings.key)
	if err != nil {
		return fmt.Errorf("failed to verify snapshot exists: %w", err)
	}
	if objInfo == nil {
		return fmt.Errorf("snapshot not found: %s", settings.key)
	}

	_, _ = fmt.Fprintf(os.Stdout, "Found snapshot: %s (size: %d bytes)\n", settings.key, objInfo.Size)

	// Download snapshot from storage
	fmt.Println("Downloading snapshot from storage...")
	reader, err := storageClient.Download(ctx, settings.key)
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
	_, _ = fmt.Fprintf(os.Stdout, "Restore completed successfully from: %s\n", settings.key)
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
		os.Exit(exitCodeForError(err))
	}
	os.Exit(exitSuccess)
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

func resolveRestoreSettings(cfg *backupconfig.ExecutorConfig) (restoreSettings, error) {
	if cfg == nil {
		return restoreSettings{}, fmt.Errorf("restore configuration is required")
	}

	settings := restoreSettings{
		key: strings.TrimSpace(os.Getenv("RESTORE_KEY")),
	}
	if settings.key == "" {
		return restoreSettings{}, fmt.Errorf("RESTORE_KEY environment variable is required")
	}

	settings.bucket = strings.TrimSpace(os.Getenv("RESTORE_BUCKET"))
	if settings.bucket == "" {
		settings.bucket = cfg.BackupBucket
	}

	settings.endpoint = strings.TrimSpace(os.Getenv("RESTORE_ENDPOINT"))
	if settings.endpoint == "" {
		settings.endpoint = cfg.BackupEndpoint
	}

	settings.region = strings.TrimSpace(os.Getenv("RESTORE_REGION"))
	if settings.region == "" {
		settings.region = cfg.BackupRegion
	}

	settings.usePathStyle = strings.EqualFold(strings.TrimSpace(os.Getenv("RESTORE_USE_PATH_STYLE")), "true")

	return settings, nil
}

func exitCodeForError(err error) int {
	if err == nil {
		return exitSuccess
	}

	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "failed to load configuration"):
		return exitConfigError
	case strings.Contains(errStr, "failed to authenticate"):
		return exitAuthError
	case strings.Contains(errStr, "failed to find leader"):
		return exitLeaderDiscovery
	case strings.Contains(errStr, "failed to get snapshot") ||
		strings.Contains(errStr, "failed to restore snapshot"):
		return exitSnapshotError
	case strings.Contains(errStr, "failed to upload backup") ||
		strings.Contains(errStr, "failed to download snapshot") ||
		strings.Contains(errStr, "failed to create storage client"):
		return exitStorageError
	case strings.Contains(errStr, "failed to verify"):
		return exitVerificationError
	default:
		return exitConfigError
	}
}

func buildStorageConfig(cfg *backupconfig.ExecutorConfig) (storage.Config, error) {
	if cfg == nil {
		return storage.Config{}, fmt.Errorf("storage configuration is required")
	}

	provider := cfg.BackupProvider
	if provider == "" {
		provider = constants.StorageProviderS3
	}

	storageConfig := storage.Config{
		Provider: storage.ProviderType(provider),
		Bucket:   cfg.BackupBucket,
		Endpoint: cfg.BackupEndpoint,
	}

	switch provider {
	case constants.StorageProviderS3:
		storageConfig.Region = cfg.BackupRegion
		if cfg.StorageCredentials != nil {
			storageConfig.Credentials = cfg.StorageCredentials
		} else {
			storageConfig.Credentials = &blobstore.Credentials{
				Region: cfg.BackupRegion,
			}
		}
		storageConfig.S3 = &storage.S3Options{
			UsePathStyle:       cfg.BackupUsePathStyle,
			InsecureSkipVerify: cfg.InsecureSkipVerify,
			EnsureExists:       true, // Always try to ensure bucket exists for backups
		}

		// Fallback: infer InsecureSkipVerify for HTTP endpoints when not explicitly set.
		if !cfg.InsecureSkipVerify && strings.HasPrefix(strings.ToLower(cfg.BackupEndpoint), "http://") {
			storageConfig.S3.InsecureSkipVerify = true
		}

	case constants.StorageProviderGCS:
		storageConfig.GCS = &storage.GCSOptions{
			Project:            cfg.GCSProject,
			CredentialsJSON:    cfg.GCSCredentialsJSON,
			UseEmulator:        cfg.GCSUseEmulator,
			InsecureSkipVerify: cfg.InsecureSkipVerify,
		}
		if !cfg.InsecureSkipVerify && cfg.GCSUseEmulator &&
			strings.HasPrefix(strings.ToLower(cfg.BackupEndpoint), "http://") {
			storageConfig.GCS.InsecureSkipVerify = true
		}

	case constants.StorageProviderAzure:
		storageConfig.Azure = &storage.AzureOptions{
			StorageAccount:     cfg.AzureStorageAccount,
			AccountKey:         cfg.AzureAccountKey,
			ConnectionString:   cfg.AzureConnectionString,
			UseManagedIdentity: cfg.AzureAccountKey == "" && cfg.AzureConnectionString == "",
			InsecureSkipVerify: cfg.InsecureSkipVerify,
		}
		if cfg.AzureContainer != "" {
			storageConfig.Bucket = cfg.AzureContainer
		}

	default:
		return storage.Config{}, fmt.Errorf("unknown storage provider: %q", cfg.BackupProvider)
	}

	return storageConfig, nil
}

// openStorageClient creates a storage client based on the configured provider.
func openStorageClient(ctx context.Context, cfg *backupconfig.ExecutorConfig) (blobstore.BlobStore, error) {
	storageConfig, err := buildStorageConfig(cfg)
	if err != nil {
		return nil, err
	}

	return storage.OpenBlobStore(ctx, storageConfig)
}
