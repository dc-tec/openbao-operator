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
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/entrypoint"
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
	oldArgs := os.Args
	os.Args = append([]string{oldArgs[0]}, args...)
	defer func() { os.Args = oldArgs }()

	cfg, err := parseRunConfig()
	if err != nil {
		setupLog.Error(err, entrypoint.AdmissionEnforcementExpectedMsg)
		os.Exit(2)
	}

	mgr, err := ctrl.NewManager(
		ctrl.GetConfigOrDie(),
		newManagerOptions(buildMetricsServerOptions(cfg), cfg.probeAddr, cfg.enableLeaderElection),
	)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	runtime, err := buildProvisionerProcessRuntime(mgr, cfg)
	if err != nil {
		setupLog.Error(err, "unable to initialize provisioner runtime")
		os.Exit(1)
	}

	if err := setupControllers(mgr, runtime); err != nil {
		setupLog.Error(err, "unable to register provisioner controllers")
		os.Exit(1)
	}

	if err := addManagerHealthChecks(mgr); err != nil {
		setupLog.Error(err, "unable to configure manager probes")
		os.Exit(1)
	}

	setupLog.Info("starting provisioner manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
