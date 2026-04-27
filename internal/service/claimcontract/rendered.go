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

// RenderSameClusterExecutionContract renders target-specific same-cluster execution inputs.
func RenderSameClusterExecutionContract(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	target *openbaov1alpha1.NamespacedReference,
	approved *ApprovedServiceContract,
	catalog *CatalogBundle,
	transitDefaults SameClusterTransitUnsealDefaults,
	bootstrapInputs SameClusterBootstrapResolvedInputs,
) (*RenderedExecutionContract, ValidationResult) {
	if claim == nil {
		return nil, ValidationResult{Valid: false, Reason: openbaov1alpha1.ReasonInvalid, Message: "OpenBaoClusterClaim is required to render execution inputs."}
	}
	if target == nil || target.Namespace == "" {
		return nil, ValidationResult{Valid: false, Reason: openbaov1alpha1.ReasonPending, Message: "Same-cluster target namespace is required to render execution inputs."}
	}
	if approved == nil {
		return nil, ValidationResult{Valid: false, Reason: openbaov1alpha1.ReasonPending, Message: "Approved service contract is required to render execution inputs."}
	}
	if catalog == nil || catalog.ServiceProfile == nil || catalog.ExposureClass == nil || catalog.BackupProfile == nil {
		return nil, ValidationResult{Valid: false, Reason: openbaov1alpha1.ReasonPending, Message: "Immutable catalog inputs are required to render execution inputs."}
	}
	if catalog.ServiceProfile.Spec.Bootstrap.ProfileRef != nil && catalog.BootstrapProfile == nil {
		return nil, ValidationResult{Valid: false, Reason: openbaov1alpha1.ReasonPending, Message: "OpenBaoBootstrapProfile is required to render execution inputs."}
	}

	renderedBackup, validation := renderBackup(claim, target, approved, catalog)
	if !validation.Valid {
		return nil, validation
	}
	renderedEntrypoint, validation := renderEntrypoint(catalog.ExposureClass, catalog.Entrypoint)
	if !validation.Valid {
		return nil, validation
	}
	renderedIngress, validation := renderIngress(catalog.ExposureClass, catalog.Entrypoint, catalog.IngressPolicy)
	if !validation.Valid {
		return nil, validation
	}
	hostnamePolicy, validation := renderedHostnamePolicy(claim, catalog.ExposureClass.Spec.HostnamePolicy)
	if !validation.Valid {
		return nil, validation
	}

	rendered := &RenderedExecutionContract{
		TargetNamespace: target.Namespace,
		Cluster: RenderedCluster{
			Version:         approved.Cluster.Version,
			Replicas:        approved.Cluster.Voters,
			ReadReplicas:    approved.Cluster.ReadReplicas,
			SecurityProfile: approved.Cluster.SecurityProfile,
		},
		Unseal: RenderedUnseal{
			Mode: approved.Unseal.Mode,
		},
		Storage: RenderedStorage{
			PrimarySize:          approved.Storage.PrimarySize,
			ReadReplicaSize:      approved.Storage.ReadReplicaSize,
			ClassName:            cloneStringPtr(approved.Storage.PrimaryClassName),
			ReadReplicaClassName: cloneStringPtr(approved.Storage.ReadReplicaClassName),
			ACMECache:            cloneACMESharedCacheConfig(approved.Storage.ACMECache),
		},
		Runtime:       cloneApprovedRuntime(approved.Runtime),
		Observability: cloneApprovedObservability(approved.Observability),
		Bootstrap: RenderedBootstrap{
			Mode:       approved.Bootstrap.Mode,
			ProfileRef: approved.Bootstrap.ProfileRef,
		},
		Exposure: RenderedExposure{
			PublishMode:              catalog.ExposureClass.Spec.PublishMode,
			HostnamePolicy:           hostnamePolicy,
			TLSPolicy:                cloneTLSPolicy(catalog.ExposureClass.Spec.TLSPolicy),
			EntrypointRef:            cloneLocalReference(catalog.ExposureClass.Spec.EntrypointRef),
			Entrypoint:               renderedEntrypoint,
			IngressPolicyRef:         cloneLocalReference(catalog.ExposureClass.Spec.IngressPolicyRef),
			Ingress:                  renderedIngress,
			Routing:                  cloneRouting(catalog.ExposureClass.Spec.Routing),
			GatewayAnnotations:       cloneStringMap(catalog.ExposureClass.Spec.GatewayAnnotations),
			ServicePolicy:            cloneServicePolicy(catalog.ExposureClass.Spec.ServicePolicy),
			ReadReplicaServicePolicy: cloneReadReplicaServicePolicy(catalog.ExposureClass.Spec.ReadReplicaServicePolicy),
		},
		Backup:     renderedBackup,
		Network:    renderedNetwork(approved.Network, renderedBackup),
		Lifecycle:  approved.Lifecycle,
		Provenance: approved.Provenance,
	}
	if approved.Unseal.Config != nil {
		rendered.Unseal.Config = approved.Unseal.Config.DeepCopy()
	}
	if catalog.BootstrapProfile != nil {
		rendered.Bootstrap.OperatorLifecycleAuth = catalog.BootstrapProfile.Spec.OperatorLifecycleAuth
		renderedAuth, validation := renderBootstrapAuth(catalog.BootstrapProfile.Spec.Auth, bootstrapInputs)
		if !validation.Valid {
			return nil, validation
		}
		rendered.Bootstrap.Auth = renderedAuth
		rendered.Bootstrap.SecretEngines = cloneBootstrapSecretEngines(catalog.BootstrapProfile.Spec.SecretEngines)
		renderedPolicies, validation := renderBootstrapPolicies(catalog.BootstrapProfile.Spec.Policies, bootstrapInputs)
		if !validation.Valid {
			return nil, validation
		}
		rendered.Bootstrap.Policies = renderedPolicies
		renderedAudit, validation := renderBootstrapAudit(catalog.BootstrapProfile.Spec.Audit, bootstrapInputs)
		if !validation.Valid {
			return nil, validation
		}
		rendered.Bootstrap.Audit = renderedAudit
	}
	if approved.Unseal.Mode == UnsealPostureModeExternal && rendered.Unseal.Config == nil {
		validation := applySameClusterTransitUnseal(rendered, transitDefaults)
		if !validation.Valid {
			return nil, validation
		}
	}

	return rendered, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered execution contract has been produced for the same-cluster materialization path.",
	}
}
