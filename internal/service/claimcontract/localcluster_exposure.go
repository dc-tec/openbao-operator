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

func renderedTLSConfig(rendered *RenderedExecutionContract) (openbaov1alpha1.TLSConfig, ValidationResult) {
	mode := openbaov1alpha1.TLSModeOperatorManaged
	if rendered != nil && rendered.Exposure.TLSPolicy != nil {
		switch rendered.Exposure.TLSPolicy.Mode {
		case openbaov1alpha1.OpenBaoExposureTLSModeExternal:
			mode = openbaov1alpha1.TLSModeExternal
		case openbaov1alpha1.OpenBaoExposureTLSModeACME:
			mode = openbaov1alpha1.TLSModeACME
		}
	}

	tls := openbaov1alpha1.TLSConfig{
		Enabled: true,
		Mode:    mode,
	}
	if mode == openbaov1alpha1.TLSModeOperatorManaged {
		tls.RotationPeriod = defaultClaimManagedTLSRotationPeriod
	}
	if mode == openbaov1alpha1.TLSModeACME {
		acme, validation := renderedACMEConfig(rendered)
		if !validation.Valid {
			return openbaov1alpha1.TLSConfig{}, validation
		}
		tls.ACME = acme
	}

	return tls, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered TLS exposure contract is compatible with OpenBaoCluster.",
	}
}

func renderedACMEConfig(rendered *RenderedExecutionContract) (*openbaov1alpha1.ACMEConfig, ValidationResult) {
	if rendered == nil || rendered.Exposure.TLSPolicy == nil || rendered.Exposure.TLSPolicy.ACME == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster ACME TLS projection requires a rendered ACME exposure policy.",
		}
	}
	policy := rendered.Exposure.TLSPolicy.ACME
	acme := &openbaov1alpha1.ACMEConfig{
		DirectoryURL: strings.TrimSpace(policy.DirectoryURL),
		Domain:       strings.TrimSpace(policy.Domain),
		Domains:      cloneStringSlice(policy.Domains),
		Email:        strings.TrimSpace(policy.Email),
		SharedCache:  cloneACMESharedCacheConfig(rendered.Storage.ACMECache),
	}
	if strings.TrimSpace(acme.DirectoryURL) == "" {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster ACME TLS projection requires spec.tlsPolicy.acme.directoryURL.",
		}
	}
	if rendered.Cluster.Replicas > 1 || rendered.Lifecycle.UpgradeStrategy == openbaov1alpha1.UpdateStrategyBlueGreen {
		if acme.SharedCache == nil {
			return nil, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Same-cluster ACME TLS projection requires storageProfile.spec.acmeCache for HA or blue/green topologies.",
			}
		}
	}
	return acme, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered ACME TLS exposure policy is compatible with OpenBaoCluster.",
	}
}

func renderedGatewayConfigForExposure(
	rendered *RenderedExecutionContract,
) (*openbaov1alpha1.GatewayConfig, ValidationResult) {
	if rendered == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Rendered execution contract is required to build same-cluster exposure configuration.",
		}
	}

	switch rendered.Exposure.PublishMode {
	case openbaov1alpha1.OpenBaoExposurePublishModeClusterInternal:
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered execution contract uses cluster-internal exposure only.",
		}
	case openbaov1alpha1.OpenBaoExposurePublishModeGateway:
		return renderedGatewayConfig(rendered)
	case openbaov1alpha1.OpenBaoExposurePublishModeIngress:
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered execution contract uses ingress exposure instead of gateway exposure.",
		}
	default:
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: fmt.Sprintf("Unsupported rendered exposure posture %q for same-cluster materialization.", rendered.Exposure.PublishMode),
		}
	}
}

func renderedIngressConfigForExposure(
	rendered *RenderedExecutionContract,
) (*openbaov1alpha1.IngressConfig, ValidationResult) {
	if rendered == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Rendered execution contract is required to build same-cluster ingress configuration.",
		}
	}

	switch rendered.Exposure.PublishMode {
	case openbaov1alpha1.OpenBaoExposurePublishModeClusterInternal, openbaov1alpha1.OpenBaoExposurePublishModeGateway:
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered execution contract does not require ingress exposure.",
		}
	case openbaov1alpha1.OpenBaoExposurePublishModeIngress:
		return renderedIngressConfig(rendered)
	default:
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: fmt.Sprintf("Unsupported rendered exposure posture %q for same-cluster materialization.", rendered.Exposure.PublishMode),
		}
	}
}

func renderedIngressConfig(rendered *RenderedExecutionContract) (*openbaov1alpha1.IngressConfig, ValidationResult) {
	if rendered == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Rendered execution contract is required to build same-cluster ingress configuration.",
		}
	}
	hostname := strings.TrimSpace(rendered.Exposure.HostnamePolicy.Value)
	if hostname == "" {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster ingress projection requires a rendered hostname.",
		}
	}
	if rendered.Exposure.Entrypoint == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster ingress projection requires a rendered OpenBaoEntrypoint.",
		}
	}
	if rendered.Exposure.Entrypoint.Mode != openbaov1alpha1.OpenBaoEntrypointModeIngress {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster ingress projection requires an OpenBaoEntrypoint with mode Ingress.",
		}
	}
	if strings.TrimSpace(rendered.Exposure.Entrypoint.ObjectRef.APIGroup) != "networking.k8s.io" ||
		strings.TrimSpace(rendered.Exposure.Entrypoint.ObjectRef.Kind) != "IngressClass" ||
		strings.TrimSpace(rendered.Exposure.Entrypoint.ObjectRef.Name) == "" {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster ingress projection requires the rendered OpenBaoEntrypoint to reference a concrete IngressClass.",
		}
	}
	if rendered.Exposure.Ingress == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster ingress projection requires rendered ingress execution inputs.",
		}
	}
	if rendered.Exposure.ServicePolicy != nil &&
		rendered.Exposure.ServicePolicy.Type != openbaov1alpha1.OpenBaoExposureServiceTypeClusterIP {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster ingress projection currently supports only ClusterIP service publication.",
		}
	}
	if rendered.Exposure.ServicePolicy != nil &&
		rendered.Exposure.ServicePolicy.BackendTLSMode == openbaov1alpha1.OpenBaoExposureBackendTLSModeRequired &&
		rendered.Exposure.Ingress.BackendTLSPublicationMode != openbaov1alpha1.OpenBaoIngressBackendTLSPublicationModeAnnotation {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster ingress projection requires explicit backend-TLS publication behavior when backend TLS is required.",
		}
	}

	ingress := &openbaov1alpha1.IngressConfig{
		Enabled:       true,
		ClassName:     cloneStringPtr(&rendered.Exposure.Ingress.ClassName),
		Host:          hostname,
		Path:          renderedPath(rendered.Exposure.Routing),
		PathType:      rendered.Exposure.Ingress.PathType,
		Annotations:   cloneStringMap(rendered.Exposure.Ingress.Annotations),
		ReadinessMode: rendered.Exposure.Ingress.ReadinessMode,
	}
	tlsSecretName, validation := renderedIngressTLSSecretName(rendered)
	if !validation.Valid {
		return nil, validation
	}
	ingress.TLSSecretName = tlsSecretName

	return ingress, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered ingress exposure contract is compatible with OpenBaoCluster.",
	}
}

func renderedIngressTLSSecretName(rendered *RenderedExecutionContract) (string, ValidationResult) {
	if rendered == nil || rendered.Exposure.TLSPolicy == nil {
		return "", ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered ingress exposure uses the default operator-managed TLS Secret.",
		}
	}
	if rendered.Exposure.TLSPolicy.Mode != openbaov1alpha1.OpenBaoExposureTLSModeExternal {
		return "", ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered ingress exposure uses the default operator-managed TLS Secret.",
		}
	}
	if rendered.Exposure.TLSPolicy.CertificateRef == nil {
		return "", ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster ingress projection requires a Secret certificateRef when external TLS is selected.",
		}
	}
	if strings.TrimSpace(rendered.Exposure.TLSPolicy.CertificateRef.Kind) != "Secret" ||
		strings.TrimSpace(rendered.Exposure.TLSPolicy.CertificateRef.Name) == "" {
		return "", ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster ingress projection currently supports only Secret-backed external TLS certificates.",
		}
	}

	return strings.TrimSpace(rendered.Exposure.TLSPolicy.CertificateRef.Name), ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered ingress exposure uses an explicit external TLS Secret.",
	}
}

func renderedPath(routing *openbaov1alpha1.OpenBaoExposureRoutingSpec) string {
	if routing == nil || strings.TrimSpace(routing.Path) == "" {
		return "/"
	}
	return strings.TrimSpace(routing.Path)
}

func renderedGatewayConfig(rendered *RenderedExecutionContract) (*openbaov1alpha1.GatewayConfig, ValidationResult) {
	if rendered == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Rendered execution contract is required to build same-cluster gateway configuration.",
		}
	}

	hostname := strings.TrimSpace(rendered.Exposure.HostnamePolicy.Value)
	if hostname == "" {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster gateway projection requires a rendered hostname.",
		}
	}
	if rendered.Exposure.Entrypoint == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster gateway projection requires a rendered OpenBaoEntrypoint.",
		}
	}
	if rendered.Exposure.Entrypoint.Mode != openbaov1alpha1.OpenBaoEntrypointModeGateway {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster gateway projection requires an OpenBaoEntrypoint with mode Gateway.",
		}
	}
	if strings.TrimSpace(rendered.Exposure.Entrypoint.ObjectRef.Kind) != "Gateway" ||
		strings.TrimSpace(rendered.Exposure.Entrypoint.ObjectRef.Name) == "" {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster gateway projection requires the rendered OpenBaoEntrypoint to reference a concrete Gateway object.",
		}
	}
	if strings.TrimSpace(rendered.Exposure.Entrypoint.ObjectRef.APIGroup) != "gateway.networking.k8s.io" {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster gateway projection currently supports only gateway.networking.k8s.io Gateway entrypoints.",
		}
	}

	tlsPassthrough := rendered.Exposure.Routing != nil && rendered.Exposure.Routing.TLSPassthrough
	gateway := &openbaov1alpha1.GatewayConfig{
		Enabled: true,
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name:      strings.TrimSpace(rendered.Exposure.Entrypoint.ObjectRef.Name),
			Namespace: strings.TrimSpace(rendered.Exposure.Entrypoint.ObjectRef.Namespace),
		},
		Hostname:       hostname,
		TLSPassthrough: tlsPassthrough,
		Annotations:    cloneStringMap(rendered.Exposure.GatewayAnnotations),
	}
	if rendered.Exposure.Entrypoint.ListenerPolicy != nil {
		gateway.ListenerName = strings.TrimSpace(rendered.Exposure.Entrypoint.ListenerPolicy.SectionName)
	}
	if rendered.Exposure.Routing != nil {
		gateway.Path = strings.TrimSpace(rendered.Exposure.Routing.Path)
	}
	if !tlsPassthrough {
		enabled := true
		if rendered.Exposure.ServicePolicy != nil &&
			rendered.Exposure.ServicePolicy.BackendTLSMode == openbaov1alpha1.OpenBaoExposureBackendTLSModeDisabled {
			enabled = false
		}
		gateway.BackendTLS = &openbaov1alpha1.BackendTLSConfig{Enabled: &enabled}
	}

	return gateway, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered gateway exposure contract is compatible with OpenBaoCluster.",
	}
}

func renderedServiceConfig(rendered *RenderedExecutionContract) *openbaov1alpha1.ServiceConfig {
	service := &openbaov1alpha1.ServiceConfig{}
	if rendered == nil || rendered.Exposure.ServicePolicy == nil || rendered.Exposure.ServicePolicy.Type == "" {
		return service
	}
	service.Type = corev1.ServiceType(rendered.Exposure.ServicePolicy.Type)
	service.Annotations = cloneStringMap(rendered.Exposure.ServicePolicy.Annotations)
	return service
}
