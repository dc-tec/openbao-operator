package controller

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/dc-tec/openbao-operator/internal/adapter/auth"
	"github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/adapter/raft"
	"github.com/dc-tec/openbao-operator/internal/adapter/security"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	appopenbaorestore "github.com/dc-tec/openbao-operator/internal/app/openbaorestore"
	openbaoclustercontroller "github.com/dc-tec/openbao-operator/internal/controller/openbaocluster"
	openbaorestorecontroller "github.com/dc-tec/openbao-operator/internal/controller/openbaorestore"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	certmanager "github.com/dc-tec/openbao-operator/internal/service/certs"
	initmanager "github.com/dc-tec/openbao-operator/internal/service/init"
)

type controllerProcessRuntime struct {
	operatorNamespace        string
	platform                 string
	singleTenantMode         bool
	admissionTracker         *admission.Tracker
	oidcRuntime              appopenbaocluster.RuntimeOIDCConfig
	openBaoRuntime           appopenbaocluster.RuntimeOpenBaoConfig
	imageVerificationRuntime appopenbaocluster.RuntimeImageVerificationConfig
}

func buildControllerProcessRuntime(
	mgr ctrl.Manager,
	cfg runConfig,
	platform string,
	singleTenantMode bool,
) (controllerProcessRuntime, error) {
	config := mgr.GetConfig()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return controllerProcessRuntime{}, fmt.Errorf("unable to create Kubernetes clientset: %w", err)
	}

	reloadSignaler := certmanager.NewKubernetesReloadSignaler(clientset)
	smartClientConfig := portopenbao.ClientConfig{
		RateLimitQPS:                   cfg.clientQPS,
		RateLimitBurst:                 cfg.clientBurst,
		CircuitBreakerFailureThreshold: cfg.clientCBFailureThreshold,
		CircuitBreakerOpenDuration:     cfg.clientCBOpenDuration,
		JWTAuthStrategy:                cfg.jwtAuthStrategy,
	}

	clientMgr := openbao.NewClientManager(smartClientConfig)
	raftMgr := raft.NewManager(clientset, raftClientFactoryProvider{clientManager: clientMgr})
	initMgr, err := initmanager.NewManager(
		config,
		clientset,
		clientMgr,
		raftMgr,
		mgr.GetEventRecorder(controllerNameOpenBaoCluster),
	)
	if err != nil {
		return controllerProcessRuntime{}, fmt.Errorf("unable to create initialization manager: %w", err)
	}
	imageVerifier := security.NewImageVerifier(mgr.GetLogger().WithName("image-verifier"), mgr.GetAPIReader(), nil)
	operatorImageVerifier := security.NewImageVerifier(
		mgr.GetLogger().WithName("operator-image-verifier"),
		mgr.GetAPIReader(),
		nil,
	)

	operatorNamespace := operatorNamespaceFromEnv()
	if missingHelperImages := unavailableHelperImageDefaultFields(); len(missingHelperImages) > 0 {
		setupLog.Info(
			"Operator-managed default helper images are unavailable until OPERATOR_VERSION is configured; "+
				"clusters can still override helper images explicitly in the cluster spec",
			"fields",
			missingHelperImages,
		)
	}

	oidcConfig := discoverStartupOIDC(config)
	admissionTracker := initializeAdmissionTracker(mgr, cfg.admissionEnforcement, cfg.admissionStartupTimeout)
	clientFactory := func(config portopenbao.ClientConfig) (portopenbao.ClusterActions, error) {
		return openbao.NewClient(config)
	}

	return controllerProcessRuntime{
		operatorNamespace: operatorNamespace,
		platform:          platform,
		singleTenantMode:  singleTenantMode,
		admissionTracker:  admissionTracker,
		oidcRuntime: appopenbaocluster.RuntimeOIDCConfig{
			Issuer:              oidcConfig.IssuerURL,
			DiscoveryURL:        oidcConfig.OIDCDiscoveryURL,
			DiscoveryCAPEM:      oidcConfig.OIDCDiscoveryCAPEM,
			JWKSURL:             oidcConfig.JWKSURL,
			JWKSCAPEM:           oidcConfig.JWKSCAPEM,
			JWTKeys:             oidcConfig.JWKSKeys,
			Discover:            openBaoClusterOIDCDiscoverer(auth.DiscoverConfig),
			DiscoveryStatusCode: portauth.DiscoveryStatusCode,
		},
		openBaoRuntime: appopenbaocluster.RuntimeOpenBaoConfig{
			TLSReload:         reloadSignaler,
			InitManager:       initMgr,
			Raft:              raftMgr,
			SmartClientConfig: smartClientConfig,
			ClientForPod: openBaoClusterPodClientFactory(
				mgr.GetClient(),
				smartClientConfig,
				clientFactory,
			),
		},
		imageVerificationRuntime: openBaoClusterImageVerificationRuntime(imageVerifier, operatorImageVerifier),
	}, nil
}

func setupControllers(mgr ctrl.Manager, runtime controllerProcessRuntime) error {
	if runtime.openBaoRuntime.Raft == nil {
		return fmt.Errorf("OpenBaoCluster Raft runtime is required")
	}
	if runtime.openBaoRuntime.InitManager == nil {
		return fmt.Errorf("OpenBaoCluster initialization manager is required")
	}

	applications := buildOpenBaoClusterApplications(mgr, runtime)
	if err := (&openbaoclustercontroller.OpenBaoClusterReconciler{
		Client: mgr.GetClient(),
		ControllerRuntime: openbaoclustercontroller.ControllerRuntime{
			APIReader:        mgr.GetAPIReader(),
			AdmissionTracker: runtime.admissionTracker,
			Recorder:         mgr.GetEventRecorder(controllerNameOpenBaoCluster),
			SingleTenantMode: runtime.singleTenantMode,
		},
		Applications: applications,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller %s: %w", controllerNameOpenBaoCluster, err)
	}

	if err := (&openbaorestorecontroller.OpenBaoRestoreReconciler{
		Client:           mgr.GetClient(),
		AdmissionTracker: runtime.admissionTracker,
		RestoreReconciler: appopenbaorestore.NewRestoreReconciler(appopenbaorestore.RestoreDependencies{
			Client:                mgr.GetClient(),
			APIReader:             mgr.GetAPIReader(),
			Scheme:                mgr.GetScheme(),
			Recorder:              mgr.GetEventRecorder(controllerNameOpenBaoRestore),
			OperatorImageVerifier: runtime.imageVerificationRuntime.OperatorImageVerifier,
			Platform:              runtime.platform,
			ClientConfig:          runtime.openBaoRuntime.SmartClientConfig,
		}),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller %s: %w", controllerNameOpenBaoRestore, err)
	}

	return nil
}

func addManagerHealthChecks(mgr ctrl.Manager) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return err
	}

	return nil
}
