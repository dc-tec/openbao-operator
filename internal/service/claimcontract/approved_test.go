// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package claimcontract

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	testAPIServerCIDR = "10.43.0.1/32"
	testSealKeyName   = "seal"
)

func TestBindApprovedServiceContract(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-v1"},
			ServiceParameters: &openbaov1alpha1.OpenBaoClusterClaimServiceParametersSpec{
				Backup: &openbaov1alpha1.OpenBaoClusterClaimBackupServiceParametersSpec{
					Location:  "payments-prod",
					Partition: "finance",
				},
			},
		},
	}
	readReplicas := int32(1)
	preUpgradeSnapshot := true
	serviceProfile := &openbaov1alpha1.OpenBaoServiceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-ha-v1", UID: types.UID("service-profile-uid")},
		Spec: openbaov1alpha1.OpenBaoServiceProfileSpec{
			Cluster: openbaov1alpha1.OpenBaoServiceProfileClusterSpec{
				Version:         "2.6.0",
				Voters:          3,
				ReadReplicas:    &readReplicas,
				SecurityProfile: openbaov1alpha1.ProfileHardened,
			},
			Storage: openbaov1alpha1.OpenBaoServiceProfileStorageSpec{
				PrimarySize:     "20Gi",
				ReadReplicaSize: "20Gi",
			},
			Bootstrap: openbaov1alpha1.OpenBaoServiceProfileBootstrapSpec{
				Mode:       openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
				ProfileRef: &openbaov1alpha1.LocalReference{Name: "oidc-standard-users-v1"},
			},
			Exposure: openbaov1alpha1.OpenBaoServiceProfileExposureSpec{
				ClassRef: openbaov1alpha1.LocalReference{Name: "internal-tls-v1"},
			},
			Backup: openbaov1alpha1.OpenBaoServiceProfileBackupSpec{
				ProfileRef: openbaov1alpha1.LocalReference{Name: "standard-daily-v1"},
			},
			Lifecycle: openbaov1alpha1.OpenBaoServiceProfileLifecycleSpec{
				UpgradeStrategy:    openbaov1alpha1.UpdateStrategyBlueGreen,
				PreUpgradeSnapshot: &preUpgradeSnapshot,
			},
		},
	}
	bootstrapProfile := &openbaov1alpha1.OpenBaoBootstrapProfile{ObjectMeta: metav1.ObjectMeta{Name: "oidc-standard-users-v1", UID: types.UID("bootstrap-uid")}}
	exposureClass := &openbaov1alpha1.OpenBaoExposureClass{ObjectMeta: metav1.ObjectMeta{Name: "internal-tls-v1", UID: types.UID("exposure-uid")}}
	backupProfile := &openbaov1alpha1.OpenBaoBackupProfile{ObjectMeta: metav1.ObjectMeta{Name: "standard-daily-v1", UID: types.UID("backup-uid")}}

	contract, result := BindApprovedServiceContract(claim, &CatalogBundle{
		ServiceProfile:   serviceProfile,
		BootstrapProfile: bootstrapProfile,
		ExposureClass:    exposureClass,
		BackupProfile:    backupProfile,
	})
	if !result.Valid {
		t.Fatalf("BindApprovedServiceContract() = %#v, want valid", result)
	}
	if contract == nil {
		t.Fatal("BindApprovedServiceContract() returned nil contract")
	}
	if contract.Cluster.Version != "2.6.0" || contract.Cluster.ReadReplicas != 1 || contract.Cluster.SecurityProfile != openbaov1alpha1.ProfileHardened {
		t.Fatalf("unexpected cluster contract: %#v", contract.Cluster)
	}
	if contract.Unseal.Mode != UnsealPostureModeExternal {
		t.Fatalf("unexpected unseal contract: %#v", contract.Unseal)
	}
	if contract.Bootstrap.ProfileRef == nil || contract.Bootstrap.ProfileRef.Name != "oidc-standard-users-v1" {
		t.Fatalf("unexpected bootstrap contract: %#v", contract.Bootstrap)
	}
	if contract.Exposure.ClassRef.Name != "internal-tls-v1" {
		t.Fatalf("unexpected exposure contract: %#v", contract.Exposure)
	}
	if contract.Backup.ProfileRef.Name != "standard-daily-v1" || contract.Backup.Parameters.Location != "payments-prod" || contract.Backup.Parameters.Partition != "finance" {
		t.Fatalf("unexpected backup contract: %#v", contract.Backup)
	}
	if contract.Lifecycle.UpgradeStrategy != openbaov1alpha1.UpdateStrategyBlueGreen || !contract.Lifecycle.PreUpgradeSnapshot {
		t.Fatalf("unexpected lifecycle contract: %#v", contract.Lifecycle)
	}

	applied := AppliedStatus(contract)
	if applied.ServiceProfileRef == nil || applied.ServiceProfileRef.UID != "service-profile-uid" {
		t.Fatalf("unexpected applied service profile ref: %#v", applied.ServiceProfileRef)
	}
	if applied.BootstrapProfileRef == nil || applied.BootstrapProfileRef.UID != "bootstrap-uid" {
		t.Fatalf("unexpected applied bootstrap profile ref: %#v", applied.BootstrapProfileRef)
	}
	if applied.ExposureClassRef == nil || applied.ExposureClassRef.UID != "exposure-uid" {
		t.Fatalf("unexpected applied exposure class ref: %#v", applied.ExposureClassRef)
	}
	if applied.BackupProfileRef == nil || applied.BackupProfileRef.UID != "backup-uid" {
		t.Fatalf("unexpected applied backup profile ref: %#v", applied.BackupProfileRef)
	}
}

func TestValidateContinuity(t *testing.T) {
	t.Parallel()

	contract := &ApprovedServiceContract{
		Provenance: ApprovedServiceProvenance{
			ServiceProfileRef: openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{Name: "standard-ha-v1", UID: "uid-new"},
		},
	}
	applied := openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{
		ServiceProfileRef: &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{Name: "standard-ha-v1", UID: "uid-old"},
	}

	result := ValidateContinuity(applied, contract)
	if result.Valid {
		t.Fatalf("ValidateContinuity() = %#v, want invalid", result)
	}
	if result.Reason != openbaov1alpha1.ReasonInvalid {
		t.Fatalf("ValidateContinuity() reason = %q, want %q", result.Reason, openbaov1alpha1.ReasonInvalid)
	}

	applied.ServiceProfileRef.Name = "standard-ha-v0"
	result = ValidateContinuity(applied, contract)
	if !result.Valid {
		t.Fatalf("ValidateContinuity() = %#v, want valid when revision name changes", result)
	}
}

func TestBindApprovedServiceContractDevelopmentDefaultsToManagedStaticUnseal(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-dev-v1"},
		},
	}
	serviceProfile := &openbaov1alpha1.OpenBaoServiceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-dev-v1", UID: types.UID("service-profile-uid")},
		Spec: openbaov1alpha1.OpenBaoServiceProfileSpec{
			Cluster: openbaov1alpha1.OpenBaoServiceProfileClusterSpec{
				Version:         "2.6.0",
				Voters:          1,
				SecurityProfile: openbaov1alpha1.ProfileDevelopment,
			},
			Storage: openbaov1alpha1.OpenBaoServiceProfileStorageSpec{
				PrimarySize: "10Gi",
			},
			Bootstrap: openbaov1alpha1.OpenBaoServiceProfileBootstrapSpec{
				Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
			},
			Exposure: openbaov1alpha1.OpenBaoServiceProfileExposureSpec{
				ClassRef: openbaov1alpha1.LocalReference{Name: "cluster-internal-v1"},
			},
			Backup: openbaov1alpha1.OpenBaoServiceProfileBackupSpec{
				ProfileRef: openbaov1alpha1.LocalReference{Name: "backup-disabled-v1"},
			},
		},
	}
	exposureClass := &openbaov1alpha1.OpenBaoExposureClass{ObjectMeta: metav1.ObjectMeta{Name: "cluster-internal-v1", UID: types.UID("exposure-uid")}}
	backupProfile := &openbaov1alpha1.OpenBaoBackupProfile{ObjectMeta: metav1.ObjectMeta{Name: "backup-disabled-v1", UID: types.UID("backup-uid")}}

	contract, result := BindApprovedServiceContract(claim, &CatalogBundle{
		ServiceProfile: serviceProfile,
		ExposureClass:  exposureClass,
		BackupProfile:  backupProfile,
	})
	if !result.Valid {
		t.Fatalf("BindApprovedServiceContract() = %#v, want valid", result)
	}
	if contract == nil {
		t.Fatal("BindApprovedServiceContract() returned nil contract")
	}
	if contract.Unseal.Mode != UnsealPostureModeManagedStatic {
		t.Fatalf("unexpected unseal contract: %#v", contract.Unseal)
	}
}

func TestBindApprovedServiceContractBindsImplementationProfiles(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-v1"},
		},
	}
	serviceProfile := &openbaov1alpha1.OpenBaoServiceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-ha-v1", UID: types.UID("service-profile-uid")},
		Spec: openbaov1alpha1.OpenBaoServiceProfileSpec{
			Cluster: openbaov1alpha1.OpenBaoServiceProfileClusterSpec{
				Version:         "2.6.0",
				Voters:          3,
				SecurityProfile: openbaov1alpha1.ProfileHardened,
			},
			Storage: openbaov1alpha1.OpenBaoServiceProfileStorageSpec{
				PrimarySize: "20Gi",
				ProfileRef:  &openbaov1alpha1.LocalReference{Name: "prod-storage-v1"},
			},
			Bootstrap: openbaov1alpha1.OpenBaoServiceProfileBootstrapSpec{
				Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
			},
			Exposure: openbaov1alpha1.OpenBaoServiceProfileExposureSpec{
				ClassRef: openbaov1alpha1.LocalReference{Name: "acme-v1"},
			},
			Unseal: &openbaov1alpha1.OpenBaoServiceProfileUnsealSpec{
				ProfileRef: &openbaov1alpha1.LocalReference{Name: "kms-v1"},
			},
			Runtime: &openbaov1alpha1.OpenBaoServiceProfileRuntimeSpec{
				ProfileRef: &openbaov1alpha1.LocalReference{Name: "runtime-v1"},
			},
			Observability: &openbaov1alpha1.OpenBaoServiceProfileObservabilitySpec{
				ProfileRef: &openbaov1alpha1.LocalReference{Name: "metrics-v1"},
			},
			Network: &openbaov1alpha1.OpenBaoServiceProfileNetworkSpec{
				ProfileRef: &openbaov1alpha1.LocalReference{Name: "network-v1"},
			},
			Backup: openbaov1alpha1.OpenBaoServiceProfileBackupSpec{
				ProfileRef: openbaov1alpha1.LocalReference{Name: "backup-v1"},
			},
			Lifecycle: openbaov1alpha1.OpenBaoServiceProfileLifecycleSpec{
				PolicyRef:       &openbaov1alpha1.LocalReference{Name: "upgrade-v1"},
				UpgradeStrategy: openbaov1alpha1.UpdateStrategyBlueGreen,
			},
		},
	}
	primaryClass := "fast-rwo"
	catalog := &CatalogBundle{
		ServiceProfile: serviceProfile,
		ExposureClass:  &openbaov1alpha1.OpenBaoExposureClass{ObjectMeta: metav1.ObjectMeta{Name: "acme-v1", UID: types.UID("exposure-uid")}},
		StorageProfile: &openbaov1alpha1.OpenBaoStorageProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "prod-storage-v1", UID: types.UID("storage-uid")},
			Spec: openbaov1alpha1.OpenBaoStorageProfileSpec{
				Primary: &openbaov1alpha1.OpenBaoStorageProfileVolumeSpec{StorageClassName: &primaryClass},
				ReadReplica: &openbaov1alpha1.OpenBaoStorageProfileReadReplicaSpec{
					UsePrimaryStorageClass: ptr.To(false),
				},
			},
		},
		UnsealProfile: &openbaov1alpha1.OpenBaoUnsealProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "kms-v1", UID: types.UID("unseal-uid")},
			Spec: openbaov1alpha1.OpenBaoUnsealProfileSpec{
				Mode: openbaov1alpha1.OpenBaoUnsealProfileModeGCPCloudKMS,
				GCPCloudKMS: &openbaov1alpha1.GCPCloudKMSSealConfig{
					Project:   "platform-prod",
					Region:    "europe-west1",
					KeyRing:   "openbao",
					CryptoKey: testSealKeyName,
				},
				CredentialsSecretRef: &corev1.LocalObjectReference{Name: "gcp-kms-creds"},
			},
		},
		RuntimeProfile: &openbaov1alpha1.OpenBaoRuntimeProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "runtime-v1", UID: types.UID("runtime-uid")},
			Spec: openbaov1alpha1.OpenBaoRuntimeProfileSpec{
				ServiceAccount: &openbaov1alpha1.ServiceAccountConfig{Name: "bao-main"},
				ImagePullSecrets: []corev1.LocalObjectReference{
					{Name: "registry-credentials"},
				},
			},
		},
		ObservabilityProfile: &openbaov1alpha1.OpenBaoObservabilityProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "metrics-v1", UID: types.UID("observability-uid")},
			Spec: openbaov1alpha1.OpenBaoObservabilityProfileSpec{
				Observability: &openbaov1alpha1.ObservabilityConfig{
					Metrics: &openbaov1alpha1.MetricsConfig{Enabled: true},
				},
			},
		},
		NetworkProfile: &openbaov1alpha1.OpenBaoNetworkProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "network-v1", UID: types.UID("network-uid")},
			Spec: openbaov1alpha1.OpenBaoNetworkProfileSpec{
				APIServerCIDR: testAPIServerCIDR,
			},
		},
		UpgradePolicy: &openbaov1alpha1.OpenBaoUpgradePolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "upgrade-v1", UID: types.UID("upgrade-uid")},
			Spec: openbaov1alpha1.OpenBaoUpgradePolicySpec{
				BlueGreen: &openbaov1alpha1.OpenBaoUpgradePolicyBlueGreenSpec{
					MinSyncDuration: "2m",
				},
			},
		},
		BackupProfile: &openbaov1alpha1.OpenBaoBackupProfile{ObjectMeta: metav1.ObjectMeta{Name: "backup-v1", UID: types.UID("backup-uid")}},
	}

	contract, result := BindApprovedServiceContract(claim, catalog)
	if !result.Valid {
		t.Fatalf("BindApprovedServiceContract() = %#v, want valid", result)
	}
	assertImplementationProfileContract(t, contract, primaryClass)
	assertImplementationProfileAppliedRefs(t, AppliedStatus(contract))
}

func assertImplementationProfileContract(t *testing.T, contract *ApprovedServiceContract, primaryClass string) {
	t.Helper()

	if contract.Storage.PrimaryClassName == nil || *contract.Storage.PrimaryClassName != primaryClass {
		t.Fatalf("storage contract = %#v, want primary storage class", contract.Storage)
	}
	if contract.Storage.ReadReplicaClassName != nil {
		t.Fatalf("readReplicaClassName = %#v, want nil when inheritance disabled", contract.Storage.ReadReplicaClassName)
	}
	if contract.Unseal.Config == nil ||
		contract.Unseal.Config.Type != unsealTypeGCPCloudKMS ||
		contract.Unseal.Config.GCPCloudKMS == nil ||
		contract.Unseal.Config.CredentialsSecretRef == nil ||
		contract.Unseal.Config.CredentialsSecretRef.Name != "gcp-kms-creds" {
		t.Fatalf("unseal contract = %#v, want GCP Cloud KMS profile", contract.Unseal)
	}
	if contract.Runtime.ServiceAccount == nil ||
		contract.Runtime.ServiceAccount.Name != "bao-main" ||
		len(contract.Runtime.ImagePullSecrets) != 1 ||
		contract.Runtime.ImagePullSecrets[0].Name != "registry-credentials" {
		t.Fatalf("runtime contract = %#v, want runtime profile", contract.Runtime)
	}
	if contract.Observability.Observability == nil ||
		contract.Observability.Observability.Metrics == nil ||
		!contract.Observability.Observability.Metrics.Enabled {
		t.Fatalf("observability contract = %#v, want metrics profile", contract.Observability)
	}
	if contract.Network.APIServerCIDR != testAPIServerCIDR {
		t.Fatalf("network contract = %#v, want network profile", contract.Network)
	}
	if contract.Lifecycle.PolicyRef == nil ||
		contract.Lifecycle.PolicyRef.Name != "upgrade-v1" ||
		contract.Lifecycle.BlueGreen == nil ||
		contract.Lifecycle.BlueGreen.Verification == nil ||
		contract.Lifecycle.BlueGreen.Verification.MinSyncDuration != "2m" {
		t.Fatalf("lifecycle contract = %#v, want upgrade policy", contract.Lifecycle)
	}
}

func assertImplementationProfileAppliedRefs(t *testing.T, applied openbaov1alpha1.OpenBaoClusterClaimAppliedStatus) {
	t.Helper()

	if applied.StorageProfileRef == nil || applied.StorageProfileRef.UID != "storage-uid" {
		t.Fatalf("storage applied ref = %#v, want storage uid", applied.StorageProfileRef)
	}
	if applied.UnsealProfileRef == nil || applied.UnsealProfileRef.UID != "unseal-uid" {
		t.Fatalf("unseal applied ref = %#v, want unseal uid", applied.UnsealProfileRef)
	}
	if applied.RuntimeProfileRef == nil || applied.RuntimeProfileRef.UID != "runtime-uid" {
		t.Fatalf("runtime applied ref = %#v, want runtime uid", applied.RuntimeProfileRef)
	}
	if applied.ObservabilityProfileRef == nil || applied.ObservabilityProfileRef.UID != "observability-uid" {
		t.Fatalf("observability applied ref = %#v, want observability uid", applied.ObservabilityProfileRef)
	}
	if applied.NetworkProfileRef == nil || applied.NetworkProfileRef.UID != "network-uid" {
		t.Fatalf("network applied ref = %#v, want network uid", applied.NetworkProfileRef)
	}
	if applied.UpgradePolicyRef == nil || applied.UpgradePolicyRef.UID != "upgrade-uid" {
		t.Fatalf("upgrade policy applied ref = %#v, want upgrade uid", applied.UpgradePolicyRef)
	}
}

func TestBindApprovedServiceContractBindsExternalUnsealProfileModes(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name         string
		profile      openbaov1alpha1.OpenBaoUnsealProfileSpec
		wantType     string
		assertConfig func(t *testing.T, config *openbaov1alpha1.UnsealConfig)
	}{
		{
			name: "transit",
			profile: openbaov1alpha1.OpenBaoUnsealProfileSpec{
				Mode: openbaov1alpha1.OpenBaoUnsealProfileModeTransit,
				Transit: &openbaov1alpha1.TransitSealConfig{
					Address:   "https://transit.example.internal:8200",
					KeyName:   "openbao",
					MountPath: "transit",
				},
			},
			wantType: unsealTypeTransit,
			assertConfig: func(t *testing.T, config *openbaov1alpha1.UnsealConfig) {
				t.Helper()
				if config.Transit == nil || config.Transit.KeyName != "openbao" {
					t.Fatalf("transit config = %#v, want projected config", config.Transit)
				}
			},
		},
		{
			name: "aws kms",
			profile: openbaov1alpha1.OpenBaoUnsealProfileSpec{
				Mode:   openbaov1alpha1.OpenBaoUnsealProfileModeAWSKMS,
				AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{Region: "eu-west-1", KMSKeyID: "alias/openbao"},
			},
			wantType: unsealTypeAWSKMS,
			assertConfig: func(t *testing.T, config *openbaov1alpha1.UnsealConfig) {
				t.Helper()
				if config.AWSKMS == nil || config.AWSKMS.KMSKeyID != "alias/openbao" {
					t.Fatalf("awskms config = %#v, want projected config", config.AWSKMS)
				}
			},
		},
		{
			name: "gcp cloud kms",
			profile: openbaov1alpha1.OpenBaoUnsealProfileSpec{
				Mode:        openbaov1alpha1.OpenBaoUnsealProfileModeGCPCloudKMS,
				GCPCloudKMS: &openbaov1alpha1.GCPCloudKMSSealConfig{Project: "platform", Region: "europe-west1", KeyRing: "openbao", CryptoKey: testSealKeyName},
			},
			wantType: unsealTypeGCPCloudKMS,
			assertConfig: func(t *testing.T, config *openbaov1alpha1.UnsealConfig) {
				t.Helper()
				if config.GCPCloudKMS == nil || config.GCPCloudKMS.CryptoKey != testSealKeyName {
					t.Fatalf("gcpckms config = %#v, want projected config", config.GCPCloudKMS)
				}
			},
		},
		{
			name: "azure key vault",
			profile: openbaov1alpha1.OpenBaoUnsealProfileSpec{
				Mode:          openbaov1alpha1.OpenBaoUnsealProfileModeAzureKeyVault,
				AzureKeyVault: &openbaov1alpha1.AzureKeyVaultSealConfig{VaultName: "openbao", KeyName: testSealKeyName},
			},
			wantType: unsealTypeAzureKeyVault,
			assertConfig: func(t *testing.T, config *openbaov1alpha1.UnsealConfig) {
				t.Helper()
				if config.AzureKeyVault == nil || config.AzureKeyVault.KeyName != testSealKeyName {
					t.Fatalf("azure key vault config = %#v, want projected config", config.AzureKeyVault)
				}
			},
		},
		{
			name: "oci kms",
			profile: openbaov1alpha1.OpenBaoUnsealProfileSpec{
				Mode:   openbaov1alpha1.OpenBaoUnsealProfileModeOCIKMS,
				OCIKMS: &openbaov1alpha1.OCIKMSSealConfig{KeyID: "ocid1.key.oc1..example", CryptoEndpoint: "https://crypto.example", ManagementEndpoint: "https://management.example"},
			},
			wantType: unsealTypeOCIKMS,
			assertConfig: func(t *testing.T, config *openbaov1alpha1.UnsealConfig) {
				t.Helper()
				if config.OCIKMS == nil || config.OCIKMS.KeyID != "ocid1.key.oc1..example" {
					t.Fatalf("ocikms config = %#v, want projected config", config.OCIKMS)
				}
			},
		},
		{
			name: "kmip",
			profile: openbaov1alpha1.OpenBaoUnsealProfileSpec{
				Mode: openbaov1alpha1.OpenBaoUnsealProfileModeKMIP,
				KMIP: &openbaov1alpha1.KMIPSealConfig{
					Endpoint:   "kmip.example.internal:5696",
					KMSKeyID:   "openbao-key",
					ClientCert: "/bao/kmip/tls.crt",
					ClientKey:  "/bao/kmip/tls.key",
				},
			},
			wantType: unsealTypeKMIP,
			assertConfig: func(t *testing.T, config *openbaov1alpha1.UnsealConfig) {
				t.Helper()
				if config.KMIP == nil || config.KMIP.KMSKeyID != "openbao-key" {
					t.Fatalf("kmip config = %#v, want projected config", config.KMIP)
				}
			},
		},
		{
			name: "pkcs11",
			profile: openbaov1alpha1.OpenBaoUnsealProfileSpec{
				Mode: openbaov1alpha1.OpenBaoUnsealProfileModePKCS11,
				PKCS11: &openbaov1alpha1.PKCS11SealConfig{
					Lib:        "/usr/lib/softhsm/libsofthsm2.so",
					TokenLabel: "openbao",
					KeyLabel:   testSealKeyName,
				},
			},
			wantType: unsealTypePKCS11,
			assertConfig: func(t *testing.T, config *openbaov1alpha1.UnsealConfig) {
				t.Helper()
				if config.PKCS11 == nil || config.PKCS11.KeyLabel != testSealKeyName {
					t.Fatalf("pkcs11 config = %#v, want projected config", config.PKCS11)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			claim := validRenderedPrimaryClaimFixture()
			catalog := validRenderedPrimaryCatalogBundleFixture()
			catalog.ServiceProfile.Spec.Unseal = &openbaov1alpha1.OpenBaoServiceProfileUnsealSpec{
				ProfileRef: &openbaov1alpha1.LocalReference{Name: "unseal-v1"},
			}
			catalog.UnsealProfile = &openbaov1alpha1.OpenBaoUnsealProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "unseal-v1"},
				Spec:       tt.profile,
			}

			contract, result := BindApprovedServiceContract(claim, catalog)
			if !result.Valid {
				t.Fatalf("BindApprovedServiceContract() = %#v, want valid", result)
			}
			if contract.Unseal.Config == nil || contract.Unseal.Config.Type != tt.wantType {
				t.Fatalf("unseal config = %#v, want type %q", contract.Unseal.Config, tt.wantType)
			}
			tt.assertConfig(t, contract.Unseal.Config)
		})
	}
}

func TestBindApprovedServiceContractRejectsHardenedStaticUnsealProfile(t *testing.T) {
	t.Parallel()

	claim := validRenderedPrimaryClaimFixture()
	catalog := validRenderedPrimaryCatalogBundleFixture()
	catalog.ServiceProfile.Spec.Unseal = &openbaov1alpha1.OpenBaoServiceProfileUnsealSpec{
		ProfileRef: &openbaov1alpha1.LocalReference{Name: "static-v1"},
	}
	catalog.UnsealProfile = &openbaov1alpha1.OpenBaoUnsealProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "static-v1"},
		Spec: openbaov1alpha1.OpenBaoUnsealProfileSpec{
			Mode: openbaov1alpha1.OpenBaoUnsealProfileModeOperatorManagedStatic,
		},
	}

	contract, result := BindApprovedServiceContract(claim, catalog)
	if result.Valid {
		t.Fatalf("BindApprovedServiceContract() = %#v, want invalid", result)
	}
	if contract != nil {
		t.Fatalf("contract = %#v, want nil", contract)
	}
}
