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

func bindApprovedRuntime(profile *openbaov1alpha1.OpenBaoRuntimeProfile) ApprovedRuntime {
	if profile == nil {
		return ApprovedRuntime{}
	}
	return ApprovedRuntime{
		ProfileRef:                &openbaov1alpha1.LocalReference{Name: profile.Name},
		ServiceAccount:            cloneServiceAccountConfig(profile.Spec.ServiceAccount),
		PodMetadata:               clonePodMetadataConfig(profile.Spec.PodMetadata),
		ImagePullSecrets:          cloneLocalObjectReferenceSlice(profile.Spec.ImagePullSecrets),
		ImageVerification:         cloneImageVerificationConfig(profile.Spec.ImageVerification),
		OperatorImageVerification: cloneImageVerificationConfig(profile.Spec.OperatorImageVerification),
		WorkloadHardening:         cloneWorkloadHardeningConfig(profile.Spec.WorkloadHardening),
		SecurityContext:           clonePodSecurityContext(profile.Spec.SecurityContext),
		HelperImages:              cloneHelperImages(profile.Spec.HelperImages),
		ReadReplica:               cloneRuntimeReadReplica(profile.Spec.ReadReplica),
	}
}

func bindApprovedObservability(profile *openbaov1alpha1.OpenBaoObservabilityProfile) ApprovedObservability {
	if profile == nil {
		return ApprovedObservability{}
	}
	return ApprovedObservability{
		ProfileRef:    &openbaov1alpha1.LocalReference{Name: profile.Name},
		Observability: cloneObservabilityConfig(profile.Spec.Observability),
		Telemetry:     cloneTelemetryConfig(profile.Spec.Telemetry),
	}
}

func bindApprovedNetwork(profile *openbaov1alpha1.OpenBaoNetworkProfile) ApprovedNetwork {
	if profile == nil {
		return ApprovedNetwork{}
	}
	return ApprovedNetwork{
		ProfileRef:           &openbaov1alpha1.LocalReference{Name: profile.Name},
		APIServerCIDR:        strings.TrimSpace(profile.Spec.APIServerCIDR),
		APIServerEndpointIPs: cloneStringSlice(profile.Spec.APIServerEndpointIPs),
		DNSNamespace:         strings.TrimSpace(profile.Spec.DNSNamespace),
		DNSEndpointIPs:       cloneStringSlice(profile.Spec.DNSEndpointIPs),
		EgressRules:          cloneEgressRules(profile.Spec.EgressRules),
		IngressRules:         cloneIngressRules(profile.Spec.IngressRules),
		TrustedIngressPeers:  cloneNetworkPolicyPeers(profile.Spec.TrustedIngressPeers),
	}
}
