package bluegreen

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestBuildSnapshotJob_PodSecurityContext_Platform(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
			Backup: &openbaov1alpha1.BackupSchedule{
				Schedule: "0 0 * * *",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint:     "https://s3.example.com",
					Bucket:       "bao",
					Region:       "us-east-1",
					UsePathStyle: true,
				},
			},
		},
	}

	const (
		jobName = "pre-upgrade-snapshot"
		phase   = "pre-upgrade"
		image   = "example.com/backup-executor@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	t.Run("openshift omits pinned IDs", func(t *testing.T) {
		mgr := &Manager{Platform: constants.PlatformOpenShift}
		job, err := mgr.buildSnapshotJob(cluster, jobName, phase, image)
		require.NoError(t, err)

		sc := job.Spec.Template.Spec.SecurityContext
		require.NotNil(t, sc)
		require.Equal(t, ptr.To(true), sc.RunAsNonRoot)
		require.NotNil(t, sc.SeccompProfile)
		require.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, sc.SeccompProfile.Type)

		require.Nil(t, sc.RunAsUser)
		require.Nil(t, sc.RunAsGroup)
		require.Nil(t, sc.FSGroup)

		require.Equal(t, ComponentUpgradeSnapshot, job.Labels[constants.LabelOpenBaoComponent])
		require.Equal(t, ComponentUpgradeSnapshot, job.Spec.Template.Labels[constants.LabelOpenBaoComponent])
		require.Equal(t, phase, job.Annotations[AnnotationSnapshotPhase])
	})

	t.Run("kubernetes pins IDs", func(t *testing.T) {
		mgr := &Manager{Platform: constants.PlatformKubernetes}
		job, err := mgr.buildSnapshotJob(cluster, jobName, phase, image)
		require.NoError(t, err)

		sc := job.Spec.Template.Spec.SecurityContext
		require.NotNil(t, sc)
		require.Equal(t, ptr.To(constants.UserBackup), sc.RunAsUser)
		require.Equal(t, ptr.To(constants.GroupBackup), sc.RunAsGroup)
		require.Equal(t, ptr.To(constants.GroupBackup), sc.FSGroup)
	})
}
