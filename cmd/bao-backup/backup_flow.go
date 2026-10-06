package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
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

	before, err := baoClient.Health(ctx)
	if err != nil {
		return categorizef(errSnapshotCategory, "observe snapshot source identity: %w", err)
	}

	digest := sha256.New()
	objInfo, err := publishBackupSnapshot(ctx, baoClient, storageClient, backupKey, digest)
	if err != nil {
		return err
	}

	after, err := baoClient.Health(ctx)
	if err != nil || before.ClusterID == "" || before.ClusterID != after.ClusterID || before.Version != after.Version {
		return categorizef(errSnapshotCategory, "source identity or version was not stable across snapshot streaming")
	}
	report, err := json.Marshal(api.BackupSnapshotSummary{
		ClusterID: after.ClusterID, Version: after.Version,
		Size: objInfo.Size, Digest: fmt.Sprintf("sha256:%x", digest.Sum(nil)),
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile("/dev/termination-log", report, 0600); err != nil {
		return categorizef(errSnapshotCategory, "write snapshot source observation: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "Backup completed successfully: %s (size: %d bytes)\n", backupKey, objInfo.Size)
	return nil
}

func publishBackupSnapshot(
	ctx context.Context,
	baoClient portopenbao.ClusterActions,
	storageClient blobstore.BlobStore,
	backupKey string,
	digest ...io.Writer,
) (*blobstore.ObjectInfo, error) {
	written, err := uploadBackupSnapshot(ctx, baoClient, storageClient, backupKey, digest...)
	if err != nil {
		return nil, cleanupFailedBackup(ctx, storageClient, backupKey, err)
	}

	objInfo, err := verifyBackupUpload(ctx, storageClient, backupKey, written)
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

// countingReader counts the bytes the storage client consumed from the snapshot stream.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// uploadBackupSnapshot streams a snapshot to storage and returns the number of bytes uploaded.
func uploadBackupSnapshot(
	ctx context.Context,
	baoClient portopenbao.ClusterActions,
	storageClient blobstore.BlobStore,
	backupKey string,
	digest ...io.Writer,
) (int64, error) {
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

	var reader io.Reader = pr
	if len(digest) > 0 {
		reader = io.TeeReader(pr, digest[0])
	}
	body := &countingReader{r: reader}
	uploadErr := storageClient.Upload(ctx, backupKey, body)
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
		return 0, categorizef(errSnapshotCategory, "failed to get snapshot: %w", snapshotResult.snapshotErr)
	}
	if uploadErr != nil {
		return 0, categorizef(errStorageCategory, "failed to upload backup: %w", uploadErr)
	}
	if snapshotResult.snapshotErr != nil {
		return 0, categorizef(errSnapshotCategory, "failed to get snapshot: %w", snapshotResult.snapshotErr)
	}
	if snapshotResult.closeErr != nil && !errors.Is(snapshotResult.closeErr, io.ErrClosedPipe) {
		return 0, categorizef(errSnapshotCategory, "failed to close snapshot stream: %w", snapshotResult.closeErr)
	}

	return body.n, nil
}

func verifyBackupUpload(
	ctx context.Context,
	storageClient blobstore.BlobStore,
	backupKey string,
	expectedSize int64,
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
	if objInfo.Size != expectedSize {
		return nil, categorize(
			errVerificationCategory,
			fmt.Errorf("backup verification failed: uploaded object has %d bytes, streamed %d", objInfo.Size, expectedSize),
		)
	}

	return objInfo, nil
}
