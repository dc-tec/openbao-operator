package openbaoclusterclaim

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

type claimRuntimeMutator func(*Runtime)

func newClaimTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(corev1) error = %v", err)
	}
	return scheme
}

func newClaimTestClientBuilder(t *testing.T, statusObjects ...client.Object) (*runtime.Scheme, *fake.ClientBuilder) {
	t.Helper()

	scheme := newClaimTestScheme(t)
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, obj := range statusObjects {
		if obj == nil {
			continue
		}
		builder = builder.WithStatusSubresource(obj)
	}
	return scheme, builder
}

func newClaimTestReconciler(t *testing.T, scheme *runtime.Scheme, c client.Client, mutators ...claimRuntimeMutator) Reconciler {
	t.Helper()

	runtimeCfg := Runtime{
		Client: c,
		Scheme: scheme,
	}
	for _, mutate := range mutators {
		mutate(&runtimeCfg)
	}
	return NewReconciler(runtimeCfg)
}

func reconcileClaimOnce(
	t *testing.T,
	c client.Client,
	reconciler Reconciler,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) (recon.Result, *openbaov1alpha1.OpenBaoClusterClaim) {
	t.Helper()

	result, err := reconciler.Reconcile(context.Background(), client.ObjectKeyFromObject(claim), testr.New(t))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := &openbaov1alpha1.OpenBaoClusterClaim{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(claim), updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	return result, updated
}
