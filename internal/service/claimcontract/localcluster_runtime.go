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
	"k8s.io/apimachinery/pkg/api/resource"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func renderedStorageSpec(rendered *RenderedExecutionContract) (openbaov1alpha1.StorageConfig, ValidationResult) {
	if rendered == nil || strings.TrimSpace(rendered.Storage.PrimarySize) == "" {
		return openbaov1alpha1.StorageConfig{}, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Rendered execution contract must specify primary storage for same-cluster materialization.",
		}
	}

	return openbaov1alpha1.StorageConfig{
			Size:             rendered.Storage.PrimarySize,
			StorageClassName: cloneStringPtr(rendered.Storage.ClassName),
		}, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered primary storage contract is compatible with OpenBaoCluster.",
		}
}

func renderedReadReplicas(rendered *RenderedExecutionContract) (*openbaov1alpha1.ReadReplicaConfig, ValidationResult) {
	if rendered == nil || rendered.Cluster.ReadReplicas <= 0 {
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered contract does not require a read-replica pool.",
		}
	}

	readReplicas := &openbaov1alpha1.ReadReplicaConfig{
		Replicas: rendered.Cluster.ReadReplicas,
	}
	if rendered.Runtime.ReadReplica != nil {
		readReplicas.Template = cloneReadReplicaTemplateConfig(rendered.Runtime.ReadReplica.Template)
	}
	if rendered.Exposure.ReadReplicaServicePolicy != nil && rendered.Exposure.ReadReplicaServicePolicy.Enabled {
		readReplicas.Service = &openbaov1alpha1.ReadReplicaServiceConfig{
			Enabled:     true,
			Type:        corev1.ServiceType(rendered.Exposure.ReadReplicaServicePolicy.Type),
			Annotations: cloneStringMap(rendered.Exposure.ReadReplicaServicePolicy.Annotations),
		}
	}
	readReplicaSize := strings.TrimSpace(rendered.Storage.ReadReplicaSize)
	if readReplicaSize == "" && rendered.Storage.ReadReplicaClassName == nil {
		return readReplicas, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered read-replica pool uses default storage sizing.",
		}
	}

	readReplicas.Storage = &openbaov1alpha1.ReadReplicaStorageConfig{
		StorageClassName: cloneStringPtr(rendered.Storage.ReadReplicaClassName),
	}
	if readReplicaSize != "" {
		size, err := resource.ParseQuantity(readReplicaSize)
		if err != nil {
			return nil, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: fmt.Sprintf("Rendered read-replica storage size %q is not a valid Kubernetes quantity.", rendered.Storage.ReadReplicaSize),
			}
		}
		readReplicas.Storage.Size = &size
	}

	return readReplicas, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered read-replica contract is compatible with OpenBaoCluster.",
	}
}

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

func applyRenderedRuntime(spec *openbaov1alpha1.OpenBaoClusterSpec, runtime ApprovedRuntime) {
	if spec == nil {
		return
	}
	spec.ServiceAccount = cloneServiceAccountConfig(runtime.ServiceAccount)
	spec.PodMetadata = clonePodMetadataConfig(runtime.PodMetadata)
	spec.ImagePullSecrets = cloneLocalObjectReferenceSlice(runtime.ImagePullSecrets)
	spec.ImageVerification = cloneImageVerificationConfig(runtime.ImageVerification)
	spec.OperatorImageVerification = cloneImageVerificationConfig(runtime.OperatorImageVerification)
	spec.WorkloadHardening = cloneWorkloadHardeningConfig(runtime.WorkloadHardening)
	spec.SecurityContext = clonePodSecurityContext(runtime.SecurityContext)
	applyRenderedHelperImages(spec, runtime.HelperImages)
}

func renderedUpgradeConfig(rendered *RenderedExecutionContract) *openbaov1alpha1.UpgradeConfig {
	upgrade := &openbaov1alpha1.UpgradeConfig{
		Strategy:           rendered.Lifecycle.UpgradeStrategy,
		PreUpgradeSnapshot: rendered.Lifecycle.PreUpgradeSnapshot,
	}
	if rendered.Runtime.HelperImages != nil {
		upgrade.Image = strings.TrimSpace(rendered.Runtime.HelperImages.Upgrade)
	}
	if rendered.Lifecycle.UpgradeStrategy == openbaov1alpha1.UpdateStrategyBlueGreen && rendered.Lifecycle.BlueGreen != nil {
		upgrade.BlueGreen = rendered.Lifecycle.BlueGreen.DeepCopy()
		if rendered.Lifecycle.PreUpgradeSnapshot {
			upgrade.BlueGreen.PreUpgradeSnapshot = true
		}
	}
	return upgrade
}

func applyRenderedHelperImages(spec *openbaov1alpha1.OpenBaoClusterSpec, images *openbaov1alpha1.OpenBaoRuntimeProfileHelperImagesSpec) {
	if spec == nil || images == nil {
		return
	}
	if image := strings.TrimSpace(images.Init); image != "" {
		spec.InitContainer = &openbaov1alpha1.InitContainerConfig{
			Enabled: true,
			Image:   image,
		}
	}
	if image := strings.TrimSpace(images.Restore); image != "" {
		spec.Restore = &openbaov1alpha1.RestoreConfig{Image: image}
	}
}

func applyRenderedObservability(spec *openbaov1alpha1.OpenBaoClusterSpec, observability ApprovedObservability) {
	if spec == nil {
		return
	}
	spec.Observability = cloneObservabilityConfig(observability.Observability)
	spec.Telemetry = cloneTelemetryConfig(observability.Telemetry)
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

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
