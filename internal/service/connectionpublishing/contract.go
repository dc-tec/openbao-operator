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

package connectionpublishing

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const (
	maxMetadataNameLength      = 253
	connectionContractLabelKey = "openbao.org/connection-contract"
	connectionContractLabelVal = "primary"
	localPublicServiceSuffix   = "-public"
)

// PublicationResult summarizes whether the claim-facing connection contract is publishable.
type PublicationResult struct {
	Publishable   bool
	Reason        openbaov1alpha1.ConditionReason
	Message       string
	ShouldRequeue bool
}

// ObservedConnection captures the normalized connection contract the claim path can publish.
type ObservedConnection struct {
	Endpoint    string
	CABundlePEM string
	ObservedAt  *metav1.Time
}

// EvaluateObservedConnection determines whether the reported remote connection contract is complete.
func EvaluateObservedConnection(connection *ObservedConnection) PublicationResult {
	if connection == nil {
		return PublicationResult{
			Publishable: false,
			Reason:      openbaov1alpha1.ReasonPending,
			Message:     "Connection contract has not been observed yet.",
		}
	}
	if strings.TrimSpace(connection.Endpoint) == "" {
		return PublicationResult{
			Publishable: false,
			Reason:      openbaov1alpha1.ReasonPending,
			Message:     "Connection contract is waiting for an endpoint.",
		}
	}
	if strings.TrimSpace(connection.CABundlePEM) == "" {
		return PublicationResult{
			Publishable: false,
			Reason:      openbaov1alpha1.ReasonPending,
			Message:     "Connection contract is waiting for CA bundle material.",
		}
	}
	if connection.ObservedAt == nil {
		return PublicationResult{
			Publishable: false,
			Reason:      openbaov1alpha1.ReasonPending,
			Message:     "Connection contract is waiting for an observation timestamp.",
		}
	}

	return PublicationResult{
		Publishable: true,
		Reason:      openbaov1alpha1.ReasonReady,
		Message:     "Remote connection contract has been observed.",
	}
}

// EvaluateLocalPublication determines whether the local same-cluster connection contract can be published safely.
func EvaluateLocalPublication(
	cluster *openbaov1alpha1.OpenBaoCluster,
	publicService *corev1.Service,
	caSecret *corev1.Secret,
) PublicationResult {
	if cluster == nil {
		return PublicationResult{
			Publishable: false,
			Reason:      openbaov1alpha1.ReasonPending,
			Message:     "Connection publication is waiting for the local concrete workload.",
		}
	}
	if cluster.Status.Phase == openbaov1alpha1.ClusterPhaseFailed {
		return PublicationResult{
			Publishable: false,
			Reason:      openbaov1alpha1.ReasonInvalid,
			Message:     "Connection publication is blocked because the local concrete workload has failed.",
		}
	}
	if cluster.Status.Phase != openbaov1alpha1.ClusterPhaseRunning &&
		cluster.Status.Phase != openbaov1alpha1.ClusterPhaseBackingUp {
		return PublicationResult{
			Publishable: false,
			Reason:      openbaov1alpha1.ReasonPending,
			Message:     "Connection publication is waiting for the local concrete workload to become ready.",
		}
	}
	if publicService == nil {
		return PublicationResult{
			Publishable: false,
			Reason:      openbaov1alpha1.ReasonPending,
			Message:     "Connection publication is waiting for the local public Service.",
		}
	}
	if caSecret == nil {
		return PublicationResult{
			Publishable: false,
			Reason:      openbaov1alpha1.ReasonPending,
			Message:     "Connection publication is waiting for the local TLS CA Secret.",
		}
	}
	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled && !gatewayReady(cluster) {
		return PublicationResult{
			Publishable: false,
			Reason:      openbaov1alpha1.ReasonPending,
			Message:     "Connection publication is waiting for local gateway integration readiness.",
		}
	}
	if cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled && !ingressReady(cluster) {
		return PublicationResult{
			Publishable: false,
			Reason:      openbaov1alpha1.ReasonPending,
			Message:     "Connection publication is waiting for local ingress integration readiness.",
		}
	}

	return EvaluateObservedConnection(ObservedLocalConnection(cluster, publicService, caSecret))
}

// SecretName returns the deterministic claim-owned Secret name for the connection contract.
func SecretName(claimName string) string {
	base := strings.TrimSpace(claimName)
	if base == "" {
		base = "claim"
	}
	base = base + "-connection"
	if len(base) <= maxMetadataNameLength {
		return base
	}

	hash := sha256.Sum256([]byte(base))
	hashSuffix := hex.EncodeToString(hash[:])[:12]
	prefixLength := maxMetadataNameLength - len(hashSuffix) - 1
	if prefixLength < 1 {
		return "connection-" + hashSuffix
	}

	return base[:prefixLength] + "-" + hashSuffix
}

// DesiredLocalClaimConnection projects the local observed connection contract into the claim-facing status contract.
func DesiredLocalClaimConnection(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
	publicService *corev1.Service,
	caSecret *corev1.Secret,
) openbaov1alpha1.OpenBaoClusterClaimConnectionStatus {
	return desiredClaimConnection(claim, ObservedLocalConnection(cluster, publicService, caSecret))
}

func desiredClaimConnection(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	connection *ObservedConnection,
) openbaov1alpha1.OpenBaoClusterClaimConnectionStatus {
	secretName := SecretName(claim.Name)

	return openbaov1alpha1.OpenBaoClusterClaimConnectionStatus{
		Endpoint: connection.Endpoint,
		CABundleRef: &openbaov1alpha1.TypedObjectReference{
			Kind: "Secret",
			Name: secretName,
		},
		SecretRef: &openbaov1alpha1.LocalReference{
			Name: secretName,
		},
		ObservedAt: connection.ObservedAt.DeepCopy(),
	}
}

// DesiredLocalSecret projects the local observed connection report into the minimal claim-owned Secret contract.
func DesiredLocalSecret(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
	publicService *corev1.Service,
	caSecret *corev1.Secret,
) *corev1.Secret {
	return desiredSecret(claim, ObservedLocalConnection(cluster, publicService, caSecret))
}

func desiredSecret(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	connection *ObservedConnection,
) *corev1.Secret {
	secretName := SecretName(claim.Name)

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1ObjectMeta(claim.Namespace, secretName, claim.Name),
		Type:       corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"endpoint": []byte(connection.Endpoint),
			"ca.crt":   []byte(connection.CABundlePEM),
		},
	}
}

// LocalPublicServiceName returns the deterministic same-cluster public Service name.
func LocalPublicServiceName(clusterName string) string {
	return strings.TrimSpace(clusterName) + localPublicServiceSuffix
}

// LocalCASecretName returns the deterministic same-cluster TLS CA Secret name.
func LocalCASecretName(clusterName string) string {
	return strings.TrimSpace(clusterName) + constants.SuffixTLSCA
}

// LocalEndpoint returns the deterministic in-cluster HTTPS endpoint for a same-cluster workload.
func LocalEndpoint(service *corev1.Service) string {
	if service == nil {
		return ""
	}
	return fmt.Sprintf("https://%s.%s.svc:%d", service.Name, service.Namespace, constants.PortAPI)
}

// LocalClaimEndpoint returns the preferred published endpoint for a same-cluster
// claim-managed workload.
func LocalClaimEndpoint(cluster *openbaov1alpha1.OpenBaoCluster, service *corev1.Service) string {
	if cluster != nil {
		if endpoint := gatewayEndpoint(cluster); endpoint != "" {
			return endpoint
		}
		if endpoint := ingressEndpoint(cluster); endpoint != "" {
			return endpoint
		}
	}
	return LocalEndpoint(service)
}

// ObservedLocalConnection normalizes the same-cluster connection contract from the concrete workload resources.
func ObservedLocalConnection(
	cluster *openbaov1alpha1.OpenBaoCluster,
	publicService *corev1.Service,
	caSecret *corev1.Secret,
) *ObservedConnection {
	if cluster == nil || publicService == nil || caSecret == nil {
		return nil
	}
	caPEM, ok := caSecret.Data["ca.crt"]
	endpoint := LocalClaimEndpoint(cluster, publicService)
	if !ok || len(caPEM) == 0 {
		return &ObservedConnection{
			Endpoint:   endpoint,
			ObservedAt: observedAtFromLocalInputs(publicService, caSecret),
		}
	}

	return &ObservedConnection{
		Endpoint:    endpoint,
		CABundlePEM: string(caPEM),
		ObservedAt:  observedAtFromLocalInputs(publicService, caSecret),
	}
}

func gatewayEndpoint(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Spec.Gateway == nil || !cluster.Spec.Gateway.Enabled {
		return ""
	}
	if !gatewayReady(cluster) {
		return ""
	}
	return externalEndpoint(cluster.Spec.Gateway.Hostname, cluster.Spec.Gateway.Path)
}

func ingressEndpoint(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Spec.Ingress == nil || !cluster.Spec.Ingress.Enabled {
		return ""
	}
	if !ingressReady(cluster) {
		return ""
	}
	return externalEndpoint(cluster.Spec.Ingress.Host, cluster.Spec.Ingress.Path)
}

func externalEndpoint(hostname, path string) string {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return ""
	}

	u := &url.URL{
		Scheme: "https",
		Host:   hostname,
	}
	path = strings.TrimSpace(path)
	switch {
	case path == "", path == "/":
		return u.String()
	case strings.HasPrefix(path, "/"):
		u.Path = path
	default:
		u.Path = "/" + path
	}
	return u.String()
}

func gatewayReady(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil {
		return false
	}
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionGatewayIntegrationReady))
	return condition != nil && condition.Status == metav1.ConditionTrue
}

func ingressReady(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil {
		return false
	}
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionIngressIntegrationReady))
	return condition != nil && condition.Status == metav1.ConditionTrue
}

func observedAtFromLocalInputs(service *corev1.Service, caSecret *corev1.Secret) *metav1.Time {
	var observed *metav1.Time
	if service != nil && !service.CreationTimestamp.IsZero() {
		observed = service.CreationTimestamp.DeepCopy()
	}
	if caSecret != nil && !caSecret.CreationTimestamp.IsZero() {
		if observed == nil || caSecret.CreationTimestamp.After(observed.Time) {
			observed = caSecret.CreationTimestamp.DeepCopy()
		}
	}
	return observed
}

func metav1ObjectMeta(namespace, secretName, claimName string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Namespace: namespace,
		Name:      secretName,
		Labels: map[string]string{
			constants.LabelAppManagedBy:          constants.LabelValueAppManagedByOpenBaoOperator,
			constants.LabelOpenBaoClaimNamespace: namespace,
			constants.LabelOpenBaoClaimName:      claimName,
			connectionContractLabelKey:           connectionContractLabelVal,
		},
	}
}
