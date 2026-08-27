package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	backupconfig "github.com/dc-tec/openbao-operator/internal/service/backup"
)

type restoreSettings struct {
	key          string
	bucket       string
	endpoint     string
	region       string
	usePathStyle bool
	force        bool
}

// runRestore executes the restore operation.
func runRestore(ctx context.Context) error {
	flag.Parse()

	fmt.Println("Starting restore operation...")

	cfg, err := backupconfig.LoadExecutorConfig()
	if err != nil {
		return categorizef(errConfigCategory, "failed to load configuration: %w", err)
	}
	fmt.Printf("Configuration loaded - cluster=%s, namespace=%s, replicas=%d\n",
		cfg.ClusterName, cfg.ClusterNamespace, cfg.ClusterReplicas)

	settings, err := resolveRestoreSettings(cfg)
	if err != nil {
		return categorize(errConfigCategory, err)
	}
	fmt.Printf("Restore key: %s\n", settings.key)

	leaderURL, err := findRestoreLeader(ctx, cfg)
	if err != nil {
		return categorizef(errLeaderCategory, "failed to find leader: %w", err)
	}
	fmt.Printf("Found leader at: %s\n", leaderURL)

	fmt.Printf("Authenticating to leader (method=%s)...\n", cfg.AuthMethod)
	token, err := authenticate(ctx, cfg, leaderURL)
	if err != nil {
		return categorizef(errAuthCategory, "failed to authenticate: %w", err)
	}
	fmt.Println("Authentication successful")

	baoClient, closeClient, err := openClusterClient(cfg, "restore", leaderURL, token)
	if err != nil {
		return categorize(errConfigCategory, err)
	}
	defer closeClient()

	restoreCfg := buildRestoreExecutorConfig(cfg, settings)

	fmt.Println("Creating storage client...")
	storageClient, err := openStorageClient(ctx, &restoreCfg)
	if err != nil {
		return categorizef(errStorageCategory, "failed to create storage client: %w", err)
	}
	defer func() {
		_ = storageClient.Close()
	}()
	fmt.Println("Storage client created")

	reader, objInfo, err := downloadRestoreSnapshot(ctx, storageClient, settings.key)
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()

	_, _ = fmt.Fprintf(os.Stdout, "Found snapshot: %s (size: %d bytes)\n", settings.key, objInfo.Size)
	fmt.Println("Snapshot downloaded successfully")
	fmt.Println("Restoring snapshot to cluster...")
	if err := baoClient.Restore(ctx, reader, portopenbao.RestoreOptions{Force: settings.force}); err != nil {
		return categorizef(errSnapshotCategory, "failed to restore snapshot: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "Restore completed successfully from: %s\n", settings.key)
	return nil
}

func findRestoreLeader(ctx context.Context, cfg *backupconfig.ExecutorConfig) (string, error) {
	fmt.Println("Finding cluster leader...")
	leaderCtx, leaderCancel := context.WithTimeout(ctx, 60*time.Second)
	defer leaderCancel()

	return findLeader(leaderCtx, cfg)
}

func buildRestoreExecutorConfig(
	cfg *backupconfig.ExecutorConfig,
	settings restoreSettings,
) backupconfig.ExecutorConfig {
	restoreCfg := *cfg
	restoreCfg.BackupBucket = settings.bucket
	restoreCfg.BackupEndpoint = settings.endpoint
	restoreCfg.BackupRegion = settings.region
	restoreCfg.BackupUsePathStyle = settings.usePathStyle

	if restoreCfg.BackupProvider == constants.StorageProviderGCS {
		endpointLower := strings.ToLower(settings.endpoint)
		if strings.Contains(endpointLower, "fake-gcs-server") || strings.HasPrefix(endpointLower, "http://") {
			restoreCfg.GCSUseEmulator = true
		}
	}

	if restoreCfg.StorageCredentials == nil {
		restoreCfg.StorageCredentials = &blobstore.Credentials{
			Region: settings.region,
		}
	} else if restoreCfg.StorageCredentials.Region == "" {
		restoreCfg.StorageCredentials.Region = settings.region
	}

	return restoreCfg
}

func downloadRestoreSnapshot(
	ctx context.Context,
	storageClient blobstore.BlobStore,
	key string,
) (io.ReadCloser, *blobstore.ObjectInfo, error) {
	fmt.Printf("Verifying snapshot exists: %s\n", key)
	objInfo, err := storageClient.Head(ctx, key)
	if err != nil {
		return nil, nil, categorizef(errVerificationCategory, "failed to verify snapshot exists: %w", err)
	}
	if objInfo == nil {
		return nil, nil, categorize(errVerificationCategory, fmt.Errorf("snapshot not found: %s", key))
	}

	fmt.Println("Downloading snapshot from storage...")
	reader, err := storageClient.Download(ctx, key)
	if err != nil {
		return nil, nil, categorizef(errStorageCategory, "failed to download snapshot: %w", err)
	}

	return reader, objInfo, nil
}

func resolveRestoreSettings(cfg *backupconfig.ExecutorConfig) (restoreSettings, error) {
	if cfg == nil {
		return restoreSettings{}, fmt.Errorf("restore configuration is required")
	}

	settings := restoreSettings{
		key: strings.TrimSpace(os.Getenv(constants.EnvRestoreKey)),
	}
	if settings.key == "" {
		return restoreSettings{}, fmt.Errorf("%s environment variable is required", constants.EnvRestoreKey)
	}

	settings.bucket = strings.TrimSpace(os.Getenv(constants.EnvRestoreBucket))
	if settings.bucket == "" {
		settings.bucket = cfg.BackupBucket
	}

	settings.endpoint = strings.TrimSpace(os.Getenv(constants.EnvRestoreEndpoint))
	if settings.endpoint == "" {
		settings.endpoint = cfg.BackupEndpoint
	}

	settings.region = strings.TrimSpace(os.Getenv(constants.EnvRestoreRegion))
	if settings.region == "" {
		settings.region = cfg.BackupRegion
	}

	settings.usePathStyle = strings.EqualFold(strings.TrimSpace(os.Getenv(constants.EnvRestoreUsePathStyle)), "true")
	forceValue := strings.TrimSpace(os.Getenv(constants.EnvRestoreForce))
	if forceValue != "" {
		force, err := strconv.ParseBool(forceValue)
		if err != nil {
			return restoreSettings{}, fmt.Errorf("invalid %s value %q: %w", constants.EnvRestoreForce, forceValue, err)
		}
		settings.force = force
	}

	return settings, nil
}
