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
