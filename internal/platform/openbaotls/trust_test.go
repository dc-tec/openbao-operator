package openbaotls

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "github.com/kubebao/openbao-operator/api/v1alpha1"
)

func TestLoadClusterTrustBundleRejectsMissingAndEmptyCA(t *testing.T) {
	for _, tc := range []struct {
		name      string
		data      map[string][]byte
		wantError bool
	}{
		{name: "missing key", data: map[string][]byte{}, wantError: true},
		{name: "empty CA", data: map[string][]byte{"ca.crt": {}}, wantError: true},
		{name: "CA present", data: map[string][]byte{"ca.crt": []byte("CA")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(scheme))
			cluster := &api.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "test"},
				Spec:       api.OpenBaoClusterSpec{TLS: api.TLSConfig{Enabled: true}},
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "target-tls-ca", Namespace: cluster.Namespace},
				Data:       tc.data,
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
			ca, err := LoadClusterTrustBundle(t.Context(), c, cluster)
			if tc.wantError {
				require.ErrorContains(t, err, `trust bundle key "ca.crt" missing or empty in secret test/target-tls-ca`)
				require.Nil(t, ca)
			} else {
				require.NoError(t, err)
				require.Equal(t, []byte("CA"), ca)
			}
		})
	}
}
