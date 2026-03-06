package backup

import (
	"context"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/dc-tec/openbao-operator/internal/adapter/storage"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

type fakeBlobStore struct {
	objects    []blobstore.ObjectInfo
	deleted    []string
	closeCount int
}

func (f *fakeBlobStore) Upload(_ context.Context, _ string, _ io.Reader) error {
	return nil
}

func (f *fakeBlobStore) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, nil
}

func (f *fakeBlobStore) Delete(_ context.Context, _ string) error {
	return nil
}

func (f *fakeBlobStore) DeleteBatch(_ context.Context, keys []string) error {
	f.deleted = append(f.deleted, keys...)
	return nil
}

func (f *fakeBlobStore) List(_ context.Context, _ string) ([]blobstore.ObjectInfo, error) {
	return f.objects, nil
}

func (f *fakeBlobStore) Head(_ context.Context, _ string) (*blobstore.ObjectInfo, error) {
	return nil, nil
}

func (f *fakeBlobStore) Close() error {
	f.closeCount++
	return nil
}

func TestApplyRetention_UsesProviderAndDeletesOldBackups(t *testing.T) {
	testCases := []struct {
		name              string
		provider          string
		credentialsData   map[string][]byte
		targetMutator     func(*openbaov1alpha1.BackupTarget)
		validateConfig    func(*testing.T, storage.Config)
		expectedProvider  storage.ProviderType
		expectedBucket    string
		expectedDeleteLen int
	}{
		{
			name:     "s3 retention cleanup",
			provider: constants.StorageProviderS3,
			credentialsData: map[string][]byte{
				blobstore.SecretKeyAccessKeyID:     []byte("test-ak"),
				blobstore.SecretKeySecretAccessKey: []byte("test-sk"),
			},
			targetMutator: func(target *openbaov1alpha1.BackupTarget) {
				target.Region = "us-east-1"
				target.UsePathStyle = true
				target.InsecureSkipVerify = true
			},
			validateConfig: func(t *testing.T, cfg storage.Config) {
				t.Helper()
				if cfg.Region != "us-east-1" {
					t.Fatalf("storage region = %q, want %q", cfg.Region, "us-east-1")
				}
				if cfg.S3 == nil {
					t.Fatal("expected S3 config to be set")
				}
				if !cfg.S3.UsePathStyle {
					t.Fatal("expected S3 UsePathStyle to be true")
				}
				if !cfg.S3.InsecureSkipVerify {
					t.Fatal("expected S3 InsecureSkipVerify to be true")
				}
				if cfg.S3.EnsureExists {
					t.Fatal("expected S3 EnsureExists to be false")
				}
			},
			expectedProvider:  storage.ProviderS3,
			expectedBucket:    "backups",
			expectedDeleteLen: 2,
		},
		{
			name:     "gcs retention cleanup",
			provider: constants.StorageProviderGCS,
			credentialsData: map[string][]byte{
				"credentials.json": []byte(`{"type":"service_account"}`),
			},
			targetMutator: func(target *openbaov1alpha1.BackupTarget) {
				target.GCS = &openbaov1alpha1.GCSTargetConfig{Project: "test-project"}
				target.Endpoint = "http://fake-gcs-server:4443"
			},
			validateConfig: func(t *testing.T, cfg storage.Config) {
				t.Helper()
				if cfg.GCS == nil {
					t.Fatal("expected GCS config to be set")
				}
				if !cfg.GCS.UseEmulator {
					t.Fatal("expected GCS UseEmulator to be true")
				}
				if !cfg.GCS.InsecureSkipVerify {
					t.Fatal("expected GCS InsecureSkipVerify to be true")
				}
			},
			expectedProvider:  storage.ProviderGCS,
			expectedBucket:    "backups",
			expectedDeleteLen: 2,
		},
		{
			name:     "azure retention cleanup uses container",
			provider: constants.StorageProviderAzure,
			credentialsData: map[string][]byte{
				"accountKey": []byte("test-account-key"),
			},
			targetMutator: func(target *openbaov1alpha1.BackupTarget) {
				target.Azure = &openbaov1alpha1.AzureTargetConfig{
					StorageAccount: "mystorage",
					Container:      "container-overwrite",
				}
				target.InsecureSkipVerify = true
			},
			validateConfig: func(t *testing.T, cfg storage.Config) {
				t.Helper()
				if cfg.Azure == nil {
					t.Fatal("expected Azure config to be set")
				}
				if cfg.Azure.StorageAccount != "mystorage" {
					t.Fatalf("azure storage account = %q, want %q", cfg.Azure.StorageAccount, "mystorage")
				}
				if !cfg.Azure.InsecureSkipVerify {
					t.Fatal("expected Azure InsecureSkipVerify to be true")
				}
			},
			expectedProvider:  storage.ProviderAzure,
			expectedBucket:    "container-overwrite",
			expectedDeleteLen: 2,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "retention-cluster",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{
						Target: openbaov1alpha1.BackupTarget{
							Provider:   tt.provider,
							Endpoint:   "https://example.com",
							Bucket:     "backups",
							PathPrefix: "clusters",
							CredentialsSecretRef: &corev1.LocalObjectReference{
								Name: "backup-creds",
							},
						},
						Retention: &openbaov1alpha1.BackupRetention{
							MaxCount: 1,
						},
					},
				},
			}
			if tt.targetMutator != nil {
				tt.targetMutator(&cluster.Spec.Backup.Target)
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backup-creds",
					Namespace: "default",
				},
				Data: tt.credentialsData,
			}

			client := fake.NewClientBuilder().
				WithScheme(testScheme).
				WithObjects(secret).
				Build()

			store := &fakeBlobStore{
				objects: []blobstore.ObjectInfo{
					{Key: "clusters/default/retention-cluster/2025-01-01T03-00-00Z-aaaaaaaa.snap", LastModified: time.Date(2025, 1, 1, 3, 0, 0, 0, time.UTC)},
					{Key: "clusters/default/retention-cluster/2025-01-02T03-00-00Z-bbbbbbbb.snap", LastModified: time.Date(2025, 1, 2, 3, 0, 0, 0, time.UTC)},
					{Key: "clusters/default/retention-cluster/2025-01-03T03-00-00Z-cccccccc.snap", LastModified: time.Date(2025, 1, 3, 3, 0, 0, 0, time.UTC)},
				},
			}

			var capturedConfig storage.Config
			originalOpenBlobStoreFn := openBlobStoreFn
			openBlobStoreFn = func(_ context.Context, cfg storage.Config) (blobstore.BlobStore, error) {
				capturedConfig = cfg
				return store, nil
			}
			defer func() {
				openBlobStoreFn = originalOpenBlobStoreFn
			}()

			manager := NewManager(client, testScheme, portopenbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), client, nil), "")
			if err := manager.applyRetention(context.Background(), logr.Discard(), cluster, NewMetrics(cluster.Namespace, cluster.Name)); err != nil {
				t.Fatalf("applyRetention() error = %v", err)
			}

			if capturedConfig.Provider != tt.expectedProvider {
				t.Fatalf("storage provider = %q, want %q", capturedConfig.Provider, tt.expectedProvider)
			}

			if capturedConfig.Bucket != tt.expectedBucket {
				t.Fatalf("storage bucket = %q, want %q", capturedConfig.Bucket, tt.expectedBucket)
			}
			if tt.validateConfig != nil {
				tt.validateConfig(t, capturedConfig)
			}

			if len(store.deleted) != tt.expectedDeleteLen {
				t.Fatalf("deleted backups = %d, want %d", len(store.deleted), tt.expectedDeleteLen)
			}

			if !slices.Contains(store.deleted, "clusters/default/retention-cluster/2025-01-01T03-00-00Z-aaaaaaaa.snap") {
				t.Fatalf("expected oldest backup to be deleted, deleted=%v", store.deleted)
			}

			if !slices.Contains(store.deleted, "clusters/default/retention-cluster/2025-01-02T03-00-00Z-bbbbbbbb.snap") {
				t.Fatalf("expected second oldest backup to be deleted, deleted=%v", store.deleted)
			}

			if slices.Contains(store.deleted, "clusters/default/retention-cluster/2025-01-03T03-00-00Z-cccccccc.snap") {
				t.Fatalf("did not expect newest backup to be deleted, deleted=%v", store.deleted)
			}
		})
	}
}
