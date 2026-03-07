package backup

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/storage"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
)

var openBlobStoreFn = storage.OpenBlobStore

func (m *Manager) openBackupStorageClient(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, ensureExists bool) (blobstore.BlobStore, error) {
	if cluster == nil || cluster.Spec.Backup == nil {
		return nil, fmt.Errorf("cluster backup configuration is required")
	}

	target := cluster.Spec.Backup.Target
	provider := storage.ProviderType(target.Provider)
	if provider == "" {
		provider = storage.ProviderS3
	}

	storageConfig := storage.Config{
		Provider:     provider,
		Bucket:       target.Bucket,
		Endpoint:     target.Endpoint,
		EnsureExists: ensureExists,
	}

	credsSecret, err := m.loadBackupCredentialsSecret(ctx, cluster)
	if err != nil {
		return nil, err
	}

	switch provider {
	case storage.ProviderS3:
		configureS3StorageConfig(&storageConfig, target, credsSecret, ensureExists)
	case storage.ProviderGCS:
		configureGCSStorageConfig(&storageConfig, target, credsSecret)
	case storage.ProviderAzure:
		configureAzureStorageConfig(&storageConfig, target, credsSecret)
	default:
		return nil, fmt.Errorf("unsupported backup storage provider %q", target.Provider)
	}

	return openBlobStoreFn(ctx, storageConfig)
}

func (m *Manager) loadBackupCredentialsSecret(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (*corev1.Secret, error) {
	target := cluster.Spec.Backup.Target
	if target.CredentialsSecretRef == nil || target.CredentialsSecretRef.Name == "" {
		return nil, nil
	}

	secretName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      target.CredentialsSecretRef.Name,
	}
	credsSecret := &corev1.Secret{}
	if err := m.client.Get(ctx, secretName, credsSecret); err != nil {
		return nil, fmt.Errorf("failed to load storage credentials secret %s/%s: %w", secretName.Namespace, secretName.Name, err)
	}

	return credsSecret, nil
}

func configureS3StorageConfig(storageConfig *storage.Config, target openbaov1alpha1.BackupTarget, credsSecret *corev1.Secret, ensureExists bool) {
	region := target.Region
	if region == "" {
		region = constants.DefaultS3Region
	}

	credentials := &blobstore.Credentials{Region: region}
	if credsSecret != nil {
		if v, ok := credsSecret.Data[blobstore.SecretKeyAccessKeyID]; ok {
			credentials.AccessKeyID = strings.TrimSpace(string(v))
		}
		if v, ok := credsSecret.Data[blobstore.SecretKeySecretAccessKey]; ok {
			credentials.SecretAccessKey = strings.TrimSpace(string(v))
		}
		if v, ok := credsSecret.Data[blobstore.SecretKeySessionToken]; ok {
			credentials.SessionToken = strings.TrimSpace(string(v))
		}
		if v, ok := credsSecret.Data[blobstore.SecretKeyRegion]; ok && strings.TrimSpace(string(v)) != "" {
			credentials.Region = strings.TrimSpace(string(v))
		}
		if v, ok := credsSecret.Data[blobstore.SecretKeyCACert]; ok {
			credentials.CACert = v
		}
	}

	storageConfig.Region = credentials.Region
	storageConfig.Credentials = credentials
	storageConfig.S3 = &storage.S3Options{
		UsePathStyle:       target.UsePathStyle,
		InsecureSkipVerify: target.InsecureSkipVerify,
		EnsureExists:       ensureExists,
	}
}

func configureGCSStorageConfig(storageConfig *storage.Config, target openbaov1alpha1.BackupTarget, credsSecret *corev1.Secret) {
	gcsOptions := &storage.GCSOptions{
		InsecureSkipVerify: target.InsecureSkipVerify,
	}
	if target.GCS != nil {
		gcsOptions.Project = target.GCS.Project
	}
	if credsSecret != nil {
		if v, ok := credsSecret.Data["credentials.json"]; ok {
			gcsOptions.CredentialsJSON = v
		}
		if v, ok := credsSecret.Data[blobstore.SecretKeyCACert]; ok {
			gcsOptions.CACert = v
		}
	}

	endpointLower := strings.ToLower(target.Endpoint)
	if strings.Contains(endpointLower, "fake-gcs-server") || strings.HasPrefix(endpointLower, "http://") {
		gcsOptions.UseEmulator = true
	}
	if strings.HasPrefix(endpointLower, "http://") {
		gcsOptions.InsecureSkipVerify = true
	}

	storageConfig.GCS = gcsOptions
}

func configureAzureStorageConfig(storageConfig *storage.Config, target openbaov1alpha1.BackupTarget, credsSecret *corev1.Secret) {
	azureOptions := &storage.AzureOptions{
		InsecureSkipVerify: target.InsecureSkipVerify,
	}
	if target.Azure != nil {
		azureOptions.StorageAccount = target.Azure.StorageAccount
		if target.Azure.Container != "" {
			storageConfig.Bucket = target.Azure.Container
		}
	}
	if credsSecret != nil {
		if v, ok := credsSecret.Data["accountKey"]; ok {
			azureOptions.AccountKey = strings.TrimSpace(string(v))
		}
		if v, ok := credsSecret.Data["connectionString"]; ok {
			azureOptions.ConnectionString = strings.TrimSpace(string(v))
		}
		if v, ok := credsSecret.Data[blobstore.SecretKeyCACert]; ok {
			azureOptions.CACert = v
		}
	}
	if azureOptions.AccountKey == "" && azureOptions.ConnectionString == "" {
		azureOptions.UseManagedIdentity = true
	}

	storageConfig.Azure = azureOptions
}
