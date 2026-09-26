//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestControllerJWTMode_MigrationContract(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)
	cluster := newMinimalClusterObj(namespace, "jwt-migration")
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: true, OIDC: &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true},
		Requests: []openbaov1alpha1.SelfInitRequest{{
			Name: "audit", Operation: openbaov1alpha1.SelfInitOperationUpdate, Path: "sys/audit/stdout",
			AuditDevice: &openbaov1alpha1.SelfInitAuditDevice{Type: "file", FileOptions: &openbaov1alpha1.FileAuditOptions{FilePath: "stdout"}},
		}},
	}
	require.NoError(t, k8sClient.Create(ctx, cluster))
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster))
	// Omission must not be defaulted on reads or unrelated updates of legacy CRs.
	require.Empty(t, cluster.Spec.ControllerJWTMode)
	cluster.Labels = map[string]string{"migration-test": "unchanged"}
	require.NoError(t, k8sClient.Update(ctx, cluster))
	require.Empty(t, cluster.Spec.ControllerJWTMode)
	cluster.Spec.ControllerJWTMode = "Unknown"
	requireInvalidRequest(t, k8sClient.Update(ctx, cluster))
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster))
	cluster.Spec.ControllerJWTMode = openbaov1alpha1.ControllerJWTModeTarget
	require.NoError(t, k8sClient.Update(ctx, cluster))
	for _, downgrade := range []openbaov1alpha1.ControllerJWTMode{"", openbaov1alpha1.ControllerJWTModeShared} {
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster))
		cluster.Spec.ControllerJWTMode = downgrade
		requireAdmissionDenied(t, k8sClient.Update(ctx, cluster))
	}
}

func TestControllerJWTMode_RequiresJWTBootstrap(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)
	cluster := newMinimalClusterObj(namespace, "jwt-invalid")
	cluster.Spec.ControllerJWTMode = openbaov1alpha1.ControllerJWTModeTarget
	cluster.Spec.SelfInit = nil
	requireAdmissionDenied(t, k8sClient.Create(ctx, cluster))
}
