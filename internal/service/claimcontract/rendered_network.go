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

func applySameClusterTransitUnseal(
	rendered *RenderedExecutionContract,
	defaults SameClusterTransitUnsealDefaults,
) ValidationResult {
	if rendered == nil {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Rendered execution contract is required to apply same-cluster transit unseal defaults.",
		}
	}
	if !defaults.configured() {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Hardened same-cluster materialization requires operator-configured transit unseal defaults.",
		}
	}

	rendered.Unseal.Transit = &RenderedTransitUnseal{
		Address:               defaults.Address,
		KeyName:               defaults.KeyName,
		MountPath:             defaults.MountPath,
		Namespace:             defaults.Namespace,
		TLSCACert:             defaults.TLSCACert,
		TLSServerName:         defaults.TLSServerName,
		CredentialsSecretName: defaults.CredentialsSecretName,
	}

	return ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered same-cluster external unseal posture has been resolved to transit defaults.",
	}
}

func (d SameClusterTransitUnsealDefaults) configured() bool {
	return strings.TrimSpace(d.Address) != "" &&
		strings.TrimSpace(d.KeyName) != "" &&
		strings.TrimSpace(d.MountPath) != "" &&
		strings.TrimSpace(d.CredentialsSecretName) != ""
}

// ApplySameClusterNetworkDefaults merges operator-configured same-cluster network defaults
// into the rendered execution contract.
func ApplySameClusterNetworkDefaults(rendered *RenderedExecutionContract, defaults SameClusterNetworkDefaults) {
	if rendered == nil {
		return
	}
	if strings.TrimSpace(rendered.Network.APIServerCIDR) == "" {
		rendered.Network.APIServerCIDR = strings.TrimSpace(defaults.APIServerCIDR)
	}
	if len(rendered.Network.APIServerEndpointIPs) == 0 {
		rendered.Network.APIServerEndpointIPs = cloneStringSlice(defaults.APIServerEndpointIPs)
	}
	if len(rendered.Network.DNSEndpointIPs) == 0 {
		rendered.Network.DNSEndpointIPs = cloneStringSlice(defaults.DNSEndpointIPs)
	}
}

func renderedNetwork(approved ApprovedNetwork, renderedBackup RenderedBackup) RenderedNetwork {
	return RenderedNetwork{
		RequiredEgressRules:  renderedRequiredEgressRules(renderedBackup),
		APIServerCIDR:        strings.TrimSpace(approved.APIServerCIDR),
		APIServerEndpointIPs: cloneStringSlice(approved.APIServerEndpointIPs),
		DNSNamespace:         strings.TrimSpace(approved.DNSNamespace),
		DNSEndpointIPs:       cloneStringSlice(approved.DNSEndpointIPs),
		EgressRules:          cloneEgressRules(approved.EgressRules),
		IngressRules:         cloneIngressRules(approved.IngressRules),
		TrustedIngressPeers:  cloneNetworkPolicyPeers(approved.TrustedIngressPeers),
	}
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
