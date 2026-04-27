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

	corev1 "k8s.io/api/core/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func renderedUnsealConfig(rendered *RenderedExecutionContract) (*openbaov1alpha1.UnsealConfig, ValidationResult) {
	if rendered == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Rendered execution contract is required to build same-cluster unseal configuration.",
		}
	}

	switch rendered.Unseal.Mode {
	case "", UnsealPostureModeManagedStatic:
		if rendered.Unseal.Config != nil {
			return rendered.Unseal.Config.DeepCopy(), ValidationResult{
				Valid:   true,
				Reason:  openbaov1alpha1.ReasonAccepted,
				Message: "Rendered operator-managed static unseal profile is compatible with OpenBaoCluster.",
			}
		}
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered execution contract uses operator-managed static unseal defaults.",
		}
	case UnsealPostureModeExternal:
		if rendered.Exposure.TLSPolicy == nil || !renderedUnsealCompatibleTLSMode(rendered.Exposure.TLSPolicy.Mode) {
			return nil, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Same-cluster hardened materialization requires External or ACME TLS exposure posture.",
			}
		}
		if rendered.Unseal.Config != nil {
			return rendered.Unseal.Config.DeepCopy(), ValidationResult{
				Valid:   true,
				Reason:  openbaov1alpha1.ReasonAccepted,
				Message: "Rendered unseal profile is compatible with OpenBaoCluster.",
			}
		}
		if rendered.Unseal.Transit == nil {
			return nil, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Rendered external-unseal posture must resolve to a concrete same-cluster transit unseal configuration.",
			}
		}
		if strings.TrimSpace(rendered.Unseal.Transit.Address) == "" ||
			strings.TrimSpace(rendered.Unseal.Transit.KeyName) == "" ||
			strings.TrimSpace(rendered.Unseal.Transit.MountPath) == "" ||
			strings.TrimSpace(rendered.Unseal.Transit.CredentialsSecretName) == "" {
			return nil, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Rendered same-cluster transit unseal configuration is incomplete.",
			}
		}

		return &openbaov1alpha1.UnsealConfig{
				Type: "transit",
				Transit: &openbaov1alpha1.TransitSealConfig{
					Address:       rendered.Unseal.Transit.Address,
					KeyName:       rendered.Unseal.Transit.KeyName,
					MountPath:     rendered.Unseal.Transit.MountPath,
					Namespace:     rendered.Unseal.Transit.Namespace,
					TLSCACert:     rendered.Unseal.Transit.TLSCACert,
					TLSServerName: rendered.Unseal.Transit.TLSServerName,
				},
				CredentialsSecretRef: &corev1.LocalObjectReference{Name: rendered.Unseal.Transit.CredentialsSecretName},
			}, ValidationResult{
				Valid:   true,
				Reason:  openbaov1alpha1.ReasonAccepted,
				Message: "Rendered same-cluster transit unseal contract is compatible with OpenBaoCluster.",
			}
	default:
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: fmt.Sprintf("Unsupported rendered unseal posture %q for same-cluster materialization.", rendered.Unseal.Mode),
		}
	}
}

func renderedUnsealCompatibleTLSMode(mode openbaov1alpha1.OpenBaoExposureTLSMode) bool {
	return mode == openbaov1alpha1.OpenBaoExposureTLSModeExternal ||
		mode == openbaov1alpha1.OpenBaoExposureTLSModeACME
}
