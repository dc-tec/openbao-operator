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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// IdentityHash returns a stable content hash for an internal contract value.
func IdentityHash(value any) string {
	payload, _ := json.Marshal(value)
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ContractIdentityStatus projects one internal contract hash into claim status.
func ContractIdentityStatus(identityHash string) *openbaov1alpha1.OpenBaoClusterClaimContractIdentityStatus {
	if identityHash == "" {
		return nil
	}

	return &openbaov1alpha1.OpenBaoClusterClaimContractIdentityStatus{IdentityHash: identityHash}
}
