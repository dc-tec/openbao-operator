package networking

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

var testScheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)
	return scheme
}()

//nolint:unparam // namespace is used by tests across the package
func newMinimalCluster(name, namespace string) *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID(name + "-uid")},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:       "2.4.4",
			Image:         "openbao/openbao:2.4.4",
			Replicas:      3,
			TLS:           openbaov1alpha1.TLSConfig{Enabled: true, RotationPeriod: "720h"},
			Storage:       openbaov1alpha1.StorageConfig{Size: "10Gi"},
			InitContainer: &openbaov1alpha1.InitContainerConfig{Image: "openbao/openbao-init:latest"},
		},
	}
}
