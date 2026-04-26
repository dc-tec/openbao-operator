package openbaotls

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestReadClientTrustBundle_PrivateACME(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "bao", Namespace: "tenant"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
				Mode:    openbaov1alpha1.TLSModeACME,
				ACME: &openbaov1alpha1.ACMEConfig{
					Domains: []string{"bao-acme.tenant.svc"},
				},
			},
			Configuration: &openbaov1alpha1.OpenBaoConfiguration{
				ACMECARoot: "/etc/bao/seal-creds/ca.crt",
			},
			Unseal: &openbaov1alpha1.UnsealConfig{
				CredentialsSecretRef: &corev1.LocalObjectReference{Name: "seal-creds"},
			},
		},
	}
	clientset := k8sfake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "seal-creds", Namespace: "tenant"},
		Data:       map[string][]byte{"pki-ca.crt": []byte("pki-ca")},
	})

	trust, err := ReadClientTrustBundle(context.Background(), clientset, cluster)
	if err != nil {
		t.Fatalf("ReadClientTrustBundle() error = %v", err)
	}
	if string(trust.CACert) != "pki-ca" {
		t.Fatalf("CACert=%q, want pki-ca", string(trust.CACert))
	}
	if trust.TLSServerName != "bao-acme.tenant.svc" {
		t.Fatalf("TLSServerName=%q, want bao-acme.tenant.svc", trust.TLSServerName)
	}
}

func TestReadClientTrustBundle_MissingKey(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "bao", Namespace: "tenant"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{Enabled: true},
		},
	}
	clientset := k8sfake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "bao-tls-ca", Namespace: "tenant"},
		Data:       map[string][]byte{},
	})

	_, err := ReadClientTrustBundle(context.Background(), clientset, cluster)
	if err == nil || !strings.Contains(err.Error(), `missing "ca.crt" key`) {
		t.Fatalf("expected missing key error, got %v", err)
	}
}
