package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/dc-tec/openbao-operator/internal/adapter/auth"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/entrypoint"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
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

func resolvePlatform(config *rest.Config, configured string) string {
	platform := configured
	if envPlatform := strings.TrimSpace(os.Getenv("OPERATOR_PLATFORM")); envPlatform != "" {
		platform = strings.ToLower(envPlatform)
	}
	if platform == "" {
		platform = constants.PlatformAuto
	}
	if platform == constants.PlatformAuto {
		detected := detectPlatform(config)
		setupLog.Info("Auto-detected target platform", "platform", detected)
		return detected
	}

	return platform
}

func newManagerOptions(
	scheme *runtime.Scheme,
	metricsServerOptions metricsserver.Options,
	probeAddr string,
	enableLeaderElection bool,
	watchNamespace string,
	claimWebhookCertDir string,
) ctrl.Options {
	singleTenantMode := watchNamespace != ""
	mgrOpts := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "openbao-controller-leader.openbao.org",
	}
	if claimWebhookCertDir != "" {
		mgrOpts.WebhookServer = webhook.NewServer(webhook.Options{
			Port:     9443,
			CertDir:  claimWebhookCertDir,
			CertName: claimAdmissionServingCertFile,
			KeyName:  claimAdmissionServingKeyFile,
		})
	}

	if singleTenantMode {
		mgrOpts.Cache = cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				watchNamespace: {},
			},
		}
		return mgrOpts
	}

	disableForCache := []client.Object{
		&corev1.Secret{},
		&batchv1.Job{},
		&appsv1.StatefulSet{},
		&corev1.Service{},
		&corev1.ConfigMap{},
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
		&gatewayv1.TLSRoute{},
		&gatewayv1.BackendTLSPolicy{},
	}
	mgrOpts.Client = client.Options{
		Cache: &client.CacheOptions{
			DisableFor: disableForCache,
		},
	}
	return mgrOpts
}

func discoverStartupOIDC(config *rest.Config) *portauth.OIDCConfig {
	oidcConfig, err := auth.DiscoverConfig(context.Background(), config, "")
	if err != nil {
		setupLog.Error(err, "Failed to discover Kubernetes OIDC configuration. Hardened profile requires OIDC.")
		if oidcConfig == nil {
			oidcConfig = &portauth.OIDCConfig{}
		}
	} else {
		setupLog.Info("Discovered Kubernetes OIDC configuration", "issuer", oidcConfig.IssuerURL)
		if oidcConfig.JWKSURL != "" {
			setupLog.Info("Selected OIDC JWKS URL for operator bootstrap", "jwksURL", oidcConfig.JWKSURL)
		}
		if len(oidcConfig.JWKSKeys) > 0 {
			setupLog.Info("Fetched OIDC JWKS public keys", "count", len(oidcConfig.JWKSKeys))
		}
	}
	if err != nil && oidcConfig.IssuerURL != "" {
		setupLog.Info("Continuing with partial OIDC discovery results", "issuer", oidcConfig.IssuerURL)
	}
	return oidcConfig
}

func initializeAdmissionTracker(
	mgr ctrl.Manager,
	admissionEnforcement string,
	admissionStartupTimeout time.Duration,
	enableServiceClaims bool,
) *admission.Tracker {
	dependencies := admission.DependenciesForFeatures(enableServiceClaims)
	admissionTracker := admission.NewTracker(
		mgr.GetAPIReader(),
		dependencies,
		admission.DefaultNamePrefixes(),
		30*time.Second,
	)

	if admission.UnsafeAdmissionDisabled() {
		setupLog.Info("UNSAFE MODE: admission policy enforcement disabled; skipping dependency checks")
		logging.LogAuditEvent(setupLog, logging.EventAdmissionUnsafeModeEnabled, map[string]string{
			"component":             "controller",
			"admission_enforcement": admissionEnforcement,
		})
		admission.SetAdmissionDependenciesReady(true)
		admissionTracker.MarkReadyForUnsafeMode()
		return admissionTracker
	}

	var admissionStatus admission.Status
	switch admissionEnforcement {
	case entrypoint.AdmissionEnforcementFail:
		setupLog.Info("Waiting for admission policy dependencies", "timeout", admissionStartupTimeout)
		status, err := admission.WaitForDependencies(
			context.Background(),
			mgr.GetAPIReader(),
			dependencies,
			admission.DefaultNamePrefixes(),
			admissionStartupTimeout,
			2*time.Second,
		)
		admissionStatus = status
		if !admissionStatus.OverallReady {
			if err == nil {
				err = fmt.Errorf("admission policy dependencies not ready")
			}
			logging.LogAuditEvent(setupLog, logging.EventAdmissionStartupBlocked, map[string]string{
				"component":             "controller",
				"admission_enforcement": admissionEnforcement,
				"summary":               admissionStatus.SummaryMessage(),
			})
			setupLog.Error(
				err,
				"Admission policy dependencies not ready; refusing to start",
				"summary",
				admissionStatus.SummaryMessage(),
			)
			os.Exit(1)
		}
	default:
		admissionCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		status, err := admission.CheckDependencies(
			admissionCtx,
			mgr.GetAPIReader(),
			dependencies,
			admission.DefaultNamePrefixes(),
		)
		admissionStatus = status
		if err != nil {
			setupLog.Error(err, "Failed to evaluate admission policy dependencies; treating admission as not ready")
			admissionStatus.OverallReady = false
		}
	}

	admission.SetAdmissionDependenciesReady(admissionStatus.OverallReady)
	admissionTracker.Set(admissionStatus)
	if admissionStatus.OverallReady {
		setupLog.Info("Admission policy dependencies ready")
		logging.LogAuditEvent(setupLog, logging.EventAdmissionDependenciesReady, map[string]string{
			"component":             "controller",
			"admission_enforcement": admissionEnforcement,
		})
	} else {
		setupLog.Info("Admission policy dependencies not ready", "summary", admissionStatus.SummaryMessage())
		logging.LogAuditEvent(setupLog, logging.EventAdmissionDependenciesNotReady, map[string]string{
			"component":             "controller",
			"admission_enforcement": admissionEnforcement,
			"summary":               admissionStatus.SummaryMessage(),
		})
	}

	return admissionTracker
}

func operatorNamespaceFromEnv() string {
	operatorNamespace := os.Getenv("POD_NAMESPACE")
	if operatorNamespace == "" {
		operatorNamespace = "openbao-operator-system"
		setupLog.Info("POD_NAMESPACE not set, using default", "namespace", operatorNamespace)
		return operatorNamespace
	}

	setupLog.Info("Using operator namespace from POD_NAMESPACE", "namespace", operatorNamespace)
	return operatorNamespace
}

func operatorServiceAccountNameFromEnv() string {
	serviceAccountName := strings.TrimSpace(os.Getenv("OPERATOR_SERVICE_ACCOUNT_NAME"))
	if serviceAccountName == "" {
		serviceAccountName = "controller"
		setupLog.Info("OPERATOR_SERVICE_ACCOUNT_NAME not set, using default", "serviceAccountName", serviceAccountName)
		return serviceAccountName
	}

	setupLog.Info(
		"Using controller service account name from OPERATOR_SERVICE_ACCOUNT_NAME",
		"serviceAccountName",
		serviceAccountName,
	)
	return serviceAccountName
}

func watchNamespaceFromEnv() string {
	return os.Getenv("WATCH_NAMESPACE")
}

func logTenancyMode(watchNamespace string) {
	if watchNamespace != "" {
		setupLog.Info("Running in single-tenant mode",
			"watch_namespace", watchNamespace,
			"caching", "enabled",
			"reconciliation", "event-driven",
		)
		return
	}

	setupLog.Info("Running in multi-tenant mode",
		"caching", "disabled",
		"reconciliation", "polling-based",
	)
}

func unavailableHelperImageDefaultFields() []string {
	checks := []struct {
		field string
		fn    func() (string, error)
	}{
		{field: "spec.initContainer.image", fn: constants.DefaultInitImage},
		{field: "spec.backup.image", fn: constants.DefaultBackupImage},
		{field: "spec.upgrade.image", fn: constants.DefaultUpgradeImage},
	}

	missing := make([]string, 0, len(checks))
	for _, check := range checks {
		if _, err := check.fn(); err == nil {
			continue
		}
		missing = append(missing, check.field)
	}

	return missing
}
