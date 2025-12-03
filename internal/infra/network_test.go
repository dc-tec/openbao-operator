package infra

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

func TestEnsureHeadlessServiceCreatesAndUpdates(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-headless", "default")

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	headless := &corev1.Service{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      headlessServiceName(cluster),
	}, headless)
	if err != nil {
		t.Fatalf("expected headless Service to exist: %v", err)
	}

	if headless.Spec.ClusterIP != corev1.ClusterIPNone {
		t.Fatalf("expected headless Service ClusterIP None, got %q", headless.Spec.ClusterIP)
	}

	if len(headless.Spec.Ports) == 0 {
		t.Fatalf("expected headless Service to have ports")
	}

	if headless.Spec.Ports[0].Port != openBaoContainerPort {
		t.Fatalf("expected headless Service port %d, got %d", openBaoContainerPort, headless.Spec.Ports[0].Port)
	}
}

func TestEnsureExternalServiceCreatesWhenConfigured(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-external", "default")
	cluster.Spec.Service = &openbaov1alpha1.ServiceConfig{
		Type: corev1.ServiceTypeLoadBalancer,
		Annotations: map[string]string{
			"service-annotation": "true",
		},
	}

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	external := &corev1.Service{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      externalServiceName(cluster),
	}, external)
	if err != nil {
		t.Fatalf("expected external Service to exist: %v", err)
	}

	if external.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("expected external Service type %q, got %q", corev1.ServiceTypeLoadBalancer, external.Spec.Type)
	}

	if external.Annotations["service-annotation"] != "true" {
		t.Fatalf("expected external Service annotation to be propagated")
	}
}

func TestEnsureExternalServiceDeletesWhenNotNeeded(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-external-delete", "default")
	cluster.Spec.Service = &openbaov1alpha1.ServiceConfig{
		Type: corev1.ServiceTypeLoadBalancer,
	}

	ctx := context.Background()

	// First reconcile creates the service
	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Remove service config
	cluster.Spec.Service = nil
	cluster.Spec.Ingress = nil
	cluster.Spec.Gateway = nil

	// Second reconcile should delete the service
	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify service is deleted
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      externalServiceName(cluster),
	}, &corev1.Service{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected external Service to be deleted, got error: %v", err)
	}
}

func TestEnsureExternalServiceCreatedWhenIngressEnabled(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-external-ingress", "default")
	cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
		Enabled: true,
		Host:    "bao.example.com",
	}

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	external := &corev1.Service{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      externalServiceName(cluster),
	}, external)
	if err != nil {
		t.Fatalf("expected external Service to exist when Ingress is enabled: %v", err)
	}
}

func TestEnsureExternalServiceCreatedWhenGatewayEnabled(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("gateway-external-svc", "gateway-ns")
	cluster.Status.Initialized = true
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled: true,
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name: "traefik-gateway",
		},
		Hostname: "bao.example.local",
	}

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	svc := &corev1.Service{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      externalServiceName(cluster),
	}, svc)
	if err != nil {
		t.Fatalf("expected external Service %s to be created when Gateway is enabled, got error: %v", externalServiceName(cluster), err)
	}
}

func TestEnsureIngressCreatesWhenEnabled(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	host := "bao.example.com"
	cluster := newMinimalCluster("infra-network", "openbao")
	cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
		Enabled: true,
		Host:    host,
		Path:    "/",
		Annotations: map[string]string{
			"ingress-annotation": "true",
		},
	}

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	ingress := &networkingv1.Ingress{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}, ingress)
	if err != nil {
		t.Fatalf("expected Ingress to exist: %v", err)
	}

	if len(ingress.Spec.Rules) != 1 || ingress.Spec.Rules[0].Host != host {
		t.Fatalf("expected Ingress host %q, got %#v", host, ingress.Spec.Rules)
	}

	if len(ingress.Spec.TLS) == 0 || ingress.Spec.TLS[0].SecretName != tlsServerSecretName(cluster) {
		t.Fatalf("expected Ingress TLS SecretName %q, got %#v", tlsServerSecretName(cluster), ingress.Spec.TLS)
	}

	if ingress.Annotations["ingress-annotation"] != "true" {
		t.Fatalf("expected Ingress annotations to be propagated")
	}
}

func TestEnsureIngressDeletesWhenDisabled(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-ingress-disable", "default")
	cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
		Enabled: true,
		Host:    "bao.example.com",
	}

	ctx := context.Background()

	// First reconcile creates the Ingress
	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Disable Ingress
	cluster.Spec.Ingress.Enabled = false

	// Second reconcile should delete the Ingress
	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify Ingress is deleted
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}, &networkingv1.Ingress{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected Ingress to be deleted, got error: %v", err)
	}
}

func TestEnsureHTTPRouteCreatesWhenEnabled(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-httproute", "default")
	cluster.Status.Initialized = true
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled: true,
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name:      "traefik-gateway",
			Namespace: "gateway-system",
		},
		Hostname: "bao.example.local",
		Path:     "/",
	}

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	httpRoute := &gatewayv1.HTTPRoute{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      httpRouteName(cluster),
	}, httpRoute)
	if err != nil {
		t.Fatalf("expected HTTPRoute to exist: %v", err)
	}

	if len(httpRoute.Spec.Hostnames) == 0 || string(httpRoute.Spec.Hostnames[0]) != "bao.example.local" {
		t.Fatalf("expected HTTPRoute hostname %q, got %#v", "bao.example.local", httpRoute.Spec.Hostnames)
	}

	if len(httpRoute.Spec.ParentRefs) == 0 || string(httpRoute.Spec.ParentRefs[0].Name) != "traefik-gateway" {
		t.Fatalf("expected HTTPRoute parent ref %q, got %#v", "traefik-gateway", httpRoute.Spec.ParentRefs)
	}
}

func TestEnsureHTTPRouteDeletesWhenDisabled(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-httproute-disable", "default")
	cluster.Status.Initialized = true
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled: true,
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name: "traefik-gateway",
		},
		Hostname: "bao.example.local",
	}

	ctx := context.Background()

	// First reconcile creates the HTTPRoute
	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Disable Gateway
	cluster.Spec.Gateway.Enabled = false

	// Second reconcile should delete the HTTPRoute
	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify HTTPRoute is deleted
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      httpRouteName(cluster),
	}, &gatewayv1.HTTPRoute{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected HTTPRoute to be deleted, got error: %v", err)
	}
}

func TestEnsureGatewayCAConfigMapCreatesWhenGatewayEnabled(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-gateway-ca", "default")
	cluster.Status.Initialized = true
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled: true,
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name: "traefik-gateway",
		},
		Hostname: "bao.example.local",
	}

	ctx := context.Background()

	// Create CA Secret first (normally created by cert manager)
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tlsCASecretName(cluster),
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("-----BEGIN CERTIFICATE-----\nMOCK CA CERT\n-----END CERTIFICATE-----\n"),
		},
	}
	if err := k8sClient.Create(ctx, caSecret); err != nil {
		t.Fatalf("failed to create CA Secret: %v", err)
	}

	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	configMap := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + tlsCASecretSuffix,
	}, configMap)
	if err != nil {
		t.Fatalf("expected Gateway CA ConfigMap to exist: %v", err)
	}

	caCert, ok := configMap.Data["ca.crt"]
	if !ok {
		t.Fatalf("expected Gateway CA ConfigMap to contain ca.crt")
	}

	if caCert != "-----BEGIN CERTIFICATE-----\nMOCK CA CERT\n-----END CERTIFICATE-----\n" {
		t.Fatalf("expected Gateway CA ConfigMap to contain CA certificate from Secret")
	}
}

func TestEnsureGatewayCAConfigMapDeletesWhenGatewayDisabled(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-gateway-ca-disable", "default")
	cluster.Status.Initialized = true
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled: true,
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name: "traefik-gateway",
		},
		Hostname: "bao.example.local",
	}

	ctx := context.Background()

	// Create CA Secret
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tlsCASecretName(cluster),
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("-----BEGIN CERTIFICATE-----\nMOCK CA CERT\n-----END CERTIFICATE-----\n"),
		},
	}
	if err := k8sClient.Create(ctx, caSecret); err != nil {
		t.Fatalf("failed to create CA Secret: %v", err)
	}

	// First reconcile creates the ConfigMap
	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Disable Gateway
	cluster.Spec.Gateway.Enabled = false

	// Second reconcile should delete the ConfigMap
	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify ConfigMap is deleted
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + tlsCASecretSuffix,
	}, &corev1.ConfigMap{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected Gateway CA ConfigMap to be deleted, got error: %v", err)
	}
}

func TestEnsureGatewayCAConfigMapUpdatesWhenCASecretChanges(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-gateway-ca-update", "default")
	cluster.Status.Initialized = true
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled: true,
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name: "traefik-gateway",
		},
		Hostname: "bao.example.local",
	}

	ctx := context.Background()

	// Create CA Secret with first certificate
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tlsCASecretName(cluster),
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("-----BEGIN CERTIFICATE-----\nFIRST CA CERT\n-----END CERTIFICATE-----\n"),
		},
	}
	if err := k8sClient.Create(ctx, caSecret); err != nil {
		t.Fatalf("failed to create CA Secret: %v", err)
	}

	// First reconcile creates the ConfigMap
	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Update CA Secret
	caSecret.Data["ca.crt"] = []byte("-----BEGIN CERTIFICATE-----\nUPDATED CA CERT\n-----END CERTIFICATE-----\n")
	if err := k8sClient.Update(ctx, caSecret); err != nil {
		t.Fatalf("failed to update CA Secret: %v", err)
	}

	// Second reconcile should update the ConfigMap
	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify ConfigMap is updated
	configMap := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + tlsCASecretSuffix,
	}, configMap)
	if err != nil {
		t.Fatalf("expected Gateway CA ConfigMap to exist: %v", err)
	}

	caCert := configMap.Data["ca.crt"]
	if caCert != "-----BEGIN CERTIFICATE-----\nUPDATED CA CERT\n-----END CERTIFICATE-----\n" {
		t.Fatalf("expected Gateway CA ConfigMap to contain updated CA certificate")
	}
}
