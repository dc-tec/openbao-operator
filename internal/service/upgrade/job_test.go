package upgrade

import (
	"context"
	"testing"

	"github.com/dc-tec/openbao-operator/internal/adapter/auth"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestBuildUpgradeExecutorJob_SecurityContext(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Image:       "test-image",
				JWTAuthRole: "test-role",
			},
		},
	}

	tests := []struct {
		name     string
		platform string
		wantUser *int64
		wantGrp  *int64
		wantFS   *int64
	}{
		{
			name:     "kubernetes platform pins IDs",
			platform: constants.PlatformKubernetes,
			wantUser: ptr.To(constants.UserBackup), // Backup and Upgrade use same IDs
			wantGrp:  ptr.To(constants.GroupBackup),
			wantFS:   ptr.To(constants.GroupBackup),
		},
		{
			name:     "openshift platform omits IDs",
			platform: constants.PlatformOpenShift,
			wantUser: nil,
			wantGrp:  nil,
			wantFS:   nil,
		},
		{
			name:     "empty platform defaults to pinning IDs",
			platform: "",
			wantUser: ptr.To(constants.UserBackup),
			wantGrp:  ptr.To(constants.GroupBackup),
			wantFS:   ptr.To(constants.GroupBackup),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job, err := buildUpgradeExecutorJob(
				cluster,
				"test-job",
				ExecutorAction("test"),
				"run-id",
				"",
				"",
				"",
				portopenbao.ClientConfig{},
				tt.platform,
			)
			if err != nil {
				t.Fatalf("buildUpgradeExecutorJob() error = %v", err)
			}

			sc := job.Spec.Template.Spec.SecurityContext
			if sc == nil {
				t.Fatal("SecurityContext is nil")
			}

			if !ptrInt64Equal(sc.RunAsUser, tt.wantUser) {
				t.Errorf("RunAsUser = %v, want %v", ptrInt64Value(sc.RunAsUser), ptrInt64Value(tt.wantUser))
			}
			if !ptrInt64Equal(sc.RunAsGroup, tt.wantGrp) {
				t.Errorf("RunAsGroup = %v, want %v", ptrInt64Value(sc.RunAsGroup), ptrInt64Value(tt.wantGrp))
			}
			if !ptrInt64Equal(sc.FSGroup, tt.wantFS) {
				t.Errorf("FSGroup = %v, want %v", ptrInt64Value(sc.FSGroup), ptrInt64Value(tt.wantFS))
			}
		})
	}
}

func TestBuildUpgradeExecutorJob_AllowsOIDCWithoutUpgradeConfig(t *testing.T) {
	t.Setenv(constants.EnvOperatorVersion, "0.0.0-test")

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled: true,
				},
			},
			Upgrade: nil,
		},
	}

	job, err := buildUpgradeExecutorJob(
		cluster,
		"test-job",
		ExecutorActionRollingStepDownLeader,
		"pod-0",
		"",
		"",
		"",
		portopenbao.ClientConfig{},
		constants.PlatformKubernetes,
	)
	if err != nil {
		t.Fatalf("buildUpgradeExecutorJob() error = %v", err)
	}

	if job.Spec.Template.Spec.ServiceAccountName != "test-cluster-upgrade-serviceaccount" {
		t.Fatalf("ServiceAccountName = %q, want %q", job.Spec.Template.Spec.ServiceAccountName, "test-cluster-upgrade-serviceaccount")
	}

	foundRole := false
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == constants.EnvUpgradeJWTAuthRole {
			foundRole = true
			if env.Value != auth.RoleNameUpgrade {
				t.Fatalf("UPGRADE_JWT_AUTH_ROLE = %q, want %q", env.Value, auth.RoleNameUpgrade)
			}
		}
	}
	if !foundRole {
		t.Fatalf("missing %s env var", constants.EnvUpgradeJWTAuthRole)
	}
}

func TestBuildUpgradeExecutorJob_RequiresJWTAuthWhenOIDCDisabled(t *testing.T) {
	t.Setenv(constants.EnvOperatorVersion, "0.0.0-test")

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled: false,
				},
			},
			Upgrade: nil,
		},
	}

	_, err := buildUpgradeExecutorJob(
		cluster,
		"test-job",
		ExecutorActionRollingStepDownLeader,
		"pod-0",
		"",
		"",
		"",
		portopenbao.ClientConfig{},
		constants.PlatformKubernetes,
	)
	if err == nil {
		t.Fatalf("buildUpgradeExecutorJob() expected error, got nil")
	}
}

func ptrInt64Equal(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptrInt64Value(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}

func TestEnsureExecutorJob_CreateAlreadyExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	require.NoError(t, batchv1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Image:       "upgrade-executor:0.1.0",
				JWTAuthRole: "upgrade-role",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*batchv1.Job); ok {
					return apierrors.NewAlreadyExists(schema.GroupResource{Group: "batch", Resource: "jobs"}, obj.GetName())
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()

	result, err := EnsureExecutorJob(
		context.Background(),
		k8sClient,
		scheme,
		logr.Discard(),
		cluster,
		ExecutorActionRollingStepDownLeader,
		"run-1",
		"",
		"",
		portopenbao.ClientConfig{},
		nil,
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, ExecutorJobName(cluster.Name, ExecutorActionRollingStepDownLeader, "run-1", "", ""), result.Name)
	assert.True(t, result.Exists)
	assert.True(t, result.Running)
	assert.False(t, result.Succeeded)
	assert.False(t, result.Failed)
}
