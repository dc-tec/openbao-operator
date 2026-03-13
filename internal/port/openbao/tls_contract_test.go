package openbao

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestComputeTLSServerName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		want    string
	}{
		{
			name: "tls disabled",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
			},
			want: "",
		},
		{
			name: "operator managed",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{Enabled: true},
				},
			},
			want: "openbao-cluster-example.local",
		},
		{
			name: "acme prefers svc domain",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
						ACME: &openbaov1alpha1.ACMEConfig{
							Domains: []string{"bao.example.com", "example-acme.default.svc"},
						},
					},
				},
			},
			want: "example-acme.default.svc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ComputeTLSServerName(tt.cluster); got != tt.want {
				t.Fatalf("ComputeTLSServerName()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveClientTrustBundle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cluster   *openbaov1alpha1.OpenBaoCluster
		want      TrustBundleSource
		wantError string
	}{
		{
			name: "operator managed uses tls-ca secret",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{Enabled: true},
				},
			},
			want: TrustBundleSource{
				SecretName: "example-tls-ca",
				SecretKey:  "ca.crt",
			},
		},
		{
			name: "public acme uses system roots",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
						ACME:    &openbaov1alpha1.ACMEConfig{},
					},
				},
			},
			want: TrustBundleSource{UseSystemRoots: true},
		},
		{
			name: "private acme uses pki-ca from seal creds secret",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
						ACME:    &openbaov1alpha1.ACMEConfig{},
					},
					Configuration: &openbaov1alpha1.OpenBaoConfiguration{
						ACMECARoot: "/etc/bao/seal-creds/ca.crt",
					},
					Unseal: &openbaov1alpha1.UnsealConfig{
						CredentialsSecretRef: &corev1.LocalObjectReference{Name: "seal-creds"},
					},
				},
			},
			want: TrustBundleSource{
				SecretName: "seal-creds",
				SecretKey:  "pki-ca.crt",
			},
		},
		{
			name: "private acme without credentials errors",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
						ACME:    &openbaov1alpha1.ACMEConfig{},
					},
					Configuration: &openbaov1alpha1.OpenBaoConfiguration{
						ACMECARoot: "/etc/bao/seal-creds/ca.crt",
					},
				},
			},
			wantError: "spec.unseal.credentialsSecretRef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveClientTrustBundle(tt.cluster)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("ResolveClientTrustBundle() error=%v, want substring %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveClientTrustBundle() error=%v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveClientTrustBundle()=%#v, want %#v", got, tt.want)
			}
		})
	}
}
