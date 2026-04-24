package openbaocluster

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestDefaultRetentionSecrets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		want    []string
	}{
		{
			name: "managed init static unseal retains root token and unseal key",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-static",
					Namespace: "default",
				},
			},
			want: []string{"managed-static-unseal-key", "managed-static-root-token"},
		},
		{
			name: "self init static unseal retains only unseal key",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "selfinit-static",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
				},
			},
			want: []string{"selfinit-static-unseal-key"},
		},
		{
			name: "managed transit unseal retains only root token",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-transit",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Unseal: &openbaov1alpha1.UnsealConfig{Type: "transit"},
				},
			},
			want: []string{"managed-transit-root-token"},
		},
		{
			name: "self init transit unseal retains no secrets",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "selfinit-transit",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
					Unseal:   &openbaov1alpha1.UnsealConfig{Type: "transit"},
				},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := defaultRetentionSecrets(tt.cluster)
			if len(got) != len(tt.want) {
				t.Fatalf("defaultRetentionSecrets() len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("defaultRetentionSecrets()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
