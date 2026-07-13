package openbaocluster

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
)

func newOpenBaoClusterTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add appsv1 scheme: %v", err)
	}
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("install gatewayv1 scheme: %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add openbao scheme: %v", err)
	}
	return scheme
}

func newStatusTestApplications(c client.Client, scheme *runtime.Scheme) *appopenbaocluster.Applications {
	integrationDeps := appopenbaocluster.StatusIntegrationDependencies{
		Client:    c,
		APIReader: c,
		Scheme:    scheme,
	}
	return appopenbaocluster.NewApplications(appopenbaocluster.ApplicationsConfig{
		Client: c,
		StatusDependencies: appopenbaocluster.StatusDependencies{
			Reader: c,
		},
		DeletionDependencies: appopenbaocluster.DeletionDependencies{Client: c},
		StatusIntegration:    integrationDeps,
	})
}

func newOpenBaoClusterStatusTestObject() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "example",
			Namespace:       "default",
			Generation:      2,
			ResourceVersion: "1",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 1,
			Profile:  openbaov1alpha1.ProfileHardened,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
			},
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
			},
		},
	}
}
