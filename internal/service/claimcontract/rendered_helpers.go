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

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func cloneACMESharedCacheConfig(config *openbaov1alpha1.ACMESharedCacheConfig) *openbaov1alpha1.ACMESharedCacheConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func cloneApprovedRuntime(runtime ApprovedRuntime) ApprovedRuntime {
	return ApprovedRuntime{
		ProfileRef:                cloneLocalReference(runtime.ProfileRef),
		ServiceAccount:            cloneServiceAccountConfig(runtime.ServiceAccount),
		PodMetadata:               clonePodMetadataConfig(runtime.PodMetadata),
		ImagePullSecrets:          cloneLocalObjectReferenceSlice(runtime.ImagePullSecrets),
		ImageVerification:         cloneImageVerificationConfig(runtime.ImageVerification),
		OperatorImageVerification: cloneImageVerificationConfig(runtime.OperatorImageVerification),
		WorkloadHardening:         cloneWorkloadHardeningConfig(runtime.WorkloadHardening),
		SecurityContext:           clonePodSecurityContext(runtime.SecurityContext),
		HelperImages:              cloneHelperImages(runtime.HelperImages),
		ReadReplica:               cloneRuntimeReadReplica(runtime.ReadReplica),
	}
}

func cloneApprovedObservability(observability ApprovedObservability) ApprovedObservability {
	return ApprovedObservability{
		ProfileRef:    cloneLocalReference(observability.ProfileRef),
		Observability: cloneObservabilityConfig(observability.Observability),
		Telemetry:     cloneTelemetryConfig(observability.Telemetry),
	}
}
