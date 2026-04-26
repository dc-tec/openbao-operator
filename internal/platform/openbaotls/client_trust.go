package openbaotls

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// ClientTrustBundle describes the TLS trust inputs for controller-side OpenBao clients.
type ClientTrustBundle struct {
	CACert        []byte
	TLSServerName string
}

// ReadClientTrustBundle resolves and reads the trust bundle needed by controller-side
// OpenBao clients. ACME clusters do not have operator-managed TLS Secrets, so this
// follows the shared OpenBao trust contract instead of assuming <cluster>-tls-ca.
func ReadClientTrustBundle(
	ctx context.Context,
	clientset kubernetes.Interface,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (ClientTrustBundle, error) {
	if clientset == nil {
		return ClientTrustBundle{}, fmt.Errorf("kubernetes clientset is required")
	}

	source, err := portopenbao.ResolveClientTrustBundle(cluster)
	if err != nil {
		return ClientTrustBundle{}, err
	}

	trust := ClientTrustBundle{
		TLSServerName: portopenbao.ComputeTLSServerName(cluster),
	}
	if source.UseSystemRoots {
		return trust, nil
	}

	secret, err := clientset.CoreV1().Secrets(cluster.Namespace).Get(ctx, source.SecretName, metav1.GetOptions{})
	if err != nil {
		msg := fmt.Errorf("failed to get OpenBao trust Secret %s/%s key %q: %w", cluster.Namespace, source.SecretName, source.SecretKey, err)
		if apierrors.IsForbidden(err) {
			return ClientTrustBundle{}, operatorerrors.WrapTransientKubernetesAPI(msg)
		}
		return ClientTrustBundle{}, msg
	}

	caCert, ok := secret.Data[source.SecretKey]
	if !ok || len(caCert) == 0 {
		return ClientTrustBundle{}, fmt.Errorf("OpenBao trust Secret %s/%s missing %q key", cluster.Namespace, source.SecretName, source.SecretKey)
	}

	trust.CACert = caCert
	return trust, nil
}
