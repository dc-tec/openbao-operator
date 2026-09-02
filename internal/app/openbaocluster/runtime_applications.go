package openbaocluster

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	initmanagerport "github.com/dc-tec/openbao-operator/internal/port/initmanager"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// RuntimeKubernetesConfig groups Kubernetes collaborators used to construct
// the OpenBaoCluster applications.
type RuntimeKubernetesConfig struct {
	Client            client.Client
	APIReader         client.Reader
	Scheme            *runtime.Scheme
	RestConfig        *rest.Config
	OperatorNamespace string
	Platform          string
	Recorder          events.EventRecorder
}

// RuntimeOIDCConfig groups OIDC discovery state used by workload bootstrap.
type RuntimeOIDCConfig struct {
	Issuer              string
	DiscoveryURL        string
	DiscoveryCAPEM      string
	JWKSURL             string
	JWKSCAPEM           string
	JWTKeys             []string
	Discover            func(context.Context, *rest.Config) (*OIDCConfig, error)
	DiscoveryStatusCode func(error) (int, bool)
}

// RuntimeOpenBaoConfig groups OpenBao-facing collaborators.
type RuntimeOpenBaoConfig struct {
	TLSReload         TLSReloadSignaler
	InitManager       initmanagerport.Manager
	SmartClientConfig portopenbao.ClientConfig
	ClientForPod      func(context.Context, *openbaov1alpha1.OpenBaoCluster, string) (portopenbao.ClusterActions, error)
}

// RuntimeImageVerificationConfig groups image verification collaborators.
type RuntimeImageVerificationConfig struct {
	ImageVerifier         imageverify.Verifier
	OperatorImageVerifier imageverify.Verifier
	Infra                 InfraImageVerificationRuntime
}

// RuntimeApplicationsConfig contains the process-level collaborators required
// to construct all OpenBaoCluster applications once at startup.
type RuntimeApplicationsConfig struct {
	Kubernetes        RuntimeKubernetesConfig
	OIDC              RuntimeOIDCConfig
	OpenBao           RuntimeOpenBaoConfig
	ImageVerification RuntimeImageVerificationConfig
}

// NewRuntimeApplications constructs the workload, admin-operations, status,
// and deletion applications from process-level collaborators.
func NewRuntimeApplications(config RuntimeApplicationsConfig) *Applications {
	clientForPod := config.OpenBao.ClientForPod
	if clientForPod == nil {
		clientForPod = func(context.Context, *openbaov1alpha1.OpenBaoCluster, string) (portopenbao.ClusterActions, error) {
			return nil, fmt.Errorf("OpenBao pod client factory is not configured")
		}
	}

	var scaleDownRuntime initmanagerport.ScaleDownRuntime
	if provider, ok := config.OpenBao.InitManager.(initmanagerport.ScaleDownProvider); ok {
		scaleDownRuntime = provider.ScaleDownRuntime()
	}
	var readReplicaScaleDownRuntime initmanagerport.ReadReplicaScaleDownRuntime
	if provider, ok := config.OpenBao.InitManager.(initmanagerport.ReadReplicaScaleDownProvider); ok {
		readReplicaScaleDownRuntime = provider.ReadReplicaScaleDownRuntime()
	}
	var autopilotRuntime initmanagerport.AutopilotRuntime
	if provider, ok := config.OpenBao.InitManager.(initmanagerport.AutopilotProvider); ok {
		autopilotRuntime = provider.AutopilotRuntime()
	}
	var membershipRuntime StatusMembershipRuntime
	if provider, ok := config.OpenBao.InitManager.(initmanagerport.MembershipProvider); ok {
		membershipRuntime = provider.MembershipRuntime()
	}

	workloadReconcilers := []SubReconciler{
		NewCertificatesReconciler(CertificatesDependencies{
			Client:   config.Kubernetes.Client,
			Scheme:   config.Kubernetes.Scheme,
			Reloader: config.OpenBao.TLSReload,
		}),
		NewInfraReconciler(InfraDependencies{
			Kubernetes: InfraKubernetesRuntime{
				Client:            config.Kubernetes.Client,
				APIReader:         config.Kubernetes.APIReader,
				Scheme:            config.Kubernetes.Scheme,
				OperatorNamespace: config.Kubernetes.OperatorNamespace,
				Platform:          config.Kubernetes.Platform,
			},
			OIDC: InfraOIDCRuntime{
				RestConfig:          config.Kubernetes.RestConfig,
				OIDCIssuer:          config.OIDC.Issuer,
				OIDCDiscoveryURL:    config.OIDC.DiscoveryURL,
				OIDCDiscoveryCAPEM:  config.OIDC.DiscoveryCAPEM,
				OIDCJWKSURL:         config.OIDC.JWKSURL,
				OIDCJWKSCAPEM:       config.OIDC.JWKSCAPEM,
				OIDCJWTKeys:         config.OIDC.JWTKeys,
				DiscoverOIDCConfig:  config.OIDC.Discover,
				DiscoveryStatusCode: config.OIDC.DiscoveryStatusCode,
			},
			ImageVerification: config.ImageVerification.Infra,
			Events:            InfraEventRuntime{Recorder: config.Kubernetes.Recorder},
			Pods: InfraPodRuntime{
				ClientForPodFunc: func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (ScaleDownPodClient, error) {
					return clientForPod(ctx, cluster, podName)
				},
			},
			ScaleDown: InfraScaleDownRuntime{
				Runtime:            scaleDownRuntime,
				ReadReplicaRuntime: readReplicaScaleDownRuntime,
			},
		}),
		NewStorageReconciler(StorageDependencies{
			Resources: StorageResourceRuntime{Client: config.Kubernetes.Client},
			Events:    StorageEventRuntime{Recorder: config.Kubernetes.Recorder},
		}),
		NewStorageResizeRestartReconciler(StorageResizeRestartDependencies{
			Resources: StorageResourceRuntime{
				Client:    config.Kubernetes.Client,
				APIReader: config.Kubernetes.APIReader,
			},
			Events: StorageEventRuntime{Recorder: config.Kubernetes.Recorder},
			Pods: StoragePodRuntime{
				ClientForPodFunc: func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (StoragePodClient, error) {
					return clientForPod(ctx, cluster, podName)
				},
			},
		}),
	}
	workloadReconcilers = AppendInitAndAutopilotReconcilers(
		workloadReconcilers,
		config.OpenBao.InitManager,
		autopilotRuntime,
		config.Kubernetes.APIReader,
		config.Kubernetes.Recorder,
		constants.RequeueShort,
	)

	return NewApplications(ApplicationsConfig{
		Client:              config.Kubernetes.Client,
		WorkloadReconcilers: workloadReconcilers,
		WorkloadPolicy:      DefaultWorkloadResultPolicy(),
		AdminOpsApplication: NewAdminOpsApplication(AdminOpsDependencies{
			Client:                config.Kubernetes.Client,
			APIReader:             config.Kubernetes.APIReader,
			Scheme:                config.Kubernetes.Scheme,
			Recorder:              config.Kubernetes.Recorder,
			SmartClientConfig:     config.OpenBao.SmartClientConfig,
			ImageVerifier:         config.ImageVerification.ImageVerifier,
			OperatorImageVerifier: config.ImageVerification.OperatorImageVerifier,
			RequeueShort:          constants.RequeueShort,
			Platform:              config.Kubernetes.Platform,
		}),
		StatusDependencies: StatusDependencies{
			Reader:            config.Kubernetes.Client,
			MembershipRuntime: membershipRuntime,
			PodObserverFactory: func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (StatusPodObserver, error) {
				actions, err := clientForPod(ctx, cluster, podName)
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
		},
		DeletionDependencies: DeletionDependencies{Client: config.Kubernetes.Client},
		StatusIntegration: StatusIntegrationDependencies{
			Client:            config.Kubernetes.Client,
			APIReader:         config.Kubernetes.APIReader,
			Scheme:            config.Kubernetes.Scheme,
			OperatorNamespace: config.Kubernetes.OperatorNamespace,
			Platform:          config.Kubernetes.Platform,
		},
		InitializationConfigured: config.OpenBao.InitManager != nil,
	})
}
