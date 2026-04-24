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
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	unsealTypeStatic        = "static"
	unsealTypeTransit       = "transit"
	unsealTypeAWSKMS        = "awskms"
	unsealTypeGCPCloudKMS   = "gcpckms"
	unsealTypeAzureKeyVault = "azurekeyvault"
	unsealTypeOCIKMS        = "ocikms"
	unsealTypeKMIP          = "kmip"
	unsealTypePKCS11        = "pkcs11"
)

// CatalogBundle captures the immutable top-level catalog objects bound for one claim.
type CatalogBundle struct {
	ServiceProfile       *openbaov1alpha1.OpenBaoServiceProfile
	BootstrapProfile     *openbaov1alpha1.OpenBaoBootstrapProfile
	ExposureClass        *openbaov1alpha1.OpenBaoExposureClass
	StorageProfile       *openbaov1alpha1.OpenBaoStorageProfile
	UnsealProfile        *openbaov1alpha1.OpenBaoUnsealProfile
	RuntimeProfile       *openbaov1alpha1.OpenBaoRuntimeProfile
	ObservabilityProfile *openbaov1alpha1.OpenBaoObservabilityProfile
	NetworkProfile       *openbaov1alpha1.OpenBaoNetworkProfile
	UpgradePolicy        *openbaov1alpha1.OpenBaoUpgradePolicy
	Entrypoint           *openbaov1alpha1.OpenBaoEntrypoint
	IngressPolicy        *openbaov1alpha1.OpenBaoIngressPolicy
	BackupProfile        *openbaov1alpha1.OpenBaoBackupProfile
	BackupTarget         *openbaov1alpha1.OpenBaoBackupTarget
	BackupBackend        *openbaov1alpha1.OpenBaoBackupBackend
	BackupAuth           *openbaov1alpha1.OpenBaoBackupAuthProfile
	TransferProfile      *openbaov1alpha1.OpenBaoTransferProfile
}

// ValidationResult summarizes whether approved-contract production succeeded.
type ValidationResult struct {
	Valid   bool
	Reason  openbaov1alpha1.ConditionReason
	Message string
}

// ApprovedServiceContract captures service semantics after immutable catalog binding.
type ApprovedServiceContract struct {
	Cluster       ApprovedCluster
	Unseal        ApprovedUnseal
	Storage       ApprovedStorage
	Runtime       ApprovedRuntime
	Observability ApprovedObservability
	Network       ApprovedNetwork
	Bootstrap     ApprovedBootstrap
	Exposure      ApprovedExposure
	Backup        ApprovedBackup
	Lifecycle     ApprovedLifecycle
	Provenance    ApprovedServiceProvenance
}

// ApprovedCluster captures the approved core service shape.
type ApprovedCluster struct {
	Version         string
	Voters          int32
	ReadReplicas    int32
	SecurityProfile openbaov1alpha1.Profile
}

// UnsealPostureMode identifies the internal unseal posture required by a claim contract.
type UnsealPostureMode string

const (
	// UnsealPostureModeManagedStatic uses the operator-managed static seal posture.
	UnsealPostureModeManagedStatic UnsealPostureMode = "ManagedStatic"
	// UnsealPostureModeExternal requires an external non-static seal posture.
	UnsealPostureModeExternal UnsealPostureMode = "External"
)

// ApprovedUnseal captures the approved unseal posture derived from service semantics.
type ApprovedUnseal struct {
	Mode       UnsealPostureMode
	ProfileRef *openbaov1alpha1.LocalReference
	Config     *openbaov1alpha1.UnsealConfig
}

// ApprovedStorage captures approved storage capacity semantics.
type ApprovedStorage struct {
	PrimarySize          string
	ReadReplicaSize      string
	PrimaryClassName     *string
	ReadReplicaClassName *string
	ACMECache            *openbaov1alpha1.ACMESharedCacheConfig
	ProfileRef           *openbaov1alpha1.LocalReference
}

// ApprovedRuntime captures approved runtime integration semantics.
type ApprovedRuntime struct {
	ProfileRef                *openbaov1alpha1.LocalReference
	ServiceAccount            *openbaov1alpha1.ServiceAccountConfig
	PodMetadata               *openbaov1alpha1.PodMetadataConfig
	ImagePullSecrets          []corev1.LocalObjectReference
	ImageVerification         *openbaov1alpha1.ImageVerificationConfig
	OperatorImageVerification *openbaov1alpha1.ImageVerificationConfig
	WorkloadHardening         *openbaov1alpha1.WorkloadHardeningConfig
	SecurityContext           *corev1.PodSecurityContext
	HelperImages              *openbaov1alpha1.OpenBaoRuntimeProfileHelperImagesSpec
	ReadReplica               *openbaov1alpha1.OpenBaoRuntimeProfileReadReplicaSpec
}

// ApprovedObservability captures approved metrics and telemetry semantics.
type ApprovedObservability struct {
	ProfileRef    *openbaov1alpha1.LocalReference
	Observability *openbaov1alpha1.ObservabilityConfig
	Telemetry     *openbaov1alpha1.TelemetryConfig
}

// ApprovedNetwork captures approved network dependency semantics.
type ApprovedNetwork struct {
	ProfileRef           *openbaov1alpha1.LocalReference
	APIServerCIDR        string
	APIServerEndpointIPs []string
	DNSNamespace         string
	DNSEndpointIPs       []string
	EgressRules          []networkingv1.NetworkPolicyEgressRule
	IngressRules         []networkingv1.NetworkPolicyIngressRule
	TrustedIngressPeers  []networkingv1.NetworkPolicyPeer
}

// ApprovedBootstrap captures the approved bootstrap posture.
type ApprovedBootstrap struct {
	Mode       openbaov1alpha1.OpenBaoBootstrapMode
	ProfileRef *openbaov1alpha1.LocalReference
}

// ApprovedExposure captures the approved exposure posture.
type ApprovedExposure struct {
	ClassRef openbaov1alpha1.LocalReference
}

// ApprovedBackup captures the approved backup posture.
type ApprovedBackup struct {
	ProfileRef openbaov1alpha1.LocalReference
	Parameters ApprovedBackupParameters
}

// ApprovedBackupParameters captures the typed claim-facing backup parameter surface.
type ApprovedBackupParameters struct {
	Location  string
	Partition string
}

// ApprovedLifecycle captures approved steady-state lifecycle posture.
type ApprovedLifecycle struct {
	UpgradeStrategy    openbaov1alpha1.UpdateStrategyType
	PreUpgradeSnapshot bool
	PolicyRef          *openbaov1alpha1.LocalReference
	BlueGreen          *openbaov1alpha1.BlueGreenConfig
}

// ApprovedServiceProvenance captures the bound immutable catalog revisions.
type ApprovedServiceProvenance struct {
	ServiceProfileRef       openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	BootstrapProfileRef     *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	ExposureClassRef        *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	StorageProfileRef       *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	UnsealProfileRef        *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	RuntimeProfileRef       *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	ObservabilityProfileRef *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	NetworkProfileRef       *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	UpgradePolicyRef        *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	BackupProfileRef        *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
}

// BindApprovedServiceContract binds immutable catalog inputs into approved service semantics.
func BindApprovedServiceContract(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	catalog *CatalogBundle,
) (*ApprovedServiceContract, ValidationResult) {
	if validation := validateApprovedCatalogInputs(claim, catalog); !validation.Valid {
		return nil, validation
	}
	serviceProfile := catalog.ServiceProfile
	exposureClass := catalog.ExposureClass
	backupProfile := catalog.BackupProfile
	bootstrapProfile := catalog.BootstrapProfile
	storageProfile := catalog.StorageProfile
	unsealProfile := catalog.UnsealProfile
	runtimeProfile := catalog.RuntimeProfile
	observabilityProfile := catalog.ObservabilityProfile
	networkProfile := catalog.NetworkProfile
	upgradePolicy := catalog.UpgradePolicy

	approvedUnseal, unsealValidation := bindApprovedUnseal(serviceProfile.Spec.Cluster.SecurityProfile, unsealProfile)
	if !unsealValidation.Valid {
		return nil, unsealValidation
	}

	contract := &ApprovedServiceContract{
		Cluster: ApprovedCluster{
			Version:         serviceProfile.Spec.Cluster.Version,
			Voters:          serviceProfile.Spec.Cluster.Voters,
			ReadReplicas:    derefInt32(serviceProfile.Spec.Cluster.ReadReplicas),
			SecurityProfile: serviceProfile.Spec.Cluster.SecurityProfile,
		},
		Unseal:        approvedUnseal,
		Storage:       bindApprovedStorage(serviceProfile, storageProfile),
		Runtime:       bindApprovedRuntime(runtimeProfile),
		Observability: bindApprovedObservability(observabilityProfile),
		Network:       bindApprovedNetwork(networkProfile),
		Bootstrap: ApprovedBootstrap{
			Mode: serviceProfile.Spec.Bootstrap.Mode,
		},
		Exposure: ApprovedExposure{
			ClassRef: openbaov1alpha1.LocalReference{Name: exposureClass.Name},
		},
		Backup: ApprovedBackup{
			ProfileRef: openbaov1alpha1.LocalReference{Name: backupProfile.Name},
			Parameters: ApprovedBackupParameters{
				Location:  backupLocation(claim),
				Partition: backupPartition(claim),
			},
		},
		Lifecycle: bindApprovedLifecycle(serviceProfile, upgradePolicy),
		Provenance: ApprovedServiceProvenance{
			ServiceProfileRef: boundRevisionReference(serviceProfile),
			ExposureClassRef:  boundRevisionReferencePtr(exposureClass),
			BackupProfileRef:  boundRevisionReferencePtr(backupProfile),
		},
	}
	if bootstrapProfile != nil {
		contract.Bootstrap.ProfileRef = &openbaov1alpha1.LocalReference{Name: bootstrapProfile.Name}
		contract.Provenance.BootstrapProfileRef = boundRevisionReferencePtr(bootstrapProfile)
	}
	if storageProfile != nil {
		contract.Provenance.StorageProfileRef = boundRevisionReferencePtr(storageProfile)
	}
	if unsealProfile != nil {
		contract.Provenance.UnsealProfileRef = boundRevisionReferencePtr(unsealProfile)
	}
	if runtimeProfile != nil {
		contract.Provenance.RuntimeProfileRef = boundRevisionReferencePtr(runtimeProfile)
	}
	if observabilityProfile != nil {
		contract.Provenance.ObservabilityProfileRef = boundRevisionReferencePtr(observabilityProfile)
	}
	if networkProfile != nil {
		contract.Provenance.NetworkProfileRef = boundRevisionReferencePtr(networkProfile)
	}
	if upgradePolicy != nil {
		contract.Provenance.UpgradePolicyRef = boundRevisionReferencePtr(upgradePolicy)
	}

	return contract, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Approved service contract has been bound from immutable catalog inputs.",
	}
}

type catalogObjectBinding struct {
	ref             *openbaov1alpha1.LocalReference
	object          metav1.Object
	requiredMessage string
	mismatchMessage string
}

func validateApprovedCatalogInputs(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	catalog *CatalogBundle,
) ValidationResult {
	if claim == nil {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoClusterClaim is required to bind an approved service contract.",
		}
	}
	if catalog == nil {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Immutable catalog inputs are required to bind an approved service contract.",
		}
	}
	serviceProfile := catalog.ServiceProfile
	if serviceProfile == nil {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoServiceProfile is required to bind an approved service contract.",
		}
	}
	if serviceProfile.Name != claim.Spec.ServiceProfileRef.Name {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Loaded OpenBaoServiceProfile does not match OpenBaoClusterClaim.spec.serviceProfileRef.",
		}
	}

	for _, binding := range approvedCatalogObjectBindings(serviceProfile, catalog) {
		if validation := validateCatalogObjectBinding(binding); !validation.Valid {
			return validation
		}
	}

	return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
}

func approvedCatalogObjectBindings(
	serviceProfile *openbaov1alpha1.OpenBaoServiceProfile,
	catalog *CatalogBundle,
) []catalogObjectBinding {
	var unsealRef *openbaov1alpha1.LocalReference
	if serviceProfile.Spec.Unseal != nil {
		unsealRef = serviceProfile.Spec.Unseal.ProfileRef
	}
	var runtimeRef *openbaov1alpha1.LocalReference
	if serviceProfile.Spec.Runtime != nil {
		runtimeRef = serviceProfile.Spec.Runtime.ProfileRef
	}
	var observabilityRef *openbaov1alpha1.LocalReference
	if serviceProfile.Spec.Observability != nil {
		observabilityRef = serviceProfile.Spec.Observability.ProfileRef
	}
	var networkRef *openbaov1alpha1.LocalReference
	if serviceProfile.Spec.Network != nil {
		networkRef = serviceProfile.Spec.Network.ProfileRef
	}

	return []catalogObjectBinding{
		{
			ref:             &serviceProfile.Spec.Exposure.ClassRef,
			object:          catalog.ExposureClass,
			requiredMessage: "OpenBaoExposureClass is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoExposureClass does not match OpenBaoServiceProfile.spec.exposure.classRef.",
		},
		{
			ref:             &serviceProfile.Spec.Backup.ProfileRef,
			object:          catalog.BackupProfile,
			requiredMessage: "OpenBaoBackupProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoBackupProfile does not match OpenBaoServiceProfile.spec.backup.profileRef.",
		},
		{
			ref:             serviceProfile.Spec.Bootstrap.ProfileRef,
			object:          catalog.BootstrapProfile,
			requiredMessage: "OpenBaoBootstrapProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoBootstrapProfile does not match OpenBaoServiceProfile.spec.bootstrap.profileRef.",
		},
		{
			ref:             serviceProfile.Spec.Storage.ProfileRef,
			object:          catalog.StorageProfile,
			requiredMessage: "OpenBaoStorageProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoStorageProfile does not match OpenBaoServiceProfile.spec.storage.profileRef.",
		},
		{
			ref:             unsealRef,
			object:          catalog.UnsealProfile,
			requiredMessage: "OpenBaoUnsealProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoUnsealProfile does not match OpenBaoServiceProfile.spec.unseal.profileRef.",
		},
		{
			ref:             runtimeRef,
			object:          catalog.RuntimeProfile,
			requiredMessage: "OpenBaoRuntimeProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoRuntimeProfile does not match OpenBaoServiceProfile.spec.runtime.profileRef.",
		},
		{
			ref:             observabilityRef,
			object:          catalog.ObservabilityProfile,
			requiredMessage: "OpenBaoObservabilityProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoObservabilityProfile does not match OpenBaoServiceProfile.spec.observability.profileRef.",
		},
		{
			ref:             networkRef,
			object:          catalog.NetworkProfile,
			requiredMessage: "OpenBaoNetworkProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoNetworkProfile does not match OpenBaoServiceProfile.spec.network.profileRef.",
		},
		{
			ref:             serviceProfile.Spec.Lifecycle.PolicyRef,
			object:          catalog.UpgradePolicy,
			requiredMessage: "OpenBaoUpgradePolicy is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoUpgradePolicy does not match OpenBaoServiceProfile.spec.lifecycle.policyRef.",
		},
	}
}

func validateCatalogObjectBinding(binding catalogObjectBinding) ValidationResult {
	if binding.ref == nil {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	if isNilCatalogObject(binding.object) {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: binding.requiredMessage,
		}
	}
	if binding.object.GetName() != binding.ref.Name {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: binding.mismatchMessage,
		}
	}
	return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
}

func isNilCatalogObject(object metav1.Object) bool {
	if object == nil {
		return true
	}
	value := reflect.ValueOf(object)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

// AppliedStatus projects the bound top-level catalog provenance onto claim status.
func AppliedStatus(contract *ApprovedServiceContract) openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	if contract == nil {
		return openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{}
	}

	return openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{
		ServiceProfileRef:       boundRevisionCopy(&contract.Provenance.ServiceProfileRef),
		BootstrapProfileRef:     boundRevisionCopy(contract.Provenance.BootstrapProfileRef),
		ExposureClassRef:        boundRevisionCopy(contract.Provenance.ExposureClassRef),
		StorageProfileRef:       boundRevisionCopy(contract.Provenance.StorageProfileRef),
		UnsealProfileRef:        boundRevisionCopy(contract.Provenance.UnsealProfileRef),
		RuntimeProfileRef:       boundRevisionCopy(contract.Provenance.RuntimeProfileRef),
		ObservabilityProfileRef: boundRevisionCopy(contract.Provenance.ObservabilityProfileRef),
		NetworkProfileRef:       boundRevisionCopy(contract.Provenance.NetworkProfileRef),
		UpgradePolicyRef:        boundRevisionCopy(contract.Provenance.UpgradePolicyRef),
		BackupProfileRef:        boundRevisionCopy(contract.Provenance.BackupProfileRef),
	}
}

// ValidateContinuity fails closed if a previously applied immutable revision name now resolves to a different UID.
func ValidateContinuity(applied openbaov1alpha1.OpenBaoClusterClaimAppliedStatus, contract *ApprovedServiceContract) ValidationResult {
	if contract == nil {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Approved service contract is required to validate continuity.",
		}
	}

	checks := []struct {
		display string
		applied *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
		desired *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	}{
		{display: "OpenBaoServiceProfile", applied: applied.ServiceProfileRef, desired: &contract.Provenance.ServiceProfileRef},
		{display: "OpenBaoBootstrapProfile", applied: applied.BootstrapProfileRef, desired: contract.Provenance.BootstrapProfileRef},
		{display: "OpenBaoExposureClass", applied: applied.ExposureClassRef, desired: contract.Provenance.ExposureClassRef},
		{display: "OpenBaoStorageProfile", applied: applied.StorageProfileRef, desired: contract.Provenance.StorageProfileRef},
		{display: "OpenBaoUnsealProfile", applied: applied.UnsealProfileRef, desired: contract.Provenance.UnsealProfileRef},
		{display: "OpenBaoRuntimeProfile", applied: applied.RuntimeProfileRef, desired: contract.Provenance.RuntimeProfileRef},
		{display: "OpenBaoObservabilityProfile", applied: applied.ObservabilityProfileRef, desired: contract.Provenance.ObservabilityProfileRef},
		{display: "OpenBaoNetworkProfile", applied: applied.NetworkProfileRef, desired: contract.Provenance.NetworkProfileRef},
		{display: "OpenBaoUpgradePolicy", applied: applied.UpgradePolicyRef, desired: contract.Provenance.UpgradePolicyRef},
		{display: "OpenBaoBackupProfile", applied: applied.BackupProfileRef, desired: contract.Provenance.BackupProfileRef},
	}
	for _, check := range checks {
		if result := validateBoundRevisionContinuity(check.display, check.applied, check.desired); !result.Valid {
			return result
		}
	}

	return ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Approved service contract continuity matches previously applied immutable revisions.",
	}
}

func validateBoundRevisionContinuity(
	display string,
	applied *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference,
	desired *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference,
) ValidationResult {
	if applied == nil || applied.Name == "" || applied.UID == "" {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	if desired == nil || desired.Name == "" || desired.UID == "" {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	if applied.Name != desired.Name {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	if applied.UID == desired.UID {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}

	return ValidationResult{
		Valid:   false,
		Reason:  openbaov1alpha1.ReasonInvalid,
		Message: display + " continuity is invalid because the bound immutable revision name now resolves to a different UID.",
	}
}

func bindApprovedStorage(
	serviceProfile *openbaov1alpha1.OpenBaoServiceProfile,
	storageProfile *openbaov1alpha1.OpenBaoStorageProfile,
) ApprovedStorage {
	storage := ApprovedStorage{}
	if serviceProfile != nil {
		storage.PrimarySize = serviceProfile.Spec.Storage.PrimarySize
		storage.ReadReplicaSize = serviceProfile.Spec.Storage.ReadReplicaSize
	}
	if storageProfile == nil {
		return storage
	}

	storage.ProfileRef = &openbaov1alpha1.LocalReference{Name: storageProfile.Name}
	if storageProfile.Spec.Primary != nil {
		storage.PrimaryClassName = cloneStringPtr(storageProfile.Spec.Primary.StorageClassName)
	}
	if storageProfile.Spec.ReadReplica != nil && storageProfile.Spec.ReadReplica.StorageClassName != nil {
		storage.ReadReplicaClassName = cloneStringPtr(storageProfile.Spec.ReadReplica.StorageClassName)
	} else if usePrimaryStorageClassForReadReplicas(storageProfile.Spec.ReadReplica) {
		storage.ReadReplicaClassName = cloneStringPtr(storage.PrimaryClassName)
	}
	if storageProfile.Spec.ACMECache != nil {
		storage.ACMECache = acmeSharedCacheFromStorageProfile(storageProfile.Spec.ACMECache)
	}
	return storage
}

func acmeSharedCacheFromStorageProfile(
	config *openbaov1alpha1.OpenBaoStorageProfileACMECacheSpec,
) *openbaov1alpha1.ACMESharedCacheConfig {
	if config == nil {
		return nil
	}
	return &openbaov1alpha1.ACMESharedCacheConfig{
		Mode:              config.Mode,
		ExistingClaimName: config.ExistingClaimName,
		Size:              config.Size,
		StorageClassName:  cloneStringPtr(config.StorageClassName),
	}
}

func usePrimaryStorageClassForReadReplicas(readReplica *openbaov1alpha1.OpenBaoStorageProfileReadReplicaSpec) bool {
	if readReplica == nil || readReplica.UsePrimaryStorageClass == nil {
		return true
	}
	return *readReplica.UsePrimaryStorageClass
}

func bindApprovedUnseal(
	securityProfile openbaov1alpha1.Profile,
	unsealProfile *openbaov1alpha1.OpenBaoUnsealProfile,
) (ApprovedUnseal, ValidationResult) {
	if unsealProfile == nil {
		return ApprovedUnseal{Mode: approvedUnsealMode(securityProfile)}, ValidationResult{
			Valid:  true,
			Reason: openbaov1alpha1.ReasonAccepted,
		}
	}

	mode := approvedUnsealModeFromProfile(unsealProfile.Spec.Mode)
	if securityProfile == openbaov1alpha1.ProfileHardened && mode == UnsealPostureModeManagedStatic {
		return ApprovedUnseal{}, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Hardened service profiles require a non-static OpenBaoUnsealProfile.",
		}
	}

	config, validation := unsealConfigFromProfile(unsealProfile)
	if !validation.Valid {
		return ApprovedUnseal{}, validation
	}
	return ApprovedUnseal{
			Mode:       mode,
			ProfileRef: &openbaov1alpha1.LocalReference{Name: unsealProfile.Name},
			Config:     config,
		}, ValidationResult{
			Valid:  true,
			Reason: openbaov1alpha1.ReasonAccepted,
		}
}

func approvedUnsealModeFromProfile(mode openbaov1alpha1.OpenBaoUnsealProfileMode) UnsealPostureMode {
	if mode == "" || mode == openbaov1alpha1.OpenBaoUnsealProfileModeOperatorManagedStatic {
		return UnsealPostureModeManagedStatic
	}
	return UnsealPostureModeExternal
}

func unsealConfigFromProfile(profile *openbaov1alpha1.OpenBaoUnsealProfile) (*openbaov1alpha1.UnsealConfig, ValidationResult) {
	if profile == nil {
		return nil, ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}

	spec := profile.Spec
	config := &openbaov1alpha1.UnsealConfig{
		CredentialsSecretRef: cloneLocalObjectReference(spec.CredentialsSecretRef),
	}
	switch spec.Mode {
	case "", openbaov1alpha1.OpenBaoUnsealProfileModeOperatorManagedStatic:
		config.Type = unsealTypeStatic
		config.Static = cloneStaticSealConfig(spec.Static)
	case openbaov1alpha1.OpenBaoUnsealProfileModeTransit:
		if spec.Transit == nil {
			return nil, missingUnsealProfileSection(profile.Name, "transit")
		}
		config.Type = unsealTypeTransit
		config.Transit = spec.Transit.DeepCopy()
	case openbaov1alpha1.OpenBaoUnsealProfileModeAWSKMS:
		if spec.AWSKMS == nil {
			return nil, missingUnsealProfileSection(profile.Name, "awskms")
		}
		config.Type = unsealTypeAWSKMS
		config.AWSKMS = spec.AWSKMS.DeepCopy()
	case openbaov1alpha1.OpenBaoUnsealProfileModeGCPCloudKMS:
		if spec.GCPCloudKMS == nil {
			return nil, missingUnsealProfileSection(profile.Name, "gcpCloudKMS")
		}
		config.Type = unsealTypeGCPCloudKMS
		config.GCPCloudKMS = spec.GCPCloudKMS.DeepCopy()
	case openbaov1alpha1.OpenBaoUnsealProfileModeAzureKeyVault:
		if spec.AzureKeyVault == nil {
			return nil, missingUnsealProfileSection(profile.Name, "azureKeyVault")
		}
		config.Type = unsealTypeAzureKeyVault
		config.AzureKeyVault = spec.AzureKeyVault.DeepCopy()
	case openbaov1alpha1.OpenBaoUnsealProfileModeOCIKMS:
		if spec.OCIKMS == nil {
			return nil, missingUnsealProfileSection(profile.Name, "ocikms")
		}
		config.Type = unsealTypeOCIKMS
		config.OCIKMS = spec.OCIKMS.DeepCopy()
	case openbaov1alpha1.OpenBaoUnsealProfileModeKMIP:
		if spec.KMIP == nil {
			return nil, missingUnsealProfileSection(profile.Name, "kmip")
		}
		config.Type = unsealTypeKMIP
		config.KMIP = spec.KMIP.DeepCopy()
	case openbaov1alpha1.OpenBaoUnsealProfileModePKCS11:
		if spec.PKCS11 == nil {
			return nil, missingUnsealProfileSection(profile.Name, "pkcs11")
		}
		config.Type = unsealTypePKCS11
		config.PKCS11 = spec.PKCS11.DeepCopy()
	default:
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoUnsealProfile uses an unsupported mode.",
		}
	}

	if config.Type == unsealTypeStatic && config.Static == nil && config.CredentialsSecretRef == nil {
		return nil, ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	return config, ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
}

func missingUnsealProfileSection(profileName, section string) ValidationResult {
	return ValidationResult{
		Valid:   false,
		Reason:  openbaov1alpha1.ReasonInvalid,
		Message: "OpenBaoUnsealProfile " + profileName + " is missing required spec." + section + " configuration.",
	}
}

func bindApprovedRuntime(profile *openbaov1alpha1.OpenBaoRuntimeProfile) ApprovedRuntime {
	if profile == nil {
		return ApprovedRuntime{}
	}
	return ApprovedRuntime{
		ProfileRef:                &openbaov1alpha1.LocalReference{Name: profile.Name},
		ServiceAccount:            cloneServiceAccountConfig(profile.Spec.ServiceAccount),
		PodMetadata:               clonePodMetadataConfig(profile.Spec.PodMetadata),
		ImagePullSecrets:          cloneLocalObjectReferenceSlice(profile.Spec.ImagePullSecrets),
		ImageVerification:         cloneImageVerificationConfig(profile.Spec.ImageVerification),
		OperatorImageVerification: cloneImageVerificationConfig(profile.Spec.OperatorImageVerification),
		WorkloadHardening:         cloneWorkloadHardeningConfig(profile.Spec.WorkloadHardening),
		SecurityContext:           clonePodSecurityContext(profile.Spec.SecurityContext),
		HelperImages:              cloneHelperImages(profile.Spec.HelperImages),
		ReadReplica:               cloneRuntimeReadReplica(profile.Spec.ReadReplica),
	}
}

func bindApprovedObservability(profile *openbaov1alpha1.OpenBaoObservabilityProfile) ApprovedObservability {
	if profile == nil {
		return ApprovedObservability{}
	}
	return ApprovedObservability{
		ProfileRef:    &openbaov1alpha1.LocalReference{Name: profile.Name},
		Observability: cloneObservabilityConfig(profile.Spec.Observability),
		Telemetry:     cloneTelemetryConfig(profile.Spec.Telemetry),
	}
}

func bindApprovedNetwork(profile *openbaov1alpha1.OpenBaoNetworkProfile) ApprovedNetwork {
	if profile == nil {
		return ApprovedNetwork{}
	}
	return ApprovedNetwork{
		ProfileRef:           &openbaov1alpha1.LocalReference{Name: profile.Name},
		APIServerCIDR:        strings.TrimSpace(profile.Spec.APIServerCIDR),
		APIServerEndpointIPs: cloneStringSlice(profile.Spec.APIServerEndpointIPs),
		DNSNamespace:         strings.TrimSpace(profile.Spec.DNSNamespace),
		DNSEndpointIPs:       cloneStringSlice(profile.Spec.DNSEndpointIPs),
		EgressRules:          cloneEgressRules(profile.Spec.EgressRules),
		IngressRules:         cloneIngressRules(profile.Spec.IngressRules),
		TrustedIngressPeers:  cloneNetworkPolicyPeers(profile.Spec.TrustedIngressPeers),
	}
}

func bindApprovedLifecycle(
	serviceProfile *openbaov1alpha1.OpenBaoServiceProfile,
	policy *openbaov1alpha1.OpenBaoUpgradePolicy,
) ApprovedLifecycle {
	lifecycle := ApprovedLifecycle{}
	if serviceProfile != nil {
		lifecycle.UpgradeStrategy = defaultUpgradeStrategy(serviceProfile.Spec.Lifecycle.UpgradeStrategy)
		lifecycle.PreUpgradeSnapshot = derefBool(serviceProfile.Spec.Lifecycle.PreUpgradeSnapshot)
		if serviceProfile.Spec.Lifecycle.PolicyRef != nil {
			lifecycle.PolicyRef = &openbaov1alpha1.LocalReference{Name: serviceProfile.Spec.Lifecycle.PolicyRef.Name}
		}
	}
	if policy != nil {
		lifecycle.PolicyRef = &openbaov1alpha1.LocalReference{Name: policy.Name}
		lifecycle.BlueGreen = blueGreenConfigFromPolicy(policy.Spec.BlueGreen)
	}
	return lifecycle
}

func blueGreenConfigFromPolicy(policy *openbaov1alpha1.OpenBaoUpgradePolicyBlueGreenSpec) *openbaov1alpha1.BlueGreenConfig {
	if policy == nil {
		return nil
	}
	blueGreen := &openbaov1alpha1.BlueGreenConfig{
		AutoPromote: derefBoolDefaultTrue(policy.AutoPromote),
	}
	if strings.TrimSpace(policy.MinSyncDuration) != "" {
		blueGreen.Verification = &openbaov1alpha1.VerificationConfig{
			MinSyncDuration: strings.TrimSpace(policy.MinSyncDuration),
		}
	}
	if policy.MaxJobFailures != nil {
		value := *policy.MaxJobFailures
		blueGreen.MaxJobFailures = &value
	}
	if policy.AutoRollback != nil {
		blueGreen.AutoRollback = &openbaov1alpha1.AutoRollbackConfig{
			Enabled:             derefBoolDefaultTrue(policy.AutoRollback.Enabled),
			OnJobFailure:        derefBoolDefaultTrue(policy.AutoRollback.OnJobFailure),
			OnValidationFailure: derefBoolDefaultTrue(policy.AutoRollback.OnValidationFailure),
		}
	}
	return blueGreen
}

func cloneLocalObjectReference(ref *corev1.LocalObjectReference) *corev1.LocalObjectReference {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}

func cloneLocalObjectReferenceSlice(refs []corev1.LocalObjectReference) []corev1.LocalObjectReference {
	if refs == nil {
		return nil
	}
	copy := make([]corev1.LocalObjectReference, len(refs))
	copy = append(copy[:0], refs...)
	return copy
}

func cloneStaticSealConfig(config *openbaov1alpha1.StaticSealConfig) *openbaov1alpha1.StaticSealConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func cloneServiceAccountConfig(config *openbaov1alpha1.ServiceAccountConfig) *openbaov1alpha1.ServiceAccountConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func clonePodMetadataConfig(config *openbaov1alpha1.PodMetadataConfig) *openbaov1alpha1.PodMetadataConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func cloneImageVerificationConfig(config *openbaov1alpha1.ImageVerificationConfig) *openbaov1alpha1.ImageVerificationConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func cloneWorkloadHardeningConfig(config *openbaov1alpha1.WorkloadHardeningConfig) *openbaov1alpha1.WorkloadHardeningConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func clonePodSecurityContext(context *corev1.PodSecurityContext) *corev1.PodSecurityContext {
	if context == nil {
		return nil
	}
	return context.DeepCopy()
}

func cloneHelperImages(images *openbaov1alpha1.OpenBaoRuntimeProfileHelperImagesSpec) *openbaov1alpha1.OpenBaoRuntimeProfileHelperImagesSpec {
	if images == nil {
		return nil
	}
	copy := *images
	return &copy
}

func cloneRuntimeReadReplica(readReplica *openbaov1alpha1.OpenBaoRuntimeProfileReadReplicaSpec) *openbaov1alpha1.OpenBaoRuntimeProfileReadReplicaSpec {
	if readReplica == nil {
		return nil
	}
	copy := *readReplica
	copy.Template = cloneReadReplicaTemplateConfig(readReplica.Template)
	return &copy
}

func cloneReadReplicaTemplateConfig(template *openbaov1alpha1.ReadReplicaTemplateConfig) *openbaov1alpha1.ReadReplicaTemplateConfig {
	if template == nil {
		return nil
	}
	return template.DeepCopy()
}

func cloneObservabilityConfig(config *openbaov1alpha1.ObservabilityConfig) *openbaov1alpha1.ObservabilityConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func cloneTelemetryConfig(config *openbaov1alpha1.TelemetryConfig) *openbaov1alpha1.TelemetryConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func cloneIngressRules(rules []networkingv1.NetworkPolicyIngressRule) []networkingv1.NetworkPolicyIngressRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]networkingv1.NetworkPolicyIngressRule, len(rules))
	for i := range rules {
		rules[i].DeepCopyInto(&out[i])
	}
	return out
}

func cloneNetworkPolicyPeers(peers []networkingv1.NetworkPolicyPeer) []networkingv1.NetworkPolicyPeer {
	if len(peers) == 0 {
		return nil
	}
	out := make([]networkingv1.NetworkPolicyPeer, len(peers))
	for i := range peers {
		peers[i].DeepCopyInto(&out[i])
	}
	return out
}

func backupLocation(claim *openbaov1alpha1.OpenBaoClusterClaim) string {
	if claim == nil || claim.Spec.ServiceParameters == nil || claim.Spec.ServiceParameters.Backup == nil {
		return ""
	}

	return claim.Spec.ServiceParameters.Backup.Location
}

func backupPartition(claim *openbaov1alpha1.OpenBaoClusterClaim) string {
	if claim == nil || claim.Spec.ServiceParameters == nil || claim.Spec.ServiceParameters.Backup == nil {
		return ""
	}

	return claim.Spec.ServiceParameters.Backup.Partition
}

func defaultUpgradeStrategy(strategy openbaov1alpha1.UpdateStrategyType) openbaov1alpha1.UpdateStrategyType {
	if strategy == "" {
		return openbaov1alpha1.UpdateStrategyRollingUpdate
	}

	return strategy
}

func approvedUnsealMode(profile openbaov1alpha1.Profile) UnsealPostureMode {
	if profile == openbaov1alpha1.ProfileHardened {
		return UnsealPostureModeExternal
	}

	return UnsealPostureModeManagedStatic
}

func derefInt32(value *int32) int32 {
	if value == nil {
		return 0
	}

	return *value
}

func derefBool(value *bool) bool {
	if value == nil {
		return false
	}

	return *value
}

func derefBoolDefaultTrue(value *bool) bool {
	if value == nil {
		return true
	}

	return *value
}

func boundRevisionReference(object metav1.Object) openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference {
	return openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
		Name: object.GetName(),
		UID:  string(object.GetUID()),
	}
}

func boundRevisionReferencePtr(object metav1.Object) *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference {
	if object == nil {
		return nil
	}

	ref := boundRevisionReference(object)
	return &ref
}

func boundRevisionCopy(ref *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference) *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference {
	if ref == nil {
		return nil
	}

	copy := *ref
	return &copy
}
