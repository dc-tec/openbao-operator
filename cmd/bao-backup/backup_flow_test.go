package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	openbaotest "github.com/dc-tec/openbao-operator/internal/platform/testutil/openbao"
	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
)

type backupFlowBlobStore struct {
	object         []byte
	uploadErr      error
	uploadReadErr  error
	skipUploadRead bool
	headResult     *blobstore.ObjectInfo
	useHeadResult  bool
	deleted        []string
}

func (s *backupFlowBlobStore) Upload(_ context.Context, _ string, body io.Reader) error {
	if s.skipUploadRead {
		return s.uploadErr
	}
	data, err := io.ReadAll(body)
	s.object = append([]byte(nil), data...)
	s.uploadReadErr = err
	if s.uploadErr != nil {
		return s.uploadErr
	}
	return err
}

func (s *backupFlowBlobStore) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.object)), nil
}

func (s *backupFlowBlobStore) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	s.object = nil
	return nil
}

func (s *backupFlowBlobStore) DeleteBatch(_ context.Context, keys []string) error {
	for _, key := range keys {
		if err := s.Delete(context.Background(), key); err != nil {
			return err
		}
	}
	return nil
}

func (s *backupFlowBlobStore) List(_ context.Context, _ string) ([]blobstore.ObjectInfo, error) {
	return nil, nil
}

func (s *backupFlowBlobStore) Head(_ context.Context, key string) (*blobstore.ObjectInfo, error) {
	if s.useHeadResult {
		return s.headResult, nil
	}
	if s.object == nil {
		return nil, nil
	}
	return &blobstore.ObjectInfo{Key: key, Size: int64(len(s.object))}, nil
}

func (s *backupFlowBlobStore) Close() error { return nil }

func TestPublishBackupSnapshot_PartialSnapshotFailureDeletesObject(t *testing.T) {
	t.Parallel()

	snapshotErr := errors.New("snapshot stream failed")
	baoClient := &openbaotest.MockClusterActions{
		SnapshotFunc: func(_ context.Context, writer io.Writer) error {
			if _, err := writer.Write([]byte("partial-snapshot")); err != nil {
				return err
			}
			return snapshotErr
		},
	}
	store := &backupFlowBlobStore{}

	_, err := publishBackupSnapshot(context.Background(), baoClient, store, "backup.snap")

	require.ErrorIs(t, err, errSnapshotCategory)
	require.ErrorIs(t, err, snapshotErr)
	require.ErrorIs(t, store.uploadReadErr, snapshotErr)
	require.Equal(t, []string{"backup.snap"}, store.deleted)
	require.Nil(t, store.object)
}

func TestPublishBackupSnapshot_UploadFailureDeletesObject(t *testing.T) {
	t.Parallel()

	uploadErr := errors.New("upload failed")
	baoClient := &openbaotest.MockClusterActions{
		SnapshotFunc: func(_ context.Context, writer io.Writer) error {
			_, err := writer.Write([]byte("complete-snapshot"))
			return err
		},
	}
	store := &backupFlowBlobStore{
		object:         []byte("partial-upload"),
		uploadErr:      uploadErr,
		skipUploadRead: true,
	}

	_, err := publishBackupSnapshot(context.Background(), baoClient, store, "backup.snap")

	require.ErrorIs(t, err, errStorageCategory)
	require.ErrorIs(t, err, uploadErr)
	require.Equal(t, []string{"backup.snap"}, store.deleted)
	require.Nil(t, store.object)
}

func TestPublishBackupSnapshot_VerificationFailureDeletesObject(t *testing.T) {
	t.Parallel()

	baoClient := &openbaotest.MockClusterActions{
		SnapshotFunc: func(_ context.Context, writer io.Writer) error {
			_, err := writer.Write([]byte("complete-snapshot"))
			return err
		},
	}
	store := &backupFlowBlobStore{
		useHeadResult: true,
		headResult:    &blobstore.ObjectInfo{Key: "backup.snap", Size: 0},
	}

	_, err := publishBackupSnapshot(context.Background(), baoClient, store, "backup.snap")

	require.ErrorIs(t, err, errVerificationCategory)
	require.Equal(t, []string{"backup.snap"}, store.deleted)
	require.Nil(t, store.object)
}
