package controller

import (
	"context"
	"fmt"
	"os"
	"strings"

	"k8s.io/client-go/kubernetes"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/openbaotls"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/configuration"
)

func operatorPolicyClientFactory(
	clientset kubernetes.Interface,
	manager *openbao.ClientManager,
) configuration.PolicyClientFactory {
	return func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (portopenbao.PolicyWriter, error) {
		trust, err := openbaotls.ReadClientTrustBundle(ctx, clientset, cluster)
		if err != nil {
			return nil, err
		}
		token, err := os.ReadFile("/var/run/secrets/tokens/openbao-token")
		if err != nil {
			return nil, fmt.Errorf("read projected OpenBao JWT: %w", err)
		}
		defer clear(token)
		jwt := strings.TrimSpace(string(token))
		if jwt == "" {
			return nil, fmt.Errorf("projected OpenBao JWT is empty")
		}
		service := cluster.Name
		if cluster.Spec.Service != nil ||
			(cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled) ||
			(cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled) {
			service += "-public"
		}
		baseURL := fmt.Sprintf("https://%s.%s.svc:%d", service, cluster.Namespace, constants.PortAPI)
		factory := manager.FactoryFor(cluster.Namespace+"/"+cluster.Name, trust.CACert, trust.TLSServerName)
		return factory.NewWithJWT(ctx, baseURL, portauth.RoleNameOperator, jwt)
	}
}
