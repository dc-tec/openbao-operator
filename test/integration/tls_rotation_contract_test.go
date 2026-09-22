//go:build integration
// +build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestCRD_OpenBaoCluster_TLSRotationPeriod(t *testing.T) {
	const requiredMessage = "spec.tls.rotationPeriod is required when spec.tls.mode is OperatorManaged"

	tests := []struct {
		name           string
		mode           openbaov1alpha1.TLSMode
		rotationPeriod *string
		wantError      string
	}{
		{"operator-managed-omitted", "OperatorManaged", nil, requiredMessage},
		{"operator-managed-empty", "OperatorManaged", ptr.To(""), "spec.tls.rotationPeriod"},
		{"operator-managed-valid", "OperatorManaged", ptr.To("720h"), ""},
		{"default-mode-omitted", "", nil, requiredMessage},
		{"external-omitted", "External", nil, ""},
		{"acme-omitted", "ACME", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMinimalClusterObj(newTestNamespace(t), tt.name)
			cluster.Spec.Replicas = 1
			cluster.Spec.TLS.Mode = tt.mode
			cluster.Spec.TLS.RotationPeriod = ""
			if tt.mode == openbaov1alpha1.TLSModeACME {
				cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
					DirectoryURL: "https://acme.example/directory",
				}
			}

			// Use an unstructured object so an explicit empty value survives omitempty.
			object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cluster)
			require.NoError(t, err)
			candidate := &unstructured.Unstructured{Object: object}
			candidate.SetGroupVersionKind(openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster"))
			if tt.rotationPeriod != nil {
				err = unstructured.SetNestedField(candidate.Object, *tt.rotationPeriod, "spec", "tls", "rotationPeriod")
				require.NoError(t, err)
			}

			err = k8sClient.Create(ctx, candidate)
			if tt.wantError != "" {
				requireInvalidRequest(t, err)
				require.ErrorContains(t, err, tt.wantError)
				require.NotContains(t, err.Error(), "no such key")
				return
			}
			require.NoError(t, err)
		})
	}
}
