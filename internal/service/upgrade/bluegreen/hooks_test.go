package bluegreen

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
)

type hookImageVerifier struct {
	digest   string
	err      error
	called   bool
	imageRef string
}

func (v *hookImageVerifier) Verify(_ context.Context, imageRef string, _ imageverify.VerifyConfig) (string, error) {
	v.called = true
	v.imageRef = imageRef
	return v.digest, v.err
}

func TestEnsurePrePromotionHookJob_ImageVerification(t *testing.T) {
	t.Parallel()

	const (
		hookImage      = "registry.example.com/custom/validation-hook:latest"
		verifiedDigest = "registry.example.com/custom/validation-hook@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	tests := []struct {
		name          string
		config        *openbaov1alpha1.ImageVerificationConfig
		verifier      *hookImageVerifier
		wantImage     string
		wantJob       bool
		wantReason    string
		wantVerifyRun bool
	}{
		{
			name: "disabled uses original image without verification",
			config: &openbaov1alpha1.ImageVerificationConfig{
				Enabled: false,
			},
			verifier:      &hookImageVerifier{err: errors.New("verification must not run")},
			wantImage:     hookImage,
			wantJob:       true,
			wantVerifyRun: false,
		},
		{
			name: "warn continues with original image after verification failure",
			config: &openbaov1alpha1.ImageVerificationConfig{
				Enabled:       true,
				FailurePolicy: imageVerificationFailurePolicyWarn,
				PublicKey:     "test-public-key",
			},
			verifier:      &hookImageVerifier{err: errors.New("signature rejected")},
			wantImage:     hookImage,
			wantJob:       true,
			wantVerifyRun: true,
		},
		{
			name: "block rejects job creation with a stable reason",
			config: &openbaov1alpha1.ImageVerificationConfig{
				Enabled:       true,
				FailurePolicy: constants.ImageVerificationFailurePolicyBlock,
				PublicKey:     "test-public-key",
			},
			verifier:      &hookImageVerifier{err: errors.New("signature rejected")},
			wantJob:       false,
			wantReason:    constants.ReasonValidationHookImageVerificationFailed,
			wantVerifyRun: true,
		},
		{
			name: "successful verification pins digest in job",
			config: &openbaov1alpha1.ImageVerificationConfig{
				Enabled:       true,
				FailurePolicy: constants.ImageVerificationFailurePolicyBlock,
				PublicKey:     "test-public-key",
			},
			verifier:      &hookImageVerifier{digest: verifiedDigest},
			wantImage:     verifiedDigest,
			wantJob:       true,
			wantVerifyRun: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					OperatorImageVerification: tt.config,
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(cluster).
				Build()
			mgr := &Manager{
				client:                k8sClient,
				scheme:                scheme,
				operatorImageVerifier: tt.verifier,
				Platform:              constants.PlatformKubernetes,
			}

			result, err := mgr.ensurePrePromotionHookJob(
				context.Background(),
				logr.Discard(),
				cluster,
				&openbaov1alpha1.ValidationHookConfig{Image: hookImage},
			)
			if tt.wantReason != "" {
				require.Error(t, err)
				reason, ok := operatorerrors.Reason(err)
				require.True(t, ok)
				require.Equal(t, tt.wantReason, reason)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.True(t, result.Running)
			}

			require.Equal(t, tt.wantVerifyRun, tt.verifier.called)
			if tt.wantVerifyRun {
				require.Equal(t, hookImage, tt.verifier.imageRef)
			}

			job := &batchv1.Job{}
			err = k8sClient.Get(context.Background(), client.ObjectKey{
				Namespace: cluster.Namespace,
				Name:      "test-cluster-validation-hook",
			}, job)
			if !tt.wantJob {
				require.True(t, apierrors.IsNotFound(err))
				return
			}

			require.NoError(t, err)
			require.Len(t, job.Spec.Template.Spec.Containers, 1)
			require.Equal(t, tt.wantImage, job.Spec.Template.Spec.Containers[0].Image)
		})
	}
}

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
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "registry-creds"},
			},
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
	require.Empty(t, job.Spec.Template.Spec.ServiceAccountName)
	require.Equal(t, cluster.Spec.ImagePullSecrets, job.Spec.Template.Spec.ImagePullSecrets)
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
	require.Equal(t, ptr.To(true), container.SecurityContext.ReadOnlyRootFilesystem)
	require.Equal(t, ptr.To(true), container.SecurityContext.RunAsNonRoot)
	require.NotNil(t, container.SecurityContext.Capabilities)
	require.Equal(t, []corev1.Capability{"ALL"}, container.SecurityContext.Capabilities.Drop)
	require.Equal(t, resource.MustParse("100m"), container.Resources.Requests[corev1.ResourceCPU])
	require.Equal(t, resource.MustParse("128Mi"), container.Resources.Requests[corev1.ResourceMemory])
	require.Equal(t, resource.MustParse("500m"), container.Resources.Limits[corev1.ResourceCPU])
	require.Equal(t, resource.MustParse("512Mi"), container.Resources.Limits[corev1.ResourceMemory])
}
