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
