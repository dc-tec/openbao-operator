package backup

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/security"
)

func TestBackupReconcile_HardenedRequiresEgressRules(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileHardened,
			Backup: &openbaov1alpha1.BackupSchedule{
				Schedule:    "0 0 * * *",
				Image:       "test-image:latest",
				JWTAuthRole: "backup",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "https://example.com",
					Bucket:   "test-bucket",
					RoleARN:  "arn:aws:iam::123456789012:role/test",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithReturnManagedFields().
		Build()

	manager := NewManager(k8sClient, testScheme, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	_, err := manager.Reconcile(context.Background(), logr.Discard(), cluster)
	require.Error(t, err)

	reason, ok := operatorerrors.Reason(err)
	require.True(t, ok)
	require.Equal(t, constants.ReasonNetworkEgressRulesRequired, reason)
}
