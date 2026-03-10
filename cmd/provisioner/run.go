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
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	provisionercontroller "github.com/dc-tec/openbao-operator/internal/controller/provisioner"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/entrypoint"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	"github.com/dc-tec/openbao-operator/internal/service/provisioner"
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

	// Admission policy enforcement
	var admissionEnforcement string
	var admissionStartupTimeout time.Duration
	var admissionCanary bool

	entrypoint.BindManagerFlags(flag.CommandLine, &metricsAddr, &probeAddr, &enableLeaderElection, &secureMetrics)
	entrypoint.BindAdmissionFlags(flag.CommandLine, &admissionEnforcement, &admissionStartupTimeout)
	flag.BoolVar(&admissionCanary, "admission-canary", false,
		"If set, perform an admission canary (dry-run) that must be denied "+
			"by the Provisioner RBAC ValidatingAdmissionPolicy. "+
			"This provides stronger assurance that enforcement is active.")

	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	admissionEnforcement, err := entrypoint.NormalizeAdmissionEnforcement(admissionEnforcement)
	if err != nil {
		setupLog.Error(err, entrypoint.AdmissionEnforcementExpectedMsg)
		os.Exit(2)
	}

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
		// (the controller ServiceAccount) in a known namespace during initialization. This prevents
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
					// SECURITY: Avoid caching Roles/RoleBindings to prevent requiring
					// cluster-wide list/watch permissions.
					&rbacv1.Role{},
					&rbacv1.RoleBinding{},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Admission policy dependency check (release-critical security boundary).
	if admission.UnsafeAdmissionDisabled() {
		setupLog.Info(
			"UNSAFE MODE: admission policy enforcement disabled; " +
				"skipping dependency checks and allowing provisioning without guardrails",
		)
		logging.LogAuditEvent(setupLog, logging.EventAdmissionUnsafeModeEnabled, map[string]string{
			"component":             "provisioner",
			"admission_enforcement": admissionEnforcement,
		})
		admission.SetAdmissionDependenciesReady(true)
	} else {
		switch admissionEnforcement {
		case entrypoint.AdmissionEnforcementFail:
			setupLog.Info("Waiting for admission policy dependencies", "timeout", admissionStartupTimeout)
			status, err := admission.WaitForDependencies(
				context.Background(),
				mgr.GetAPIReader(),
				admission.DefaultDependencies(),
				admission.DefaultNamePrefixes(),
				admissionStartupTimeout,
				2*time.Second,
			)
			admission.SetAdmissionDependenciesReady(status.OverallReady)
			if !status.OverallReady {
				if err == nil {
					err = fmt.Errorf("admission policy dependencies not ready")
				}
				logging.LogAuditEvent(setupLog, logging.EventAdmissionStartupBlocked, map[string]string{
					"component":             "provisioner",
					"admission_enforcement": admissionEnforcement,
					"summary":               status.SummaryMessage(),
				})
				setupLog.Error(
					err,
					"Admission policy dependencies not ready; refusing to start",
					"summary",
					status.SummaryMessage(),
				)
				os.Exit(1)
			}
			setupLog.Info("Admission policy dependencies ready")
			logging.LogAuditEvent(setupLog, logging.EventAdmissionDependenciesReady, map[string]string{
				"component":             "provisioner",
				"admission_enforcement": admissionEnforcement,
			})

			if admissionCanary {
				// Verify enforcement via a dry-run forbidden RBAC request.
				clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
				if err != nil {
					logging.LogAuditEvent(setupLog, logging.EventAdmissionCanaryFailed, map[string]string{
						"component": "provisioner",
						"reason":    "clientset_creation_failed",
					})
					setupLog.Error(err, "Failed to create Kubernetes clientset for admission canary; refusing to start")
					os.Exit(1)
				}
				canaryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				// Use a commonly-present namespace that is typically not considered a system namespace.
				// This makes the canary assert the Role name restriction, not just the system namespace restriction.
				if err := admission.VerifyProvisionerRBACEnforcement(canaryCtx, clientset, "default"); err != nil {
					logging.LogAuditEvent(setupLog, logging.EventAdmissionCanaryFailed, map[string]string{
						"component": "provisioner",
						"reason":    "policy_not_enforced",
					})
					setupLog.Error(err, "Admission canary failed; refusing to start")
					os.Exit(1)
				}
				setupLog.Info("Admission canary succeeded")
				logging.LogAuditEvent(setupLog, logging.EventAdmissionCanaryPassed, map[string]string{
					"component": "provisioner",
				})
			}
		default: // warn
			checkCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			status, err := admission.CheckDependencies(
				checkCtx,
				mgr.GetAPIReader(),
				admission.DefaultDependencies(),
				admission.DefaultNamePrefixes(),
			)
			if err != nil {
				setupLog.Error(err, "Failed to evaluate admission policy dependencies; treating admission as not ready")
				status.OverallReady = false
			}
			admission.SetAdmissionDependenciesReady(status.OverallReady)
			if status.OverallReady {
				setupLog.Info("Admission policy dependencies ready")
				logging.LogAuditEvent(setupLog, logging.EventAdmissionDependenciesReady, map[string]string{
					"component":             "provisioner",
					"admission_enforcement": admissionEnforcement,
				})
			} else {
				setupLog.Info("Admission policy dependencies not ready", "summary", status.SummaryMessage())
				logging.LogAuditEvent(setupLog, logging.EventAdmissionDependenciesNotReady, map[string]string{
					"component":             "provisioner",
					"admission_enforcement": admissionEnforcement,
					"summary":               status.SummaryMessage(),
				})
			}
		}
	}

	// Create provisioner manager for namespace onboarding
	provisionerMgr, err := provisioner.NewManager(context.Background(),
		mgr.GetClient(),
		setupLog.WithName("provisioner"))
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
		APIReader:         mgr.GetAPIReader(),
		Scheme:            mgr.GetScheme(),
		Recorder:          mgr.GetEventRecorder("namespace-provisioner"),
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
		APIReader:   mgr.GetAPIReader(),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorder("namespace-provisioner-tenant-secrets"),
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
