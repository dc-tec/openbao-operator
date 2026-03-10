package bluegreen

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestEnsurePrePromotionHookJob_AppliesRestrictedPodSecurityDefaults(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	require.NoError(t, batchv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		Build()

	mgr := &Manager{
		client:   k8sClient,
		scheme:   scheme,
		Platform: constants.PlatformKubernetes,
	}

	result, err := mgr.ensurePrePromotionHookJob(context.Background(), logr.Discard(), cluster, &openbaov1alpha1.ValidationHookConfig{
		Image:   "openbao/openbao:2.4.4",
		Command: []string{"/bin/sh", "-ec"},
		Args:    []string{"echo test"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Running)

	job := &batchv1.Job{}
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      "test-cluster-validation-hook",
	}, job))

	require.Equal(t, ptr.To(false), job.Spec.Template.Spec.AutomountServiceAccountToken)
	require.NotNil(t, job.Spec.Template.Spec.SecurityContext)
	require.Equal(t, ptr.To(true), job.Spec.Template.Spec.SecurityContext.RunAsNonRoot)
	require.Equal(t, ptr.To(constants.UserNonRoot), job.Spec.Template.Spec.SecurityContext.RunAsUser)
	require.Equal(t, ptr.To(constants.UserNonRoot), job.Spec.Template.Spec.SecurityContext.RunAsGroup)
	require.NotNil(t, job.Spec.Template.Spec.SecurityContext.SeccompProfile)
	require.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, job.Spec.Template.Spec.SecurityContext.SeccompProfile.Type)

	require.Len(t, job.Spec.Template.Spec.Containers, 1)
	container := job.Spec.Template.Spec.Containers[0]
	require.NotNil(t, container.SecurityContext)
	require.Equal(t, ptr.To(false), container.SecurityContext.AllowPrivilegeEscalation)
	require.Equal(t, ptr.To(true), container.SecurityContext.RunAsNonRoot)
	require.NotNil(t, container.SecurityContext.Capabilities)
	require.Equal(t, []corev1.Capability{"ALL"}, container.SecurityContext.Capabilities.Drop)
}
