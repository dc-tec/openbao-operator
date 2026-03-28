package provisioner

import (
	"flag"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/dc-tec/openbao-operator/internal/platform/entrypoint"
)

type runConfig struct {
	metricsAddr             string
	enableLeaderElection    bool
	probeAddr               string
	secureMetrics           bool
	admissionEnforcement    string
	admissionStartupTimeout time.Duration
	admissionCanary         bool
}

func parseRunConfig() (runConfig, error) {
	cfg := runConfig{}

	entrypoint.BindManagerFlags(
		flag.CommandLine,
		&cfg.metricsAddr,
		&cfg.probeAddr,
		&cfg.enableLeaderElection,
		&cfg.secureMetrics,
	)
	entrypoint.BindAdmissionFlags(flag.CommandLine, &cfg.admissionEnforcement, &cfg.admissionStartupTimeout)
	flag.BoolVar(&cfg.admissionCanary, "admission-canary", false,
		"If set, perform an admission canary (dry-run) that must be denied "+
			"by the Provisioner RBAC ValidatingAdmissionPolicy. "+
			"This provides stronger assurance that enforcement is active.")

	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	admissionEnforcement, err := entrypoint.NormalizeAdmissionEnforcement(cfg.admissionEnforcement)
	if err != nil {
		return runConfig{}, err
	}
	cfg.admissionEnforcement = admissionEnforcement

	return cfg, nil
}

func buildMetricsServerOptions(cfg runConfig) metricsserver.Options {
	metricsServerOptions := metricsserver.Options{
		BindAddress:   cfg.metricsAddr,
		SecureServing: cfg.secureMetrics,
	}

	if cfg.secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	return metricsServerOptions
}
