package networking

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestBuildBackendTLSPolicyHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tlsMode  openbaov1alpha1.TLSMode
		hostname string
		want     string
	}{
		{
			name:    "operator managed defaults to stable TLS server name",
			tlsMode: openbaov1alpha1.TLSModeOperatorManaged,
			want:    "openbao-cluster-example.local",
		},
		{
			name:    "external defaults to stable TLS server name",
			tlsMode: openbaov1alpha1.TLSModeExternal,
			want:    "openbao-cluster-example.local",
		},
		{
			name:     "explicit hostname is preserved",
			tlsMode:  openbaov1alpha1.TLSModeExternal,
			hostname: "backend.example.test",
			want:     "backend.example.test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{Enabled: true, Mode: tt.tlsMode},
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled: true,
						GatewayRef: openbaov1alpha1.GatewayReference{
							Name: "shared-gateway",
						},
						Hostname:   "bao.example.test",
						BackendTLS: &openbaov1alpha1.BackendTLSConfig{Hostname: tt.hostname},
					},
				},
			}

			policy := buildBackendTLSPolicy(cluster)
			if policy == nil {
				t.Fatal("buildBackendTLSPolicy() returned nil")
			}
			if got := string(policy.Spec.Validation.Hostname); got != tt.want {
				t.Fatalf("BackendTLSPolicy hostname = %q, want %q", got, tt.want)
			}
			if tt.hostname == "" && string(policy.Spec.Validation.Hostname) != portopenbao.ComputeTLSServerName(cluster) {
				t.Fatalf("BackendTLSPolicy hostname does not match ComputeTLSServerName()")
			}
		})
	}
}
