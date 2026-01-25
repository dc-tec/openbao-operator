package bluegreen

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/security"
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

func TestHandlePhaseIdle_BlocksOnFailedSnapshot(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	_ = openbaov1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	// Define cluster with Blue/Green strategy and PreUpgradeSnapshot enabled
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
			Image:    "openbao:2.5.0",
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
				BlueGreen: &openbaov1alpha1.BlueGreenConfig{
					PreUpgradeSnapshot: true,
				},
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				JWTAuthRole: "test-role",
				Target: openbaov1alpha1.BackupTarget{
					Bucket: "test-bucket",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.0",
			Initialized:    true,
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase:        openbaov1alpha1.PhaseIdle,
				BlueRevision: "rev-blue",
			},
		},
	}

	// Create a FAILED snapshot job
	// We need to calculate the deterministic job name first
	// For testing simplicty, we can trust the manager to find it if we name it correctly used by the Manager
	// But calculateRevision depends on image/version/replicas.
	// Let's create the manager first to calculate the name, then inject the job.

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	mgr := NewManager(fakeClient, scheme, nil, openbaoapi.ClientConfig{}, security.NewImageVerifier(testLogger(), fakeClient, nil), nil, "kubernetes")

	jobName := preUpgradeSnapshotJobName(cluster)

	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelOpenBaoComponent: ComponentUpgradeSnapshot,
			},
		},
		Status: batchv1.JobStatus{
			Failed: 1,
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: corev1.ConditionTrue},
			},
		},
	}

	// Add job to client
	err := fakeClient.Create(context.Background(), failedJob)
	assert.NoError(t, err)

	// Set the status to indicate we already tried creating it
	cluster.Status.BlueGreen.PreUpgradeSnapshotJobName = jobName

	// Run handlePhaseIdle
	outcome, err := mgr.handlePhaseIdle(context.Background(), testLogger(), cluster, "")

	// Expectation: Should fail or hold, NOT advance
	// Currently (BUG), it likely returns advance(PhaseDeployingGreen)

	if err != nil {
		t.Logf("Got error as expected: %v", err)
	} else {
		// If no error, check outcome
		assert.NotEqual(t, phaseOutcomeAdvance, outcome.kind, "Should not advance when snapshot failed")
		assert.NotEqual(t, openbaov1alpha1.PhaseDeployingGreen, outcome.nextPhase, "Should not transition to DeployingGreen")
	}
}

func testLogger() logr.Logger {
	return logr.Discard() // Simplistic logger
}

func TestPreUpgradeSnapshotJobName(t *testing.T) {
	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		wantChange func(c *openbaov1alpha1.OpenBaoCluster)
	}{
		{
			name: "stable for same inputs",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "snapshot-cluster"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version:  "2.4.4",
					Image:    "example.com/openbao:2.4.4",
					Replicas: 3,
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.3",
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						BlueRevision: "blue",
					},
				},
			},
		},
		{
			name: "changes when target version changes",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "snapshot-cluster"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version:  "2.4.4",
					Image:    "example.com/openbao:2.4.4",
					Replicas: 3,
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.3",
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						BlueRevision: "blue",
					},
				},
			},
			wantChange: func(c *openbaov1alpha1.OpenBaoCluster) { c.Spec.Version = "2.4.5" },
		},
		{
			name: "stays within DNS label limit for long cluster names",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "this-is-a-very-long-cluster-name-that-must-be-trimmed-to-fit"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version:  "2.4.4",
					Image:    "example.com/openbao:2.4.4",
					Replicas: 3,
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.3",
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						BlueRevision: "blue",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := preUpgradeSnapshotJobName(tt.cluster)
			if first == "" {
				t.Fatalf("expected non-empty job name")
			}
			second := preUpgradeSnapshotJobName(tt.cluster)
			if first != second {
				t.Fatalf("expected stable job name, got %q then %q", first, second)
			}
			if len(first) > 63 {
				t.Fatalf("expected job name <= 63 chars, got %d: %q", len(first), first)
			}

			if tt.wantChange != nil {
				changed := tt.cluster.DeepCopy()
				tt.wantChange(changed)
				third := preUpgradeSnapshotJobName(changed)
				if third == first {
					t.Fatalf("expected job name to change after input change, got %q", third)
				}
			}
		})
	}
}
