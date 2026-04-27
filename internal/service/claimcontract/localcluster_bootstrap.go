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
	"fmt"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func renderedSelfInitConfig(
	rendered *RenderedExecutionContract,
	requests []openbaov1alpha1.SelfInitRequest,
) *openbaov1alpha1.SelfInitConfig {
	selfInit := &openbaov1alpha1.SelfInitConfig{
		Enabled:  true,
		Requests: requests,
	}
	if rendered != nil && rendered.Bootstrap.OperatorLifecycleAuth.Mode == openbaov1alpha1.OpenBaoBootstrapLifecycleAuthModeJWT {
		selfInit.OIDC = &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true}
		if rendered.Bootstrap.OperatorLifecycleAuth.JWT != nil {
			selfInit.OIDC.Audience = rendered.Bootstrap.OperatorLifecycleAuth.JWT.Audience
		}
	}
	return selfInit
}

func renderedSelfInitRequests(rendered *RenderedExecutionContract) ([]openbaov1alpha1.SelfInitRequest, ValidationResult) {
	if rendered == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Rendered bootstrap contract is required to build self-init requests.",
		}
	}

	var requests []openbaov1alpha1.SelfInitRequest
	if rendered.Bootstrap.Auth != nil {
		for i, method := range rendered.Bootstrap.Auth.Methods {
			mountPath := strings.TrimPrefix(method.Path, "/")
			requests = append(requests, openbaov1alpha1.SelfInitRequest{
				Name:      fmt.Sprintf("enable-auth-%d", i+1),
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/auth/" + mountPath,
				AuthMethod: &openbaov1alpha1.SelfInitAuthMethod{
					Type:          method.Type,
					ConfigFromRef: cloneTypedObjectReference(method.ConfigFromRef),
				},
			})
		}
	}
	if rendered.Bootstrap.SecretEngines != nil {
		for i, mount := range rendered.Bootstrap.SecretEngines.Mounts {
			requests = append(requests, openbaov1alpha1.SelfInitRequest{
				Name:      fmt.Sprintf("enable-engine-%d", i+1),
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/mounts/" + strings.TrimPrefix(mount.Path, "/"),
				SecretEngine: &openbaov1alpha1.SelfInitSecretEngine{
					Type: mount.Type,
				},
			})
		}
	}
	if rendered.Bootstrap.Policies != nil {
		for i, bundle := range rendered.Bootstrap.Policies.Bundles {
			policyName := strings.Trim(strings.TrimSpace(bundle.Name), "/")
			if policyName == "" {
				return nil, ValidationResult{
					Valid:   false,
					Reason:  openbaov1alpha1.ReasonInvalid,
					Message: "Rendered bootstrap policy bundle must specify a non-empty name.",
				}
			}
			if bundle.ContentFromRef.Name == "" || bundle.ContentFromRef.Kind == "" {
				return nil, ValidationResult{
					Valid:   false,
					Reason:  openbaov1alpha1.ReasonInvalid,
					Message: fmt.Sprintf("Rendered bootstrap policy bundle %q must resolve to a projected content ref.", bundle.Name),
				}
			}
			requests = append(requests, openbaov1alpha1.SelfInitRequest{
				Name:      fmt.Sprintf("apply-policy-%d", i+1),
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/policies/acl/" + policyName,
				Policy: &openbaov1alpha1.SelfInitPolicy{
					ContentFromRef: cloneTypedObjectReference(&bundle.ContentFromRef),
				},
			})
		}
	}
	if rendered.Bootstrap.Audit != nil {
		for i, device := range rendered.Bootstrap.Audit.Devices {
			auditPath := strings.Trim(strings.TrimSpace(device.Path), "/")
			if auditPath == "" {
				return nil, ValidationResult{
					Valid:   false,
					Reason:  openbaov1alpha1.ReasonInvalid,
					Message: "Rendered bootstrap audit device must resolve to a non-empty path.",
				}
			}
			requests = append(requests, openbaov1alpha1.SelfInitRequest{
				Name:      fmt.Sprintf("enable-audit-%d", i+1),
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/audit/" + auditPath,
				AuditDevice: &openbaov1alpha1.SelfInitAuditDevice{
					Type:        device.Type,
					SinkFromRef: cloneTypedObjectReference(device.SinkFromRef),
				},
			})
		}
	}

	return requests, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered bootstrap contract is compatible with same-cluster self-init projection.",
	}
}
