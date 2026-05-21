//go:build integration
// +build integration

package provisioner_test

import (
	"context"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	provisionersvc "github.com/dc-tec/openbao-operator/internal/service/provisioner"
)

func TestTenantSecretsRBAC_SetupWithManager_SynchronizesSecretAllowlists(t *testing.T) {
	setAdmissionReady(t)

	ctx := context.Background()
	liveClient := startProvisionerControllers(t)

	createNamespace(t, ctx, liveClient, operatorNamespace)
	createNamespace(t, ctx, liveClient, "tenant-a")

	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-a",
			Namespace: operatorNamespace,
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{
			TargetNamespace: "tenant-a",
		},
	}
	require.NoError(t, liveClient.Create(ctx, tenant))
	waitForTenantProvisioned(t, ctx, liveClient, types.NamespacedName{
		Namespace: operatorNamespace,
		Name:      tenant.Name,
	})

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-a",
			Namespace: "tenant-a",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 1,
			Profile:  openbaov1alpha1.ProfileDevelopment,
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
			TLS: openbaov1alpha1.TLSConfig{
				Enabled:        true,
				RotationPeriod: "720h",
			},
			InitContainer: &openbaov1alpha1.InitContainerConfig{
				Image: "openbao/openbao-init:latest",
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				Schedule: "0 3 * * *",
				Target: openbaov1alpha1.BackupTarget{
					Bucket:               "backups",
					CredentialsSecretRef: &corev1.LocalObjectReference{Name: "backup-creds"},
				},
				TokenSecretRef: &corev1.LocalObjectReference{Name: "backup-token"},
			},
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				JWTAuthRole: "upgrade-role",
			},
			Unseal: &openbaov1alpha1.UnsealConfig{
				CredentialsSecretRef: &corev1.LocalObjectReference{Name: "unseal-creds"},
			},
		},
	}
	require.NoError(t, liveClient.Create(ctx, cluster))
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-a",
			Namespace: "tenant-a",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "cluster-a",
			Source: openbaov1alpha1.RestoreSource{
				Target: openbaov1alpha1.BackupTarget{
					Bucket:               "restore-backups",
					CredentialsSecretRef: &corev1.LocalObjectReference{Name: "restore-creds"},
				},
				Key: "snapshots/restore.snap",
			},
			TokenSecretRef: &corev1.LocalObjectReference{Name: "restore-token"},
		},
	}
	require.NoError(t, liveClient.Create(ctx, restore))

	wantWriterSecrets := []string{"cluster-a-root-token", "cluster-a-tls-ca", "cluster-a-tls-server", "cluster-a-unseal-key"}
	wantReaderSecrets := []string{"backup-creds", "backup-token", "restore-creds", "restore-token", "unseal-creds"}
	sort.Strings(wantWriterSecrets)
	sort.Strings(wantReaderSecrets)

	require.Eventually(t, func() bool {
		writerRole := &rbacv1.Role{}
		if err := liveClient.Get(ctx, types.NamespacedName{
			Namespace: "tenant-a",
			Name:      provisionersvc.TenantSecretsWriterRoleName,
		}, writerRole); err != nil {
			return false
		}
		if !slices.Equal(extractSecretResourceNames(writerRole.Rules), wantWriterSecrets) {
			return false
		}

		readerRole := &rbacv1.Role{}
		if err := liveClient.Get(ctx, types.NamespacedName{
			Namespace: "tenant-a",
			Name:      provisionersvc.TenantSecretsReaderRoleName,
		}, readerRole); err != nil {
			return false
		}
		if !slices.Equal(extractSecretResourceNames(readerRole.Rules), wantReaderSecrets) {
			return false
		}

		writerBinding := &rbacv1.RoleBinding{}
		if err := liveClient.Get(ctx, types.NamespacedName{
			Namespace: "tenant-a",
			Name:      provisionersvc.TenantSecretsWriterRoleBindingName,
		}, writerBinding); err != nil {
			return false
		}

		readerBinding := &rbacv1.RoleBinding{}
		if err := liveClient.Get(ctx, types.NamespacedName{
			Namespace: "tenant-a",
			Name:      provisionersvc.TenantSecretsReaderRoleBindingName,
		}, readerBinding); err != nil {
			return false
		}

		return true
	}, 20*time.Second, 200*time.Millisecond, "expected manager-driven reconcile to synchronize tenant secret RBAC")
}

func extractSecretResourceNames(rules []rbacv1.PolicyRule) []string {
	var out []string
	for i := range rules {
		rule := rules[i]
		if !slices.Contains(rule.Resources, "secrets") {
			continue
		}
		if len(rule.ResourceNames) == 0 {
			continue
		}
		out = append(out, rule.ResourceNames...)
	}
	sort.Strings(out)
	return out
}
