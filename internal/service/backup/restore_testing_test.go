package backup

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestSnapshotSummaryRequiresOwnedSuccessfulExecutor(t *testing.T) {
	cluster := newTestClusterWithBackup("source", "test")
	job := newBackupJobForCluster(cluster, "backup", time.Now())
	good := api.BackupSnapshotSummary{ClusterID: "source-id", Version: "2.7.0", Size: 123, Digest: "sha256:" + strings.Repeat("a", 64)}
	for _, tc := range []string{"valid", "foreign owner", "failed container", "malformed", "oversized"} {
		t.Run(tc, func(t *testing.T) {
			data, err := json.Marshal(good)
			require.NoError(t, err)
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: job.Namespace, Labels: map[string]string{"batch.kubernetes.io/job-name": job.Name},
				OwnerReferences: []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "Job", Name: job.Name, UID: job.UID, Controller: ptr.To(true)}}},
				Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: ComponentBackup, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Message: string(data)}}}}}}
			switch tc {
			case "foreign owner":
				pod.OwnerReferences[0].UID = "foreign"
			case "failed container":
				pod.Status.ContainerStatuses[0].State.Terminated.ExitCode = 1
			case "malformed":
				pod.Status.ContainerStatuses[0].State.Terminated.Message = "not-json"
			case "oversized":
				pod.Status.ContainerStatuses[0].State.Terminated.Message = strings.Repeat("x", 4097)
			}
			c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(pod).Build()
			got, err := newBackupManager(c).snapshotSummary(t.Context(), job)
			require.NoError(t, err)
			if tc == "valid" {
				require.Equal(t, &good, got)
			} else {
				require.Nil(t, got)
			}
		})
	}
}
