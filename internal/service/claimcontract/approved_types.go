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

// ApprovedServiceContract captures service semantics after immutable catalog binding.
type ApprovedServiceContract struct {
	Cluster       ApprovedCluster
	Unseal        ApprovedUnseal
	Storage       ApprovedStorage
	Runtime       ApprovedRuntime
	Observability ApprovedObservability
	Network       ApprovedNetwork
	Bootstrap     ApprovedBootstrap
	Exposure      ApprovedExposure
	Backup        ApprovedBackup
	Lifecycle     ApprovedLifecycle
	Provenance    ApprovedServiceProvenance
}

// ApprovedCluster captures the approved core service shape.
type ApprovedCluster struct {
	Version         string
	Voters          int32
	ReadReplicas    int32
	SecurityProfile openbaov1alpha1.Profile
}

// UnsealPostureMode identifies the internal unseal posture required by a claim contract.
type UnsealPostureMode string

const (
	// UnsealPostureModeManagedStatic uses the operator-managed static seal posture.
	UnsealPostureModeManagedStatic UnsealPostureMode = "ManagedStatic"
	// UnsealPostureModeExternal requires an external non-static seal posture.
	UnsealPostureModeExternal UnsealPostureMode = "External"
)

// ApprovedUnseal captures the approved unseal posture derived from service semantics.
type ApprovedUnseal struct {
	Mode       UnsealPostureMode
	ProfileRef *openbaov1alpha1.LocalReference
	Config     *openbaov1alpha1.UnsealConfig
}

// ApprovedStorage captures approved storage capacity semantics.
type ApprovedStorage struct {
	PrimarySize          string
	ReadReplicaSize      string
	PrimaryClassName     *string
	ReadReplicaClassName *string
	ACMECache            *openbaov1alpha1.ACMESharedCacheConfig
	ProfileRef           *openbaov1alpha1.LocalReference
}

// ApprovedRuntime captures approved runtime integration semantics.
type ApprovedRuntime struct {
	ProfileRef                *openbaov1alpha1.LocalReference
	ServiceAccount            *openbaov1alpha1.ServiceAccountConfig
	PodMetadata               *openbaov1alpha1.PodMetadataConfig
	ImagePullSecrets          []corev1.LocalObjectReference
	ImageVerification         *openbaov1alpha1.ImageVerificationConfig
	OperatorImageVerification *openbaov1alpha1.ImageVerificationConfig
	WorkloadHardening         *openbaov1alpha1.WorkloadHardeningConfig
	SecurityContext           *corev1.PodSecurityContext
	HelperImages              *openbaov1alpha1.OpenBaoRuntimeProfileHelperImagesSpec
	ReadReplica               *openbaov1alpha1.OpenBaoRuntimeProfileReadReplicaSpec
}

// ApprovedObservability captures approved metrics and telemetry semantics.
type ApprovedObservability struct {
	ProfileRef    *openbaov1alpha1.LocalReference
	Observability *openbaov1alpha1.ObservabilityConfig
	Telemetry     *openbaov1alpha1.TelemetryConfig
}

// ApprovedNetwork captures approved network dependency semantics.
type ApprovedNetwork struct {
	ProfileRef           *openbaov1alpha1.LocalReference
	APIServerCIDR        string
	APIServerEndpointIPs []string
	DNSNamespace         string
	DNSEndpointIPs       []string
	EgressRules          []networkingv1.NetworkPolicyEgressRule
	IngressRules         []networkingv1.NetworkPolicyIngressRule
	TrustedIngressPeers  []networkingv1.NetworkPolicyPeer
}

// ApprovedBootstrap captures the approved bootstrap posture.
type ApprovedBootstrap struct {
	Mode       openbaov1alpha1.OpenBaoBootstrapMode
	ProfileRef *openbaov1alpha1.LocalReference
}

// ApprovedExposure captures the approved exposure posture.
type ApprovedExposure struct {
	ClassRef openbaov1alpha1.LocalReference
}

// ApprovedBackup captures the approved backup posture.
type ApprovedBackup struct {
	ProfileRef openbaov1alpha1.LocalReference
	Parameters ApprovedBackupParameters
}

// ApprovedBackupParameters captures the typed claim-facing backup parameter surface.
type ApprovedBackupParameters struct {
	Location  string
	Partition string
}

// ApprovedLifecycle captures approved steady-state lifecycle posture.
type ApprovedLifecycle struct {
	UpgradeStrategy    openbaov1alpha1.UpdateStrategyType
	PreUpgradeSnapshot bool
	PolicyRef          *openbaov1alpha1.LocalReference
	BlueGreen          *openbaov1alpha1.BlueGreenConfig
}

// ApprovedServiceProvenance captures the bound immutable catalog revisions.
type ApprovedServiceProvenance struct {
	ServiceProfileRef       openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	BootstrapProfileRef     *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	ExposureClassRef        *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	StorageProfileRef       *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	UnsealProfileRef        *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	RuntimeProfileRef       *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	ObservabilityProfileRef *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	NetworkProfileRef       *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	UpgradePolicyRef        *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	BackupProfileRef        *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
}
