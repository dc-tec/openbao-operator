/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"crypto/tls"
	"flag"
	"os"
	"strings"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/admission"
	"github.com/dc-tec/openbao-operator/internal/auth"
	certmanager "github.com/dc-tec/openbao-operator/internal/certs"
	"github.com/dc-tec/openbao-operator/internal/constants"
	openbaoclustercontroller "github.com/dc-tec/openbao-operator/internal/controller/openbaocluster"
	openbaorestorecontroller "github.com/dc-tec/openbao-operator/internal/controller/openbaorestore"
	initmanager "github.com/dc-tec/openbao-operator/internal/init"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/dc-tec/openbao-operator/internal/openbao"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func detectPlatform(cfg *rest.Config) string {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return constants.PlatformKubernetes
	}

	groups, err := clientset.Discovery().ServerGroups()
	if err != nil {
		return constants.PlatformKubernetes
	}

	for _, g := range groups.Groups {
		if g.Name == "security.openshift.io" {
			return constants.PlatformOpenShift
		}
	}

	return constants.PlatformKubernetes
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(openbaov1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))
	utilruntime.Must(gatewayv1alpha2.Install(scheme))
}

// Run starts the OpenBaoCluster controller manager.
// The Controller is responsible for reconciling OpenBaoCluster resources,
// managing StatefulSets, and executing upgrades.
// args are the command-line arguments (typically os.Args[2:] after the command name).
func Run(args []string) {
	// Set os.Args for flag parsing
	oldArgs := os.Args
	os.Args = append([]string{oldArgs[0]}, args...)
	defer func() { os.Args = oldArgs }()
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	var platform string

	// Smart Client Limits
	var clientQPS float64
	var clientBurst int
	var clientCBFailureThreshold int
	var clientCBOpenDuration time.Duration

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8443", "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics server")
	flag.StringVar(&platform, "platform", "auto",
		"The target platform (auto, kubernetes, openshift). Defaults to auto. "+
			"This flag is deprecated and will be removed in a future release. "+
			"Use the OPERATOR_PLATFORM environment variable instead.")

	flag.Float64Var(&clientQPS, "openbao-client-qps", 50.0,
		"The queries per second (QPS) limit for OpenBao API clients.")
	flag.IntVar(&clientBurst, "openbao-client-burst", 100,
		"The burst limit for OpenBao API clients.")
	flag.IntVar(&clientCBFailureThreshold, "openbao-client-cb-failure-threshold", 50,
		"The number of consecutive failures before opening the circuit breaker.")
	flag.DurationVar(&clientCBOpenDuration, "openbao-client-cb-open-duration", 30*time.Second,
		"The duration the circuit breaker remains open before testing the connection.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	platform = strings.ToLower(strings.TrimSpace(platform))

	// Allow environment variable to override flag (useful for Helm charts).
	if envPlatform := strings.TrimSpace(os.Getenv("OPERATOR_PLATFORM")); envPlatform != "" {
		platform = strings.ToLower(envPlatform)
	}

	if platform == "" {
		platform = constants.PlatformAuto
	}

	if platform == constants.PlatformAuto {
		detected := detectPlatform(ctrl.GetConfigOrDie())
		setupLog.Info("Auto-detected target platform", "platform", detected)
		platform = detected
	}
	setupLog.Info("Target platform configured", "platform", platform)

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'.
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	// Detect single-tenant mode via WATCH_NAMESPACE environment variable.
	// When set, the controller operates in single-tenant mode with:
	// - Namespace-scoped caching (higher performance)
	// - Event-driven reconciliation via Owns() watches
	// - Simplified RBAC (no Provisioner required)
	watchNamespace := os.Getenv("WATCH_NAMESPACE")
	singleTenantMode := watchNamespace != ""

	if singleTenantMode {
		setupLog.Info("Running in single-tenant mode",
			"watch_namespace", watchNamespace,
			"caching", "enabled",
			"reconciliation", "event-driven",
		)
	} else {
		setupLog.Info("Running in multi-tenant mode",
			"caching", "disabled",
			"reconciliation", "polling-based",
		)
	}

	// Configure manager options based on tenancy mode
	var mgrOpts ctrl.Options
	mgrOpts.Scheme = scheme
	mgrOpts.Metrics = metricsServerOptions
	mgrOpts.HealthProbeBindAddress = probeAddr
	mgrOpts.LeaderElection = enableLeaderElection
	mgrOpts.LeaderElectionID = "openbao-controller-leader.openbao.org"

	if singleTenantMode {
		// SINGLE-TENANT MODE: Enable namespace-scoped caching
		// The controller has full RBAC permissions in the watched namespace,
		// so we can use informers/cache for high-performance reconciliation.
		mgrOpts.Cache = cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				watchNamespace: {},
			},
		}
		// No need to disable cache for any resources - we have full access
	} else {
		// MULTI-TENANT MODE: Disable cache for namespace-scoped resources
		// SECURITY: The controller uses namespace-scoped permissions via tenant Roles,
		// so it does not have cluster-wide list/watch permissions required for cache sync.
		// Disabling cache prevents errors during manager startup and ensures the controller
		// uses direct API calls (GET) for these resources instead.
		//
		// Resources disabled:
		// - Secrets: Prevents secret enumeration attacks (only 'get' permission granted)
		// - Jobs: Controller manages Jobs but only has namespace-scoped permissions
		// - StatefulSets: Controller manages StatefulSets but only has namespace-scoped permissions
		// - Services: Controller manages Services but only has namespace-scoped permissions
		// - ConfigMaps: Controller manages ConfigMaps but only has namespace-scoped permissions
		// - Ingress: Controller manages Ingress but only has namespace-scoped permissions
		// - NetworkPolicy: Controller manages NetworkPolicy but only has namespace-scoped permissions
		// - Roles/RoleBindings: Controller manages Roles/RoleBindings for OpenBao pod discovery
		// - ServiceAccounts: Controller manages ServiceAccounts for OpenBao clusters
		// - Pods: Controller reads Pods for health checks and leader detection
		// - PersistentVolumeClaims: Controller manages PVCs via StatefulSet volume claim templates
		// - Endpoints/EndpointSlices: Controller reads for service discovery
		// - Gateway HTTPRoute/TLSRoute/BackendTLSPolicy: Controller manages Gateway routes
		disableForCache := []client.Object{
			&corev1.Secret{},
			&batchv1.Job{},
			&appsv1.StatefulSet{},
			&corev1.Service{},
			&corev1.ConfigMap{},
			// The controller must not list/watch namespaces. It only operates within
			// namespaces where tenant Roles grant scoped permissions.
			&corev1.Namespace{},
			&networkingv1.Ingress{},
			&networkingv1.NetworkPolicy{},
			&rbacv1.Role{},
			&rbacv1.RoleBinding{},
			&corev1.ServiceAccount{},
			&corev1.Pod{},
			&corev1.PersistentVolumeClaim{},
			&discoveryv1.EndpointSlice{},
			&gatewayv1.HTTPRoute{},
			&gatewayv1alpha2.TLSRoute{},
			&gatewayv1.BackendTLSPolicy{},
		}
		mgrOpts.Client = client.Options{
			Cache: &client.CacheOptions{
				DisableFor: disableForCache,
			},
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Create Kubernetes clientset for ReloadSignaler
	config := mgr.GetConfig()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		setupLog.Error(err, "unable to create Kubernetes clientset")
		os.Exit(1)
	}

	// Create TLS reload signaler that annotates pods with the active TLS
	// certificate hash. A sidecar running inside the pod can watch this
	// annotation or the mounted TLS volume and send SIGHUP locally, avoiding
	// the need for pods/exec privileges in the operator.
	reloadSignaler := certmanager.NewKubernetesReloadSignaler(clientset)

	// Create smart client configuration
	smartClientConfig := openbao.ClientConfig{
		RateLimitQPS:                   clientQPS,
		RateLimitBurst:                 clientBurst,
		CircuitBreakerFailureThreshold: clientCBFailureThreshold,
		CircuitBreakerOpenDuration:     clientCBOpenDuration,
	}

	// Create ClientManager for OpenBao client lifecycle with explicit state management.
	// This replaces the global sync.Map with per-manager state for better test isolation.
	clientMgr := openbao.NewClientManager(smartClientConfig)
	// Note: clientMgr.Close() is not deferred here because the manager should live
	// for the lifetime of the operator process.

	// Create initialization manager
	initMgr := initmanager.NewManager(config, clientset, clientMgr)

	// Get operator namespace from POD_NAMESPACE environment variable (set by Kubernetes)
	// Default to "openbao-operator-system" for backward compatibility
	operatorNamespace := os.Getenv("POD_NAMESPACE")
	if operatorNamespace == "" {
		operatorNamespace = "openbao-operator-system"
		setupLog.Info("POD_NAMESPACE not set, using default", "namespace", operatorNamespace)
	} else {
		setupLog.Info("Using operator namespace from POD_NAMESPACE", "namespace", operatorNamespace)
	}

	// Discover OIDC configuration immediately at startup
	config = mgr.GetConfig()
	oidcConfig, err := auth.DiscoverConfig(context.Background(), config, "")
	if err != nil {
		setupLog.Error(err, "Failed to discover Kubernetes OIDC configuration. Hardened profile requires OIDC.")
		// TIGHTENED: Do not exit; just log. If a user tries to use Hardened mode later,
		// the Reconciler will fail then. This allows the operator to run on clusters
		// without OIDC if they only use Development mode.
		if oidcConfig == nil {
			oidcConfig = &auth.OIDCConfig{}
		}
	} else {
		setupLog.Info("Discovered Kubernetes OIDC configuration", "issuer", oidcConfig.IssuerURL)
		if len(oidcConfig.JWKSKeys) > 0 {
			setupLog.Info("Fetched OIDC JWKS public keys", "count", len(oidcConfig.JWKSKeys))
		}
	}
	if err != nil && oidcConfig.IssuerURL != "" {
		setupLog.Info("Continuing with partial OIDC discovery results", "issuer", oidcConfig.IssuerURL)
	}

	// Admission policy dependency check (release-critical security boundary).
	admissionCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admissionStatus, err := admission.CheckDependencies(
		admissionCtx,
		mgr.GetAPIReader(),
		admission.DefaultDependencies(),
		[]string{"openbao-operator-", ""},
	)
	if err != nil {
		setupLog.Error(err, "Failed to evaluate admission policy dependencies; treating admission as not ready")
		admissionStatus.OverallReady = false
	}
	admission.SetAdmissionDependenciesReady(admissionStatus.OverallReady)
	if admissionStatus.OverallReady {
		setupLog.Info("Admission policy dependencies ready")
	} else {
		setupLog.Info("Admission policy dependencies not ready", "summary", admissionStatus.SummaryMessage())
	}

	// Pass these values into the Reconciler struct
	if err := (&openbaoclustercontroller.OpenBaoClusterReconciler{
		Client:            mgr.GetClient(),
		APIReader:         mgr.GetAPIReader(),
		Scheme:            mgr.GetScheme(),
		TLSReload:         reloadSignaler,
		InitManager:       initMgr,
		OperatorNamespace: operatorNamespace,
		OIDCIssuer:        oidcConfig.IssuerURL,
		OIDCJWTKeys:       oidcConfig.JWKSKeys,
		AdmissionStatus:   &admissionStatus,
		Recorder:          mgr.GetEventRecorderFor(constants.ControllerNameOpenBaoCluster),
		SingleTenantMode:  singleTenantMode,
		SmartClientConfig: smartClientConfig,
		Platform:          platform,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OpenBaoCluster")
		os.Exit(1)
	}

	// Set up OpenBaoRestore controller
	if err := (&openbaorestorecontroller.OpenBaoRestoreReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(constants.ControllerNameOpenBaoRestore),
		Platform: platform,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OpenBaoRestore")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting controller manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
