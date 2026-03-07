package backup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/storage"
)

func TestConfigureAzureStorageConfig_UsesManagedIdentityWithoutStaticCredentials(t *testing.T) {
	storageConfig := &storage.Config{
		Bucket: "backups",
	}

	configureAzureStorageConfig(storageConfig, openbaov1alpha1.BackupTarget{
		Provider: "azure",
		Bucket:   "backups",
		Azure: &openbaov1alpha1.AzureTargetConfig{
			StorageAccount: "storageaccount",
		},
	}, nil)

	require.NotNil(t, storageConfig.Azure)
	assert.True(t, storageConfig.Azure.UseManagedIdentity)
	assert.Equal(t, "storageaccount", storageConfig.Azure.StorageAccount)
}

func TestConfigureAzureStorageConfig_PrefersStaticCredentialsWhenProvided(t *testing.T) {
	storageConfig := &storage.Config{
		Bucket: "backups",
	}
	secret := &corev1.Secret{
		Data: map[string][]byte{
			"accountKey": []byte("secret"),
		},
	}

	configureAzureStorageConfig(storageConfig, openbaov1alpha1.BackupTarget{
		Provider: "azure",
		Bucket:   "backups",
		Azure: &openbaov1alpha1.AzureTargetConfig{
			StorageAccount: "storageaccount",
		},
	}, secret)

	require.NotNil(t, storageConfig.Azure)
	assert.False(t, storageConfig.Azure.UseManagedIdentity)
	assert.Equal(t, "secret", storageConfig.Azure.AccountKey)
}
