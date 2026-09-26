package controller

import (
	"context"
	"fmt"

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
	tokens portauth.ControllerTokenProvider,
) configuration.PolicyClientFactory {
	return func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (portopenbao.PolicyClient, error) {
		trust, err := openbaotls.ReadClientTrustBundle(ctx, clientset, cluster)
		if err != nil {
			return nil, err
		}
		jwt, err := tokens.Token(ctx, cluster)
		if err != nil {
			return nil, err
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
