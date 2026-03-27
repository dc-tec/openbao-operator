package init

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

// newOpenBaoClient constructs a minimal OpenBao client for talking to the pod-0 instance
// of the StatefulSet using the per-cluster TLS CA bundle.
func (m *Manager) newOpenBaoClient(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (*openbao.Client, error) {
	if strings.TrimSpace(cluster.Name) == "" || strings.TrimSpace(cluster.Namespace) == "" {
		return nil, fmt.Errorf("cluster name and namespace are required to build OpenBao client")
	}

	baseURL := fmt.Sprintf("https://%s-0.%s.%s.svc:%d", cluster.Name, cluster.Name, cluster.Namespace, constants.PortAPI)

	caSecretName := cluster.Name + constants.SuffixTLSCA
	secret, err := m.clientset.CoreV1().Secrets(cluster.Namespace).Get(ctx, caSecretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("TLS CA Secret %s/%s not found", cluster.Namespace, caSecretName)
		}
		if apierrors.IsForbidden(err) {
			return nil, operatorerrors.WrapTransientKubernetesAPI(
				fmt.Errorf("failed to get TLS CA Secret %s/%s: %w", cluster.Namespace, caSecretName, err),
			)
		}
		return nil, fmt.Errorf("failed to get TLS CA Secret %s/%s: %w", cluster.Namespace, caSecretName, err)
	}

	caCert, ok := secret.Data["ca.crt"]
	if !ok || len(caCert) == 0 {
		return nil, fmt.Errorf("TLS CA Secret %s/%s missing 'ca.crt' key", cluster.Namespace, caSecretName)
	}

	clusterKey := fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name)
	factory := m.clientMgr.FactoryFor(clusterKey, caCert)
	if factory == nil {
		return nil, fmt.Errorf("client manager returned nil factory for cluster %s", clusterKey)
	}

	client, err := factory.New(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenBao client for %s: %w", baseURL, err)
	}

	return client, nil
}
