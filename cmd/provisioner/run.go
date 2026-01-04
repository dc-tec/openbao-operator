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

package provisioner

import (
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	provisionercontroller "github.com/dc-tec/openbao-operator/internal/controller/provisioner"
	"github.com/dc-tec/openbao-operator/internal/provisioner"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(openbaov1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))
	utilruntime.Must(gatewayv1alpha2.Install(scheme))
}

// Run starts the Provisioner controller manager.
// The Provisioner is responsible for onboarding new tenant namespaces
// by creating the necessary RoleBindings that grant the Controller access.
// args are the command-line arguments (typically os.Args[2:] after the command name).
func Run(args []string) {
	// Set os.Args for flag parsing
	oldArgs := os.Args
	os.Args = append([]string{oldArgs[0]}, args...)
	defer func() { os.Args = oldArgs }()
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8443", "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// This matches the controller metrics endpoint and relies on the shared
		// metrics-auth-role ClusterRole plus the provisioner-specific
		// ClusterRoleBinding in config/rbac.
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "openbao-provisioner-leader.openbao.org",
		// No webhook server for Provisioner
		// SECURITY: Disable cache for ServiceAccounts to align with RBAC permissions that only grant
		// 'get' (not 'list' or 'watch'). The Provisioner only needs to get a specific ServiceAccount
		// (the delegate ServiceAccount) in a known namespace during initialization. This prevents
		// the cache from requiring cluster-wide list/watch permissions and eliminates the ability
		// for a compromised Provisioner to enumerate ServiceAccounts across the cluster.
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.ServiceAccount{},
					// SECURITY: The Provisioner must not list/watch namespaces to avoid
					// cluster topology enumeration. It only needs direct GET/PATCH on
					// specific namespaces declared in OpenBaoTenant.Spec.TargetNamespace.
					&corev1.Namespace{},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Create provisioner manager for namespace onboarding
	// SECURITY: The manager uses impersonation to enforce least privilege
	provisionerMgr, err := provisioner.NewManager(mgr.GetClient(), mgr.GetConfig(), setupLog.WithName("provisioner"))
	if err != nil {
		setupLog.Error(err, "unable to create provisioner manager")
		os.Exit(1)
	}

	// Get operator namespace for security validation
	operatorNS := os.Getenv("OPERATOR_NAMESPACE")
	if operatorNS == "" {
		operatorNS = "openbao-operator-system"
	}

	// Register namespace provisioner controller
	if err := (&provisionercontroller.NamespaceProvisionerReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		Provisioner:       provisionerMgr,
		OperatorNamespace: operatorNS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NamespaceProvisioner")
		os.Exit(1)
	}

	// Register tenant secrets RBAC sync controller.
	// This reconciler maintains per-namespace Secret allowlists for the controller ServiceAccount
	// to reduce Secret blast radius in tenant namespaces.
	if err := (&provisionercontroller.TenantSecretsRBACReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Provisioner: provisionerMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TenantSecretsRBAC")
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

	setupLog.Info("starting provisioner manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
