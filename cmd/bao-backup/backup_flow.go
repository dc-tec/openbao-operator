package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	backupconfig "github.com/dc-tec/openbao-operator/internal/service/backup"
)

func run(ctx context.Context) error {
	flag.Parse()

	cfg, err := backupconfig.LoadExecutorConfig()
	if err != nil {
		return categorizef(errConfigCategory, "failed to load configuration: %w", err)
	}

	leaderURL, err := findBackupLeader(ctx, cfg)
	if err != nil {
		return categorizef(errLeaderCategory, "failed to find leader: %w", err)
	}

	token, err := authenticate(ctx, cfg, leaderURL)
	if err != nil {
		return categorizef(errAuthCategory, "failed to authenticate: %w", err)
	}

	baoClient, closeClient, err := openClusterClient(cfg, "backup", leaderURL, token)
	if err != nil {
		return categorize(errConfigCategory, err)
	}
	defer closeClient()

	backupKey, err := resolveBackupKey(cfg, time.Now().UTC())
	if err != nil {
		return categorizef(errConfigCategory, "failed to generate backup key: %w", err)
	}

	storageClient, err := openStorageClient(ctx, cfg)
	if err != nil {
		return categorizef(errStorageCategory, "failed to create storage client: %w", err)
	}
	defer func() {
		_ = storageClient.Close()
	}()

	if err := uploadBackupSnapshot(ctx, baoClient, storageClient, backupKey); err != nil {
		return err
	}

	objInfo, err := verifyBackupUpload(ctx, storageClient, backupKey)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stdout, "Backup completed successfully: %s (size: %d bytes)\n", backupKey, objInfo.Size)
	return nil
}

func findBackupLeader(ctx context.Context, cfg *backupconfig.ExecutorConfig) (string, error) {
	leaderCtx, leaderCancel := context.WithTimeout(ctx, 30*time.Second)
	defer leaderCancel()

	return findLeader(leaderCtx, cfg)
}

func resolveBackupKey(cfg *backupconfig.ExecutorConfig, now time.Time) (string, error) {
	if cfg.BackupKey != "" {
		return cfg.BackupKey, nil
	}

	return backupconfig.GenerateBackupKey(
		cfg.BackupPathPrefix,
		cfg.ClusterNamespace,
		cfg.ClusterName,
		cfg.BackupFilenamePrefix,
		now,
	)
}

func uploadBackupSnapshot(
	ctx context.Context,
	baoClient portopenbao.ClusterActions,
	storageClient blobstore.BlobStore,
	backupKey string,
) error {
	pr, pw := io.Pipe()
	snapshotErrCh := make(chan error, 1)
	go func() {
		defer func() {
			if err := pw.Close(); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "bao-backup warning: failed to close pipe writer: %v\n", err)
			}
		}()
		snapshotErrCh <- baoClient.Snapshot(ctx, pw)
	}()

	if err := storageClient.Upload(ctx, backupKey, pr); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		return categorizef(errStorageCategory, "failed to upload backup: %w", err)
	}

	_ = pr.Close()
	if err := <-snapshotErrCh; err != nil {
		return categorizef(errSnapshotCategory, "failed to get snapshot: %w", err)
	}

	return nil
}

func verifyBackupUpload(
	ctx context.Context,
	storageClient blobstore.BlobStore,
	backupKey string,
) (*blobstore.ObjectInfo, error) {
	objInfo, err := storageClient.Head(ctx, backupKey)
	if err != nil {
		return nil, categorizef(errVerificationCategory, "failed to verify backup upload: %w", err)
	}
	if objInfo == nil {
		return nil, categorize(
			errVerificationCategory,
			fmt.Errorf("backup verification failed: object not found after upload"),
		)
	}
	if objInfo.Size == 0 {
		return nil, categorize(
			errVerificationCategory,
			fmt.Errorf("backup verification failed: uploaded object has zero size"),
		)
	}

	return objInfo, nil
}
