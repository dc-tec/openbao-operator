package controller

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/dc-tec/openbao-operator/internal/adapter/auth"
	"github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/adapter/security"
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
	operatorNamespace                               string
	operatorServiceAccountName                      string
	platform                                        string
	singleTenantMode                                bool
	enableServiceClaims                             bool
	serviceClaimsAPIServerCIDR                      string
	serviceClaimsAPIServerEndpointIPs               []string
	serviceClaimsDNSEndpointIPs                     []string
	serviceClaimsTransitUnsealAddress               string
	serviceClaimsTransitUnsealKeyName               string
	serviceClaimsTransitUnsealMountPath             string
	serviceClaimsTransitUnsealNamespace             string
	serviceClaimsTransitUnsealTLSCACert             string
	serviceClaimsTransitUnsealTLSServerName         string
	serviceClaimsTransitUnsealCredentialsSecretName string
	admissionTracker                                *admission.Tracker
	oidcRuntime                                     openbaoclustercontroller.OIDCRuntime
	openBaoRuntime                                  openbaoclustercontroller.OpenBaoRuntime
	imageVerificationRuntime                        openbaoclustercontroller.ImageVerificationRuntime
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
	initMgr := initmanager.NewManager(config, clientset, clientMgr, mgr.GetEventRecorder(controllerNameOpenBaoCluster))
	imageVerifier := security.NewImageVerifier(mgr.GetLogger().WithName("image-verifier"), mgr.GetClient(), nil)
	operatorImageVerifier := security.NewImageVerifier(
		mgr.GetLogger().WithName("operator-image-verifier"),
		mgr.GetClient(),
		nil,
	)

	operatorNamespace := operatorNamespaceFromEnv()
	operatorServiceAccountName := operatorServiceAccountNameFromEnv()
	if missingHelperImages := unavailableHelperImageDefaultFields(); len(missingHelperImages) > 0 {
		setupLog.Info(
			"Operator-managed default helper images are unavailable until OPERATOR_VERSION is configured; "+
				"clusters can still override helper images explicitly in the cluster spec",
			"fields",
			missingHelperImages,
		)
	}

	oidcConfig := discoverStartupOIDC(config)
	admissionTracker := initializeAdmissionTracker(
		mgr,
		cfg.admissionEnforcement,
		cfg.admissionStartupTimeout,
		cfg.enableServiceClaims,
	)

	return controllerProcessRuntime{
		operatorNamespace:                               operatorNamespace,
		operatorServiceAccountName:                      operatorServiceAccountName,
		platform:                                        platform,
		singleTenantMode:                                singleTenantMode,
		enableServiceClaims:                             cfg.enableServiceClaims,
		serviceClaimsAPIServerCIDR:                      cfg.serviceClaimsAPIServerCIDR,
		serviceClaimsAPIServerEndpointIPs:               append([]string(nil), cfg.serviceClaimsAPIServerEndpointIPs...),
		serviceClaimsDNSEndpointIPs:                     append([]string(nil), cfg.serviceClaimsDNSEndpointIPs...),
		serviceClaimsTransitUnsealAddress:               cfg.serviceClaimsTransitUnsealAddress,
		serviceClaimsTransitUnsealKeyName:               cfg.serviceClaimsTransitUnsealKeyName,
		serviceClaimsTransitUnsealMountPath:             cfg.serviceClaimsTransitUnsealMountPath,
		serviceClaimsTransitUnsealNamespace:             cfg.serviceClaimsTransitUnsealNamespace,
		serviceClaimsTransitUnsealTLSCACert:             cfg.serviceClaimsTransitUnsealTLSCACert,
		serviceClaimsTransitUnsealTLSServerName:         cfg.serviceClaimsTransitUnsealTLSServerName,
		serviceClaimsTransitUnsealCredentialsSecretName: cfg.serviceClaimsTransitUnsealCredentialsSecretName,
		admissionTracker:                                admissionTracker,
		oidcRuntime: openbaoclustercontroller.OIDCRuntime{
			OIDCIssuer:         oidcConfig.IssuerURL,
			OIDCDiscoveryURL:   oidcConfig.OIDCDiscoveryURL,
			OIDCDiscoveryCAPEM: oidcConfig.OIDCDiscoveryCAPEM,
			OIDCJWKSURL:        oidcConfig.JWKSURL,
			OIDCJWKSCAPEM:      oidcConfig.JWKSCAPEM,
			OIDCJWTKeys:        oidcConfig.JWKSKeys,
			DiscoverOIDCConfig: auth.DiscoverConfig,
			OIDCStatusCode:     portauth.DiscoveryStatusCode,
		},
		openBaoRuntime: openbaoclustercontroller.OpenBaoRuntime{
			TLSReload:         reloadSignaler,
			InitManager:       initMgr,
			SmartClientConfig: smartClientConfig,
			OpenBaoClientFactory: func(config portopenbao.ClientConfig) (portopenbao.ClusterActions, error) {
				return openbao.NewClient(config)
			},
		},
		imageVerificationRuntime: openbaoclustercontroller.ImageVerificationRuntime{
			ImageVerifier:         imageVerifier,
			OperatorImageVerifier: operatorImageVerifier,
		},
	}, nil
}

func setupControllers(mgr ctrl.Manager, runtime controllerProcessRuntime) error {
	if err := (&openbaoclustercontroller.OpenBaoClusterReconciler{
		Client: mgr.GetClient(),
		ControllerRuntime: openbaoclustercontroller.ControllerRuntime{
			APIReader:         mgr.GetAPIReader(),
			Scheme:            mgr.GetScheme(),
			RestConfig:        mgr.GetConfig(),
			OperatorNamespace: runtime.operatorNamespace,
			AdmissionTracker:  runtime.admissionTracker,
			Recorder:          mgr.GetEventRecorder(controllerNameOpenBaoCluster),
			Platform:          runtime.platform,
			SingleTenantMode:  runtime.singleTenantMode,
		},
		OIDCRuntime:              runtime.oidcRuntime,
		OpenBaoRuntime:           runtime.openBaoRuntime,
		ImageVerificationRuntime: runtime.imageVerificationRuntime,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller %s: %w", controllerNameOpenBaoCluster, err)
	}

	if err := (&openbaorestorecontroller.OpenBaoRestoreReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		AdmissionTracker: runtime.admissionTracker,
		Recorder:         mgr.GetEventRecorder(controllerNameOpenBaoRestore),
		RestoreReconciler: appopenbaorestore.NewRestoreReconciler(appopenbaorestore.RestoreDependencies{
			Client:                mgr.GetClient(),
			Scheme:                mgr.GetScheme(),
			Recorder:              mgr.GetEventRecorder(controllerNameOpenBaoRestore),
			OperatorImageVerifier: runtime.imageVerificationRuntime.OperatorImageVerifier,
			Platform:              runtime.platform,
			ClientConfig:          runtime.openBaoRuntime.SmartClientConfig,
		}),
		OperatorImageVerifier: runtime.imageVerificationRuntime.OperatorImageVerifier,
		Platform:              runtime.platform,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller %s: %w", controllerNameOpenBaoRestore, err)
	}

	if err := setupClaimControllers(mgr, runtime); err != nil {
		return err
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
