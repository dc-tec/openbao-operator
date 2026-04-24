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

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func renderBootstrapAuth(
	auth *openbaov1alpha1.OpenBaoBootstrapAuthSpec,
	inputs SameClusterBootstrapResolvedInputs,
) (*RenderedBootstrapAuthSpec, ValidationResult) {
	if auth == nil {
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered bootstrap contract does not require auth-method bootstrap entries.",
		}
	}

	rendered := &RenderedBootstrapAuthSpec{}
	if len(auth.Methods) == 0 {
		return rendered, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered bootstrap contract does not require auth-method bootstrap entries.",
		}
	}

	rendered.Methods = make([]RenderedBootstrapAuthMethodSpec, 0, len(auth.Methods))
	for _, method := range auth.Methods {
		entry := RenderedBootstrapAuthMethodSpec{
			Type: method.Type,
			Path: method.Path,
		}
		if method.ConfigRef != nil {
			artifact, ok := inputs.AuthMethodConfigs[BootstrapAuthMethodIdentity(method.Type, method.Path)]
			if !ok {
				return nil, ValidationResult{
					Valid:   false,
					Reason:  openbaov1alpha1.ReasonPending,
					Message: "Rendered bootstrap auth-method dependencies are not fully resolved yet.",
				}
			}
			entry.ConfigFromRef = cloneTypedObjectReference(&artifact.Ref)
		}
		rendered.Methods = append(rendered.Methods, entry)
	}

	return rendered, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered bootstrap auth-method contract is ready for same-cluster execution.",
	}
}

// BootstrapAuthMethodIdentity returns the stable identity key for one bootstrap
// auth-method entry inside the rendered same-cluster contract.
func BootstrapAuthMethodIdentity(methodType, path string) string {
	return strings.TrimSpace(methodType) + "|" + strings.Trim(strings.TrimSpace(path), "/")
}

func renderBootstrapPolicies(
	policies *openbaov1alpha1.OpenBaoBootstrapPoliciesSpec,
	inputs SameClusterBootstrapResolvedInputs,
) (*RenderedBootstrapPoliciesSpec, ValidationResult) {
	if policies == nil {
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered bootstrap contract does not require policy-bundle entries.",
		}
	}

	rendered := &RenderedBootstrapPoliciesSpec{}
	if len(policies.Bundles) == 0 {
		return rendered, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered bootstrap contract does not require policy-bundle entries.",
		}
	}

	rendered.Bundles = make([]RenderedBootstrapPolicyBundleSpec, 0, len(policies.Bundles))
	for _, bundle := range policies.Bundles {
		artifact, ok := inputs.PolicyBundleContents[BootstrapPolicyBundleIdentity(bundle)]
		if !ok {
			return nil, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonPending,
				Message: "Rendered bootstrap policy-bundle dependencies are not fully resolved yet.",
			}
		}
		rendered.Bundles = append(rendered.Bundles, RenderedBootstrapPolicyBundleSpec{
			Name:           bundle.Name,
			ContentFromRef: artifact.Ref,
		})
	}

	return rendered, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered bootstrap policy-bundle contract is ready for same-cluster execution.",
	}
}

// BootstrapPolicyBundleIdentity returns the stable identity key for one bootstrap
// policy-bundle entry inside the rendered same-cluster contract.
func BootstrapPolicyBundleIdentity(bundle openbaov1alpha1.OpenBaoBootstrapPolicyBundleSpec) string {
	return strings.TrimSpace(bundle.Name) + "|" +
		strings.TrimSpace(bundle.ContentRef.Kind) + "|" +
		strings.TrimSpace(bundle.ContentRef.Namespace) + "|" +
		strings.TrimSpace(bundle.ContentRef.Name)
}

func renderBootstrapAudit(
	audit *openbaov1alpha1.OpenBaoBootstrapAuditSpec,
	inputs SameClusterBootstrapResolvedInputs,
) (*RenderedBootstrapAuditSpec, ValidationResult) {
	if audit == nil {
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered bootstrap contract does not require audit-device entries.",
		}
	}

	rendered := &RenderedBootstrapAuditSpec{}
	if len(audit.Devices) == 0 {
		return rendered, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered bootstrap contract does not require audit-device entries.",
		}
	}

	rendered.Devices = make([]RenderedBootstrapAuditDeviceSpec, 0, len(audit.Devices))
	for _, device := range audit.Devices {
		if device.SinkRef == nil {
			return nil, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Rendered bootstrap audit-device projection requires sinkRef-backed sink wiring.",
			}
		}
		sink, ok := inputs.AuditDeviceSinks[BootstrapAuditDeviceIdentity(device)]
		if !ok {
			return nil, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonPending,
				Message: "Rendered bootstrap audit-device dependencies are not fully resolved yet.",
			}
		}
		rendered.Devices = append(rendered.Devices, RenderedBootstrapAuditDeviceSpec{
			Type:        device.Type,
			SinkFromRef: cloneTypedObjectReference(&sink.Artifact.Ref),
			Path:        sink.Path,
		})
	}

	return rendered, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered bootstrap audit-device contract is ready for same-cluster execution.",
	}
}

// BootstrapAuditDeviceIdentity returns the stable identity key for one bootstrap
// audit-device entry inside the rendered same-cluster contract.
func BootstrapAuditDeviceIdentity(device openbaov1alpha1.OpenBaoBootstrapAuditDeviceSpec) string {
	kind, namespace, name := "", "", ""
	if device.SinkRef != nil {
		kind = device.SinkRef.Kind
		namespace = device.SinkRef.Namespace
		name = device.SinkRef.Name
	}
	return strings.TrimSpace(device.Type) + "|" +
		strings.TrimSpace(kind) + "|" +
		strings.TrimSpace(namespace) + "|" +
		strings.TrimSpace(name)
}

func cloneBootstrapSecretEngines(secretEngines *openbaov1alpha1.OpenBaoBootstrapSecretEnginesSpec) *openbaov1alpha1.OpenBaoBootstrapSecretEnginesSpec {
	if secretEngines == nil {
		return nil
	}
	copy := *secretEngines
	if secretEngines.Mounts != nil {
		copy.Mounts = append([]openbaov1alpha1.OpenBaoBootstrapSecretEngineMountSpec(nil), secretEngines.Mounts...)
	}
	return &copy
}

func cloneTypedObjectReference(ref *openbaov1alpha1.TypedObjectReference) *openbaov1alpha1.TypedObjectReference {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}
