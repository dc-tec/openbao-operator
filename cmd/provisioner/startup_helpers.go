package provisioner

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/entrypoint"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
)

func newManagerOptions(
	metricsServerOptions metricsserver.Options,
	probeAddr string,
	enableLeaderElection bool,
) ctrl.Options {
	return ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "openbao-provisioner-leader.openbao.org",
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.ServiceAccount{},
					&corev1.Namespace{},
					&rbacv1.Role{},
					&rbacv1.RoleBinding{},
				},
			},
		},
	}
}

func initializeAdmissionTracker(mgr ctrl.Manager, cfg runConfig) *admission.Tracker {
	admissionTracker := admission.NewTracker(
		mgr.GetAPIReader(),
		admission.DefaultDependencies(),
		admission.DefaultNamePrefixes(),
		30*time.Second,
	)
	if admission.UnsafeAdmissionDisabled() {
		setupLog.Info(
			"UNSAFE MODE: admission policy enforcement disabled; " +
				"skipping dependency checks and allowing provisioning without guardrails",
		)
		logging.LogAuditEvent(setupLog, logging.EventAdmissionUnsafeModeEnabled, map[string]string{
			"component":             "provisioner",
			"admission_enforcement": cfg.admissionEnforcement,
		})
		admission.SetAdmissionDependenciesReady(true)
		admissionTracker.MarkReadyForUnsafeMode()
		return admissionTracker
	}

	switch cfg.admissionEnforcement {
	case entrypoint.AdmissionEnforcementFail:
		setupLog.Info("Waiting for admission policy dependencies", "timeout", cfg.admissionStartupTimeout)
		status, err := admission.WaitForDependencies(
			context.Background(),
			mgr.GetAPIReader(),
			admission.DefaultDependencies(),
			admission.DefaultNamePrefixes(),
			cfg.admissionStartupTimeout,
			2*time.Second,
		)
		admission.SetAdmissionDependenciesReady(status.OverallReady)
		if !status.OverallReady {
			if err == nil {
				err = fmt.Errorf("admission policy dependencies not ready")
			}
			logging.LogAuditEvent(setupLog, logging.EventAdmissionStartupBlocked, map[string]string{
				"component":             "provisioner",
				"admission_enforcement": cfg.admissionEnforcement,
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
		admissionTracker.Set(status)
		setupLog.Info("Admission policy dependencies ready")
		logging.LogAuditEvent(setupLog, logging.EventAdmissionDependenciesReady, map[string]string{
			"component":             "provisioner",
			"admission_enforcement": cfg.admissionEnforcement,
		})

		if cfg.admissionCanary {
			verifyAdmissionCanary(mgr)
		}
	default:
		checkCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		status := admission.Status{}
		checkedStatus, err := admission.CheckDependencies(
			checkCtx,
			mgr.GetAPIReader(),
			admission.DefaultDependencies(),
			admission.DefaultNamePrefixes(),
		)
		if err != nil {
			setupLog.Error(err, "Failed to evaluate admission policy dependencies; treating admission as not ready")
		} else {
			status = checkedStatus
		}
		admission.SetAdmissionDependenciesReady(status.OverallReady)
		admissionTracker.Set(status)
		if status.OverallReady {
			setupLog.Info("Admission policy dependencies ready")
			logging.LogAuditEvent(setupLog, logging.EventAdmissionDependenciesReady, map[string]string{
				"component":             "provisioner",
				"admission_enforcement": cfg.admissionEnforcement,
			})
		} else {
			setupLog.Info("Admission policy dependencies not ready", "summary", status.SummaryMessage())
			logging.LogAuditEvent(setupLog, logging.EventAdmissionDependenciesNotReady, map[string]string{
				"component":             "provisioner",
				"admission_enforcement": cfg.admissionEnforcement,
				"summary":               status.SummaryMessage(),
			})
		}
	}

	return admissionTracker
}

func verifyAdmissionCanary(mgr ctrl.Manager) {
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

func operatorNamespaceFromEnv() string {
	operatorNS := os.Getenv("POD_NAMESPACE")
	if operatorNS == "" {
		operatorNS = os.Getenv("OPERATOR_NAMESPACE")
	}
	if operatorNS == "" {
		operatorNS = "openbao-operator-system"
	}

	return operatorNS
}
