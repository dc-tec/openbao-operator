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
				ExecutorImage: "test-image",
				JWTAuthRole:   "test-role",
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
			wantUser: ptr.To(int64(constants.UserBackup)),
			wantGrp:  ptr.To(int64(constants.GroupBackup)),
			wantFS:   ptr.To(int64(constants.GroupBackup)),
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
