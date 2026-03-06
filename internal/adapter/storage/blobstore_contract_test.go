package storage

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"gocloud.dev/blob/memblob"

	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
)

type blobStoreFactory func(t *testing.T) blobstore.BlobStore

func runBlobStoreContract(t *testing.T, factory blobStoreFactory) {
	t.Helper()

	t.Run("upload_download_round_trip", func(t *testing.T) {
		store := factory(t)
		defer func() { _ = store.Close() }()

		ctx := context.Background()
		key := "contract/roundtrip"
		want := []byte("openbao-contract-data")

		if err := store.Upload(ctx, key, bytes.NewReader(want)); err != nil {
			t.Fatalf("Upload() error = %v", err)
		}

		r, err := store.Download(ctx, key)
		if err != nil {
			t.Fatalf("Download() error = %v", err)
		}
		defer func() { _ = r.Close() }()

		got, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}

		if !bytes.Equal(got, want) {
			t.Fatalf("downloaded payload = %q, want %q", got, want)
		}
	})

	t.Run("head_and_list_expose_metadata", func(t *testing.T) {
		store := factory(t)
		defer func() { _ = store.Close() }()

		ctx := context.Background()
		if err := store.Upload(ctx, "contract/list/b", bytes.NewReader([]byte("b"))); err != nil {
			t.Fatalf("Upload(b) error = %v", err)
		}
		if err := store.Upload(ctx, "contract/list/a", bytes.NewReader([]byte("aa"))); err != nil {
			t.Fatalf("Upload(a) error = %v", err)
		}

		info, err := store.Head(ctx, "contract/list/a")
		if err != nil {
			t.Fatalf("Head() error = %v", err)
		}
		if info == nil {
			t.Fatal("Head() returned nil metadata")
		}
		if info.Key != "contract/list/a" {
			t.Fatalf("Head().Key = %q, want %q", info.Key, "contract/list/a")
		}
		if info.Size != 2 {
			t.Fatalf("Head().Size = %d, want %d", info.Size, 2)
		}
		if info.LastModified.Equal(time.Time{}) {
			t.Fatal("Head().LastModified should be set")
		}

		objects, err := store.List(ctx, "contract/list/")
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(objects) != 2 {
			t.Fatalf("List() returned %d objects, want 2", len(objects))
		}
		if objects[0].Key != "contract/list/a" || objects[1].Key != "contract/list/b" {
			t.Fatalf("List() keys = [%s, %s], want sorted [contract/list/a, contract/list/b]", objects[0].Key, objects[1].Key)
		}
	})

	t.Run("delete_and_delete_batch_are_idempotent", func(t *testing.T) {
		store := factory(t)
		defer func() { _ = store.Close() }()

		ctx := context.Background()
		keys := []string{"contract/delete/one", "contract/delete/two"}
		for _, key := range keys {
			if err := store.Upload(ctx, key, bytes.NewReader([]byte(key))); err != nil {
				t.Fatalf("Upload(%s) error = %v", key, err)
			}
		}

		if err := store.Delete(ctx, keys[0]); err != nil {
			t.Fatalf("Delete(%s) error = %v", keys[0], err)
		}
		if err := store.Delete(ctx, keys[0]); err != nil {
			t.Fatalf("Delete(%s) second call error = %v", keys[0], err)
		}

		if err := store.DeleteBatch(ctx, keys); err != nil {
			t.Fatalf("DeleteBatch() error = %v", err)
		}
		if err := store.DeleteBatch(ctx, keys); err != nil {
			t.Fatalf("DeleteBatch() second call error = %v", err)
		}

		for _, key := range keys {
			info, err := store.Head(ctx, key)
			if err != nil {
				t.Fatalf("Head(%s) error = %v", key, err)
			}
			if info != nil {
				t.Fatalf("Head(%s) = %+v, want nil after delete", key, info)
			}
		}
	})
}

func TestBucketSatisfiesBlobStorePort(t *testing.T) {
	var _ blobstore.BlobStore = (*Bucket)(nil)
}

func TestBucketBlobStoreContract(t *testing.T) {
	runBlobStoreContract(t, func(t *testing.T) blobstore.BlobStore {
		t.Helper()
		return NewBucket(memblob.OpenBucket(nil))
	})
}
