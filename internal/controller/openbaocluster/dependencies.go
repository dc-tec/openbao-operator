package openbaocluster

import (
	"context"
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	initmanagerport "github.com/dc-tec/openbao-operator/internal/port/initmanager"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	portsecurity "github.com/dc-tec/openbao-operator/internal/port/security"
	"k8s.io/client-go/rest"
)

func (r *OpenBaoClusterReconciler) infraDependencies() appopenbaocluster.InfraDependencies {
	var scaleDownRuntime initmanagerport.ScaleDownRuntime
	if provider, ok := r.InitManager.(initmanagerport.ScaleDownProvider); ok {
		scaleDownRuntime = provider.ScaleDownRuntime()
	}
	var readReplicaScaleDownRuntime initmanagerport.ReadReplicaScaleDownRuntime
	if provider, ok := r.InitManager.(initmanagerport.ReadReplicaScaleDownProvider); ok {
		readReplicaScaleDownRuntime = provider.ReadReplicaScaleDownRuntime()
	}

	return appopenbaocluster.InfraDependencies{
		Kubernetes: appopenbaocluster.InfraKubernetesRuntime{
			Client:            r.Client,
			APIReader:         r.APIReader,
			Scheme:            r.ControllerRuntime.Scheme,
			OperatorNamespace: r.OperatorNamespace,
			Platform:          r.Platform,
		},
		OIDC: appopenbaocluster.InfraOIDCRuntime{
			RestConfig:          r.RestConfig,
			OIDCIssuer:          r.OIDCIssuer,
			OIDCDiscoveryURL:    r.OIDCDiscoveryURL,
			OIDCDiscoveryCAPEM:  r.OIDCDiscoveryCAPEM,
			OIDCJWKSURL:         r.OIDCJWKSURL,
			OIDCJWKSCAPEM:       r.OIDCJWKSCAPEM,
			OIDCJWTKeys:         r.OIDCJWTKeys,
			DiscoverOIDCConfig:  r.discoverOIDCConfigAdapter(),
			DiscoveryStatusCode: r.oidcDiscoveryStatusCodeAdapter(),
		},
		ImageVerification: appopenbaocluster.InfraImageVerificationRuntime{
			OperatorImageVerifier:              r.OperatorImageVerifier,
			VerifyImageFunc:                    r.verifyImageRef,
			VerifyOperatorImage:                portsecurity.VerifyOperatorImageForCluster,
			IsMainImageVerificationEnabled:     portsecurity.IsMainImageVerificationEnabled,
			IsOperatorImageVerificationEnabled: portsecurity.IsOperatorImageVerificationEnabled,
		},
		Events: appopenbaocluster.InfraEventRuntime{
			Recorder: r.Recorder,
		},
		Pods: appopenbaocluster.InfraPodRuntime{
			ClientForPodFunc: func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (appopenbaocluster.ScaleDownPodClient, error) {
				return r.clientForPod(ctx, cluster, podName)
			},
		},
		ScaleDown: appopenbaocluster.InfraScaleDownRuntime{
			Runtime:            scaleDownRuntime,
			ReadReplicaRuntime: readReplicaScaleDownRuntime,
		},
	}
}

func (r *OpenBaoClusterReconciler) certificatesDependencies() appopenbaocluster.CertificatesDependencies {
	return appopenbaocluster.CertificatesDependencies{
		Client:   r.Client,
		Scheme:   r.ControllerRuntime.Scheme,
		Reloader: r.TLSReload,
	}
}

func (r *OpenBaoClusterReconciler) acmeIntegrationDependencies() appopenbaocluster.ACMEIntegrationDependencies {
	return appopenbaocluster.ACMEIntegrationDependencies{
		Client:            r.Client,
		APIReader:         r.APIReader,
		Scheme:            r.ControllerRuntime.Scheme,
		OperatorNamespace: r.OperatorNamespace,
		Platform:          r.Platform,
	}
}

func (r *OpenBaoClusterReconciler) gatewayIntegrationDependencies() appopenbaocluster.GatewayIntegrationDependencies {
	return appopenbaocluster.GatewayIntegrationDependencies{
		Client:            r.Client,
		APIReader:         r.APIReader,
		Scheme:            r.ControllerRuntime.Scheme,
		OperatorNamespace: r.OperatorNamespace,
		Platform:          r.Platform,
	}
}

func (r *OpenBaoClusterReconciler) ingressIntegrationDependencies() appopenbaocluster.IngressIntegrationDependencies {
	return appopenbaocluster.IngressIntegrationDependencies{
		Client:            r.Client,
		APIReader:         r.APIReader,
		Scheme:            r.ControllerRuntime.Scheme,
		OperatorNamespace: r.OperatorNamespace,
		Platform:          r.Platform,
	}
}

func (r *OpenBaoClusterReconciler) apiServerNetworkDependencies() appopenbaocluster.APIServerNetworkDependencies {
	return appopenbaocluster.APIServerNetworkDependencies{
		Client:            r.Client,
		APIReader:         r.APIReader,
		Scheme:            r.ControllerRuntime.Scheme,
		OperatorNamespace: r.OperatorNamespace,
		Platform:          r.Platform,
	}
}

func (r *OpenBaoClusterReconciler) storageDependencies() appopenbaocluster.StorageDependencies {
	return appopenbaocluster.StorageDependencies{
		Resources: appopenbaocluster.StorageResourceRuntime{
			Client: r.Client,
		},
		Events: appopenbaocluster.StorageEventRuntime{
			Recorder: r.Recorder,
		},
	}
}

func (r *OpenBaoClusterReconciler) storageResizeRestartDependencies() appopenbaocluster.StorageResizeRestartDependencies {
	return appopenbaocluster.StorageResizeRestartDependencies{
		Resources: appopenbaocluster.StorageResourceRuntime{
			Client:    r.Client,
			APIReader: r.APIReader,
		},
		Events: appopenbaocluster.StorageEventRuntime{
			Recorder: r.Recorder,
		},
		Pods: appopenbaocluster.StoragePodRuntime{
			ClientForPodFunc: func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (appopenbaocluster.StoragePodClient, error) {
				return r.clientForPod(ctx, cluster, podName)
			},
		},
	}
}

func (r *OpenBaoClusterReconciler) statusDependencies() appopenbaocluster.StatusDependencies {
	var membershipRuntime appopenbaocluster.StatusMembershipRuntime
	if provider, ok := r.InitManager.(initmanagerport.MembershipProvider); ok {
		membershipRuntime = provider.MembershipRuntime()
	}

	return appopenbaocluster.StatusDependencies{
		Reader:            r.Client,
		MembershipRuntime: membershipRuntime,
		PodObserverFactory: func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (appopenbaocluster.StatusPodObserver, error) {
			actions, err := r.clientForPod(ctx, cluster, podName)
			if err != nil {
				return nil, err
			}

			observer, ok := actions.(interface {
				Health(ctx context.Context) (*portopenbao.HealthStatus, error)
			})
			if !ok {
				return nil, fmt.Errorf("OpenBao client for pod %s does not expose health observation", podName)
			}
			return observer, nil
		},
	}
}

func (r *OpenBaoClusterReconciler) adminOpsDependencies() appopenbaocluster.AdminOpsDependencies {
	return appopenbaocluster.AdminOpsDependencies{
		Client:                r.Client,
		APIReader:             r.APIReader,
		Scheme:                r.ControllerRuntime.Scheme,
		Recorder:              r.Recorder,
		OperatorNamespace:     r.OperatorNamespace,
		OIDCIssuer:            r.OIDCIssuer,
		OIDCDiscoveryURL:      r.OIDCDiscoveryURL,
		OIDCDiscoveryCAPEM:    r.OIDCDiscoveryCAPEM,
		OIDCJWKSURL:           r.OIDCJWKSURL,
		OIDCJWKSCAPEM:         r.OIDCJWKSCAPEM,
		OIDCJWTKeys:           r.OIDCJWTKeys,
		SmartClientConfig:     r.SmartClientConfig,
		ImageVerifier:         r.ImageVerifier,
		OperatorImageVerifier: r.OperatorImageVerifier,
		RequeueShort:          constants.RequeueShort,
		Platform:              r.Platform,
	}
}

func (r *OpenBaoClusterReconciler) deletionDependencies() appopenbaocluster.DeletionDependencies {
	return appopenbaocluster.DeletionDependencies{
		Client: r.Client,
	}
}

func (r *OpenBaoClusterReconciler) discoverOIDCConfigAdapter() func(ctx context.Context, cfg *rest.Config) (*appopenbaocluster.OIDCConfig, error) {
	return func(ctx context.Context, cfg *rest.Config) (*appopenbaocluster.OIDCConfig, error) {
		if r.DiscoverOIDCConfig == nil {
			return nil, fmt.Errorf("OIDC discovery is not configured")
		}
		discovered, err := r.DiscoverOIDCConfig(ctx, cfg, "")
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

func (r *OpenBaoClusterReconciler) oidcDiscoveryStatusCodeAdapter() func(err error) (int, bool) {
	return func(err error) (int, bool) {
		if r.OIDCStatusCode == nil {
			return 0, false
		}
		return r.OIDCStatusCode(err)
	}
}
