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
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const testConnectionSecretName = "payments-bao-connection"
const (
	testClaimNamespace = "payments"
	testClaimName      = "payments-bao"
	testExternalHost   = "payments-bao.example.internal"
	testLocalEndpoint  = "https://payments-bao-public.payments.svc:8200"
)

func readyLocalPublicationInputs(namespace, clusterName string) (*corev1.Service, *corev1.Secret) {
	return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:              LocalPublicServiceName(clusterName),
				Namespace:         namespace,
				CreationTimestamp: metav1.NewTime(time.Date(2026, time.April, 20, 17, 0, 0, 0, time.UTC)),
			},
		}, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:              LocalCASecretName(clusterName),
				Namespace:         namespace,
				CreationTimestamp: metav1.NewTime(time.Date(2026, time.April, 20, 18, 0, 0, 0, time.UTC)),
			},
			Data: map[string][]byte{
				"ca.crt": []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"),
			},
		}
}

func assertLocalPublicationWaitsForIntegrationReady(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster) {
	t.Helper()

	service, caSecret := readyLocalPublicationInputs(cluster.Namespace, cluster.Name)
	result := EvaluateLocalPublication(cluster, service, caSecret)
	if result.Publishable || result.Reason != openbaov1alpha1.ReasonPending {
		t.Fatalf("EvaluateLocalPublication() = %#v, want pending while integration is not ready", result)
	}
}

func assertDesiredLocalClaimConnectionUsesExternalEndpoint(
	t *testing.T,
	cluster *openbaov1alpha1.OpenBaoCluster,
	wantEndpoint string,
) {
	t.Helper()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{}
	claim.Namespace = testClaimNamespace
	claim.Name = testClaimName

	service, caSecret := readyLocalPublicationInputs(cluster.Namespace, cluster.Name)
	connection := DesiredLocalClaimConnection(claim, cluster, service, caSecret)
	if connection.Endpoint != wantEndpoint {
		t.Fatalf("connection.Endpoint = %q, want %q", connection.Endpoint, wantEndpoint)
	}

	secret := DesiredLocalSecret(claim, cluster, service, caSecret)
	if string(secret.Data["endpoint"]) != connection.Endpoint {
		t.Fatalf("secret endpoint = %q, want %q", string(secret.Data["endpoint"]), connection.Endpoint)
	}
}

func TestEvaluateObservedConnection(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Date(2026, time.April, 20, 18, 0, 0, 0, time.UTC))

	tests := []struct {
		name       string
		connection *ObservedConnection
		want       PublicationResult
	}{
		{
			name:       "nil connection",
			connection: nil,
			want: PublicationResult{
				Publishable: false,
				Reason:      openbaov1alpha1.ReasonPending,
				Message:     "Connection contract has not been observed yet.",
			},
		},
		{
			name: "missing endpoint",
			connection: &ObservedConnection{
				CABundlePEM: "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
				ObservedAt:  &now,
			},
			want: PublicationResult{
				Publishable: false,
				Reason:      openbaov1alpha1.ReasonPending,
				Message:     "Connection contract is waiting for an endpoint.",
			},
		},
		{
			name: "missing ca bundle",
			connection: &ObservedConnection{
				Endpoint:   "https://payments-bao.example.internal",
				ObservedAt: &now,
			},
			want: PublicationResult{
				Publishable: false,
				Reason:      openbaov1alpha1.ReasonPending,
				Message:     "Connection contract is waiting for CA bundle material.",
			},
		},
		{
			name: "missing observation time",
			connection: &ObservedConnection{
				Endpoint:    "https://payments-bao.example.internal",
				CABundlePEM: "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
			},
			want: PublicationResult{
				Publishable: false,
				Reason:      openbaov1alpha1.ReasonPending,
				Message:     "Connection contract is waiting for an observation timestamp.",
			},
		},
		{
			name: "complete contract",
			connection: &ObservedConnection{
				Endpoint:    "https://payments-bao.example.internal",
				CABundlePEM: "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
				ObservedAt:  &now,
			},
			want: PublicationResult{
				Publishable: true,
				Reason:      openbaov1alpha1.ReasonReady,
				Message:     "Remote connection contract has been observed.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EvaluateObservedConnection(tt.connection); got != tt.want {
				t.Fatalf("EvaluateObservedConnection() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestEvaluateLocalPublication(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if result := EvaluateLocalPublication(cluster, nil, nil); result.Publishable || result.Reason != openbaov1alpha1.ReasonPending {
		t.Fatalf("EvaluateLocalPublication() = %#v, want pending before running", result)
	}

	cluster.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "payments-bao-public", Namespace: "payments"}}
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "payments-bao-tls-ca",
			Namespace:         "payments",
			CreationTimestamp: metav1.NewTime(time.Date(2026, time.April, 20, 18, 0, 0, 0, time.UTC)),
		},
		Data: map[string][]byte{"ca.crt": []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n")},
	}
	service.CreationTimestamp = metav1.NewTime(time.Date(2026, time.April, 20, 17, 0, 0, 0, time.UTC))
	if result := EvaluateLocalPublication(cluster, service, caSecret); !result.Publishable {
		t.Fatalf("EvaluateLocalPublication() = %#v, want publishable", result)
	}

	cluster.Status.Phase = openbaov1alpha1.ClusterPhaseBackingUp
	if result := EvaluateLocalPublication(cluster, service, caSecret); !result.Publishable {
		t.Fatalf("EvaluateLocalPublication() during backup = %#v, want publishable", result)
	}
}

func TestEvaluateLocalPublicationGatewayWaitsForIntegrationReady(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name = testClaimName
	cluster.Namespace = testClaimNamespace
	cluster.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled:  true,
		Hostname: testExternalHost,
	}

	assertLocalPublicationWaitsForIntegrationReady(t, cluster)
}

func TestEvaluateLocalPublicationIngressWaitsForIntegrationReady(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name = testClaimName
	cluster.Namespace = testClaimNamespace
	cluster.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
	cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
		Enabled: true,
		Host:    testExternalHost,
	}

	assertLocalPublicationWaitsForIntegrationReady(t, cluster)
}

func TestSecretName(t *testing.T) {
	t.Parallel()

	if got := SecretName("payments-bao"); got != testConnectionSecretName {
		t.Fatalf("SecretName() = %q, want %q", got, testConnectionSecretName)
	}

	longName := strings.Repeat("a", maxMetadataNameLength)
	if got := SecretName(longName); len(got) > maxMetadataNameLength {
		t.Fatalf("len(SecretName()) = %d, want <= %d", len(got), maxMetadataNameLength)
	}
}

func TestDesiredLocalClaimConnectionAndSecret(t *testing.T) {
	t.Parallel()

	claim := &openbaov1alpha1.OpenBaoClusterClaim{}
	claim.Namespace = testClaimNamespace
	claim.Name = testClaimName

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name = testClaimName
	cluster.Namespace = testClaimNamespace
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:              LocalPublicServiceName(cluster.Name),
			Namespace:         cluster.Namespace,
			CreationTimestamp: metav1.NewTime(time.Date(2026, time.April, 20, 17, 0, 0, 0, time.UTC)),
		},
	}
	now := metav1.NewTime(time.Date(2026, time.April, 20, 18, 0, 0, 0, time.UTC))
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              LocalCASecretName(cluster.Name),
			Namespace:         cluster.Namespace,
			CreationTimestamp: now,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"),
		},
	}

	connection := DesiredLocalClaimConnection(claim, cluster, service, caSecret)
	if connection.Endpoint != testLocalEndpoint {
		t.Fatalf("connection.Endpoint = %q, want local service endpoint", connection.Endpoint)
	}
	if connection.SecretRef == nil || connection.SecretRef.Name != testConnectionSecretName {
		t.Fatalf("connection.SecretRef = %#v, want %s", connection.SecretRef, testConnectionSecretName)
	}
	if connection.CABundleRef == nil || connection.CABundleRef.Name != testConnectionSecretName || connection.CABundleRef.Kind != "Secret" {
		t.Fatalf("connection.CABundleRef = %#v, want Secret/%s", connection.CABundleRef, testConnectionSecretName)
	}
	if connection.ObservedAt == nil || !connection.ObservedAt.Equal(&now) {
		t.Fatalf("connection.ObservedAt = %#v, want %v", connection.ObservedAt, now)
	}

	secret := DesiredLocalSecret(claim, cluster, service, caSecret)
	if string(secret.Data["endpoint"]) != connection.Endpoint {
		t.Fatalf("secret endpoint = %q, want %q", string(secret.Data["endpoint"]), connection.Endpoint)
	}
	if string(secret.Data["ca.crt"]) != string(caSecret.Data["ca.crt"]) {
		t.Fatalf("secret ca.crt = %q, want %q", string(secret.Data["ca.crt"]), string(caSecret.Data["ca.crt"]))
	}
}

func TestDesiredLocalClaimConnectionAndSecretUsesGatewayHostname(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name = testClaimName
	cluster.Namespace = testClaimNamespace
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled:  true,
		Hostname: testExternalHost,
		Path:     "/vault",
	}
	cluster.Status.Conditions = []metav1.Condition{{
		Type:   string(openbaov1alpha1.ConditionGatewayIntegrationReady),
		Status: metav1.ConditionTrue,
	}}

	assertDesiredLocalClaimConnectionUsesExternalEndpoint(t, cluster, "https://payments-bao.example.internal/vault")
}

func TestDesiredLocalClaimConnectionAndSecretUsesIngressHostname(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name = testClaimName
	cluster.Namespace = testClaimNamespace
	cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
		Enabled: true,
		Host:    testExternalHost,
		Path:    "/vault",
	}
	cluster.Status.Conditions = []metav1.Condition{{
		Type:   string(openbaov1alpha1.ConditionIngressIntegrationReady),
		Status: metav1.ConditionTrue,
	}}

	assertDesiredLocalClaimConnectionUsesExternalEndpoint(t, cluster, "https://payments-bao.example.internal/vault")
}

func TestLocalClaimEndpointFallsBackToServiceWhenGatewayNotReady(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name = testClaimName
	cluster.Namespace = testClaimNamespace
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled:  true,
		Hostname: testExternalHost,
		Path:     "/vault",
	}

	service, _ := readyLocalPublicationInputs(cluster.Namespace, cluster.Name)
	if got := LocalClaimEndpoint(cluster, service); got != "https://payments-bao-public.payments.svc:8200" {
		t.Fatalf("LocalClaimEndpoint() = %q, want local service endpoint", got)
	}
}

func TestLocalClaimEndpointFallsBackToServiceWhenIngressNotReady(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name = testClaimName
	cluster.Namespace = testClaimNamespace
	cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
		Enabled: true,
		Host:    testExternalHost,
		Path:    "/vault",
	}

	service, _ := readyLocalPublicationInputs(cluster.Namespace, cluster.Name)
	if got := LocalClaimEndpoint(cluster, service); got != "https://payments-bao-public.payments.svc:8200" {
		t.Fatalf("LocalClaimEndpoint() = %q, want local service endpoint", got)
	}
}

func TestObservedLocalConnectionUsesLatestLocalInputTimestamp(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name = testClaimName
	cluster.Namespace = testClaimNamespace

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:              LocalPublicServiceName(cluster.Name),
			Namespace:         cluster.Namespace,
			CreationTimestamp: metav1.NewTime(time.Date(2026, time.April, 20, 19, 0, 0, 0, time.UTC)),
		},
	}
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              LocalCASecretName(cluster.Name),
			Namespace:         cluster.Namespace,
			CreationTimestamp: metav1.NewTime(time.Date(2026, time.April, 20, 18, 0, 0, 0, time.UTC)),
		},
		Data: map[string][]byte{
			"ca.crt": []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"),
		},
	}

	connection := ObservedLocalConnection(cluster, service, caSecret)
	if connection == nil {
		t.Fatal("ObservedLocalConnection() = nil")
	}
	if connection.ObservedAt == nil {
		t.Fatal("connection.ObservedAt = nil")
	}
	want := metav1.NewTime(time.Date(2026, time.April, 20, 19, 0, 0, 0, time.UTC))
	if !connection.ObservedAt.Equal(&want) {
		t.Fatalf("connection.ObservedAt = %v, want %v", connection.ObservedAt, want)
	}
}
