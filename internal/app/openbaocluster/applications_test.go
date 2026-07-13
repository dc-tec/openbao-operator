package openbaocluster

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestNewRuntimeApplicationsBuildsApplicationBoundary(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	applications := NewRuntimeApplications(RuntimeApplicationsConfig{
		Kubernetes: RuntimeKubernetesConfig{
			Client:    k8sClient,
			APIReader: k8sClient,
			Scheme:    scheme,
		},
	})

	require.NotNil(t, applications)
	assert.False(t, applications.InitializationConfigured())
}

func TestApplicationsRequireConfiguredBoundary(t *testing.T) {
	ctx := context.Background()
	logger := logr.Discard()
	cluster := &openbaov1alpha1.OpenBaoCluster{}

	var applications *Applications
	_, err := applications.ReconcileWorkload(ctx, logger, cluster.DeepCopy(), cluster, nil)
	assert.EqualError(t, err, "workload application client is required")
	_, err = applications.ReconcileAdminOps(ctx, logger, cluster.DeepCopy(), cluster, nil)
	assert.EqualError(t, err, "admin operations application is required")
	_, err = applications.GatherStatusState(ctx, logger, cluster)
	assert.EqualError(t, err, "status application is required")
	assert.EqualError(t, applications.HandleDeletion(ctx, logger, cluster), "deletion application is required")
	assert.False(t, applications.InitializationConfigured())
}

func TestApplicationsReportsConfiguredInitialization(t *testing.T) {
	applications := NewApplications(ApplicationsConfig{InitializationConfigured: true})
	assert.True(t, applications.InitializationConfigured())
}
