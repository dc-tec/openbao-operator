package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/dc-tec/openbao-operator/internal/adapter/storage"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
	backupconfig "github.com/dc-tec/openbao-operator/internal/service/backup"
)

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
			EnsureExists:       true,
		}

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

func openStorageClient(ctx context.Context, cfg *backupconfig.ExecutorConfig) (blobstore.BlobStore, error) {
	storageConfig, err := buildStorageConfig(cfg)
	if err != nil {
		return nil, err
	}

	return storage.OpenBlobStore(ctx, storageConfig)
}
