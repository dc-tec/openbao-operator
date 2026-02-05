package upgrade

import (
	"testing"

	"github.com/dc-tec/openbao-operator/internal/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/openbao"
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
				openbao.ClientConfig{},
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
		openbao.ClientConfig{},
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
			if env.Value != constants.RoleNameUpgrade {
				t.Fatalf("UPGRADE_JWT_AUTH_ROLE = %q, want %q", env.Value, constants.RoleNameUpgrade)
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
		openbao.ClientConfig{},
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
