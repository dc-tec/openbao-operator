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

func renderedNetworkConfig(rendered *RenderedExecutionContract) (*openbaov1alpha1.NetworkConfig, ValidationResult) {
	if rendered == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Rendered execution contract is required to build same-cluster network configuration.",
		}
	}

	egressRules := cloneEgressRules(rendered.Network.EgressRules)
	egressRules = append(egressRules, cloneEgressRules(rendered.Network.RequiredEgressRules)...)
	apiServerCIDR := strings.TrimSpace(rendered.Network.APIServerCIDR)
	apiServerEndpointIPs := cloneStringSlice(rendered.Network.APIServerEndpointIPs)
	dnsNamespace := strings.TrimSpace(rendered.Network.DNSNamespace)
	dnsEndpointIPs := cloneStringSlice(rendered.Network.DNSEndpointIPs)
	ingressRules := cloneIngressRules(rendered.Network.IngressRules)
	trustedIngressPeers := cloneNetworkPolicyPeers(rendered.Network.TrustedIngressPeers)
	if len(egressRules) == 0 {
		if rendered.Cluster.SecurityProfile == openbaov1alpha1.ProfileHardened &&
			strings.TrimSpace(rendered.Backup.Schedule) != "" &&
			rendered.Backup.TargetRef != nil {
			return nil, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Same-cluster hardened backup projection requires rendered network egress rules.",
			}
		}
		if apiServerCIDR == "" && len(apiServerEndpointIPs) == 0 && dnsNamespace == "" && len(dnsEndpointIPs) == 0 &&
			len(ingressRules) == 0 && len(trustedIngressPeers) == 0 {
			return nil, ValidationResult{
				Valid:   true,
				Reason:  openbaov1alpha1.ReasonAccepted,
				Message: "Rendered execution contract does not require explicit additional network egress rules.",
			}
		}
	}

	return &openbaov1alpha1.NetworkConfig{
			APIServerCIDR:        apiServerCIDR,
			APIServerEndpointIPs: apiServerEndpointIPs,
			DNSNamespace:         dnsNamespace,
			DNSEndpointIPs:       dnsEndpointIPs,
			EgressRules:          egressRules,
			IngressRules:         ingressRules,
			TrustedIngressPeers:  trustedIngressPeers,
		}, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered execution contract network inputs are compatible with OpenBaoCluster.",
		}
}
