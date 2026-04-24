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
	"strings"

	kvalidation "k8s.io/apimachinery/pkg/util/validation"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func renderEntrypoint(
	exposureClass *openbaov1alpha1.OpenBaoExposureClass,
	entrypoint *openbaov1alpha1.OpenBaoEntrypoint,
) (*RenderedExposureEntrypoint, ValidationResult) {
	if exposureClass == nil || exposureClass.Spec.EntrypointRef == nil {
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered exposure does not require a reusable entrypoint.",
		}
	}
	if entrypoint == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoEntrypoint is required to render the selected exposure posture.",
		}
	}
	if entrypoint.Name != exposureClass.Spec.EntrypointRef.Name {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Loaded OpenBaoEntrypoint does not match OpenBaoExposureClass.spec.entrypointRef.",
		}
	}

	return &RenderedExposureEntrypoint{
			Ref:            boundReference(entrypoint.Name, string(entrypoint.UID)),
			Mode:           entrypoint.Spec.Mode,
			ObjectRef:      entrypoint.Spec.ObjectRef,
			ListenerPolicy: cloneEntrypointListenerPolicy(entrypoint.Spec.ListenerPolicy),
		}, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered exposure entrypoint has been resolved from immutable catalog inputs.",
		}
}

func renderIngress(
	exposureClass *openbaov1alpha1.OpenBaoExposureClass,
	entrypoint *openbaov1alpha1.OpenBaoEntrypoint,
	policy *openbaov1alpha1.OpenBaoIngressPolicy,
) (*RenderedExposureIngress, ValidationResult) {
	if exposureClass == nil || exposureClass.Spec.PublishMode != openbaov1alpha1.OpenBaoExposurePublishModeIngress {
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered exposure does not require ingress execution inputs.",
		}
	}
	if entrypoint == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoEntrypoint is required to render ingress exposure inputs.",
		}
	}
	if entrypoint.Spec.Mode != openbaov1alpha1.OpenBaoEntrypointModeIngress {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Rendered ingress exposure requires an OpenBaoEntrypoint with mode Ingress.",
		}
	}
	if entrypoint.Spec.ObjectRef.APIGroup != "networking.k8s.io" || entrypoint.Spec.ObjectRef.Kind != "IngressClass" {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Rendered ingress exposure currently supports only networking.k8s.io IngressClass entrypoints.",
		}
	}
	if strings.TrimSpace(entrypoint.Spec.ObjectRef.Name) == "" {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Rendered ingress exposure requires the selected OpenBaoEntrypoint to reference a concrete IngressClass name.",
		}
	}
	if exposureClass.Spec.IngressPolicyRef == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoExposureClass with publishMode Ingress must select an OpenBaoIngressPolicy.",
		}
	}
	if policy == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoIngressPolicy is required to render ingress exposure inputs.",
		}
	}
	if policy.Name != exposureClass.Spec.IngressPolicyRef.Name {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Loaded OpenBaoIngressPolicy does not match OpenBaoExposureClass.spec.ingressPolicyRef.",
		}
	}

	return &RenderedExposureIngress{
			PolicyRef:                 boundReference(policy.Name, string(policy.UID)),
			ClassName:                 entrypoint.Spec.ObjectRef.Name,
			PathType:                  defaultIngressPathType(policy.Spec.PathType),
			Annotations:               cloneStringMap(policy.Spec.Annotations),
			BackendTLSPublicationMode: ingressBackendTLSPublicationMode(policy.Spec.BackendTLS),
			ReadinessMode:             defaultIngressReadinessMode(policy.Spec.ReadinessMode),
		}, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered ingress execution inputs have been produced from immutable ingress policy inputs.",
		}
}

func renderedHostnamePolicy(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	hostname openbaov1alpha1.OpenBaoExposureHostnamePolicySpec,
) (openbaov1alpha1.OpenBaoExposureHostnamePolicySpec, ValidationResult) {
	result := hostname
	requestedHostname := claimExposureHostname(claim)
	if requestedHostname != "" {
		if result.Claim == nil || !result.Claim.Enabled {
			return openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{}, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "OpenBaoClusterClaim.spec.serviceParameters.exposure.hostname is not allowed by the selected exposure class.",
			}
		}
		if errs := kvalidation.IsDNS1123Subdomain(requestedHostname); len(errs) > 0 {
			return openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{}, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "OpenBaoClusterClaim.spec.serviceParameters.exposure.hostname must be a valid DNS subdomain.",
			}
		}
		if !hostnameAllowedBySuffix(requestedHostname, result) {
			return openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{}, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "OpenBaoClusterClaim.spec.serviceParameters.exposure.hostname is outside the suffixes allowed by the selected exposure class.",
			}
		}
		result.Value = requestedHostname
		return result, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered hostname has been resolved from bounded claim exposure parameters.",
		}
	}
	if result.Mode != openbaov1alpha1.OpenBaoExposureHostnamePolicyModeGenerated {
		return result, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered hostname uses the selected exposure class policy.",
		}
	}
	if strings.TrimSpace(result.DomainSuffix) == "" {
		return result, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered hostname uses the selected exposure class policy.",
		}
	}
	result.Value = claim.Name + "." + strings.TrimSpace(result.DomainSuffix)
	return result, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered hostname has been generated from exposure class policy.",
	}
}

func claimExposureHostname(claim *openbaov1alpha1.OpenBaoClusterClaim) string {
	if claim == nil || claim.Spec.ServiceParameters == nil || claim.Spec.ServiceParameters.Exposure == nil {
		return ""
	}
	return strings.TrimSpace(strings.ToLower(claim.Spec.ServiceParameters.Exposure.Hostname))
}

func hostnameAllowedBySuffix(hostname string, policy openbaov1alpha1.OpenBaoExposureHostnamePolicySpec) bool {
	allowedSuffixes := hostnameAllowedSuffixes(policy)
	for _, suffix := range allowedSuffixes {
		if suffix == "" {
			continue
		}
		if hostname == suffix || strings.HasSuffix(hostname, "."+suffix) {
			return true
		}
	}
	return false
}

func hostnameAllowedSuffixes(policy openbaov1alpha1.OpenBaoExposureHostnamePolicySpec) []string {
	if policy.Claim != nil && len(policy.Claim.AllowedSuffixes) > 0 {
		return normalizeHostnameSuffixes(policy.Claim.AllowedSuffixes)
	}
	return normalizeHostnameSuffixes([]string{policy.DomainSuffix})
}

func normalizeHostnameSuffixes(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.Trim(strings.TrimSpace(strings.ToLower(value)), ".")
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func cloneLocalReference(ref *openbaov1alpha1.LocalReference) *openbaov1alpha1.LocalReference {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}

func cloneEntrypointListenerPolicy(
	policy *openbaov1alpha1.OpenBaoEntrypointListenerPolicySpec,
) *openbaov1alpha1.OpenBaoEntrypointListenerPolicySpec {
	if policy == nil {
		return nil
	}
	copy := *policy
	return &copy
}

func cloneTLSPolicy(policy *openbaov1alpha1.OpenBaoExposureTLSPolicySpec) *openbaov1alpha1.OpenBaoExposureTLSPolicySpec {
	if policy == nil {
		return nil
	}
	copy := *policy
	if policy.CertificateRef != nil {
		ref := *policy.CertificateRef
		copy.CertificateRef = &ref
	}
	if policy.ACME != nil {
		copy.ACME = &openbaov1alpha1.OpenBaoExposureACMEPolicySpec{
			DirectoryURL: policy.ACME.DirectoryURL,
			Domain:       policy.ACME.Domain,
			Domains:      cloneStringSlice(policy.ACME.Domains),
			Email:        policy.ACME.Email,
		}
	}
	return &copy
}

func cloneRouting(routing *openbaov1alpha1.OpenBaoExposureRoutingSpec) *openbaov1alpha1.OpenBaoExposureRoutingSpec {
	if routing == nil {
		return nil
	}
	copy := *routing
	return &copy
}

func cloneServicePolicy(policy *openbaov1alpha1.OpenBaoExposureServicePolicySpec) *openbaov1alpha1.OpenBaoExposureServicePolicySpec {
	if policy == nil {
		return nil
	}
	copy := *policy
	copy.Annotations = cloneStringMap(policy.Annotations)
	return &copy
}

func cloneReadReplicaServicePolicy(policy *openbaov1alpha1.OpenBaoExposureReadReplicaServicePolicySpec) *openbaov1alpha1.OpenBaoExposureReadReplicaServicePolicySpec {
	if policy == nil {
		return nil
	}
	copy := *policy
	copy.Annotations = cloneStringMap(policy.Annotations)
	return &copy
}

func defaultIngressPathType(pathType openbaov1alpha1.IngressPathType) openbaov1alpha1.IngressPathType {
	if pathType == "" {
		return openbaov1alpha1.IngressPathTypePrefix
	}
	return pathType
}

func defaultIngressReadinessMode(mode openbaov1alpha1.IngressReadinessMode) openbaov1alpha1.IngressReadinessMode {
	if mode == "" {
		return openbaov1alpha1.IngressReadinessModeLoadBalancerPublished
	}
	return mode
}

func ingressBackendTLSPublicationMode(
	backendTLS *openbaov1alpha1.OpenBaoIngressPolicyBackendTLSSpec,
) openbaov1alpha1.OpenBaoIngressBackendTLSPublicationMode {
	if backendTLS == nil || backendTLS.PublicationMode == "" {
		return openbaov1alpha1.OpenBaoIngressBackendTLSPublicationModeNone
	}
	return backendTLS.PublicationMode
}
