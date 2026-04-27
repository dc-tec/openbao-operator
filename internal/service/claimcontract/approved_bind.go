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

import openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"

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
