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
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func cloneLocalObjectReference(ref *corev1.LocalObjectReference) *corev1.LocalObjectReference {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}

func cloneLocalObjectReferenceSlice(refs []corev1.LocalObjectReference) []corev1.LocalObjectReference {
	if refs == nil {
		return nil
	}
	copy := make([]corev1.LocalObjectReference, len(refs))
	copy = append(copy[:0], refs...)
	return copy
}

func cloneStaticSealConfig(config *openbaov1alpha1.StaticSealConfig) *openbaov1alpha1.StaticSealConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func cloneServiceAccountConfig(config *openbaov1alpha1.ServiceAccountConfig) *openbaov1alpha1.ServiceAccountConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func clonePodMetadataConfig(config *openbaov1alpha1.PodMetadataConfig) *openbaov1alpha1.PodMetadataConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func cloneImageVerificationConfig(config *openbaov1alpha1.ImageVerificationConfig) *openbaov1alpha1.ImageVerificationConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func cloneWorkloadHardeningConfig(config *openbaov1alpha1.WorkloadHardeningConfig) *openbaov1alpha1.WorkloadHardeningConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func clonePodSecurityContext(context *corev1.PodSecurityContext) *corev1.PodSecurityContext {
	if context == nil {
		return nil
	}
	return context.DeepCopy()
}

func cloneHelperImages(images *openbaov1alpha1.OpenBaoRuntimeProfileHelperImagesSpec) *openbaov1alpha1.OpenBaoRuntimeProfileHelperImagesSpec {
	if images == nil {
		return nil
	}
	copy := *images
	return &copy
}

func cloneRuntimeReadReplica(readReplica *openbaov1alpha1.OpenBaoRuntimeProfileReadReplicaSpec) *openbaov1alpha1.OpenBaoRuntimeProfileReadReplicaSpec {
	if readReplica == nil {
		return nil
	}
	copy := *readReplica
	copy.Template = cloneReadReplicaTemplateConfig(readReplica.Template)
	return &copy
}

func cloneReadReplicaTemplateConfig(template *openbaov1alpha1.ReadReplicaTemplateConfig) *openbaov1alpha1.ReadReplicaTemplateConfig {
	if template == nil {
		return nil
	}
	return template.DeepCopy()
}

func cloneObservabilityConfig(config *openbaov1alpha1.ObservabilityConfig) *openbaov1alpha1.ObservabilityConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func cloneTelemetryConfig(config *openbaov1alpha1.TelemetryConfig) *openbaov1alpha1.TelemetryConfig {
	if config == nil {
		return nil
	}
	return config.DeepCopy()
}

func cloneIngressRules(rules []networkingv1.NetworkPolicyIngressRule) []networkingv1.NetworkPolicyIngressRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]networkingv1.NetworkPolicyIngressRule, len(rules))
	for i := range rules {
		rules[i].DeepCopyInto(&out[i])
	}
	return out
}

func cloneNetworkPolicyPeers(peers []networkingv1.NetworkPolicyPeer) []networkingv1.NetworkPolicyPeer {
	if len(peers) == 0 {
		return nil
	}
	out := make([]networkingv1.NetworkPolicyPeer, len(peers))
	for i := range peers {
		peers[i].DeepCopyInto(&out[i])
	}
	return out
}

func backupLocation(claim *openbaov1alpha1.OpenBaoClusterClaim) string {
	if claim == nil || claim.Spec.ServiceParameters == nil || claim.Spec.ServiceParameters.Backup == nil {
		return ""
	}

	return claim.Spec.ServiceParameters.Backup.Location
}

func backupPartition(claim *openbaov1alpha1.OpenBaoClusterClaim) string {
	if claim == nil || claim.Spec.ServiceParameters == nil || claim.Spec.ServiceParameters.Backup == nil {
		return ""
	}

	return claim.Spec.ServiceParameters.Backup.Partition
}

func derefInt32(value *int32) int32 {
	if value == nil {
		return 0
	}

	return *value
}

func derefBool(value *bool) bool {
	if value == nil {
		return false
	}

	return *value
}

func derefBoolDefaultTrue(value *bool) bool {
	if value == nil {
		return true
	}

	return *value
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
