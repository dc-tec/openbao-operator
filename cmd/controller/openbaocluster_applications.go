package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/platform/openbaotls"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	portsecurity "github.com/dc-tec/openbao-operator/internal/port/security"
)

func buildOpenBaoClusterApplications(
	mgr ctrl.Manager,
	runtime controllerProcessRuntime,
) *appopenbaocluster.Applications {
	return appopenbaocluster.NewRuntimeApplications(appopenbaocluster.RuntimeApplicationsConfig{
		Kubernetes: appopenbaocluster.RuntimeKubernetesConfig{
			Client:            mgr.GetClient(),
			APIReader:         mgr.GetAPIReader(),
			Scheme:            mgr.GetScheme(),
			RestConfig:        mgr.GetConfig(),
			OperatorNamespace: runtime.operatorNamespace,
			Platform:          runtime.platform,
			Recorder:          mgr.GetEventRecorder(controllerNameOpenBaoCluster),
		},
		OIDC:              runtime.oidcRuntime,
		OpenBao:           runtime.openBaoRuntime,
		ImageVerification: runtime.imageVerificationRuntime,
	})
}

func openBaoClusterImageVerificationRuntime(
	imageVerifier imageverify.Verifier,
	operatorImageVerifier imageverify.Verifier,
) appopenbaocluster.RuntimeImageVerificationConfig {
	return appopenbaocluster.RuntimeImageVerificationConfig{
		ImageVerifier:         imageVerifier,
		OperatorImageVerifier: operatorImageVerifier,
		Infra: appopenbaocluster.InfraImageVerificationRuntime{
			OperatorImageVerifier: operatorImageVerifier,
			VerifyImageFunc: func(
				ctx context.Context,
				logger logr.Logger,
				cluster *openbaov1alpha1.OpenBaoCluster,
				imageRef string,
			) (string, error) {
				if !portsecurity.IsMainImageVerificationEnabled(cluster) {
					return "", nil
				}
				return portsecurity.VerifyImageForCluster(ctx, logger, imageVerifier, cluster, imageRef)
			},
			VerifyOperatorImage:                portsecurity.VerifyOperatorImageForCluster,
			IsMainImageVerificationEnabled:     portsecurity.IsMainImageVerificationEnabled,
			IsOperatorImageVerificationEnabled: portsecurity.IsOperatorImageVerificationEnabled,
		},
	}
}

func openBaoClusterPodClientFactory(
	c client.Client,
	baseConfig portopenbao.ClientConfig,
	factory portopenbao.ClientFactory,
) func(context.Context, *openbaov1alpha1.OpenBaoCluster, string) (portopenbao.ClusterActions, error) {
	return func(
		ctx context.Context,
		cluster *openbaov1alpha1.OpenBaoCluster,
		podName string,
	) (portopenbao.ClusterActions, error) {
		if factory == nil {
			return nil, fmt.Errorf("OpenBao client factory is not configured")
		}

		config := baseConfig
		config.BaseURL = "https://" + fmt.Sprintf("%s.%s.%s.svc:8200", podName, cluster.Name, cluster.Namespace)
		config.TLSServerName = portopenbao.ComputeTLSServerName(cluster)

		caCert, err := openbaotls.LoadClusterTrustBundle(ctx, c, cluster)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to load cluster trust bundle for %s/%s: %w",
				cluster.Namespace,
				cluster.Name,
				err,
			)
		}
		config.CACert = caCert

		return factory(config)
	}
}

func openBaoClusterOIDCDiscoverer(discover portauth.DiscoverConfigFunc) func(
	ctx context.Context,
	config *rest.Config,
) (*appopenbaocluster.OIDCConfig, error) {
	return func(ctx context.Context, config *rest.Config) (*appopenbaocluster.OIDCConfig, error) {
		if discover == nil {
			return nil, fmt.Errorf("OIDC discovery is not configured")
		}
		discovered, err := discover(ctx, config, "")
		if err != nil {
			return nil, err
		}
		if discovered == nil {
			return nil, nil
		}
		return &appopenbaocluster.OIDCConfig{
			IssuerURL:          discovered.IssuerURL,
			OIDCDiscoveryURL:   discovered.OIDCDiscoveryURL,
			OIDCDiscoveryCAPEM: discovered.OIDCDiscoveryCAPEM,
			JWKSURL:            discovered.JWKSURL,
			JWKSCAPEM:          discovered.JWKSCAPEM,
			JWKSKeys:           discovered.JWKSKeys,
		}, nil
	}
}
