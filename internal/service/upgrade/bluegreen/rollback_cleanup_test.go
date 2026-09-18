package bluegreen

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

func rollbackDataClaim(cluster *openbaov1alpha1.OpenBaoCluster, name string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: cluster.Namespace, UID: types.UID(name + "-uid"),
		Labels: resourceidentity.Labels(cluster), Annotations: map[string]string{constants.AnnotationOpenBaoOwnerUID: string(cluster.UID)},
	}}
}

func TestRollbackCleanupWaitsForTerminatingPodsAndRetiresData(t *testing.T) {
	t.Parallel()
	cluster := newPhaseMachineCluster()
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseRollbackCleanup
	cluster.Status.BlueGreen.OperationID = "bg-v2-rollback"
	green := rollbackDataClaim(cluster, "data-example-green-0")
	green.Finalizers = []string{"test.example/hold"}
	blue := rollbackDataClaim(cluster, "data-example-blue-0")
	cache := rollbackDataClaim(cluster, "example-acme-cache")
	pod := newRevisionPod(cluster, deploymentNameSuffix, "example-green-0")
	now := metav1.Now()
	pod.DeletionTimestamp, pod.Finalizers = &now, []string{"test.example/hold"}
	scheme := newBlueGreenTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster).
		WithObjects(cluster, succeededExecutorJobWithRunID(cluster, ActionRemoveGreenPeers, rollbackRunID(cluster)), green, blue, cache, pod).Build()
	m := &Manager{client: c, reader: c, scheme: scheme, clusterOps: &clusterOpsStub{ok: true, podName: "example-blue-0"}}

	for range 2 {
		outcome, err := m.handlePhaseRollbackCleanup(t.Context(), logr.Discard(), cluster)
		require.NoError(t, err)
		require.Equal(t, phaseOutcomeRequeueAfter, outcome.kind)
		require.Equal(t, openbaov1alpha1.PhaseRollbackCleanup, cluster.Status.BlueGreen.Phase)
		require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(green), green))
		require.Nil(t, green.DeletionTimestamp, "Terminating Pod still protects its data")
	}
	pod.Finalizers = nil
	require.NoError(t, c.Update(t.Context(), pod))
	for range 2 {
		outcome, err := m.handlePhaseRollbackCleanup(t.Context(), logr.Discard(), cluster)
		require.NoError(t, err)
		require.Equal(t, phaseOutcomeRequeueAfter, outcome.kind)
		require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(green), green))
		require.NotNil(t, green.DeletionTimestamp)
		require.Equal(t, deploymentNameSuffix, cluster.Status.BlueGreen.GreenRevision)
	}
	green.Finalizers = nil
	require.NoError(t, c.Update(t.Context(), green))
	outcome, err := m.handlePhaseRollbackCleanup(t.Context(), logr.Discard(), cluster)
	require.NoError(t, err)
	require.Equal(t, phaseOutcomeDone, outcome.kind)
	require.Equal(t, openbaov1alpha1.PhaseIdle, cluster.Status.BlueGreen.Phase)
	for _, preserved := range []*corev1.PersistentVolumeClaim{blue, cache} {
		got := &corev1.PersistentVolumeClaim{}
		require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(preserved), got))
		require.Equal(t, preserved.UID, got.UID)
		require.Nil(t, got.DeletionTimestamp)
	}
}

func TestRetireRollbackGreenDataOwnershipAndReferences(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		alter      func(*openbaov1alpha1.OpenBaoCluster, *corev1.PersistentVolumeClaim) []client.Object
		wantErr    bool
		wantDelete bool
	}{
		{name: "owned discarded data", wantDelete: true},
		{name: "another cluster UID", alter: func(_ *openbaov1alpha1.OpenBaoCluster, p *corev1.PersistentVolumeClaim) []client.Object {
			p.Annotations[constants.AnnotationOpenBaoOwnerUID] = "other"
			return nil
		}, wantErr: true},
		{name: "unmanaged claim", alter: func(_ *openbaov1alpha1.OpenBaoCluster, p *corev1.PersistentVolumeClaim) []client.Object {
			delete(p.Labels, constants.LabelAppManagedBy)
			return nil
		}, wantErr: true},
		{name: "Blue equals Green", alter: func(c *openbaov1alpha1.OpenBaoCluster, _ *corev1.PersistentVolumeClaim) []client.Object {
			c.Status.BlueGreen.BlueRevision = deploymentNameSuffix
			return nil
		}, wantErr: true},
		{name: "not rollback cleanup", alter: func(c *openbaov1alpha1.OpenBaoCluster, _ *corev1.PersistentVolumeClaim) []client.Object {
			c.Status.BlueGreen.Phase = openbaov1alpha1.PhaseSyncing
			return nil
		}, wantErr: true},
		{name: "read pool cannot be Green", alter: func(c *openbaov1alpha1.OpenBaoCluster, _ *corev1.PersistentVolumeClaim) []client.Object {
			c.Status.BlueGreen.GreenRevision = "read"
			return nil
		}, wantErr: true},
		{name: "foreign terminating mount", alter: func(c *openbaov1alpha1.OpenBaoCluster, p *corev1.PersistentVolumeClaim) []client.Object {
			now := metav1.Now()
			return []client.Object{&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: c.Namespace, DeletionTimestamp: &now, Finalizers: []string{"test.example/hold"}}, Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: p.Name}}}}}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cluster := newPhaseMachineCluster()
			cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseRollbackCleanup
			claim := rollbackDataClaim(cluster, "data-example-green-0")
			objects := []client.Object{claim}
			if tc.alter != nil {
				objects = append(objects, tc.alter(cluster, claim)...)
			}
			for _, name := range []string{"data-example-blue-0", "data-example-green-01", "data-example-greenish-0", "data-example-green-0-copy", "example-acme-cache", "example-audit"} {
				objects = append(objects, rollbackDataClaim(cluster, name))
			}
			c := fake.NewClientBuilder().WithScheme(newBlueGreenTestScheme(t)).WithObjects(objects...).Build()
			m := &Manager{client: c, reader: c}
			done, err := m.retireRollbackGreenData(t.Context(), cluster)
			require.Equal(t, tc.wantErr, err != nil, "error=%v", err)
			require.False(t, done)
			err = c.Get(t.Context(), client.ObjectKeyFromObject(claim), &corev1.PersistentVolumeClaim{})
			require.Equal(t, tc.wantDelete, apierrors.IsNotFound(err))
			for _, object := range objects[1:] {
				if _, ok := object.(*corev1.PersistentVolumeClaim); ok {
					require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(object), &corev1.PersistentVolumeClaim{}))
				}
			}
		})
	}
}

func TestRetireRollbackGreenStatefulSetGuards(t *testing.T) {
	t.Parallel()
	for _, owned := range []bool{false, true} {
		t.Run(map[bool]string{false: "foreign", true: "owned"}[owned], func(t *testing.T) {
			cluster := newPhaseMachineCluster()
			cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseRollbackCleanup
			sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "example-green", Namespace: cluster.Namespace, UID: "green-sts", Finalizers: []string{"test.example/hold"}}}
			if owned {
				proof := succeededExecutorJob(cluster, ActionJoinGreenNonVoters)
				sts.OwnerReferences, sts.Annotations = proof.OwnerReferences, proof.Annotations
				sts.Spec.Template.Labels = map[string]string{constants.LabelOpenBaoRevision: deploymentNameSuffix}
			}
			claim := rollbackDataClaim(cluster, "data-example-green-0")
			c := fake.NewClientBuilder().WithScheme(newBlueGreenTestScheme(t)).WithObjects(sts, claim).Build()
			m := &Manager{client: c, reader: c}
			for range 2 {
				done, err := m.retireRollbackGreenData(t.Context(), cluster)
				require.False(t, done)
				require.Equal(t, !owned, err != nil, "error=%v", err)
				require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(claim), claim))
				require.Nil(t, claim.DeletionTimestamp)
			}
		})
	}
}

func TestRetireRollbackDataUsesDeletePreconditions(t *testing.T) {
	t.Parallel()
	cluster := newPhaseMachineCluster()
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseRollbackCleanup
	claim := rollbackDataClaim(cluster, "data-example-green-0")
	c := fake.NewClientBuilder().WithScheme(newBlueGreenTestScheme(t)).WithObjects(claim).Build()
	writer := interceptor.NewClient(c, interceptor.Funcs{Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
		o := (&client.DeleteOptions{}).ApplyOptions(opts)
		require.NotNil(t, o.Preconditions)
		require.Equal(t, obj.GetUID(), *o.Preconditions.UID)
		require.Equal(t, obj.GetResourceVersion(), *o.Preconditions.ResourceVersion)
		return apierrors.NewConflict(corev1.Resource("persistentvolumeclaims"), obj.GetName(), nil)
	}})
	m := &Manager{client: writer, reader: c}
	done, err := m.retireRollbackGreenData(t.Context(), cluster)
	require.False(t, done)
	require.True(t, apierrors.IsConflict(err))
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(claim), claim))
}

func TestRollbackCleanupRequiresRemovedPeersAndBlueLeader(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		removed, leader bool
	}{
		{name: "removal still running", leader: true},
		{name: "Blue leader unavailable", removed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cluster := newPhaseMachineCluster()
			cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseRollbackCleanup
			job := succeededExecutorJobWithRunID(cluster, ActionRemoveGreenPeers, rollbackRunID(cluster))
			if !tc.removed {
				job.Status.Conditions = nil
				job.Status.Succeeded = 0
			}
			claim := rollbackDataClaim(cluster, "data-example-green-0")
			scheme := newBlueGreenTestScheme(t)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job, claim).Build()
			m := &Manager{client: c, reader: c, scheme: scheme, clusterOps: &clusterOpsStub{ok: tc.leader, podName: "example-blue-0"}}
			outcome, err := m.handlePhaseRollbackCleanup(t.Context(), logr.Discard(), cluster)
			require.NoError(t, err)
			require.Equal(t, phaseOutcomeRequeueAfter, outcome.kind)
			require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(claim), claim))
			require.Nil(t, claim.DeletionTimestamp)
		})
	}
}
