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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const defaultClaimManagedTLSRotationPeriod = "720h"

// Keep claim-managed local cluster names short enough for downstream StatefulSet-
// derived labels and future revisioned StatefulSet names.
const maxClaimManagedLocalClusterNameLength = 35

// ClaimManagedLocalClusterName returns a deterministic workload-safe concrete
// OpenBaoCluster name for a same-cluster claim.
func ClaimManagedLocalClusterName(claimName string) string {
	base := strings.TrimSpace(claimName)
	if base == "" {
		base = "claim"
	}

	if len(base) <= maxClaimManagedLocalClusterNameLength {
		return base
	}

	hash := sha256.Sum256([]byte(base))
	hashSuffix := hex.EncodeToString(hash[:])[:12]
	prefixLength := maxClaimManagedLocalClusterNameLength - len(hashSuffix) - 1
	if prefixLength < 1 {
		return "claim-" + hashSuffix
	}

	prefix := strings.TrimRight(base[:prefixLength], "-")
	if prefix == "" {
		return "claim-" + hashSuffix
	}

	return prefix + "-" + hashSuffix
}

// DesiredSameClusterCluster projects the rendered same-cluster execution contract
// into a concrete claim-managed OpenBaoCluster.
func DesiredSameClusterCluster(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	rendered *RenderedExecutionContract,
) (*openbaov1alpha1.OpenBaoCluster, ValidationResult) {
	if claim == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoClusterClaim is required to build the same-cluster concrete workload.",
		}
	}
	if rendered == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Rendered execution contract is required to build the same-cluster concrete workload.",
		}
	}
	if strings.TrimSpace(rendered.TargetNamespace) == "" {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Rendered execution contract must specify a target namespace for same-cluster materialization.",
		}
	}
	if rendered.Bootstrap.Mode != openbaov1alpha1.OpenBaoBootstrapModeSelfInit {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster claim materialization currently supports only SelfInit bootstrap mode.",
		}
	}
	selfInitRequests, selfInitResult := renderedSelfInitRequests(rendered)
	if !selfInitResult.Valid {
		return nil, selfInitResult
	}
	auditDevices, auditResult := renderedAuditDevices(rendered)
	if !auditResult.Valid {
		return nil, auditResult
	}
	if len(selfInitRequests) == 0 && len(auditDevices) == 0 {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster claim materialization requires at least one concrete self-init request or declarative audit device after bootstrap rendering.",
		}
	}

	storage, storageResult := renderedStorageSpec(rendered)
	if !storageResult.Valid {
		return nil, storageResult
	}
	unseal, unsealResult := renderedUnsealConfig(rendered)
	if !unsealResult.Valid {
		return nil, unsealResult
	}
	backup, backupResult := renderedBackupSchedule(rendered)
	if !backupResult.Valid {
		return nil, backupResult
	}
	if rendered.Lifecycle.PreUpgradeSnapshot && backup == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster pre-upgrade snapshots require a rendered backup contract that can be projected into OpenBaoCluster.spec.backup.",
		}
	}
	upgrade := renderedUpgradeConfig(rendered)
	tlsConfig, tlsResult := renderedTLSConfig(rendered)
	if !tlsResult.Valid {
		return nil, tlsResult
	}
	network, networkResult := renderedNetworkConfig(rendered)
	if !networkResult.Valid {
		return nil, networkResult
	}
	ingress, ingressResult := renderedIngressConfigForExposure(rendered)
	if !ingressResult.Valid {
		return nil, ingressResult
	}
	gateway, exposureResult := renderedGatewayConfigForExposure(rendered)
	if !exposureResult.Valid {
		return nil, exposureResult
	}

	spec := openbaov1alpha1.OpenBaoClusterSpec{
		Version:  rendered.Cluster.Version,
		Image:    constants.GetOpenBaoImage(rendered.Cluster.Version),
		Replicas: rendered.Cluster.Replicas,
		Profile:  rendered.Cluster.SecurityProfile,
		TLS:      tlsConfig,
		Storage:  storage,
		Service:  renderedServiceConfig(rendered),
		Upgrade:  upgrade,
		SelfInit: renderedSelfInitConfig(rendered, selfInitRequests),
	}
	if len(auditDevices) > 0 {
		spec.Audit = auditDevices
	}
	if unseal != nil {
		spec.Unseal = unseal
	}
	applyRenderedRuntime(&spec, rendered.Runtime)
	applyRenderedObservability(&spec, rendered.Observability)
	if backup != nil {
		spec.Backup = backup
	}
	if network != nil {
		spec.Network = network
	}
	if gateway != nil {
		spec.Gateway = gateway
	}
	if ingress != nil {
		spec.Ingress = ingress
	}
	if rendered.Cluster.ReadReplicas > 0 {
		readReplicas, readReplicaResult := renderedReadReplicas(rendered)
		if !readReplicaResult.Valid {
			return nil, readReplicaResult
		}
		spec.ReadReplicas = readReplicas
	}

	return &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rendered.TargetNamespace,
				Name:      ClaimManagedLocalClusterName(claim.Name),
			},
			Spec: spec,
		}, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Same-cluster concrete OpenBaoCluster projection has been built from the rendered execution contract.",
		}
}
