//go:build integration
// +build integration

package bootstrap

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

func newTestClientWithObjects(t interface{ Helper() }, objs ...client.Object) client.Client {
	t.Helper()

	builder := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithReturnManagedFields()
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

func tlsCASecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixTLSCA
}

func deleteConfigMap(ctx context.Context, k8sClient client.Client, cluster *openbaov1alpha1.OpenBaoCluster) error {
	configMap := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      resourceidentity.ConfigMapName(cluster),
	}, configMap)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if err := k8sClient.Delete(ctx, configMap); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

func deleteSecrets(ctx context.Context, k8sClient client.Client, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil {
		return nil
	}

	secretNames := []string{}

	mode := cluster.Spec.TLS.Mode
	if mode == "" {
		mode = openbaov1alpha1.TLSModeOperatorManaged
	}
	if cluster.Spec.TLS.Enabled && mode == openbaov1alpha1.TLSModeOperatorManaged {
		secretNames = append(secretNames, resourceidentity.TLSServerSecretName(cluster), tlsCASecretName(cluster))
	}

	staticUnseal := cluster.Spec.Unseal == nil || cluster.Spec.Unseal.Type == "" || cluster.Spec.Unseal.Type == "static"
	if staticUnseal {
		secretNames = append(secretNames, resourceidentity.UnsealSecretName(cluster))
	}

	for _, name := range secretNames {
		if name == "" {
			continue
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: cluster.Namespace,
			},
		}
		if err := k8sClient.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
