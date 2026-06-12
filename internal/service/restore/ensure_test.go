package restore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEnsureRestoreServiceAccount_WithWorkloadIdentityAnnotations(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
			UID:       types.UID("test-cluster-uid"),
		},
	}
	target := openbaov1alpha1.BackupTarget{
		WorkloadIdentity: &openbaov1alpha1.WorkloadIdentityConfig{
			ServiceAccountAnnotations: map[string]string{
				"iam.gke.io/gcp-service-account": "backup@project.iam.gserviceaccount.com",
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	err := EnsureRestoreServiceAccount(context.Background(), client, scheme, cluster, target)
	require.NoError(t, err)

	var sa corev1.ServiceAccount
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      restoreServiceAccountName(cluster),
	}, &sa)
	require.NoError(t, err)
	require.Equal(t, "backup@project.iam.gserviceaccount.com", sa.Annotations["iam.gke.io/gcp-service-account"])
	require.Equal(t, "restore", sa.Labels["openbao.org/service-account-role"])
}
