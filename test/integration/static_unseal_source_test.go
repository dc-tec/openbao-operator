//go:build integration

package integration

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestVAP_OpenBaoCluster_StaticUnsealSourceIsImmutable(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	staticWith := func(secret string) *openbaov1alpha1.UnsealConfig {
		unseal := &openbaov1alpha1.UnsealConfig{Type: "static"}
		if secret != "" {
			unseal.CredentialsSecretRef = &corev1.LocalObjectReference{Name: secret}
		}
		return unseal
	}
	tests := []struct {
		name      string
		initial   *openbaov1alpha1.UnsealConfig
		updated   *openbaov1alpha1.UnsealConfig
		wantError bool
	}{
		{name: "add reference to generated key", initial: nil, updated: staticWith("original-key"), wantError: true},
		{name: "change reference", initial: staticWith("original-key"), updated: staticWith("other-key"), wantError: true},
		{name: "remove reference", initial: staticWith("original-key"), updated: staticWith(""), wantError: true},
		{name: "keep reference", initial: staticWith("original-key"), updated: staticWith("original-key")},
		{name: "omit default static config", initial: nil, updated: staticWith("")},
	}
	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMinimalClusterObj(namespace, fmt.Sprintf("static-unseal-%d", index))
			cluster.Spec.Unseal = tt.initial
			require.NoError(t, k8sClient.Create(ctx, cluster))
			require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, cluster))

			cluster.Spec.Unseal = tt.updated
			err := k8sClient.Update(ctx, cluster, client.DryRunAll)
			if tt.wantError {
				requireAdmissionDenied(t, err)
				require.ErrorContains(t, err, "spec.unseal.credentialsSecretRef cannot be added, changed, or removed")
				return
			}
			require.NoError(t, err)
		})
	}
}
