package bluegreen

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestEnsureJob_CreateAlreadyExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	require.NoError(t, batchv1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
			UID:       "test-cluster-uid",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*batchv1.Job); ok {
					if err := c.Create(ctx, obj, opts...); err != nil {
						return err
					}
					return apierrors.NewAlreadyExists(schema.GroupResource{Group: "batch", Resource: "jobs"}, obj.GetName())
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()

	jobName := "upgrade-test-job"
	result, err := ensureJob(
		context.Background(),
		k8sClient,
		k8sClient,
		scheme,
		logr.Discard(),
		cluster,
		jobName,
		func(name string) (*batchv1.Job, error) {
			return &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: cluster.Namespace,
				},
			}, nil
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, jobName, result.Name)
	assert.True(t, result.Exists)
	assert.True(t, result.Running)
	assert.False(t, result.Succeeded)
	assert.False(t, result.Failed)
}

func TestGetJobStatusRejectsForeignSucceededJob(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, batchv1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{
		Name:      "test-cluster",
		Namespace: "default",
		UID:       "test-cluster-uid",
	}}
	foreignJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-test-job",
			Namespace: cluster.Namespace,
		},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(foreignJob).Build()

	result, err := getJobStatus(context.Background(), k8sClient, cluster, foreignJob.Name)
	require.Nil(t, result)
	assert.ErrorContains(t, err, "requires managed controller owner OpenBaoCluster")
}
