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
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/dc-tec/openbao-operator/internal/adapter/auth"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/entrypoint"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const platformDiscoveryTimeout = 10 * time.Second

func detectPlatform(ctx context.Context, cfg *rest.Config) (string, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("create platform discovery client: %w", err)
	}

	discoveryCtx, cancel := context.WithTimeout(ctx, platformDiscoveryTimeout)
	defer cancel()
	// Request the named API groups in legacy JSON format so discovery remains
	// cancellable. ServerGroups does not accept the startup context.
	groups := &metav1.APIGroupList{}
	if err := discoveryClient.RESTClient().Get().AbsPath("/apis").
		SetHeader("Accept", "application/json").Do(discoveryCtx).Into(groups); err != nil {
		return "", fmt.Errorf("discover target platform: %w", err)
	}

	for _, group := range groups.Groups {
		if group.Name == "security.openshift.io" {
			return constants.PlatformOpenShift, nil
		}
	}
	return constants.PlatformKubernetes, nil
}

func configuredPlatform(configured, environment string) (string, error) {
	platform := configured
	if strings.TrimSpace(environment) != "" {
		platform = environment
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" {
		platform = constants.PlatformAuto
	}
	switch platform {
	case constants.PlatformAuto, constants.PlatformKubernetes, constants.PlatformOpenShift:
		return platform, nil
	default:
		return "", fmt.Errorf("invalid target platform %q: expected auto, kubernetes, or openshift", platform)
	}
}

func resolvePlatform(ctx context.Context, config *rest.Config, configured string) (string, error) {
	if configured == constants.PlatformAuto {
		detected, err := detectPlatform(ctx, config)
		if err != nil {
			return "", err
		}
		setupLog.Info("Auto-detected target platform", "platform", detected)
		return detected, nil
	}
	return configured, nil
}

func newManagerOptions(
	scheme *runtime.Scheme,
	metricsServerOptions metricsserver.Options,
	probeAddr string,
	enableLeaderElection bool,
	watchNamespace string,
) ctrl.Options {
	singleTenantMode := watchNamespace != ""
	mgrOpts := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "openbao-controller-leader.openbao.org",
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
		&policyv1.PodDisruptionBudget{},
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

func discoverStartupOIDC(ctx context.Context, config *rest.Config) *portauth.OIDCConfig {
	oidcConfig, err := auth.DiscoverConfig(ctx, config, "")
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
	ctx context.Context,
	reader client.Reader,
	admissionEnforcement string,
	admissionStartupTimeout time.Duration,
) (*admission.Tracker, error) {
	admissionTracker := admission.NewTracker(
		reader,
		admission.DefaultDependencies(),
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
		return admissionTracker, nil
	}

	var admissionStatus admission.Status
	switch admissionEnforcement {
	case entrypoint.AdmissionEnforcementFail:
		setupLog.Info("Waiting for admission policy dependencies", "timeout", admissionStartupTimeout)
		status, err := admission.WaitForDependencies(
			ctx,
			reader,
			admission.DefaultDependencies(),
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
			admission.SetAdmissionDependenciesReady(false)
			return nil, fmt.Errorf("admission policy dependencies not ready (%s): %w",
				admissionStatus.SummaryMessage(), err)
		}
	default:
		admissionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		status, err := admission.CheckDependencies(
			admissionCtx,
			reader,
			admission.DefaultDependencies(),
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

	return admissionTracker, nil
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
