package openbaocluster

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/deletionops"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestDefaultRetentionSecretsReturnsGeneratedCandidates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		want    []string
	}{
		{
			name: "managed init static unseal includes root token and unseal key candidates",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-static",
					Namespace: "default",
				},
			},
			want: []string{
				"managed-static" + constants.SuffixUnsealKey,
				"managed-static" + constants.SuffixRootToken,
			},
		},
		{
			name: "self init static unseal still includes generated candidates",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "selfinit-static",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{Enabled: true},
				},
			},
			want: []string{
				"selfinit-static" + constants.SuffixUnsealKey,
				"selfinit-static" + constants.SuffixRootToken,
			},
		},
		{
			name: "managed transit unseal still includes generated candidates",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-transit",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Unseal: &openbaov1alpha1.UnsealConfig{Type: "transit"},
				},
			},
			want: []string{
				"managed-transit" + constants.SuffixUnsealKey,
				"managed-transit" + constants.SuffixRootToken,
			},
		},
		{
			name: "self init transit unseal still includes generated candidates",
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
			want: []string{
				"selfinit-transit" + constants.SuffixUnsealKey,
				"selfinit-transit" + constants.SuffixRootToken,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := deletionops.DefaultRetentionSecrets(tt.cluster)
			if len(got) != len(tt.want) {
				t.Fatalf("DefaultRetentionSecrets() len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("DefaultRetentionSecrets()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
