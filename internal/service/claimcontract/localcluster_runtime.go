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
