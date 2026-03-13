package backup

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestEnsureBackupServiceAccount_WithWorkloadIdentityAnnotations(t *testing.T) {
	cluster := newTestClusterWithBackup("test-cluster", "default")
	cluster.Spec.Backup.Target.WorkloadIdentity = &openbaov1alpha1.WorkloadIdentityConfig{
		ServiceAccountAnnotations: map[string]string{
			"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/openbao-backup",
		},
	}

	client := newTestClient(t, cluster)
	err := EnsureBackupServiceAccount(context.Background(), client, testScheme, cluster)
	require.NoError(t, err)

	var sa corev1.ServiceAccount
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      backupServiceAccountName(cluster),
	}, &sa)
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::123456789012:role/openbao-backup", sa.Annotations["eks.amazonaws.com/role-arn"])
	require.Equal(t, "backup", sa.Labels["openbao.org/service-account-role"])
}
