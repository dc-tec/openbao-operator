package networking

import (
	"context"
	"errors"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const testIngressClassName = "nginx"

func TestValidateIngressIntegration(t *testing.T) {
	t.Parallel()

	newIngressClass := func() *networkingv1.IngressClass {
		return &networkingv1.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: testIngressClassName}}
	}
	newIngress := func(addresses ...string) *networkingv1.Ingress {
		statusAddresses := make([]networkingv1.IngressLoadBalancerIngress, 0, len(addresses))
		for _, address := range addresses {
			statusAddresses = append(statusAddresses, networkingv1.IngressLoadBalancerIngress{Hostname: address})
		}
		return &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
			Status: networkingv1.IngressStatus{
				LoadBalancer: networkingv1.IngressLoadBalancerStatus{Ingress: statusAddresses},
			},
		}
	}

	tests := []struct {
		name      string
		cluster   *openbaov1alpha1.OpenBaoCluster
		objects   []client.Object
		wantError error
	}{
		{
			name:    "disabled ingress returns nil",
			cluster: newMinimalCluster("example", "default"),
		},
		{
			name: "load balancer published succeeds",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				className := testIngressClassName
				cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
					Enabled:       true,
					ClassName:     &className,
					Host:          "bao.example.test",
					ReadinessMode: openbaov1alpha1.IngressReadinessModeLoadBalancerPublished,
				}
				return cluster
			}(),
			objects: []client.Object{newIngressClass(), newIngress("lb.example.test")},
		},
		{
			name: "missing ingress class is explicit",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				className := testIngressClassName
				cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
					Enabled:   true,
					ClassName: &className,
					Host:      "bao.example.test",
				}
				return cluster
			}(),
			objects:   []client.Object{newIngress()},
			wantError: ErrIngressClassMissing,
		},
		{
			name: "load balancer pending is explicit",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				className := testIngressClassName
				cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
					Enabled:       true,
					ClassName:     &className,
					Host:          "bao.example.test",
					ReadinessMode: openbaov1alpha1.IngressReadinessModeLoadBalancerPublished,
				}
				return cluster
			}(),
			objects:   []client.Object{newIngressClass(), newIngress()},
			wantError: ErrIngressLoadBalancerPending,
		},
		{
			name: "created readiness mode succeeds without load balancer address",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				className := testIngressClassName
				cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
					Enabled:       true,
					ClassName:     &className,
					Host:          "bao.example.test",
					ReadinessMode: openbaov1alpha1.IngressReadinessModeCreated,
				}
				return cluster
			}(),
			objects: []client.Object{newIngressClass(), newIngress()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(testScheme)
			if len(tt.objects) > 0 {
				builder = builder.WithObjects(tt.objects...)
			}

			k8sClient := builder.Build()
			manager := NewManagerWithReader(k8sClient, k8sClient, testScheme, "openbao-operator-system", "")
			err := manager.ValidateIngressIntegration(context.Background(), tt.cluster)
			if tt.wantError == nil {
				if err != nil {
					t.Fatalf("ValidateIngressIntegration() error = %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("ValidateIngressIntegration() error = %v, want %v", err, tt.wantError)
			}
		})
	}
}

func TestBuildIngressUsesConfiguredPathType(t *testing.T) {
	t.Parallel()

	cluster := newMinimalCluster("example", "default")
	className := testIngressClassName
	cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
		Enabled:   true,
		ClassName: &className,
		Host:      "bao.example.test",
		Path:      "/vault",
		PathType:  openbaov1alpha1.IngressPathTypeExact,
	}

	ingress := buildIngress(cluster)
	if ingress == nil {
		t.Fatal("buildIngress() = nil, want ingress")
	}
	if ingress.Spec.IngressClassName == nil || *ingress.Spec.IngressClassName != testIngressClassName {
		t.Fatalf("ingressClassName = %#v, want nginx", ingress.Spec.IngressClassName)
	}
	if len(ingress.Spec.Rules) != 1 || ingress.Spec.Rules[0].HTTP == nil || len(ingress.Spec.Rules[0].HTTP.Paths) != 1 {
		t.Fatalf("ingress rules = %#v, want one HTTP path", ingress.Spec.Rules)
	}
	path := ingress.Spec.Rules[0].HTTP.Paths[0]
	if path.PathType == nil || *path.PathType != networkingv1.PathTypeExact {
		t.Fatalf("pathType = %#v, want Exact", path.PathType)
	}
	if path.Backend.Service == nil || path.Backend.Service.Port.Number != 8200 {
		t.Fatalf("backend = %#v, want API service backend", path.Backend.Service)
	}
	if len(ingress.Spec.TLS) != 1 || ingress.Spec.TLS[0].SecretName == "" {
		t.Fatalf("tls = %#v, want default TLS secret", ingress.Spec.TLS)
	}
}

func TestBuildIngressUsesExplicitTLSSecret(t *testing.T) {
	t.Parallel()

	cluster := newMinimalCluster("example", "default")
	cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
		Enabled:       true,
		Host:          "bao.example.test",
		TLSSecretName: "edge-server-tls",
	}

	ingress := buildIngress(cluster)
	if ingress == nil {
		t.Fatal("buildIngress() = nil, want ingress")
	}
	if len(ingress.Spec.TLS) != 1 || ingress.Spec.TLS[0].SecretName != "edge-server-tls" {
		t.Fatalf("tls secret = %#v, want edge-server-tls", ingress.Spec.TLS)
	}
}
