package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	backupconfig "github.com/dc-tec/openbao-operator/internal/service/backup"
)

const backupCleanupTimeout = 30 * time.Second

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

	objInfo, err := publishBackupSnapshot(ctx, baoClient, storageClient, backupKey)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stdout, "Backup completed successfully: %s (size: %d bytes)\n", backupKey, objInfo.Size)
	return nil
}

func publishBackupSnapshot(
	ctx context.Context,
	baoClient portopenbao.ClusterActions,
	storageClient blobstore.BlobStore,
	backupKey string,
) (*blobstore.ObjectInfo, error) {
	if err := uploadBackupSnapshot(ctx, baoClient, storageClient, backupKey); err != nil {
		return nil, cleanupFailedBackup(ctx, storageClient, backupKey, err)
	}

	objInfo, err := verifyBackupUpload(ctx, storageClient, backupKey)
	if err != nil {
		return nil, cleanupFailedBackup(ctx, storageClient, backupKey, err)
	}

	return objInfo, nil
}

func cleanupFailedBackup(
	ctx context.Context,
	storageClient blobstore.BlobStore,
	backupKey string,
	failure error,
) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backupCleanupTimeout)
	defer cancel()

	if err := storageClient.Delete(cleanupCtx, backupKey); err != nil {
		return errors.Join(
			failure,
			categorizef(errStorageCategory, "failed to delete incomplete backup %s: %w", backupKey, err),
		)
	}
	return failure
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
	type snapshotStreamResult struct {
		snapshotErr error
		closeErr    error
	}
	snapshotResultCh := make(chan snapshotStreamResult, 1)
	go func() {
		snapshotErr := baoClient.Snapshot(ctx, pw)
		snapshotResultCh <- snapshotStreamResult{
			snapshotErr: snapshotErr,
			closeErr:    pw.CloseWithError(snapshotErr),
		}
	}()

	uploadErr := storageClient.Upload(ctx, backupKey, pr)
	var uploadAbortErr error
	if uploadErr != nil {
		uploadAbortErr = fmt.Errorf("upload stopped snapshot stream: %w", uploadErr)
		_ = pr.CloseWithError(uploadAbortErr)
	} else {
		_ = pr.Close()
	}
	snapshotResult := <-snapshotResultCh

	independentSnapshotFailure := snapshotResult.snapshotErr != nil &&
		(uploadAbortErr == nil || !errors.Is(snapshotResult.snapshotErr, uploadAbortErr))
	if independentSnapshotFailure {
		return categorizef(errSnapshotCategory, "failed to get snapshot: %w", snapshotResult.snapshotErr)
	}
	if uploadErr != nil {
		return categorizef(errStorageCategory, "failed to upload backup: %w", uploadErr)
	}
	if snapshotResult.snapshotErr != nil {
		return categorizef(errSnapshotCategory, "failed to get snapshot: %w", snapshotResult.snapshotErr)
	}
	if snapshotResult.closeErr != nil && !errors.Is(snapshotResult.closeErr, io.ErrClosedPipe) {
		return categorizef(errSnapshotCategory, "failed to close snapshot stream: %w", snapshotResult.closeErr)
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
