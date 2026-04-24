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

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// AdoptionCompatibilityIssue identifies one OpenBaoCluster spec field that does
// not match the catalog-rendered desired state.
type AdoptionCompatibilityIssue struct {
	Field   string
	Reason  string
	Message string
}

// AdoptionCompatibilityResult summarizes whether an existing OpenBaoCluster can
// be safely adopted by a catalog-rendered claim without changing its service
// shape.
type AdoptionCompatibilityResult struct {
	Compatible bool
	Issues     []AdoptionCompatibilityIssue
}

const (
	adoptionReasonInputMissing = "InputMissing"
	adoptionReasonSpecMismatch = "SpecMismatch"
)

// EvaluateSameClusterAdoptionCompatibility compares an existing OpenBaoCluster
// with the catalog-rendered desired OpenBaoCluster for the same claim. Adoption
// is compatible only when the catalog-rendered service shape already matches the
// existing cluster across fields controlled by the claims catalog.
func EvaluateSameClusterAdoptionCompatibility(
	existing *openbaov1alpha1.OpenBaoCluster,
	desired *openbaov1alpha1.OpenBaoCluster,
) AdoptionCompatibilityResult {
	result := AdoptionCompatibilityResult{Compatible: true}
	if existing == nil {
		result.addIssue("metadata", adoptionReasonInputMissing, "Existing OpenBaoCluster is required for adoption compatibility evaluation.")
		return result
	}
	if desired == nil {
		result.addIssue("metadata", adoptionReasonInputMissing, "Catalog-rendered OpenBaoCluster is required for adoption compatibility evaluation.")
		return result
	}

	compareAdoptionField(&result, "metadata.namespace", existing.Namespace, desired.Namespace)
	compareAdoptionField(&result, "metadata.name", existing.Name, desired.Name)
	compareAdoptionField(&result, "spec.version", existing.Spec.Version, desired.Spec.Version)
	compareAdoptionField(&result, "spec.image", existing.Spec.Image, desired.Spec.Image)
	compareAdoptionField(&result, "spec.replicas", existing.Spec.Replicas, desired.Spec.Replicas)
	compareAdoptionField(&result, "spec.profile", existing.Spec.Profile, desired.Spec.Profile)
	compareAdoptionField(&result, "spec.paused", existing.Spec.Paused, desired.Spec.Paused)
	compareAdoptionField(&result, "spec.maintenance", existing.Spec.Maintenance, desired.Spec.Maintenance)
	compareAdoptionField(&result, "spec.runtime", existing.Spec.Runtime, desired.Spec.Runtime)
	compareAdoptionField(&result, "spec.breakGlassAck", existing.Spec.BreakGlassAck, desired.Spec.BreakGlassAck)
	compareAdoptionField(&result, "spec.storage", existing.Spec.Storage, desired.Spec.Storage)
	compareAdoptionField(&result, "spec.tls", existing.Spec.TLS, desired.Spec.TLS)
	compareAdoptionField(&result, "spec.service", existing.Spec.Service, desired.Spec.Service)
	compareAdoptionField(&result, "spec.readReplicas", existing.Spec.ReadReplicas, desired.Spec.ReadReplicas)
	compareAdoptionField(&result, "spec.unseal", existing.Spec.Unseal, desired.Spec.Unseal)
	compareAdoptionField(&result, "spec.selfInit", existing.Spec.SelfInit, desired.Spec.SelfInit)
	compareAdoptionField(&result, "spec.backup", existing.Spec.Backup, desired.Spec.Backup)
	compareAdoptionField(&result, "spec.restore", existing.Spec.Restore, desired.Spec.Restore)
	compareAdoptionField(&result, "spec.upgrade", existing.Spec.Upgrade, desired.Spec.Upgrade)
	compareAdoptionField(&result, "spec.network", existing.Spec.Network, desired.Spec.Network)
	compareAdoptionField(&result, "spec.ingress", existing.Spec.Ingress, desired.Spec.Ingress)
	compareAdoptionField(&result, "spec.gateway", existing.Spec.Gateway, desired.Spec.Gateway)
	compareAdoptionField(&result, "spec.configuration", existing.Spec.Configuration, desired.Spec.Configuration)
	compareAdoptionField(&result, "spec.deletionPolicy", existing.Spec.DeletionPolicy, desired.Spec.DeletionPolicy)
	compareAdoptionField(&result, "spec.audit", existing.Spec.Audit, desired.Spec.Audit)
	compareAdoptionField(&result, "spec.plugins", existing.Spec.Plugins, desired.Spec.Plugins)
	compareAdoptionField(&result, "spec.serviceAccount", existing.Spec.ServiceAccount, desired.Spec.ServiceAccount)
	compareAdoptionField(&result, "spec.podMetadata", existing.Spec.PodMetadata, desired.Spec.PodMetadata)
	compareAdoptionField(&result, "spec.imagePullSecrets", existing.Spec.ImagePullSecrets, desired.Spec.ImagePullSecrets)
	compareAdoptionField(&result, "spec.imageVerification", existing.Spec.ImageVerification, desired.Spec.ImageVerification)
	compareAdoptionField(&result, "spec.operatorImageVerification", existing.Spec.OperatorImageVerification, desired.Spec.OperatorImageVerification)
	compareAdoptionField(&result, "spec.workloadHardening", existing.Spec.WorkloadHardening, desired.Spec.WorkloadHardening)
	compareAdoptionField(&result, "spec.securityContext", existing.Spec.SecurityContext, desired.Spec.SecurityContext)
	compareAdoptionField(&result, "spec.observability", existing.Spec.Observability, desired.Spec.Observability)
	compareAdoptionField(&result, "spec.telemetry", existing.Spec.Telemetry, desired.Spec.Telemetry)
	compareAdoptionField(&result, "spec.initContainer", existing.Spec.InitContainer, desired.Spec.InitContainer)

	return result
}

func compareAdoptionField(result *AdoptionCompatibilityResult, field string, existing, desired any) {
	if result == nil || reflect.DeepEqual(existing, desired) {
		return
	}
	result.addIssue(field, adoptionReasonSpecMismatch, "Existing OpenBaoCluster field does not match the catalog-rendered desired state.")
}

func (result *AdoptionCompatibilityResult) addIssue(field, reason, message string) {
	if result == nil {
		return
	}
	result.Compatible = false
	result.Issues = append(result.Issues, AdoptionCompatibilityIssue{
		Field:   field,
		Reason:  reason,
		Message: message,
	})
}
