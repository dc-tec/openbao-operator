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
	"context"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/entrypoint"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(openbaov1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))
}

// Run starts the Provisioner controller manager.
// The Provisioner is responsible for onboarding new tenant namespaces
// by creating the necessary RoleBindings that grant the Controller access.
// args are the command-line arguments (typically os.Args[2:] after the command name).
func Run(ctx context.Context, args []string) error {
	cfg, err := parseRunConfig(args, os.Stderr)
	if err != nil {
		return &entrypoint.UsageError{Err: err}
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&cfg.logOptions)))
	config, err := entrypoint.LoadConfig(cfg.kubeconfig)
	if err != nil {
		return fmt.Errorf("load Kubernetes configuration: %w", err)
	}
	mgr, err := ctrl.NewManager(
		config,
		newManagerOptions(buildMetricsServerOptions(cfg), cfg.probeAddr, cfg.enableLeaderElection),
	)
	if err != nil {
		return fmt.Errorf("unable to start manager: %w", err)
	}

	processRuntime, err := buildProvisionerProcessRuntime(ctx, mgr, cfg)
	if err != nil {
		return fmt.Errorf("unable to initialize provisioner runtime: %w", err)
	}

	if err := setupControllers(mgr, processRuntime); err != nil {
		return fmt.Errorf("unable to register provisioner controllers: %w", err)
	}

	if err := addManagerHealthChecks(ctx, mgr); err != nil {
		return fmt.Errorf("unable to configure manager probes: %w", err)
	}

	setupLog.Info("starting provisioner manager")
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("problem running manager: %w", err)
	}
	return nil
}
